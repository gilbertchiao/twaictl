package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilbertchiao/twaictl/internal/testutil"
)

func TestVCSFirewallLsGolden(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/firewalls/", 200, "vcs/firewalls")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "firewall", "ls")
	require.Equal(t, ExitOK, code, stderr)
	assert.Equal(t, "project=101", f.find("GET", vcsBase+"/firewalls/").Query)
	testutil.AssertGolden(t, "vcs_firewall_ls.txt", []byte(stdout))
}

func TestVCSFirewallGetGolden(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/firewalls/", 200, "vcs/firewalls")
	f.onFixture("GET", vcsBase+"/firewalls/301/", 200, "vcs/firewall_detail")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "firewall", "get", "web-fw")
	require.Equal(t, ExitOK, code, stderr)
	testutil.AssertGolden(t, "vcs_firewall_get.txt", []byte(stdout)) // 含 Rules: allow-ssh(501), deny-all(502)；Networks: default_network(328291)；Created 09:02:03
}

func TestVCSFirewallCreateResolvesRuleAndNetworkNames(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/firewall_rules/", 200, "vcs/firewall_rules")
	f.onFixture("GET", vcsBase+"/networks/", 200, "vcs/networks")
	f.on("POST", vcsBase+"/firewalls/", 200, `{"id": 303, "name": "new-fw", "status": "BUILD", "desc": "d", "user": {"username": "alice"}}`)
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "firewall", "create", "--name", "new-fw", "--desc", "d",
		"--rule", "allow-ssh", "--rule", "502", "--network", "backend")
	require.Equal(t, ExitOK, code, stderr)
	req := f.find("POST", vcsBase+"/firewalls/")
	require.NotNil(t, req)
	assert.JSONEq(t, `{"name":"new-fw","project":101,"desc":"d","rules":[501,502],"associate_networks":[328292]}`, string(req.Body))
	assert.Contains(t, stdout, "new-fw")
	assert.Contains(t, stderr, "已建立 firewall 303")
}

func TestVCSFirewallCreateRequiresName(t *testing.T) {
	f := newFakeAPI(t)
	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "firewall", "create")
	assert.Equal(t, ExitGeneral, code)
	assert.Contains(t, stderr, "--name")
	assert.Nil(t, f.find("POST", vcsBase+"/firewalls/"))
}

func TestVCSFirewallUpdateReplaceAndClear(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/firewalls/", 200, "vcs/firewalls")
	f.onFixture("GET", vcsBase+"/firewall_rules/", 200, "vcs/firewall_rules")
	f.onFixture("PATCH", vcsBase+"/firewalls/301/", 200, "vcs/firewall_detail")
	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "firewall", "update", "web-fw", "--rule", "deny-all", "--clear-networks")
	require.Equal(t, ExitOK, code, stderr)
	req := f.find("PATCH", vcsBase+"/firewalls/301/")
	require.NotNil(t, req)
	assert.JSONEq(t, `{"rules":[502],"associate_networks":[]}`, string(req.Body))

	code, _, stderr = runCLI(t, f.env(t), "", "vcs", "firewall", "update", "web-fw", "--rule", "502", "--clear-rules")
	assert.Equal(t, ExitGeneral, code)
	assert.Contains(t, stderr, "--rule 與 --clear-rules")

	code, _, stderr = runCLI(t, f.env(t), "", "vcs", "firewall", "update", "web-fw")
	assert.Equal(t, ExitGeneral, code)
	assert.Contains(t, stderr, "至少指定一個")
}

// TestVCSFirewallUpdateNetworkClearNetworksMutuallyExclusive 確認 --network 與
// --clear-networks 互斥，與 --rule／--clear-rules 那支互斥檢查對稱（見
// TestVCSFirewallUpdateReplaceAndClear 的 --rule 分支）。
func TestVCSFirewallUpdateNetworkClearNetworksMutuallyExclusive(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/firewalls/", 200, "vcs/firewalls")
	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "firewall", "update", "web-fw", "--network", "backend", "--clear-networks")
	assert.Equal(t, ExitGeneral, code)
	assert.Contains(t, stderr, "--network 與 --clear-networks")
	assert.Nil(t, f.find("PATCH", vcsBase+"/firewalls/301/"))
}

// TestVCSFirewallUpdateDescEmptyString 確認 --desc "" 會送出 {"desc":""}（清除描述），
// 而不是被當成「未指定」略過——changedString 對空字串回非 nil 已在
// TestVCSFirewallRuleUpdateSendsOnlyChangedFlags（--src-port ""）證明，這裡補上
// vcs firewall update 自己的一個案例。
func TestVCSFirewallUpdateDescEmptyString(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/firewalls/", 200, "vcs/firewalls")
	f.onFixture("PATCH", vcsBase+"/firewalls/301/", 200, "vcs/firewall_detail")
	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "firewall", "update", "web-fw", "--desc", "")
	require.Equal(t, ExitOK, code, stderr)
	req := f.find("PATCH", vcsBase+"/firewalls/301/")
	require.NotNil(t, req)
	assert.JSONEq(t, `{"desc":""}`, string(req.Body))
}

func TestVCSFirewallRmMultiple(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/firewalls/", 200, "vcs/firewalls")
	f.onText("DELETE", vcsBase+"/firewalls/301/", 204, "")
	f.onText("DELETE", vcsBase+"/firewalls/302/", 204, "")
	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "firewall", "rm", "web-fw", "302", "-y")
	require.Equal(t, ExitOK, code, stderr)
	assert.NotNil(t, f.find("DELETE", vcsBase+"/firewalls/301/"))
	assert.NotNil(t, f.find("DELETE", vcsBase+"/firewalls/302/"))
	assert.Contains(t, stderr, "已刪除 firewall 301")
}
