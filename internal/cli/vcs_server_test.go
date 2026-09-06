package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilbertchiao/twaictl/internal/testutil"
)

// TestVCSServerLsGolden 驗證 `vcs server ls` 帶 project 查詢並依 golden 輸出。
func TestVCSServerLsGolden(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/servers/", 200, "vcs/servers")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "server", "ls")
	require.Equal(t, ExitOK, code, stderr)
	assert.Equal(t, "project=101", f.find("GET", vcsBase+"/servers/").Query)
	testutil.AssertGolden(t, "vcs_server_ls.txt", []byte(stdout))
}

func TestVCSServerLsJSONIsRawArray(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/servers/", 200, "vcs/servers")
	code, stdout, _ := runCLI(t, f.env(t), "", "vcs", "server", "ls", "-o", "json")
	require.Equal(t, ExitOK, code)
	assert.JSONEq(t, fixture(t, "vcs/servers"), stdout)
}

func TestVCSServerLsHelpListsColumns(t *testing.T) {
	f := newFakeAPI(t)
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "server", "ls", "--help")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stdout, "可用欄位")
	assert.Contains(t, stdout, "PRIVATE IP")
}

// TestVCSServerGetGolden 驗證 `vcs server get` 直式輸出（含 server_detail 才有的欄位）。
func TestVCSServerGetGolden(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/servers/3658462/", 200, "vcs/server_detail")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "server", "get", "3658462")
	require.Equal(t, ExitOK, code, stderr)
	testutil.AssertGolden(t, "vcs_server_get.txt", []byte(stdout))
}

func TestVCSServerGetJSONIsRawObject(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/servers/3658462/", 200, "vcs/server_detail")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "server", "get", "3658462", "-o", "json")
	require.Equal(t, ExitOK, code, stderr)
	assert.JSONEq(t, fixture(t, "vcs/server_detail"), stdout)
}

func TestVCSServerGetRejectsNonNumericID(t *testing.T) {
	f := newFakeAPI(t)
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "server", "get", "web-01-vm")
	assert.Equal(t, ExitGeneral, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "數字")
	assert.Nil(t, f.find("GET", vcsBase+"/servers/web-01-vm/"))
}
