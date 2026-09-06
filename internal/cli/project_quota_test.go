package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilbertchiao/twaictl/internal/testutil"
)

func TestProjectQuotaGolden(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/project_quotas/", 200, "vcs/project_quotas")
	code, stdout, stderr := runCLI(t, f.env(t), "", "project", "quota")
	require.Equal(t, ExitOK, code, stderr)
	assert.Equal(t, "project=101", f.find("GET", vcsBase+"/project_quotas/").Query)
	testutil.AssertGolden(t, "project_quota.txt", []byte(stdout))
	assert.Contains(t, stdout, "unlimited")
	assert.Contains(t, stdout, "memory (MB)")
}

func TestProjectQuotaUserGolden(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/projects/101/user_quotas/", 200, "vcs/user_quotas")
	code, stdout, stderr := runCLI(t, f.env(t), "", "project", "quota", "--user")
	require.Equal(t, ExitOK, code, stderr)
	testutil.AssertGolden(t, "project_quota_user.txt", []byte(stdout))
	assert.Contains(t, stdout, "bob")
}

func TestProjectQuotaJSONIsRaw(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/project_quotas/", 200, "vcs/project_quotas")
	code, stdout, stderr := runCLI(t, f.env(t), "", "project", "quota", "-o", "json")
	require.Equal(t, ExitOK, code, stderr)
	assert.JSONEq(t, fixture(t, "vcs/project_quotas"), stdout)
}

func TestProjectQuotaEmptyIsError(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/project_quotas/", 200, "[]")
	code, _, stderr := runCLI(t, f.env(t), "", "project", "quota")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "quota")
}

func TestProjectSolutionsGolden(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/projects/101/solutions/", 200, "vcs/project_solutions")
	f.onFixture("GET", "/api/v3/solutions/", 200, "vcs/solutions")
	code, stdout, stderr := runCLI(t, f.env(t), "", "project", "solutions")
	require.Equal(t, ExitOK, code, stderr)
	testutil.AssertGolden(t, "project_solutions.txt", []byte(stdout))
	assert.Contains(t, stdout, "99")
}

// TestProjectSolutionsJSONSkipsCommonLookup 驗證 -o json 不需要對照名稱，因此不呼叫
// Common 的 /solutions/（即使 Common 暫時失敗，已經取得的 id 清單仍能正常輸出）。
func TestProjectSolutionsJSONSkipsCommonLookup(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/projects/101/solutions/", 200, "vcs/project_solutions")
	code, stdout, stderr := runCLI(t, f.env(t), "", "project", "solutions", "-o", "json")
	require.Equal(t, ExitOK, code, stderr)
	assert.JSONEq(t, fixture(t, "vcs/project_solutions"), stdout)
	assert.Nil(t, f.find("GET", "/api/v3/solutions/"))
}
