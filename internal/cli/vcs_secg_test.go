package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilbertchiao/twaictl/internal/testutil"
)

func TestVCSSecgLsGolden(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/sites/", 200, "vcs/sites")
	f.onFixture("GET", vcsBase+"/servers/", 200, "vcs/servers")
	f.onFixture("GET", vcsBase+"/security_groups/", 200, "vcs/security_groups")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "secg", "ls", "web-01")
	require.Equal(t, ExitOK, code, stderr)
	assert.Equal(t, "project=101&server=3658462", f.find("GET", vcsBase+"/security_groups/").Query)
	testutil.AssertGolden(t, "vcs_secg_ls.txt", []byte(stdout))
	assert.Contains(t, stdout, "1000-2000")
	assert.Contains(t, stdout, "any")
}

func TestVCSSecgLsJSONIsRaw(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/security_groups/", 200, "vcs/security_groups")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "secg", "ls", "7", "--server", "3658462", "-o", "json")
	require.Equal(t, ExitOK, code, stderr)
	assert.JSONEq(t, fixture(t, "vcs/security_groups"), stdout)
}

// TestVCSSecgLsRejectsBadServer 驗證 --server 不是正整數（0）或不是數字（abc）時，
// 都在送出任何請求（含 project 解析）之前就被拒絕。
func TestVCSSecgLsRejectsBadServer(t *testing.T) {
	f := newFakeAPI(t)
	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "secg", "ls", "7", "--server", "0")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "正整數")
	assert.Empty(t, f.Requests())

	f2 := newFakeAPI(t)
	code, _, stderr = runCLI(t, f2.env(t), "", "vcs", "secg", "ls", "7", "--server", "abc")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "數字 id")
	assert.Empty(t, f2.Requests())
}

func TestVCSSecgAddRuleSendsPatch(t *testing.T) {
	f := newFakeAPI(t)
	f.on("PATCH", vcsBase+"/security_groups/c23dd442-bf00-470e-9803-8f4a11e61e2d/", 201, "")
	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "secg", "add-rule", "c23dd442-bf00-470e-9803-8f4a11e61e2d", "--direction", "ingress", "--protocol", "tcp", "--port-min", "22", "--remote", "0.0.0.0/0")
	require.Equal(t, ExitOK, code, stderr)
	req := f.find("PATCH", vcsBase+"/security_groups/c23dd442-bf00-470e-9803-8f4a11e61e2d/")
	require.NotNil(t, req)
	assert.JSONEq(t, `{"direction":"ingress","protocol":"tcp","port_range_min":22,"port_range_max":22,"remote_ip_prefix":"0.0.0.0/0","project":101}`, string(req.Body))
	assert.Contains(t, stderr, "已新增規則")
}

func TestVCSSecgAddRuleValidatesEnums(t *testing.T) {
	f := newFakeAPI(t)
	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "secg", "add-rule", "sg", "--direction", "sideways", "--protocol", "tcp")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "egress")
	code, _, stderr = runCLI(t, f.env(t), "", "vcs", "secg", "add-rule", "sg", "--direction", "ingress", "--protocol", "tcp", "--port-min", "70000")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "65535")
	assert.Empty(t, f.Requests())
}

// TestVCSSecgAddRulePortRange 驗證 --port-min 大於 --port-max 時擋下（不送任何請求），
// 以及只給 --port-max 時對稱補上 --port-min（與既有「只給 --port-min → --port-max 補相同值」對稱）。
func TestVCSSecgAddRulePortRange(t *testing.T) {
	f := newFakeAPI(t)
	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "secg", "add-rule", "sg", "--direction", "ingress", "--protocol", "tcp", "--port-min", "2000", "--port-max", "1000")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "不可大於")
	assert.Empty(t, f.Requests())

	f.on("PATCH", vcsBase+"/security_groups/sg/", 201, "")
	code, _, stderr = runCLI(t, f.env(t), "", "vcs", "secg", "add-rule", "sg", "--direction", "ingress", "--protocol", "tcp", "--port-max", "443")
	require.Equal(t, ExitOK, code, stderr)
	req := f.find("PATCH", vcsBase+"/security_groups/sg/")
	require.NotNil(t, req)
	assert.JSONEq(t, `{"direction":"ingress","protocol":"tcp","port_range_min":443,"port_range_max":443,"project":101}`, string(req.Body))
}

func TestVCSSecgRmRuleConfirmAndProjectQuery(t *testing.T) {
	f := newFakeAPI(t)
	f.on("DELETE", vcsBase+"/security_group_rules/r-1/", 202, "")
	code, _, stderr := runCLI(t, f.env(t), "n\n", "vcs", "secg", "rm-rule", "r-1")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Nil(t, f.find("DELETE", vcsBase+"/security_group_rules/r-1/"))
	code, _, stderr = runCLI(t, f.env(t), "", "vcs", "secg", "rm-rule", "r-1", "-y")
	require.Equal(t, ExitOK, code, stderr)
	assert.Equal(t, "project=101", f.find("DELETE", vcsBase+"/security_group_rules/r-1/").Query)
}
