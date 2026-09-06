package cli

import (
	"encoding/json"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lbSpecFixturePath 回傳 testdata/fixtures/vcs/lb_spec.yaml 的絕對路徑（不依賴 cwd，
// 與 fixture() 使用相同的 runtime.Caller 技巧，但這裡需要的是路徑本身而非內容，
// 因為 --spec 是要傳給 CLI 的檔案路徑參數）。
func lbSpecFixturePath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "testdata", "fixtures", "vcs", "lb_spec.yaml")
}

func TestVCSLbCreateShorthandBuildsSpec(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/networks/", 200, "vcs/networks")
	f.onFixture("POST", vcsBase+"/loadbalancers/", 200, "vcs/loadbalancer_created")
	code, stdout, stderr := runCLI(t, f.env(t), "",
		"vcs", "lb", "create", "--name", "web", "--network", "default_network", "--eip", "42",
		"--member", "10.0.0.11:80", "--member", "10.0.0.12:80:2", "--port", "80")
	require.Equal(t, ExitOK, code, stderr)

	req := f.find("POST", vcsBase+"/loadbalancers/")
	require.NotNil(t, req)
	assert.JSONEq(t, `{"name":"web","private_net":328291,"ip":42,
		"pools":[{"name":"web-pool","protocol":"HTTP","method":"ROUND_ROBIN",
			"members":[{"ip":"10.0.0.11","port":80},{"ip":"10.0.0.12","port":80,"weight":2}]}],
		"listeners":[{"name":"web-listener","pool_name":"web-pool","protocol":"HTTP","protocol_port":80}]}`,
		string(req.Body))
	assert.Contains(t, stdout, "web-lb")
	assert.Contains(t, stderr, "已建立 load balancer 5")
	// --eip 42 是數字，ResolveIPID 不應打 GET /ips/。
	assert.Nil(t, f.find("GET", vcsBase+"/ips/"))
}

func TestVCSLbCreateShorthandOptions(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/networks/", 200, "vcs/networks")
	f.onFixture("GET", vcsBase+"/secrets/", 200, "vcs/secrets")
	f.onFixture("POST", vcsBase+"/loadbalancers/", 200, "vcs/loadbalancer_created")
	code, _, stderr := runCLI(t, f.env(t), "",
		"vcs", "lb", "create", "--name", "web", "--network", "default_network",
		"--member", "10.0.0.11:80", "--port", "443",
		"--pool-protocol", "TCP", "--method", "SOURCE_IP",
		"--listener-protocol", "TERMINATED_HTTPS", "--tls-secret", "7", "--monitor-type", "TCP")
	require.Equal(t, ExitOK, code, stderr)

	req := f.find("POST", vcsBase+"/loadbalancers/")
	require.NotNil(t, req)
	assert.JSONEq(t, `{"name":"web","private_net":328291,
		"pools":[{"name":"web-pool","protocol":"TCP","method":"SOURCE_IP","monitor_type":"TCP",
			"members":[{"ip":"10.0.0.11","port":80}]}],
		"listeners":[{"name":"web-listener","pool_name":"web-pool","protocol":"TERMINATED_HTTPS",
			"protocol_port":443,"default_tls_container_ref":7}]}`,
		string(req.Body))

	// --listener-protocol 未指定時等於 --pool-protocol。
	f2 := newFakeAPI(t)
	f2.onFixture("GET", vcsBase+"/networks/", 200, "vcs/networks")
	f2.onFixture("POST", vcsBase+"/loadbalancers/", 200, "vcs/loadbalancer_created")
	code2, _, stderr2 := runCLI(t, f2.env(t), "",
		"vcs", "lb", "create", "--name", "web", "--network", "default_network",
		"--member", "10.0.0.11:80", "--port", "80", "--pool-protocol", "TCP")
	require.Equal(t, ExitOK, code2, stderr2)
	req2 := f2.find("POST", vcsBase+"/loadbalancers/")
	require.NotNil(t, req2)
	assert.JSONEq(t, `{"name":"web","private_net":328291,
		"pools":[{"name":"web-pool","protocol":"TCP","method":"ROUND_ROBIN","members":[{"ip":"10.0.0.11","port":80}]}],
		"listeners":[{"name":"web-listener","pool_name":"web-pool","protocol":"TCP","protocol_port":80}]}`,
		string(req2.Body))

	// --tls-secret 但 listener 非 TERMINATED_HTTPS → exit 1。
	f3 := newFakeAPI(t)
	code3, _, stderr3 := runCLI(t, f3.env(t), "",
		"vcs", "lb", "create", "--name", "web", "--network", "default_network",
		"--member", "10.0.0.11:80", "--port", "80", "--tls-secret", "7")
	assert.Equal(t, ExitGeneral, code3, stderr3)
	assert.Nil(t, f3.find("GET", vcsBase+"/networks/"), "驗證失敗不該送出任何請求")
}

// TestVCSLbCreateTerminatedHTTPSRequiresTLSSecret 驗證 --listener-protocol
// TERMINATED_HTTPS 但未指定 --tls-secret 時在送出任何請求前報錯（exit 1）。
// 注意：--pool-protocol 的合法值不含 TERMINATED_HTTPS（lbPoolProtocols 只有
// TCP/HTTP/HTTPS），所以 effectiveListener 唯一會等於 TERMINATED_HTTPS 的路徑是
// 明確指定 --listener-protocol。
func TestVCSLbCreateTerminatedHTTPSRequiresTLSSecret(t *testing.T) {
	f := newFakeAPI(t)
	code, _, stderr := runCLI(t, f.env(t), "",
		"vcs", "lb", "create", "--name", "web", "--network", "default_network",
		"--member", "10.0.0.11:80", "--port", "443", "--listener-protocol", "TERMINATED_HTTPS")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "TERMINATED_HTTPS 需指定 --tls-secret")
	assert.Nil(t, f.find("GET", vcsBase+"/networks/"), "驗證失敗不該送出任何請求")
}

func TestVCSLbCreateSpecFile(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/networks/", 200, "vcs/networks")
	f.onFixture("POST", vcsBase+"/loadbalancers/", 200, "vcs/loadbalancer_created")
	code, _, stderr := runCLI(t, f.env(t), "",
		"vcs", "lb", "create", "--name", "web", "--network", "default_network", "--spec", lbSpecFixturePath(t))
	require.Equal(t, ExitOK, code, stderr)
	req := f.find("POST", vcsBase+"/loadbalancers/")
	require.NotNil(t, req)
	assert.JSONEq(t, `{"name":"web","private_net":328291,
		"pools":[{"name":"spec-pool","protocol":"HTTP","method":"ROUND_ROBIN",
			"members":[{"ip":"10.0.1.1","port":8080},{"ip":"10.0.1.2","port":8080}]}],
		"listeners":[{"name":"spec-listener","pool_name":"spec-pool","protocol":"HTTP","protocol_port":8080}]}`,
		string(req.Body))

	// --spec 與 --member 並用 → exit 1。
	f2 := newFakeAPI(t)
	code2, _, stderr2 := runCLI(t, f2.env(t), "",
		"vcs", "lb", "create", "--name", "web", "--network", "default_network",
		"--spec", lbSpecFixturePath(t), "--member", "10.0.0.11:80")
	assert.Equal(t, ExitGeneral, code2, stderr2)
	assert.Nil(t, f2.find("GET", vcsBase+"/networks/"))
}

func TestVCSLbCreateValidation(t *testing.T) {
	cases := [][]string{
		{"vcs", "lb", "create", "--network", "default_network", "--member", "10.0.0.11:80", "--port", "80"},
		{"vcs", "lb", "create", "--name", "web", "--member", "10.0.0.11:80", "--port", "80"},
		{"vcs", "lb", "create", "--name", "web", "--network", "default_network", "--port", "80"},
		{"vcs", "lb", "create", "--name", "web", "--network", "default_network", "--member", "10.0.0.11:80"},
		{"vcs", "lb", "create", "--name", "web", "--network", "default_network", "--member", "x", "--port", "80"},
		{"vcs", "lb", "create", "--name", "web", "--network", "default_network", "--member", "1.2.3.4", "--port", "80"},
		{"vcs", "lb", "create", "--name", "web", "--network", "default_network", "--member", "1.2.3.4:abc", "--port", "80"},
	}
	for _, args := range cases {
		f := newFakeAPI(t)
		code, _, stderr := runCLI(t, f.env(t), "", args...)
		assert.Equal(t, ExitGeneral, code, "args=%v stderr=%s", args, stderr)
		assert.Nil(t, f.find("POST", vcsBase+"/loadbalancers/"), "args=%v", args)
	}
}

func TestVCSLbCreateWait(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/networks/", 200, "vcs/networks")
	f.onFixture("POST", vcsBase+"/loadbalancers/", 200, "vcs/loadbalancer_created")
	f.onSequence("GET", vcsBase+"/loadbalancers/5/",
		seq{status: 200, body: `{"id": 5, "status": "BUILD"}`},
		seq{status: 200, body: `{"id": 5, "status": "ACTIVE"}`})
	code, _, stderr := runCLI(t, f.env(t), "",
		"vcs", "lb", "create", "--name", "web", "--network", "default_network",
		"--member", "10.0.0.11:80", "--port", "80", "--wait", "--wait-interval", "1ms")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stderr, "ACTIVE")
}

func TestVCSLbUpdateSpec(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/loadbalancers/", 200, "vcs/loadbalancers")
	f.onFixture("PATCH", vcsBase+"/loadbalancers/5/", 200, "vcs/loadbalancer_detail")
	code, stdout, stderr := runCLI(t, f.env(t), "",
		"vcs", "lb", "update", "web-lb", "--spec", lbSpecFixturePath(t))
	require.Equal(t, ExitOK, code, stderr)
	req := f.find("PATCH", vcsBase+"/loadbalancers/5/")
	require.NotNil(t, req)
	assert.JSONEq(t, `{"pools":[{"name":"spec-pool","protocol":"HTTP","method":"ROUND_ROBIN",
		"members":[{"ip":"10.0.1.1","port":8080},{"ip":"10.0.1.2","port":8080}]}],
		"listeners":[{"name":"spec-listener","pool_name":"spec-pool","protocol":"HTTP","protocol_port":8080}]}`,
		string(req.Body))
	assert.Contains(t, stdout, "web-lb")
}

func TestVCSLbActionAssociateIP(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/loadbalancers/", 200, "vcs/loadbalancers")
	f.on("PUT", vcsBase+"/loadbalancers/5/action/", 202, "")
	code, _, stderr := runCLI(t, f.env(t), "",
		"vcs", "lb", "action", "web-lb", "associate-ip", "--eip", "42")
	require.Equal(t, ExitOK, code, stderr)
	req := f.find("PUT", vcsBase+"/loadbalancers/5/action/")
	require.NotNil(t, req)
	assert.JSONEq(t, `{"action":"associateIP","ip":42}`, string(req.Body))
	assert.Contains(t, stderr, "已送出 associate-ip")
}

func TestVCSLbActionDisassociateConfirm(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/loadbalancers/", 200, "vcs/loadbalancers")
	f.on("PUT", vcsBase+"/loadbalancers/5/action/", 202, "")
	code, _, stderr := runCLI(t, f.env(t), "n\n", "vcs", "lb", "action", "web-lb", "disassociate-ip")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Nil(t, f.find("PUT", vcsBase+"/loadbalancers/5/action/"))

	code2, _, stderr2 := runCLI(t, f.env(t), "", "vcs", "lb", "action", "web-lb", "disassociate-ip", "-y")
	require.Equal(t, ExitOK, code2, stderr2)
	req := f.find("PUT", vcsBase+"/loadbalancers/5/action/")
	require.NotNil(t, req)
	assert.JSONEq(t, `{"action":"disassociateIP"}`, string(req.Body))
}

func TestVCSLbActionUpdateListener(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/loadbalancers/", 200, "vcs/loadbalancers")
	f.on("PUT", vcsBase+"/loadbalancers/5/action/", 202, "")
	code, _, stderr := runCLI(t, f.env(t), "",
		"vcs", "lb", "action", "web-lb", "update-listener", "--listener", "web-listener", "--timeout-client-data", "60000")
	require.Equal(t, ExitOK, code, stderr)
	req := f.find("PUT", vcsBase+"/loadbalancers/5/action/")
	require.NotNil(t, req)
	assert.JSONEq(t, `{"action":"updateListenerParams","listener_name":"web-listener","timeout_client_data":60000}`, string(req.Body))

	f2 := newFakeAPI(t)
	f2.onFixture("GET", vcsBase+"/loadbalancers/", 200, "vcs/loadbalancers")
	code2, _, stderr2 := runCLI(t, f2.env(t), "", "vcs", "lb", "action", "web-lb", "update-listener")
	assert.Equal(t, ExitGeneral, code2, stderr2)
	assert.Nil(t, f2.find("PUT", vcsBase+"/loadbalancers/5/action/"))
}

// TestVCSLbActionUpdateListenerRequiresTimeoutFlag 驗證 update-listener 即使已指定
// --listener，若四個 --timeout-* 一個都沒帶，仍要在送出請求前報錯（exit 1）——
// 只帶 --listener 沒有任何逾時參數等於這次動作什麼都不會改變，靜默送出沒有意義。
func TestVCSLbActionUpdateListenerRequiresTimeoutFlag(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/loadbalancers/", 200, "vcs/loadbalancers")
	code, _, stderr := runCLI(t, f.env(t), "",
		"vcs", "lb", "action", "web-lb", "update-listener", "--listener", "web-listener")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "update-listener 至少需指定一個 --timeout-*")
	assert.Nil(t, f.find("PUT", vcsBase+"/loadbalancers/5/action/"))
}

// TestVCSLbActionRejectsFlagsNotAllowedForAction 驗證動作專屬旗標依 action 限制：
// --eip 只能搭配 associate-ip，--listener 與四個 --timeout-* 只能搭配 update-listener，
// 用在其他動作時在送出請求前就直接報錯，而不是被靜默忽略。
func TestVCSLbActionRejectsFlagsNotAllowedForAction(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"eip 用在 disassociate-ip", []string{"vcs", "lb", "action", "web-lb", "disassociate-ip", "--eip", "42"}},
		{"eip 用在 update-listener", []string{"vcs", "lb", "action", "web-lb", "update-listener", "--listener", "l", "--eip", "42"}},
		{"listener 用在 associate-ip", []string{"vcs", "lb", "action", "web-lb", "associate-ip", "--eip", "42", "--listener", "l"}},
		{"timeout-client-data 用在 associate-ip", []string{"vcs", "lb", "action", "web-lb", "associate-ip", "--eip", "42", "--timeout-client-data", "1000"}},
		{"timeout-member-connect 用在 disassociate-ip", []string{"vcs", "lb", "action", "web-lb", "disassociate-ip", "--timeout-member-connect", "1000"}},
		{"timeout-member-data 用在 associate-ip", []string{"vcs", "lb", "action", "web-lb", "associate-ip", "--eip", "42", "--timeout-member-data", "1000"}},
		{"timeout-tcp-inspect 用在 disassociate-ip", []string{"vcs", "lb", "action", "web-lb", "disassociate-ip", "--timeout-tcp-inspect", "1000"}},
	}
	for _, c := range cases {
		f := newFakeAPI(t)
		f.onFixture("GET", vcsBase+"/loadbalancers/", 200, "vcs/loadbalancers")
		code, _, stderr := runCLI(t, f.env(t), "", c.args...)
		assert.Equal(t, ExitGeneral, code, "case=%s stderr=%s", c.name, stderr)
		assert.Nil(t, f.find("PUT", vcsBase+"/loadbalancers/5/action/"), "case=%s：不合法的旗標組合不該送出請求", c.name)
	}
}

// TestVCSLbActionWaitRequiresLeavingActive 驗證 `lb action --wait` 改用
// WaitLoadBalancerUpdated 之後，load balancer 必須先觀察到離開 ACTIVE 才會判定完成
// （而不是像舊寫法那樣，第一次輪詢讀到原本就是 ACTIVE 就立刻誤判為已完成）。
func TestVCSLbActionWaitRequiresLeavingActive(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/loadbalancers/", 200, "vcs/loadbalancers")
	f.on("PUT", vcsBase+"/loadbalancers/5/action/", 202, "")
	f.onSequence("GET", vcsBase+"/loadbalancers/5/",
		seq{status: 200, body: `{"id": 5, "status": "ACTIVE"}`},
		seq{status: 200, body: `{"id": 5, "status": "UPDATING"}`},
		seq{status: 200, body: `{"id": 5, "status": "ACTIVE"}`})
	code, _, stderr := runCLI(t, f.env(t), "",
		"vcs", "lb", "action", "web-lb", "associate-ip", "--eip", "42", "--wait", "--wait-interval", "1ms")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stderr, "UPDATING")

	// load balancer 一直停在 ACTIVE（從未被觀察到離開過）：--wait 不會在第一輪就誤判為
	// 已完成，會持續輪詢直到 --wait-timeout 逾時（exit 5）。
	f2 := newFakeAPI(t)
	f2.onFixture("GET", vcsBase+"/loadbalancers/", 200, "vcs/loadbalancers")
	f2.on("PUT", vcsBase+"/loadbalancers/5/action/", 202, "")
	f2.on("GET", vcsBase+"/loadbalancers/5/", 200, `{"id": 5, "status": "ACTIVE"}`)
	code2, _, stderr2 := runCLI(t, f2.env(t), "",
		"vcs", "lb", "action", "web-lb", "associate-ip", "--eip", "42", "--wait",
		"--wait-timeout", "30ms", "--wait-interval", "1ms")
	assert.Equal(t, ExitWaitTimeout, code2, stderr2)
}

// TestVCSLbCreateJSONOutputIsSingleObjectRegardlessOfArrayResponse 驗證
// `vcs lb create -o json` 不論 POST 回應是陣列（vcs/loadbalancer_created fixture）
// 還是單一物件，輸出的都是單一物件——修正前陣列回應時 Raw 會是整個陣列，導致
// 加不加 --wait 輸出形狀不一致。
func TestVCSLbCreateJSONOutputIsSingleObjectRegardlessOfArrayResponse(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/networks/", 200, "vcs/networks")
	f.onFixture("POST", vcsBase+"/loadbalancers/", 200, "vcs/loadbalancer_created")
	code, stdout, stderr := runCLI(t, f.env(t), "",
		"vcs", "lb", "create", "--name", "web", "--network", "default_network",
		"--member", "10.0.0.11:80", "--port", "80", "-o", "json")
	require.Equal(t, ExitOK, code, stderr)

	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got), "stdout 應該是單一 JSON 物件，不是陣列：%s", stdout)
	assert.Equal(t, float64(5), got["id"])
}

// TestVCSLbCreatePortValidation 驗證 --port 套用與 --member 的 port 相同的
// 1–65535 範圍檢查，不會被原樣送到 API。
func TestVCSLbCreatePortValidation(t *testing.T) {
	for _, port := range []string{"0", "70000"} {
		f := newFakeAPI(t)
		code, _, stderr := runCLI(t, f.env(t), "",
			"vcs", "lb", "create", "--name", "web", "--network", "default_network",
			"--member", "10.0.0.11:80", "--port", port)
		assert.Equal(t, ExitGeneral, code, "port=%s stderr=%s", port, stderr)
		assert.Nil(t, f.find("GET", vcsBase+"/networks/"), "port=%s：不合法的 --port 不該送出任何請求", port)
	}
}

// TestVCSLbCreateSpecAtPrefixOptional 驗證 `--spec @file` 的 `@` 前綴可省略
// （與不加前綴等價，`--spec` 只接受檔案路徑，沒有內嵌字串的用法）。
func TestVCSLbCreateSpecAtPrefixOptional(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/networks/", 200, "vcs/networks")
	f.onFixture("POST", vcsBase+"/loadbalancers/", 200, "vcs/loadbalancer_created")
	code, _, stderr := runCLI(t, f.env(t), "",
		"vcs", "lb", "create", "--name", "web", "--network", "default_network",
		"--spec", "@"+lbSpecFixturePath(t))
	require.Equal(t, ExitOK, code, stderr)
	req := f.find("POST", vcsBase+"/loadbalancers/")
	require.NotNil(t, req)
	assert.JSONEq(t, `{"name":"web","private_net":328291,
		"pools":[{"name":"spec-pool","protocol":"HTTP","method":"ROUND_ROBIN",
			"members":[{"ip":"10.0.1.1","port":8080},{"ip":"10.0.1.2","port":8080}]}],
		"listeners":[{"name":"spec-listener","pool_name":"spec-pool","protocol":"HTTP","protocol_port":8080}]}`,
		string(req.Body))
}

// TestVCSLbCreateSpecMissingFileSendsNoRequests 驗證 --spec 指向不存在的檔案時，
// 讀檔失敗發生在任何網路請求之前（與 `vcs lb update` 一致）。
func TestVCSLbCreateSpecMissingFileSendsNoRequests(t *testing.T) {
	f := newFakeAPI(t)
	code, _, stderr := runCLI(t, f.env(t), "",
		"vcs", "lb", "create", "--name", "web", "--network", "default_network",
		"--spec", "/not/exists")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Empty(t, f.Requests(), "--spec 檔案不存在時不該送出任何請求")
}

// TestVCSLbCreateEipByAddressResolvesViaIPList 驗證 --eip 為位址（而非數字 id）時，
// 會呼叫 GET /ips/ 解析（對照 TestVCSLbCreateShorthandBuildsSpec 已涵蓋的數字分支：
// 數字不查 API）。
func TestVCSLbCreateEipByAddressResolvesViaIPList(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/networks/", 200, "vcs/networks")
	f.onFixture("GET", vcsBase+"/ips/", 200, "vcs/ips")
	f.onFixture("POST", vcsBase+"/loadbalancers/", 200, "vcs/loadbalancer_created")
	code, _, stderr := runCLI(t, f.env(t), "",
		"vcs", "lb", "create", "--name", "web", "--network", "default_network",
		"--eip", "203.0.113.219", "--member", "10.0.0.11:80", "--port", "80")
	require.Equal(t, ExitOK, code, stderr)
	req := f.find("POST", vcsBase+"/loadbalancers/")
	require.NotNil(t, req)
	assert.NotNil(t, f.find("GET", vcsBase+"/ips/"), "--eip 為位址時應查 GET /ips/")
	var body map[string]any
	require.NoError(t, json.Unmarshal(req.Body, &body))
	assert.Equal(t, float64(1393847), body["ip"])
}

// TestVCSLbCreateWaitColumnsUsesDetailColumns 驗證 --wait 時 --columns 的合法值改用
// lbDetailColumns（而不是 lbColumns）驗證：VIP 只存在於 --wait 之後重新查詢的
// LBDetailSerializer，非 --wait 的 POST 回應（LBSerializer）沒有這個欄位。
func TestVCSLbCreateWaitColumnsUsesDetailColumns(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/networks/", 200, "vcs/networks")
	f.onFixture("POST", vcsBase+"/loadbalancers/", 200, "vcs/loadbalancer_created")
	f.on("GET", vcsBase+"/loadbalancers/5/", 200, `{"id": 5, "status": "ACTIVE", "vip": "10.0.0.50"}`)
	code, stdout, stderr := runCLI(t, f.env(t), "",
		"vcs", "lb", "create", "--name", "web", "--network", "default_network",
		"--member", "10.0.0.11:80", "--port", "80", "--wait", "--wait-interval", "1ms",
		"--columns", "VIP")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stdout, "10.0.0.50")
}

// TestParseLbMemberIPv6 驗證 parseLbMember 支援 IPv6 格式 `[ipv6]:port[:weight]`，
// 與 IPv4 的 `ip:port[:weight]` 共用同一套 port/weight 驗證邏輯。
func TestParseLbMemberIPv6(t *testing.T) {
	cases := []struct {
		name       string
		input      string
		wantIP     string
		wantPort   int64
		wantWeight *int64
	}{
		{"純 IPv6 無 weight", "[2001:db8::1]:80", "2001:db8::1", 80, nil},
		{"IPv6 含 weight", "[2001:db8::1]:8080:2", "2001:db8::1", 8080, int64Ptr(2)},
		{"IPv6 loopback", "[::1]:80", "::1", 80, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			member, err := parseLbMember(tc.input)
			require.NoError(t, err)
			require.NotNil(t, member.Ip)
			assert.Equal(t, tc.wantIP, *member.Ip)
			require.NotNil(t, member.Port)
			assert.Equal(t, tc.wantPort, *member.Port)
			if tc.wantWeight == nil {
				assert.Nil(t, member.Weight)
			} else {
				require.NotNil(t, member.Weight)
				assert.Equal(t, *tc.wantWeight, *member.Weight)
			}
		})
	}
}

// TestParseLbMemberIPv6Errors 驗證 IPv6 相關的錯誤情境：中括號內缺 port、
// 以及未加中括號的裸 IPv6 位址（訊息需提示改用 [ ]）。
func TestParseLbMemberIPv6Errors(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantMsg string
	}{
		{"中括號內缺 port", "[2001:db8::1]", "]"},
		{"裸 IPv6 未加中括號", "2001:db8::1:80", "[ ]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseLbMember(tc.input)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantMsg)
		})
	}
}

// int64Ptr 回傳指向 v 的指標，供表格測試比較 *int64 欄位使用。
func int64Ptr(v int64) *int64 { return &v }

// TestVCSLbCreateShorthandIPv6Member 驗證 `vcs lb create` 簡寫模式的 --member 端對端
// 支援 IPv6（透過 CLI 層而非只呼叫 parseLbMember，確保 buildShorthandSpec／POST body
// 也能正確帶出 IPv6 member）。
func TestVCSLbCreateShorthandIPv6Member(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/networks/", 200, "vcs/networks")
	f.onFixture("POST", vcsBase+"/loadbalancers/", 200, "vcs/loadbalancer_created")
	code, _, stderr := runCLI(t, f.env(t), "",
		"vcs", "lb", "create", "--name", "web", "--network", "default_network",
		"--member", "[2001:db8::1]:8080:2", "--port", "8080")
	require.Equal(t, ExitOK, code, stderr)

	req := f.find("POST", vcsBase+"/loadbalancers/")
	require.NotNil(t, req)
	var body map[string]any
	require.NoError(t, json.Unmarshal(req.Body, &body))
	pools := body["pools"].([]any)
	members := pools[0].(map[string]any)["members"].([]any)
	member := members[0].(map[string]any)
	assert.Equal(t, "2001:db8::1", member["ip"])
	assert.Equal(t, float64(8080), member["port"])
	assert.Equal(t, float64(2), member["weight"])
}
