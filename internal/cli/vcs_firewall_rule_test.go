package cli

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilbertchiao/twaictl/internal/testutil"
)

func TestVCSFirewallRuleLsGolden(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/firewall_rules/", 200, "vcs/firewall_rules")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "firewall", "rule", "ls")
	require.Equal(t, ExitOK, code, stderr)
	assert.Equal(t, "project=101", f.find("GET", vcsBase+"/firewall_rules/").Query)
	testutil.AssertGolden(t, "vcs_firewall_rule_ls.txt", []byte(stdout))
}

func TestVCSFirewallRuleGetByNameShowsTaipeiTime(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/firewall_rules/", 200, "vcs/firewall_rules")
	f.onFixture("GET", vcsBase+"/firewall_rules/501/", 200, "vcs/firewall_rule_detail")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "firewall", "rule", "get", "allow-ssh")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stdout, "allow-ssh")
	assert.Contains(t, stdout, "2026-08-28 09:02:03")
	testutil.AssertGolden(t, "vcs_firewall_rule_get.txt", []byte(stdout))
}

// TestVCSFirewallRuleGetJSONOutputIsSingleObject 確認 `-o json` 輸出的是單一
// JSON object（而不是 spec 疑似筆誤標成的陣列，見 GetFirewallRule 的 A33 說明），
// 且保留原始值不轉時區（--output json 一律保留 API 原值）。
func TestVCSFirewallRuleGetJSONOutputIsSingleObject(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/firewall_rules/", 200, "vcs/firewall_rules")
	f.onFixture("GET", vcsBase+"/firewall_rules/501/", 200, "vcs/firewall_rule_detail")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "firewall", "rule", "get", "501", "-o", "json")
	require.Equal(t, ExitOK, code, stderr)

	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got), "stdout 必須是單一 JSON object，不是陣列：%s", stdout)
	assert.InDelta(t, 501, got["id"], 0)
	assert.Equal(t, "2026-08-28T01:02:03Z", got["create_time"])
}

func TestVCSFirewallRuleCreateBody(t *testing.T) {
	f := newFakeAPI(t)
	f.on("POST", vcsBase+"/firewall_rules/", 200, `{"id": 503, "name": "allow-web", "protocol": "tcp", "action": "allow", "destination_port": "80:443"}`)
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "firewall", "rule", "create",
		"--name", "allow-web", "--protocol", "tcp", "--action", "allow", "--dst-port", "80:443")
	require.Equal(t, ExitOK, code, stderr)
	req := f.find("POST", vcsBase+"/firewall_rules/")
	require.NotNil(t, req)
	assert.JSONEq(t, `{"name":"allow-web","project":101,"protocol":"tcp","action":"allow","destination_port":"80:443"}`, string(req.Body))
	assert.Contains(t, stdout, "allow-web")
	assert.Contains(t, stderr, "已建立 firewall rule 503")
}

func TestVCSFirewallRuleCreateValidatesFlags(t *testing.T) {
	f := newFakeAPI(t)
	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "firewall", "rule", "create", "--protocol", "tcp")
	assert.Equal(t, ExitGeneral, code)
	assert.Contains(t, stderr, "--name")
	code, _, stderr = runCLI(t, f.env(t), "", "vcs", "firewall", "rule", "create", "--name", "r", "--protocol", "gre")
	assert.Equal(t, ExitGeneral, code)
	assert.Contains(t, stderr, "--protocol")
	code, _, stderr = runCLI(t, f.env(t), "", "vcs", "firewall", "rule", "create", "--name", "r", "--action", "drop")
	assert.Equal(t, ExitGeneral, code)
	assert.Contains(t, stderr, "--action")
	assert.Nil(t, f.find("POST", vcsBase+"/firewall_rules/"))
}

func TestVCSFirewallRuleUpdateSendsOnlyChangedFlags(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/firewall_rules/", 200, "vcs/firewall_rules")
	f.on("PATCH", vcsBase+"/firewall_rules/502/", 200, `{"id": 502, "name": "deny-all", "action": "reject"}`)
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "firewall", "rule", "update", "deny-all", "--action", "reject", "--src-port", "")
	require.Equal(t, ExitOK, code, stderr)
	req := f.find("PATCH", vcsBase+"/firewall_rules/502/")
	require.NotNil(t, req)
	assert.JSONEq(t, `{"action":"reject","source_port":""}`, string(req.Body))
	assert.Contains(t, stdout, "reject")

	code, _, stderr = runCLI(t, f.env(t), "", "vcs", "firewall", "rule", "update", "deny-all")
	assert.Equal(t, ExitGeneral, code)
	assert.Contains(t, stderr, "至少指定一個")
}

// TestVCSFirewallRuleUpdateRejectsEmptyEnumFlags 確認 update 明確指定
// --protocol/--action/--name 為空字串時報錯，而不是被 validateEnumList「空字串代表
// 未指定」的語意放行、送出一個在 API 端沒有意義的 {"protocol": ""}／{"name": ""}。
// create 不受影響（create 的空字串代表不送出該欄位，語意不同，見
// firewallRuleFlags.validate）；--name 與 --protocol/--action 不同，不是「可清除」欄位
// （不像 --src-port 空字串代表清除限制），因此同樣要擋下。
func TestVCSFirewallRuleUpdateRejectsEmptyEnumFlags(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/firewall_rules/", 200, "vcs/firewall_rules")

	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "firewall", "rule", "update", "deny-all", "--protocol", "")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "--protocol")
	assert.Nil(t, f.find("PATCH", vcsBase+"/firewall_rules/502/"))

	code, _, stderr = runCLI(t, f.env(t), "", "vcs", "firewall", "rule", "update", "deny-all", "--action", "")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "--action")
	assert.Nil(t, f.find("PATCH", vcsBase+"/firewall_rules/502/"))

	code, _, stderr = runCLI(t, f.env(t), "", "vcs", "firewall", "rule", "update", "deny-all", "--name", "")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "--name")
	assert.Nil(t, f.find("PATCH", vcsBase+"/firewall_rules/502/"))
}

func TestVCSFirewallRuleRmRequiresConfirm(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/firewall_rules/", 200, "vcs/firewall_rules")
	f.onText("DELETE", vcsBase+"/firewall_rules/501/", 204, "")
	f.onText("DELETE", vcsBase+"/firewall_rules/502/", 204, "")
	code, _, stderr := runCLI(t, f.env(t), "n\n", "vcs", "firewall", "rule", "rm", "allow-ssh")
	assert.NotEqual(t, ExitOK, code, stderr)
	assert.Nil(t, f.find("DELETE", vcsBase+"/firewall_rules/501/"))

	code, _, stderr = runCLI(t, f.env(t), "", "vcs", "firewall", "rule", "rm", "allow-ssh", "502", "-y")
	require.Equal(t, ExitOK, code, stderr)
	assert.NotNil(t, f.find("DELETE", vcsBase+"/firewall_rules/501/"))
	assert.NotNil(t, f.find("DELETE", vcsBase+"/firewall_rules/502/"))
	assert.Contains(t, stderr, "已刪除 firewall rule 501")
}
