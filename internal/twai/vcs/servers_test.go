package vcs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// siteBodyWithServerStatus 產生只有 servers[0].status 不同的 site JSON body，
// 供本檔 WaitServerStatus 測試共用，避免重複貼一大段幾乎相同的 JSON 字面值。
func siteBodyWithServerStatus(status string) []byte {
	return []byte(fmt.Sprintf(`{"callback":"","create_time":"2026-08-27T01:02:03Z","id":7,"is_preemptive":false,`+
		`"name":"web-01","platform":"p","progress":100,"project":101,"public_ip":"",`+
		`"servers":[{"hostname":"web-01-vm","flavor_id":1,"status":%q}],"solution":11,"status":"Ready",`+
		`"termination_protection":false,"user":{"username":"alice","display_name":"Alice","email":"a@example.com","id":"u-1"}}`, status))
}

// siteBodyWithNoServerStatus 產生 servers[0] 完全沒有 status 欄位的 site JSON body
// （對應真實 API 該欄位為 null／缺漏時，srv.Status 解析為 nil 的情境）。
func siteBodyWithNoServerStatus() []byte {
	return []byte(`{"callback":"","create_time":"2026-08-27T01:02:03Z","id":7,"is_preemptive":false,` +
		`"name":"web-01","platform":"p","progress":100,"project":101,"public_ip":"",` +
		`"servers":[{"hostname":"web-01-vm","flavor_id":1}],"solution":11,"status":"Ready",` +
		`"termination_protection":false,"user":{"username":"alice","display_name":"Alice","email":"a@example.com","id":"u-1"}}`)
}

// siteBodyWithServers 產生多台 server 的 site JSON body，供 WaitServerStatus「要看全部
// server」的測試共用；hostname 依索引命名為 web-01-vm-N。statuses 中的空字串代表該台
// server 完全沒有 status 欄位（對應真實 API status 為 null／缺漏的情境，同
// siteBodyWithNoServerStatus）。
func siteBodyWithServers(statuses ...string) []byte {
	servers := make([]string, len(statuses))
	for i, status := range statuses {
		if status == "" {
			servers[i] = fmt.Sprintf(`{"hostname":"web-01-vm-%d","flavor_id":1}`, i)
		} else {
			servers[i] = fmt.Sprintf(`{"hostname":"web-01-vm-%d","flavor_id":1,"status":%q}`, i, status)
		}
	}
	return []byte(fmt.Sprintf(`{"callback":"","create_time":"2026-08-27T01:02:03Z","id":7,"is_preemptive":false,`+
		`"name":"web-01","platform":"p","progress":100,"project":101,"public_ip":"",`+
		`"servers":[%s],"solution":11,"status":"Ready",`+
		`"termination_protection":false,"user":{"username":"alice","display_name":"Alice","email":"a@example.com","id":"u-1"}}`,
		strings.Join(servers, ",")))
}

func TestListServersSendsProject(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/servers/", 200, fixture(t, "servers"))
	s := newService(t, f)

	res, err := s.ListServers(context.Background(), 101)
	require.NoError(t, err)
	require.Len(t, res.Value, 2)
	assert.Equal(t, "web-01-vm", res.Value[0].Hostname)
	assert.Equal(t, int64(7), res.Value[0].Site)
	assert.Equal(t, "project=101", f.Requests()[0].Query)
}

func TestGetServerReturnsDetail(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/servers/3658462/", 200, fixture(t, "server_detail"))
	s := newService(t, f)

	res, err := s.GetServer(context.Background(), 3658462)
	require.NoError(t, err)
	assert.Equal(t, "web-01-vm", res.Value.Hostname)
	assert.Contains(t, string(res.Raw), "security_groups", "Raw 應保留 spec 沒有定義、但真實 API 會回傳的欄位")
}

func TestSiteActionSendsStatusBody(t *testing.T) {
	f := newFakeAPI(t)
	f.on("PUT", vcsBase+"/sites/7/action/", 202, nil)
	s := newService(t, f)

	err := s.SiteAction(context.Background(), 7, "stop")
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, json.Unmarshal(f.Requests()[0].Body, &body))
	assert.Equal(t, map[string]any{"status": "stop"}, body)
}

func TestSiteEventLogsReturnsList(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/sites/7/event_logs/", 200, fixture(t, "event_logs"))
	s := newService(t, f)

	res, err := s.SiteEventLogs(context.Background(), 7)
	require.NoError(t, err)
	require.Len(t, res.Value, 3)
	assert.Equal(t, "CREATE_COMPLETE", res.Value[1].Status)
}

func TestMetricsCPUDefaultDoesNotSendTimeRange(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/servers/3658462/cpu/", 200, fixture(t, "metrics_cpu"))
	s := newService(t, f)

	res, err := s.Metrics(context.Background(), 3658462, "cpu", "", "")
	require.NoError(t, err)
	require.Len(t, res.Value, 2)
	assert.Equal(t, "3.2", res.Value[0].Value)
	assert.Equal(t, "%", res.Value[0].Unit)
	assert.Empty(t, f.Requests()[0].Query, "未指定 --begin/--end 時不送對應參數")
}

func TestMetricsNetOutSendsMeterNameAndTimeRange(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/servers/3658462/net/", 200, fixture(t, "metrics_net_out"))
	s := newService(t, f)

	res, err := s.Metrics(context.Background(), 3658462, "net-out", "2026-08-27T00:00:00Z", "2026-08-27T01:00:00Z")
	require.NoError(t, err)
	require.Len(t, res.Value, 2)
	assert.Equal(t, "7546", res.Value[0].Value)
	q := f.Requests()[0].Query
	assert.Contains(t, q, "meter_name=network_outgoing_bytes_rate")
	assert.Contains(t, q, "begin_time=2026-08-27T00")
	assert.Contains(t, q, "end_time=2026-08-27T01")
}

// TestMetricsDecodesNetOutKey 驗證真實 net-out 回應的 value 鍵是 network_outgoing_bytes_rate
// （而非 spec 標示、net-in/net-out 共用的 network_incoming_bytes_rate，見 docs/api-notes.md A22）。
func TestMetricsDecodesNetOutKey(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/servers/3658462/net/", 200, fixture(t, "metrics_net_out"))
	s := newService(t, f)

	res, err := s.Metrics(context.Background(), 3658462, "net-out", "", "")
	require.NoError(t, err)
	require.Len(t, res.Value, 2)
	assert.Equal(t, "7546", res.Value[0].Value)
	assert.Equal(t, "B", res.Value[0].Unit)
	assert.Equal(t, "81864", res.Value[1].Value)
	assert.Empty(t, res.Value[0].Device, "非 disk meter 的 Device 應為空")
}

// TestMetricsDecodesNestedDiskResponse 驗證真實 disk-read/disk-write 回應是「每裝置一組」的
// 巢狀結構（spec 標示為扁平陣列，見 docs/api-notes.md A22）：外層元素為裝置（name + utils），
// 需要展開 utils 陣列並把 name 標到每一筆 MetricPoint.Device。
func TestMetricsDecodesNestedDiskResponse(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/servers/3658462/disk/", 200, fixture(t, "metrics_disk"))
	s := newService(t, f)

	res, err := s.Metrics(context.Background(), 3658462, "disk-write", "", "")
	require.NoError(t, err)
	require.Len(t, res.Value, 4)
	assert.Equal(t, "/dev/vda", res.Value[0].Device)
	assert.Equal(t, "/dev/vda", res.Value[1].Device)
	assert.Equal(t, "all", res.Value[2].Device)
	assert.Equal(t, "all", res.Value[3].Device)
	assert.Equal(t, "1317.548967985", res.Value[0].Value)
	assert.Equal(t, "B/s", res.Value[0].Unit)
	assert.Equal(t, "1348.103634877", res.Value[1].Value)
}

// TestMetricsFlatStillWorks 確認通用解析改寫後，扁平回應（cpu/memory）的行為與改寫前一致。
func TestMetricsFlatStillWorks(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/servers/3658462/cpu/", 200, fixture(t, "metrics_cpu"))
	s := newService(t, f)

	res, err := s.Metrics(context.Background(), 3658462, "cpu", "", "")
	require.NoError(t, err)
	require.Len(t, res.Value, 2)
	assert.Equal(t, "3.2", res.Value[0].Value)
	assert.Equal(t, "%", res.Value[0].Unit)
	assert.Empty(t, res.Value[0].Device)
	assert.Equal(t, "4.5", res.Value[1].Value)
}

// TestMetricsPrefersExpectedValueKeyOverLexicographicFirst 驗證 value 鍵優先採用各 meter
// 預期的鍵名（例如 cpu_util），即使回應裡混入字典序更前面的額外鍵（例如 "avg"）也不會誤選；
// 只有完全沒有任何預期鍵時才退回「字典序第一個非保留鍵」的舊規則。
func TestMetricsPrefersExpectedValueKeyOverLexicographicFirst(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/servers/3658462/cpu/", 200,
		[]byte(`[{"avg":"1.0","cpu_util":"3.2","timestamp":"2026-08-27T00:00:00Z","unit":"%"}]`))
	s := newService(t, f)

	res, err := s.Metrics(context.Background(), 3658462, "cpu", "", "")
	require.NoError(t, err)
	require.Len(t, res.Value, 1)
	assert.Equal(t, "3.2", res.Value[0].Value, "應優先採用預期鍵 cpu_util，而非字典序更前面的 avg")
}

// TestMetricsFallsBackToLexicographicKeyWhenNoExpectedKeyPresent 驗證回應中完全沒有任何
// 預期鍵時（例如上游改版新增了未知欄位名稱），仍退回「字典序第一個非保留鍵」的舊規則，
// 維持向下相容而不是直接報錯。
func TestMetricsFallsBackToLexicographicKeyWhenNoExpectedKeyPresent(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/servers/3658462/cpu/", 200,
		[]byte(`[{"zzz":"9.9","aaa":"1.1","timestamp":"2026-08-27T00:00:00Z","unit":"%"}]`))
	s := newService(t, f)

	res, err := s.Metrics(context.Background(), 3658462, "cpu", "", "")
	require.NoError(t, err)
	require.Len(t, res.Value, 1)
	assert.Equal(t, "1.1", res.Value[0].Value, "沒有任何預期鍵時退回字典序第一個非保留鍵")
}

// TestMetricsValueKeySelectionIsPerRequestedMeter 是 meterValueKeys 的鑑別測試：同一個
// 回應元素刻意同時帶有 disk_read_bytes_rate 與 disk_write_bytes_rate 兩個鍵（目前真實
// API 未觀察到這種情況，但用來驗證「只看這次查的 meter」而非「全域共用清單、依字典序找
// 第一個存在的鍵」——字典序 disk_read 排在 disk_write 之前，若沿用全域清單，查 disk-write
// 會被誤選成 disk_read 的值）。分別對 disk-write／disk-read 送出查詢，斷言各自採用自己
// 對應的鍵，不會被另一個 meter 的鍵蓋過。
func TestMetricsValueKeySelectionIsPerRequestedMeter(t *testing.T) {
	f := newFakeAPI(t)
	body := []byte(`[{"disk_read_bytes_rate":"100","disk_write_bytes_rate":"200","timestamp":"2026-08-27T00:00:00Z","unit":"B/s"}]`)
	f.on("GET", vcsBase+"/servers/3658462/disk/", 200, body)
	s := newService(t, f)

	resWrite, err := s.Metrics(context.Background(), 3658462, "disk-write", "", "")
	require.NoError(t, err)
	require.Len(t, resWrite.Value, 1)
	assert.Equal(t, "200", resWrite.Value[0].Value,
		"disk-write 應採用 disk_write_bytes_rate，不能被字典序更前面的 disk_read_bytes_rate 蓋過")

	resRead, err := s.Metrics(context.Background(), 3658462, "disk-read", "", "")
	require.NoError(t, err)
	require.Len(t, resRead.Value, 1)
	assert.Equal(t, "100", resRead.Value[0].Value, "disk-read 應採用 disk_read_bytes_rate")
}

// TestMetricsRejectsElementWithoutValue 驗證元素除了 timestamp/unit 之外沒有任何欄位時
// （沒有可當作 value 的鍵）回傳明確錯誤，而不是靜默給空字串。
func TestMetricsRejectsElementWithoutValue(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/servers/3658462/cpu/", 200, []byte(`[{"timestamp":"2026-08-27T00:00:00Z","unit":"%"}]`))
	s := newService(t, f)

	_, err := s.Metrics(context.Background(), 3658462, "cpu", "", "")
	assert.ErrorContains(t, err, "缺少數值欄位")
}

// TestMetricsRejectsNonNumericStringValue 驗證數值欄位是「非數字字串」時（例如 "abc"），
// 錯誤訊息明確指出不是數字字串，而不是與其他異常型別共用同一句「型別不是字串或數字」。
func TestMetricsRejectsNonNumericStringValue(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/servers/3658462/cpu/", 200, []byte(`[{"timestamp":"2026-08-27T00:00:00Z","unit":"%","value":"abc"}]`))
	s := newService(t, f)

	_, err := s.Metrics(context.Background(), 3658462, "cpu", "", "")
	assert.ErrorContains(t, err, "不是數字字串")
}

// TestMetricsRejectsNonStringNonNumberValue 驗證數值欄位是 bool 這類非字串非數字型別時，
// 維持原本「型別不是字串或數字」的錯誤訊息。
func TestMetricsRejectsNonStringNonNumberValue(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/servers/3658462/cpu/", 200, []byte(`[{"timestamp":"2026-08-27T00:00:00Z","unit":"%","value":true}]`))
	s := newService(t, f)

	_, err := s.Metrics(context.Background(), 3658462, "cpu", "", "")
	assert.ErrorContains(t, err, "型別不是字串或數字")
}

func TestMetricsDiskWriteSendsMeterName(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/servers/3658462/disk/", 200, []byte(`[{"disk_read_bytes_rate":"512","timestamp":"2026-08-27T01:00:00Z","unit":"B/s"}]`))
	s := newService(t, f)

	_, err := s.Metrics(context.Background(), 3658462, "disk-write", "", "")
	require.NoError(t, err)
	assert.Contains(t, f.Requests()[0].Query, "meter_name=disk_write_bytes_rate")
}

func TestMetricsMemory(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/servers/3658462/memory/", 200, []byte(`[{"memory_usage":"55.1","timestamp":"2026-08-27T01:00:00Z","unit":"%"}]`))
	s := newService(t, f)

	res, err := s.Metrics(context.Background(), 3658462, "memory", "", "")
	require.NoError(t, err)
	require.Len(t, res.Value, 1)
	assert.Equal(t, "55.1", res.Value[0].Value)
}

func TestMetricsUnsupportedMeterIsErrorAndSendsNothing(t *testing.T) {
	f := newFakeAPI(t)
	s := newService(t, f)

	_, err := s.Metrics(context.Background(), 3658462, "bogus", "", "")
	assert.Error(t, err)
	assert.Empty(t, f.Requests())
}

func TestResolveServerIDForSiteMatchesSite(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/servers/", 200, fixture(t, "servers"))
	s := newService(t, f)

	id, err := s.ResolveServerIDForSite(context.Background(), 101, 7)
	require.NoError(t, err)
	assert.Equal(t, int64(3658462), id)
}

func TestResolveServerIDForSiteNoMatchIsError(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/servers/", 200, fixture(t, "servers"))
	s := newService(t, f)

	_, err := s.ResolveServerIDForSite(context.Background(), 101, 99)
	assert.ErrorContains(t, err, "沒有 server")
}

func TestResolveServerIDForSiteMultipleMatchesIsError(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/servers/", 200, []byte(`[
		{"id":1,"site":7,"hostname":"a","status":"ACTIVE","flavor":"f","image":"i","keypair":"k","os":"Linux","os_version":"22.04","availability_zone":"nova","fqdn":"","platform":"p","private_nets":[],"public_nets":[],"auto_scaling_policy":{}},
		{"id":2,"site":7,"hostname":"b","status":"ACTIVE","flavor":"f","image":"i","keypair":"k","os":"Linux","os_version":"22.04","availability_zone":"nova","fqdn":"","platform":"p","private_nets":[],"public_nets":[],"auto_scaling_policy":{}}
	]`))
	s := newService(t, f)

	_, err := s.ResolveServerIDForSite(context.Background(), 101, 7)
	assert.ErrorContains(t, err, "--server")
}

func TestWaitServerStatusPollsUntilTarget(t *testing.T) {
	f := newFakeAPI(t)
	f.onSequence("GET", vcsBase+"/sites/7/",
		seq{Status: 200, Body: siteBodyWithServerStatus("STOPPING")},
		seq{Status: 200, Body: siteBodyWithServerStatus("SHUTOFF")})
	s := newService(t, f)

	var seen []string
	err := s.WaitServerStatus(context.Background(), 7, []string{"SHUTOFF"}, "", time.Millisecond, func(st string) { seen = append(seen, st) })
	require.NoError(t, err)
	assert.Equal(t, []string{"web-01-vm=STOPPING", "web-01-vm=SHUTOFF"}, seen)
}

// TestWaitServerStatusRequiresAllServers 驗證 site 有多台 server 時，要全部都進入 targets
// 才算完成：其中一台仍是 STOPPING 時不算數，等兩台都到 SHUTOFF 才完成。
func TestWaitServerStatusRequiresAllServers(t *testing.T) {
	f := newFakeAPI(t)
	f.onSequence("GET", vcsBase+"/sites/7/",
		seq{Status: 200, Body: siteBodyWithServers("SHUTOFF", "STOPPING")},
		seq{Status: 200, Body: siteBodyWithServers("SHUTOFF", "SHUTOFF")})
	s := newService(t, f)

	err := s.WaitServerStatus(context.Background(), 7, []string{"SHUTOFF"}, "", time.Millisecond, func(string) {})
	require.NoError(t, err)
	assert.Len(t, f.Requests(), 2)
}

// TestWaitServerStatusAnyServerErrorFails 驗證多台 server 中任一台進入 ERROR 就視為失敗，
// 即使目標的那一台已經到達 targets。
func TestWaitServerStatusAnyServerErrorFails(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/sites/7/", 200, siteBodyWithServers("ACTIVE", "ERROR"))
	s := newService(t, f)

	err := s.WaitServerStatus(context.Background(), 7, []string{"ACTIVE"}, "", time.Millisecond, func(string) {})
	assert.ErrorContains(t, err, "ERROR")
	assert.ErrorContains(t, err, "web-01-vm-1", "錯誤訊息必須指出哪一台 server 進入 ERROR")
}

// TestWaitServerStatusAllErrorServersAreListed 驗證有多台 server 同時進入 ERROR 時，
// 錯誤訊息必須列出「全部」進入 ERROR 的 server，而不是只有第一台
// （多台 server 各自失敗時，使用者需要知道完整清單才能逐一排查）。
func TestWaitServerStatusAllErrorServersAreListed(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/sites/7/", 200, siteBodyWithServers("ERROR", "ACTIVE", "ERROR"))
	s := newService(t, f)

	err := s.WaitServerStatus(context.Background(), 7, []string{"ACTIVE"}, "", time.Millisecond, func(string) {})
	assert.ErrorContains(t, err, "web-01-vm-0")
	assert.ErrorContains(t, err, "web-01-vm-2")
	assert.NotContains(t, err.Error(), "web-01-vm-1", "第 1 台是 ACTIVE，不應出現在 ERROR 清單中")
}

func TestWaitServerStatusErrorStatusIsFailure(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/sites/7/", 200, siteBodyWithServerStatus("ERROR"))
	s := newService(t, f)

	err := s.WaitServerStatus(context.Background(), 7, []string{"ACTIVE"}, "", time.Millisecond, func(string) {})
	assert.ErrorContains(t, err, "ERROR")
	assert.ErrorContains(t, err, "web-01-vm")
}

func TestWaitServerStatusNoServersIsFailure(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/sites/7/", 200, []byte(`{"callback":"","create_time":"2026-08-27T01:02:03Z","id":7,"is_preemptive":false,"name":"web-01","platform":"p","progress":100,"project":101,"public_ip":"","servers":[],"solution":11,"status":"Ready","termination_protection":false,"user":{"username":"alice","display_name":"Alice","email":"a@example.com","id":"u-1"}}`))
	s := newService(t, f)

	err := s.WaitServerStatus(context.Background(), 7, []string{"ACTIVE"}, "", time.Millisecond, func(string) {})
	assert.ErrorContains(t, err, "沒有 server 可供判斷狀態")
}

// TestWaitServerStatusMustLeave 驗證 mustLeave 非空、site 有兩台 server 時，要「每一台」都先
// 觀察到離開 mustLeave、且事後全部回到 targets 才算完成（reboot 的情境：送出後 server 通常
// 仍是 ACTIVE；兩台不會剛好同時重開，須各自追蹤是否已離開過 ACTIVE）。
func TestWaitServerStatusMustLeave(t *testing.T) {
	f := newFakeAPI(t)
	f.onSequence("GET", vcsBase+"/sites/7/",
		// 兩台都還沒開始重開。
		seq{Status: 200, Body: siteBodyWithServers("ACTIVE", "ACTIVE")},
		// 第 0 台先重開（已離開 ACTIVE），第 1 台還沒開始。
		seq{Status: 200, Body: siteBodyWithServers("REBOOT", "ACTIVE")},
		// 第 0 台重開完成回到 ACTIVE，第 1 台才開始重開（已離開 ACTIVE）；
		// 此時第 0 台雖已回到 targets，但第 1 台還在 REBOOT，尚未完成。
		seq{Status: 200, Body: siteBodyWithServers("ACTIVE", "REBOOT")},
		// 兩台都已各自離開過 ACTIVE、且都回到 ACTIVE：完成。
		seq{Status: 200, Body: siteBodyWithServers("ACTIVE", "ACTIVE")})
	s := newService(t, f)

	var seen []string
	err := s.WaitServerStatus(context.Background(), 7, []string{"ACTIVE"}, "ACTIVE", time.Millisecond, func(st string) { seen = append(seen, st) })
	require.NoError(t, err)
	assert.Equal(t, []string{
		"web-01-vm-0=ACTIVE, web-01-vm-1=ACTIVE",
		"web-01-vm-0=REBOOT, web-01-vm-1=ACTIVE",
		"web-01-vm-0=ACTIVE, web-01-vm-1=REBOOT",
		"web-01-vm-0=ACTIVE, web-01-vm-1=ACTIVE",
	}, seen)
	assert.Len(t, f.Requests(), 4)
}

// TestWaitServerStatusMustLeaveIgnoresEmptyStatus 驗證空／未知 status（status 欄位缺漏，
// 解析為空字串）不會被當成「已離開 mustLeave」：中間夾一輪空狀態的話，下一輪剛好又是
// ACTIVE 時，不可以提早判定 reboot 已完成。
func TestWaitServerStatusMustLeaveIgnoresEmptyStatus(t *testing.T) {
	f := newFakeAPI(t)
	f.onSequence("GET", vcsBase+"/sites/7/",
		seq{Status: 200, Body: siteBodyWithServerStatus("ACTIVE")},
		seq{Status: 200, Body: siteBodyWithNoServerStatus()},
		seq{Status: 200, Body: siteBodyWithServerStatus("ACTIVE")},
		seq{Status: 200, Body: siteBodyWithServerStatus("REBOOT")},
		seq{Status: 200, Body: siteBodyWithServerStatus("ACTIVE")})
	s := newService(t, f)

	var seen []string
	err := s.WaitServerStatus(context.Background(), 7, []string{"ACTIVE"}, "ACTIVE", time.Millisecond, func(st string) { seen = append(seen, st) })
	require.NoError(t, err)
	assert.Equal(t, []string{
		"web-01-vm=ACTIVE", "web-01-vm=", "web-01-vm=ACTIVE", "web-01-vm=REBOOT", "web-01-vm=ACTIVE",
	}, seen)
	assert.Len(t, f.Requests(), 5)
}
