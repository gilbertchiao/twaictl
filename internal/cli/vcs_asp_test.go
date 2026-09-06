package cli

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilbertchiao/twaictl/internal/testutil"
)

func TestVCSAspLsGolden(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/auto_scaling_policies/", 200, "vcs/auto_scaling_policies")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "asp", "ls")
	require.Equal(t, ExitOK, code, stderr)
	assert.Equal(t, "project=101", f.find("GET", vcsBase+"/auto_scaling_policies/").Query)
	testutil.AssertGolden(t, "vcs_asp_ls.txt", []byte(stdout))
}

// TestVCSAspLsColumnsGolden 覆蓋 702（無 description）的 nil 分支：--columns 明確指定 desc 時，
// 702 那一列的 DESC 欄位須輸出空字串而非 panic。
func TestVCSAspLsColumnsGolden(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/auto_scaling_policies/", 200, "vcs/auto_scaling_policies")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "asp", "ls", "--columns", "id,name,desc,user")
	require.Equal(t, ExitOK, code, stderr)
	testutil.AssertGolden(t, "vcs_asp_ls_columns.txt", []byte(stdout))
}

func TestVCSAspGetGolden(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/auto_scaling_policies/", 200, "vcs/auto_scaling_policies")
	f.onFixture("GET", vcsBase+"/auto_scaling_policies/701/", 200, "vcs/asp_detail")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "asp", "get", "web-asp")
	require.Equal(t, ExitOK, code, stderr)
	testutil.AssertGolden(t, "vcs_asp_get.txt", []byte(stdout))
}

func TestVCSAspCreateMapsMeterShortName(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("POST", vcsBase+"/auto_scaling_policies/", 200, "vcs/asp_detail")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "asp", "create", "--name", "web-asp",
		"--meter", "cpu", "--scale-up", "80", "--scale-down", "20", "--max-size", "5", "--desc", "scale web tier")
	require.Equal(t, ExitOK, code, stderr)
	req := f.find("POST", vcsBase+"/auto_scaling_policies/")
	require.NotNil(t, req)
	assert.JSONEq(t, `{"name":"web-asp","project":101,"description":"scale web tier","meter_name":"cpu_util","scaleup_threshold":80,"scaledown_threshold":20,"scale_max_size":5}`, string(req.Body))
	assert.Contains(t, stdout, "cpu_util")
	assert.Contains(t, stderr, "已建立 auto scaling policy 701")
}

func TestVCSAspCreateValidatesFlags(t *testing.T) {
	f := newFakeAPI(t)
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"--meter", "cpu", "--scale-up", "80", "--max-size", "5"}, "--name"},
		{[]string{"--name", "a", "--scale-up", "80", "--max-size", "5"}, "--meter"},
		{[]string{"--name", "a", "--meter", "gpu", "--scale-up", "80", "--max-size", "5"}, "--meter"},
		{[]string{"--name", "a", "--meter", "cpu", "--max-size", "5"}, "--scale-up"},
		{[]string{"--name", "a", "--meter", "cpu", "--scale-up", "80"}, "--max-size"},
		{[]string{"--name", "a", "--meter", "cpu", "--scale-up", "80", "--max-size", "0"}, "--max-size"},
		{[]string{"--name", "a", "--meter", "cpu", "--scale-up", "0", "--max-size", "5"}, "--scale-up"},
		{[]string{"--name", "a", "--meter", "cpu", "--scale-up", "80", "--scale-down", "-1", "--max-size", "5"}, "--scale-down"},
		{[]string{"--name", "a", "--meter", "cpu", "--scale-up", "20", "--scale-down", "80", "--max-size", "5"}, "--scale-down 必須小於 --scale-up"},
	}
	for _, tc := range cases {
		code, _, stderr := runCLI(t, f.env(t), "", append([]string{"vcs", "asp", "create"}, tc.args...)...)
		assert.Equal(t, ExitGeneral, code, tc.args)
		assert.Contains(t, stderr, tc.want, tc.args)
	}
	assert.Nil(t, f.find("POST", vcsBase+"/auto_scaling_policies/"))
}

func TestVCSAspRm(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/auto_scaling_policies/", 200, "vcs/auto_scaling_policies")
	f.onText("DELETE", vcsBase+"/auto_scaling_policies/701/", 204, "")
	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "asp", "rm", "web-asp", "-y")
	require.Equal(t, ExitOK, code, stderr)
	assert.NotNil(t, f.find("DELETE", vcsBase+"/auto_scaling_policies/701/"))
	assert.Contains(t, stderr, "已刪除 auto scaling policy 701")
}

// TestVCSAspAttachBySiteName 驗證 --site（名稱解析）與 --lb（名稱解析）都真的走過
// resolve 流程：--lb 用 "web-lb"（testdata/fixtures/vcs/loadbalancers.json 內 id 5 那筆
// 的 name），而不是像先前那樣直接給一個 fixture 內根本不存在的數字 id（313004）——
// 數字 ref 會被 ResolveLoadBalancerID 短路（twai.IsNumericRef），完全不會查
// GET /loadbalancers/，導致這裡註冊的 fixture 從未被真正命中過。
// 送出的請求 body 因此帶著解析後的 id（5），至於 POST 的回應（asp_attached fixture）
// 仍照原樣回 loadbalancer=313004——mock 的回應本來就不需要跟請求內容一致，
// stdout 對 313004 的斷言驗證的是「渲染 API 回應」這件事，與 --lb 解析是否正確無關。
func TestVCSAspAttachBySiteName(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/auto_scaling_policies/", 200, "vcs/auto_scaling_policies")
	f.onFixture("GET", vcsBase+"/sites/", 200, "vcs/sites")
	f.onFixture("GET", vcsBase+"/servers/", 200, "vcs/servers")
	f.onFixture("GET", vcsBase+"/loadbalancers/", 200, "vcs/loadbalancers")
	f.onFixture("POST", vcsBase+"/servers/3658462/auto_scaling_policy/", 201, "vcs/asp_attached")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "asp", "attach", "web-asp", "--site", "web-01",
		"--lb", "web-lb", "--port", "8080", "--scale-up-action", "https://hook/up")
	require.Equal(t, ExitOK, code, stderr)
	assert.NotNil(t, f.find("GET", vcsBase+"/loadbalancers/"), "--lb 用名稱時應真的查過 load balancer 清單")
	req := f.find("POST", vcsBase+"/servers/3658462/auto_scaling_policy/")
	require.NotNil(t, req)
	assert.JSONEq(t, `{"auto_scaling_policy":701,"loadbalancer":5,"protocol_port":8080,"scaleup_action":"https://hook/up"}`, string(req.Body))
	assert.Contains(t, stderr, "已將 auto scaling policy 701 掛到 server 3658462")
	assert.Contains(t, stdout, "313004")
}

// TestVCSAspAttachByServerID 驗證 <id>／--server 都用數字 id 時，完全不查任何清單
// （既有 --server 不查 sites 的斷言之外，policy ref "702" 也是數字，ResolveAutoScalingPolicyID
// 同樣會短路，不查 /auto_scaling_policies/，因此不註冊那個 fixture——先前註冊了卻從未
// 命中過，是死的 test setup）。
func TestVCSAspAttachByServerID(t *testing.T) {
	f := newFakeAPI(t)
	f.on("POST", vcsBase+"/servers/3658463/auto_scaling_policy/", 201, `{"auto_scaling_policy": 702}`)
	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "asp", "attach", "702", "--server", "3658463")
	require.Equal(t, ExitOK, code, stderr)
	assert.JSONEq(t, `{"auto_scaling_policy":702}`, string(f.find("POST", vcsBase+"/servers/3658463/auto_scaling_policy/").Body))
	assert.Nil(t, f.find("GET", vcsBase+"/sites/"), "--server 直接用 id，不應查 sites")
	assert.Nil(t, f.find("GET", vcsBase+"/auto_scaling_policies/"), "<id> 直接用數字，不應查 auto scaling policy 清單")
}

// TestVCSAspAttachJSONOutputIsSingleObject 確認 `-o json` 走的 renderOne 的 Raw 路徑
// 對 attach 也是單一 JSON object（不是陣列），且 --output json 保留 API 原值。
func TestVCSAspAttachJSONOutputIsSingleObject(t *testing.T) {
	f := newFakeAPI(t)
	f.on("POST", vcsBase+"/servers/3658463/auto_scaling_policy/", 201, `{"auto_scaling_policy": 702}`)
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "asp", "attach", "702", "--server", "3658463", "-o", "json")
	require.Equal(t, ExitOK, code, stderr)

	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got), "stdout 必須是單一 JSON object：%s", stdout)
	assert.InDelta(t, 702, got["auto_scaling_policy"], 0)
}

func TestVCSAspAttachRequiresExactlyOneTarget(t *testing.T) {
	f := newFakeAPI(t)
	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "asp", "attach", "702")
	assert.Equal(t, ExitGeneral, code)
	assert.Contains(t, stderr, "--site 或 --server")
	code, _, stderr = runCLI(t, f.env(t), "", "vcs", "asp", "attach", "702", "--site", "web-01", "--server", "1")
	assert.Equal(t, ExitGeneral, code)
	assert.Contains(t, stderr, "不可同時使用")
	code, _, stderr = runCLI(t, f.env(t), "", "vcs", "asp", "attach", "702", "--server", "1", "--port", "70000")
	assert.Equal(t, ExitGeneral, code)
	assert.Contains(t, stderr, "--port")
}

// TestVCSAspAttachPortWithoutLB 確認 --port 可以單獨指定、不強制搭配 --lb：spec 把
// protocol_port 與 loadbalancer 定義為彼此獨立的選填欄位（AttachAutoScalingPolicyInput
// 也支援單獨送 ProtocolPort），CLI 不該比 API 更嚴格地縮限這個組合；--port 的用途在
// 沒有 --lb 時由 API 自行決定，twaictl 只在 help 文字提醒、不擋下。
func TestVCSAspAttachPortWithoutLB(t *testing.T) {
	f := newFakeAPI(t)
	f.on("POST", vcsBase+"/servers/3658463/auto_scaling_policy/", 201, `{"auto_scaling_policy": 702}`)
	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "asp", "attach", "702", "--server", "3658463", "--port", "8080")
	require.Equal(t, ExitOK, code, stderr)
	req := f.find("POST", vcsBase+"/servers/3658463/auto_scaling_policy/")
	require.NotNil(t, req)
	assert.JSONEq(t, `{"auto_scaling_policy":702,"protocol_port":8080}`, string(req.Body))
}

func TestVCSAspDetachRequiresConfirm(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/sites/", 200, "vcs/sites")
	f.onFixture("GET", vcsBase+"/servers/", 200, "vcs/servers")
	f.onText("DELETE", vcsBase+"/servers/3658462/auto_scaling_policy/", 204, "")
	code, _, _ := runCLI(t, f.env(t), "n\n", "vcs", "asp", "detach", "--site", "web-01")
	assert.NotEqual(t, ExitOK, code)
	assert.Nil(t, f.find("DELETE", vcsBase+"/servers/3658462/auto_scaling_policy/"))

	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "asp", "detach", "--site", "web-01", "-y")
	require.Equal(t, ExitOK, code, stderr)
	assert.NotNil(t, f.find("DELETE", vcsBase+"/servers/3658462/auto_scaling_policy/"))
	assert.Contains(t, stderr, "已移除 server 3658462 的 auto scaling policy")
}

// TestVCSAspDetachByServerID 驗證 --server 直接用 id 時（resolveAspTarget 的
// serverRef 分支），完全不查 sites／servers 清單，與 attach 的 --server 分支
// （TestVCSAspAttachByServerID）對稱。
func TestVCSAspDetachByServerID(t *testing.T) {
	f := newFakeAPI(t)
	f.onText("DELETE", vcsBase+"/servers/3658463/auto_scaling_policy/", 204, "")
	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "asp", "detach", "--server", "3658463", "-y")
	require.Equal(t, ExitOK, code, stderr)
	assert.NotNil(t, f.find("DELETE", vcsBase+"/servers/3658463/auto_scaling_policy/"))
	assert.Nil(t, f.find("GET", vcsBase+"/sites/"), "--server 直接用 id，不應查 sites")
	assert.Contains(t, stderr, "已移除 server 3658463 的 auto scaling policy")
}
