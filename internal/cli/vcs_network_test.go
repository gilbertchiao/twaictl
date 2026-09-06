package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilbertchiao/twaictl/internal/testutil"
)

func TestVCSNetworkLsGolden(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/networks/", 200, "vcs/networks")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "network", "ls")
	require.Equal(t, ExitOK, code, stderr)
	assert.Equal(t, "project=101", f.find("GET", vcsBase+"/networks/").Query)
	testutil.AssertGolden(t, "vcs_network_ls.txt", []byte(stdout))
}

func TestVCSNetworkGetByNameShowsFirewall(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/networks/", 200, "vcs/networks")
	f.onFixture("GET", vcsBase+"/networks/328291/", 200, "vcs/network_detail")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "network", "get", "default_network")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stdout, "fw-default")
	assert.Contains(t, stdout, "101.101.101.101, 1.1.1.1")
}

func TestVCSNetworkCreateSendsBody(t *testing.T) {
	f := newFakeAPI(t)
	f.on("POST", vcsBase+"/networks/", 201, fixture(t, "vcs/network_detail"))
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "network", "create", "--name", "n1", "--cidr", "10.0.0.0/24", "--router")
	require.Equal(t, ExitOK, code, stderr)
	assert.JSONEq(t, `{"name":"n1","project":101,"cidr":"10.0.0.0/24","with_router":true}`, string(f.find("POST", vcsBase+"/networks/").Body))
	assert.Contains(t, stdout, "default_network", "改用 renderOne 後應為直式 key: value 輸出")

	code, _, stderr = runCLI(t, f.env(t), "", "vcs", "network", "create", "--name", "n1")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "--cidr")
}

// TestVCSNetworkCreateValidatesColumnsBeforeRequest 驗證 --columns 指定不存在的欄位時，
// 在送出任何請求之前就被拒絕（與其他 create 命令一致）。
func TestVCSNetworkCreateValidatesColumnsBeforeRequest(t *testing.T) {
	f := newFakeAPI(t)
	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "network", "create", "--name", "n1", "--cidr", "10.0.0.0/24", "--columns", "BOGUS")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "BOGUS")
	assert.Nil(t, f.find("POST", vcsBase+"/networks/"))
}

func TestVCSNetworkRmConfirm(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/networks/", 200, "vcs/networks")
	f.on("DELETE", vcsBase+"/networks/328292/", 202, "")
	code, _, stderr := runCLI(t, f.env(t), "n\n", "vcs", "network", "rm", "backend")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Nil(t, f.find("DELETE", vcsBase+"/networks/328292/"))
	code, _, stderr = runCLI(t, f.env(t), "", "vcs", "network", "rm", "backend", "-y")
	require.Equal(t, ExitOK, code, stderr)
	assert.NotNil(t, f.find("DELETE", vcsBase+"/networks/328292/"))
}
