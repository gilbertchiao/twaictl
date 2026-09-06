package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilbertchiao/twaictl/internal/testutil"
)

// TestVCSActionStopConfirmsAndSendsStatus 驗證 stop 需確認、body 為 {"status":"stop"}、-y 略過。
func TestVCSActionStopConfirmsAndSendsStatus(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/sites/", 200, "vcs/sites")
	f.on("PUT", vcsBase+"/sites/7/action/", 202, "")
	code, _, stderr := runCLI(t, f.env(t), "n\n", "vcs", "action", "web-01", "stop")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Nil(t, f.find("PUT", vcsBase+"/sites/7/action/"))

	code, _, stderr = runCLI(t, f.env(t), "", "vcs", "action", "web-01", "stop", "-y")
	require.Equal(t, ExitOK, code, stderr)
	assert.JSONEq(t, `{"status":"stop"}`, string(f.find("PUT", vcsBase+"/sites/7/action/").Body))
	assert.Contains(t, stderr, "已送出 stop 請求：7")
}

// TestVCSActionStartNoConfirmAndRejectsUnknown 驗證 start 不需確認；未知動作列出合法值且不送請求。
func TestVCSActionStartNoConfirmAndRejectsUnknown(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/sites/", 200, "vcs/sites")
	f.on("PUT", vcsBase+"/sites/7/action/", 202, "")
	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "action", "7", "start")
	require.Equal(t, ExitOK, code, stderr)
	assert.NotContains(t, stderr, "確定嗎")

	code, _, stderr = runCLI(t, f.env(t), "", "vcs", "action", "7", "explode")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "unshelve")
	assert.Len(t, f.Requests(), 2, "只應有前一次的 project 解析與 action 請求")
}

// TestVCSActionWaitPollsServerStatus 驗證 --wait 輪詢 GET /sites/{id}/ 直到 servers[0].status 到達目標。
func TestVCSActionWaitPollsServerStatus(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/sites/", 200, "vcs/sites")
	f.on("PUT", vcsBase+"/sites/7/action/", 202, "")
	stopping := strings.Replace(fixture(t, "vcs/site_ready"), `"status": "ACTIVE"`, `"status": "STOPPING"`, 1)
	stopped := strings.Replace(fixture(t, "vcs/site_ready"), `"status": "ACTIVE"`, `"status": "SHUTOFF"`, 1)
	// server 已 SHUTOFF 但 site 仍在 Stopping（真實環境會落後 45～120 秒，A39）→ 要再等到 NotReady。
	stoppedSiteStopping := strings.Replace(stopped, `"status": "Ready"`, `"status": "Stopping"`, 1)
	stoppedSiteNotReady := strings.Replace(stopped, `"status": "Ready"`, `"status": "NotReady"`, 1)
	f.onSequence("GET", vcsBase+"/sites/7/",
		seq{200, stopping}, seq{200, stoppedSiteStopping}, seq{200, stoppedSiteStopping}, seq{200, stoppedSiteNotReady})
	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "action", "7", "stop", "-y", "--wait", "--wait-interval", "10ms")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stderr, "SHUTOFF")
	assert.Contains(t, stderr, "NotReady")

	var pollCount int
	for _, r := range f.Requests() {
		if r.Method == "GET" && r.Path == vcsBase+"/sites/7/" {
			pollCount++
		}
	}
	assert.Equal(t, 4, pollCount, "server 到 SHUTOFF 後還要輪詢到 site 離開 Stopping 才算完成")
}

// TestVCSActionJSONOutputsSummary 驗證 -o json 輸出單一物件摘要，且 --wait 完成後不重新查詢。
func TestVCSActionJSONOutputsSummary(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/sites/", 200, "vcs/sites")
	f.on("PUT", vcsBase+"/sites/7/action/", 202, "")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "action", "web-01", "start", "-o", "json")
	require.Equal(t, ExitOK, code, stderr)
	assert.JSONEq(t, `{"site_id":7,"action":"start"}`, stdout)
}

// TestVCSActionYAMLOutputsSummary 驗證 renderSummaryOne 補上 -o yaml 後，`vcs action`
// 也能輸出（單一物件、非陣列）YAML 摘要。
func TestVCSActionYAMLOutputsSummary(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/sites/", 200, "vcs/sites")
	f.on("PUT", vcsBase+"/sites/7/action/", 202, "")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "action", "web-01", "start", "-o", "yaml")
	require.Equal(t, ExitOK, code, stderr)
	assert.Equal(t, "action: start\nsite_id: 7\n", stdout)
}

// TestVCSActionRebootWaitRequiresLeavingActive 驗證 reboot --wait 不會在第一次輪詢
// （送出 reboot 後 server 通常仍是 ACTIVE，尚未真的開始重開）就誤判為完成，
// 必須先觀察到 server 離開 ACTIVE（進入 REBOOT）才算完成。
func TestVCSActionRebootWaitRequiresLeavingActive(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/sites/", 200, "vcs/sites")
	f.on("PUT", vcsBase+"/sites/7/action/", 202, "")
	active := fixture(t, "vcs/site_ready")
	rebooting := strings.Replace(active, `"status": "ACTIVE"`, `"status": "REBOOT"`, 1)
	f.onSequence("GET", vcsBase+"/sites/7/",
		seq{200, active}, seq{200, active}, seq{200, rebooting}, seq{200, active})
	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "action", "7", "reboot", "-y", "--wait", "--wait-interval", "10ms")
	require.Equal(t, ExitOK, code, stderr)

	var pollCount int
	for _, r := range f.Requests() {
		if r.Method == "GET" && r.Path == vcsBase+"/sites/7/" {
			pollCount++
		}
	}
	assert.Equal(t, 5, pollCount, "第一、二次輪詢都還是 ACTIVE，不能誤判為完成；要等觀察到 REBOOT 之後再回到 ACTIVE，最後再多一次輪詢確認 site 為 Ready（A39）才算數")
}

// TestVCSActionRejectsEmptyAction 驗證空字串動作在送出任何請求前就被拒絕
// （若不擋下，會直接送出 {"status":""} 給 API，因為 validateEnumList 對空字串是合法的）。
func TestVCSActionRejectsEmptyAction(t *testing.T) {
	f := newFakeAPI(t)
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "action", "7", "")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "請指定動作")
	assert.Empty(t, f.Requests(), "空字串動作不可送出任何請求（含 project 解析）")
}

// TestVCSEventsGolden 驗證 `vcs events` 依 golden 輸出（時間轉台北）。
func TestVCSEventsGolden(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/sites/", 200, "vcs/sites")
	f.onFixture("GET", vcsBase+"/sites/7/event_logs/", 200, "vcs/event_logs")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "events", "web-01")
	require.Equal(t, ExitOK, code, stderr)
	testutil.AssertGolden(t, "vcs_events.txt", []byte(stdout))
}

func TestVCSEventsJSONIsRawArray(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/sites/7/event_logs/", 200, "vcs/event_logs")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "events", "7", "-o", "json")
	require.Equal(t, ExitOK, code, stderr)
	assert.JSONEq(t, fixture(t, "vcs/event_logs"), stdout)
}

// TestVCSMetricsResolvesServerAndSendsMeterName 驗證 metrics 由 site 解析 server、預設 cpu；
// --meter net-out 帶 meter_name=network_outgoing_bytes_rate；--begin/--end 原樣傳遞；golden。
func TestVCSMetricsResolvesServerAndSendsMeterName(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/sites/", 200, "vcs/sites")
	f.onFixture("GET", vcsBase+"/servers/", 200, "vcs/servers")
	f.onFixture("GET", vcsBase+"/servers/3658462/cpu/", 200, "vcs/metrics_cpu")
	f.onFixture("GET", vcsBase+"/servers/3658462/net/", 200, "vcs/metrics_net_out")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "metrics", "web-01")
	require.Equal(t, ExitOK, code, stderr)
	testutil.AssertGolden(t, "vcs_metrics.txt", []byte(stdout))

	code, _, stderr = runCLI(t, f.env(t), "", "vcs", "metrics", "web-01", "--meter", "net-out", "--begin", "2026-08-27T00:00:00Z", "--end", "2026-08-27T01:00:00Z")
	require.Equal(t, ExitOK, code, stderr)
	q := f.find("GET", vcsBase+"/servers/3658462/net/").Query
	assert.Contains(t, q, "meter_name=network_outgoing_bytes_rate")
	assert.Contains(t, q, "begin_time=2026-08-27T00%3A00%3A00Z")
}

// TestVCSMetricsServerFlagSkipsListServers 驗證 --server 指定時不呼叫 /servers/。
func TestVCSMetricsServerFlagSkipsListServers(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/servers/3658462/cpu/", 200, "vcs/metrics_cpu")
	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "metrics", "7", "--server", "3658462")
	require.Equal(t, ExitOK, code, stderr)
	assert.Nil(t, f.find("GET", vcsBase+"/sites/"), "site 為數字時不查 site 清單")
	assert.Nil(t, f.find("GET", vcsBase+"/servers/"), "--server 指定時不查 server 清單")
	assert.NotNil(t, f.find("GET", vcsBase+"/servers/3658462/cpu/"))
}

// TestVCSMetricsSiteWithoutServerIsError 驗證 site 沒有對應 server 時為一般錯誤。
func TestVCSMetricsSiteWithoutServerIsError(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/servers/", 200, "vcs/servers")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "metrics", "99")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "沒有 server")
}

func TestVCSMetricsInvalidMeterIsError(t *testing.T) {
	f := newFakeAPI(t)
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "metrics", "7", "--meter", "bogus")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "net-out")
	assert.Empty(t, f.Requests(), "未通過 --meter 驗證時不可送出任何請求（含 project 解析）")
}

// TestVCSMetricsRejectsEmptyMeter 驗證 --meter "" 明確被拒絕（而非被 validateEnumList
// 當成「未指定」放行），錯誤訊息列出合法值，且在送出任何請求之前完成。
func TestVCSMetricsRejectsEmptyMeter(t *testing.T) {
	f := newFakeAPI(t)
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "metrics", "7", "--meter", "")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "net-out")
	assert.Empty(t, f.Requests())
}

// TestVCSMetricsRejectsBadServer 驗證 --server 非正整數在送出任何請求（含 project 解析）
// 之前就被拒絕。
func TestVCSMetricsRejectsBadServer(t *testing.T) {
	f := newFakeAPI(t)
	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "metrics", "7", "--server", "0")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "正整數")
	assert.Empty(t, f.Requests())
}

// TestVCSMetricsEmptyPrintsHeaderAndProgressHint 驗證 0 筆資料時 table 仍印表頭並在 stderr 提示。
func TestVCSMetricsEmptyPrintsHeaderAndProgressHint(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/servers/3658462/cpu/", 200, "[]")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "metrics", "7", "--server", "3658462")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stdout, "TIME")
	assert.Contains(t, stderr, "沒有資料")
}

// TestVCSMetricsCpuHasNoDeviceColumn 驗證非 disk meter（例如 cpu）的預設欄位不含 DEVICE。
func TestVCSMetricsCpuHasNoDeviceColumn(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/servers/3658462/cpu/", 200, "vcs/metrics_cpu")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "metrics", "7", "--server", "3658462")
	require.Equal(t, ExitOK, code, stderr)
	assert.NotContains(t, stdout, "DEVICE")
}

// TestVCSMetricsDiskGolden 驗證 disk-write 巢狀回應（每裝置一組，見 docs/api-notes.md A22）
// 通用解析後展開成多筆 MetricPoint，且輸出含 DEVICE 欄位（golden）。
func TestVCSMetricsDiskGolden(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/sites/", 200, "vcs/sites")
	f.onFixture("GET", vcsBase+"/servers/", 200, "vcs/servers")
	f.onFixture("GET", vcsBase+"/servers/3658462/disk/", 200, "vcs/metrics_disk")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "metrics", "web-01", "--meter", "disk-write")
	require.Equal(t, ExitOK, code, stderr)
	testutil.AssertGolden(t, "vcs_metrics_disk.txt", []byte(stdout))
}
