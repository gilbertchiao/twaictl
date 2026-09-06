package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilbertchiao/twaictl/internal/config"
	"github.com/gilbertchiao/twaictl/internal/testutil"
)

func TestVCSLsGoldenAndAllFlag(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/sites/", 200, "vcs/sites")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "ls")
	require.Equal(t, ExitOK, code, stderr)
	testutil.AssertGolden(t, "vcs_ls.txt", []byte(stdout))
	assert.Equal(t, "project=101", f.find("GET", vcsBase+"/sites/").Query)

	_, _, _ = runCLI(t, f.env(t), "", "vcs", "ls", "--all")
	assert.Equal(t, "project=101&all_users=1", f.Requests()[len(f.Requests())-1].Query)
}

func TestVCSLsJSONIsRawArray(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/sites/", 200, "vcs/sites")
	code, stdout, _ := runCLI(t, f.env(t), "", "vcs", "ls", "-o", "json")
	require.Equal(t, ExitOK, code)
	assert.JSONEq(t, fixture(t, "vcs/sites"), stdout)
}

func TestVCSLsRequiresProjectCode(t *testing.T) {
	f := newFakeAPI(t)
	env := f.env(t)
	delete(env, config.EnvProjectCode)
	code, _, stderr := runCLI(t, env, "", "vcs", "ls")
	assert.Equal(t, ExitConfig, code)
	assert.Contains(t, stderr, "--project")
}

func TestVCSLsHelpListsColumns(t *testing.T) {
	// README 原本說欄位名稱「與 --help 或 -o json 一致」，但 --help 從不列欄位，
	// -o json 是 API 欄位名（public_ip）而非表格欄位（PUBLIC IP）；--columns 的可用欄位
	// 應該直接可以從 --help 查到。
	f := newFakeAPI(t)
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "ls", "--help")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stdout, "可用欄位")
	assert.Contains(t, stdout, "PUBLIC IP")
}

func TestVCSGetByNameGolden(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/sites/", 200, "vcs/sites")
	f.onFixture("GET", vcsBase+"/sites/7/", 200, "vcs/site_ready")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "get", "web-01")
	require.Equal(t, ExitOK, code, stderr)
	testutil.AssertGolden(t, "vcs_get.txt", []byte(stdout))
}

func TestVCSGetByIDJSONIsRawObject(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/sites/7/", 200, "vcs/site_ready")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "get", "7", "-o", "json")
	require.Equal(t, ExitOK, code, stderr)
	assert.JSONEq(t, fixture(t, "vcs/site_ready"), stdout)
}

func TestVCSGetUnknownNameIsExitGeneral(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/sites/", 200, "vcs/sites")
	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "get", "no-such-site")
	assert.Equal(t, ExitGeneral, code)
	assert.Contains(t, stderr, "找不到 site")
}

func TestVCSRmAsksForConfirmationAndHonoursYes(t *testing.T) {
	f := newFakeAPI(t)
	f.on("DELETE", vcsBase+"/sites/7/", 204, "")
	code, stdout, stderr := runCLI(t, f.env(t), "n\n", "vcs", "rm", "7")
	assert.Equal(t, ExitGeneral, code)
	assert.Contains(t, stderr, "確定嗎")
	assert.Contains(t, stderr, "已取消")
	assert.Empty(t, stdout)
	assert.Nil(t, f.find("DELETE", vcsBase+"/sites/7/"), "取消後不可送出 DELETE")

	code, _, stderr = runCLI(t, f.env(t), "", "vcs", "rm", "7", "-y")
	require.Equal(t, ExitOK, code, stderr)
	assert.NotNil(t, f.find("DELETE", vcsBase+"/sites/7/"))
	assert.Contains(t, stderr, "已送出刪除請求：7")
}

func TestVCSRmDryRunSkipsConfirmation(t *testing.T) {
	// --dry-run 不送出請求，要求確認毫無意義，反而會讓 CI / 腳本在空 stdin 下卡住或誤判取消。
	// project code 用數字 id（101）讓 projectID() 解析也不觸發任何請求，
	// 這樣印出的第一個（也是唯一一個）curl 命令才會是 DELETE 本身。
	f := newFakeAPI(t)
	env := f.env(t)
	env[config.EnvProjectCode] = "101"
	code, stdout, stderr := runCLI(t, env, "", "vcs", "rm", "7", "--dry-run")
	assert.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stdout, "curl -X DELETE")
	assert.NotContains(t, stderr, "確定嗎")
	assert.Empty(t, f.Requests(), "dry-run 不可送出請求")
}

func TestVCSRmUnknownNameStopsBeforeAnyDelete(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/sites/", 200, "vcs/sites")
	f.on("DELETE", vcsBase+"/sites/7/", 204, "")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "rm", "web-01", "nope", "-y")
	assert.Equal(t, ExitGeneral, code)
	assert.Contains(t, stderr, "找不到 site")
	assert.Empty(t, stdout)
	assert.Nil(t, f.find("DELETE", vcsBase+"/sites/7/"), "任一名稱解析失敗時不可送出任何 DELETE")
}

func TestVCSRmWaitPollsUntilGone(t *testing.T) {
	f := newFakeAPI(t)
	f.on("DELETE", vcsBase+"/sites/7/", 204, "")
	f.onSequence("GET", vcsBase+"/sites/7/", seq{200, fixture(t, "vcs/site_ready")}, seq{404, `{"message":"not found"}`})
	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "rm", "7", "-y", "--wait", "--wait-interval", "1ms")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stderr, "狀態：Deleted")
}

func TestVCSRmWaitTimeoutIsExit5(t *testing.T) {
	f := newFakeAPI(t)
	f.on("DELETE", vcsBase+"/sites/7/", 204, "")
	f.onFixture("GET", vcsBase+"/sites/7/", 200, "vcs/site_ready")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "rm", "7", "-y", "--wait",
		"--wait-timeout", "20ms", "--wait-interval", "1ms")
	assert.Equal(t, ExitWaitTimeout, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "7", "逾時錯誤訊息必須附上已送出刪除請求的實例 id")

	// -o json 時 progress 會被靜音，錯誤本身仍須附上 id。
	f2 := newFakeAPI(t)
	f2.on("DELETE", vcsBase+"/sites/7/", 204, "")
	f2.onFixture("GET", vcsBase+"/sites/7/", 200, "vcs/site_ready")
	code2, stdout2, stderr2 := runCLI(t, f2.env(t), "", "vcs", "rm", "7", "-y", "--wait", "-o", "json",
		"--wait-timeout", "20ms", "--wait-interval", "1ms")
	assert.Equal(t, ExitWaitTimeout, code2)
	assert.Empty(t, stdout2)
	assert.NotContains(t, stderr2, "狀態：", "-o json 應靜音 progress")
	assert.Contains(t, stderr2, "7", "即使靜音 progress，逾時錯誤本身仍須附上實例 id")
}

func TestVCSRmAPIErrorIsExitAPI(t *testing.T) {
	f := newFakeAPI(t)
	f.on("DELETE", vcsBase+"/sites/7/", 403, `{"message":"forbidden"}`)
	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "rm", "7", "-y")
	assert.Equal(t, ExitAPI, code)
	assert.Contains(t, stderr, "403")
}

func TestVCSRmMultipleIDsStopsOnFirstError(t *testing.T) {
	f := newFakeAPI(t)
	f.on("DELETE", vcsBase+"/sites/7/", 204, "")
	f.on("DELETE", vcsBase+"/sites/8/", 403, `{"message":"forbidden"}`)
	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "rm", "7", "8", "-y")
	assert.Equal(t, ExitAPI, code)
	assert.Contains(t, stderr, "已送出刪除請求：7")
	assert.NotNil(t, f.find("DELETE", vcsBase+"/sites/7/"))
	assert.NotNil(t, f.find("DELETE", vcsBase+"/sites/8/"))
}

func TestVCSFlavorLsGolden(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/flavors/", 200, "vcs/flavors")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "flavor", "ls")
	require.Equal(t, ExitOK, code, stderr)
	testutil.AssertGolden(t, "vcs_flavor_ls.txt", []byte(stdout))
	assert.Equal(t, "project=101", f.find("GET", vcsBase+"/flavors/").Query)

	_, _, _ = runCLI(t, f.env(t), "", "vcs", "flavor", "ls", "--all-projects")
	assert.Empty(t, f.Requests()[len(f.Requests())-1].Query, "--all-projects 不可帶 project 過濾")
}

func TestVCSFlavorLsJSONIsRawArray(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/flavors/", 200, "vcs/flavors")
	code, stdout, _ := runCLI(t, f.env(t), "", "vcs", "flavor", "ls", "-o", "json")
	require.Equal(t, ExitOK, code)
	assert.JSONEq(t, fixture(t, "vcs/flavors"), stdout)
}

func TestVCSImageLsGolden(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/images/", 200, "vcs/images")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "image", "ls")
	require.Equal(t, ExitOK, code, stderr)
	testutil.AssertGolden(t, "vcs_image_ls.txt", []byte(stdout))
	assert.Equal(t, "project=101", f.find("GET", vcsBase+"/images/").Query)
}

func TestVCSImageLsJSONIsRawArray(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/images/", 200, "vcs/images")
	code, stdout, _ := runCLI(t, f.env(t), "", "vcs", "image", "ls", "-o", "json")
	require.Equal(t, ExitOK, code)
	assert.JSONEq(t, fixture(t, "vcs/images"), stdout)
}

func TestVCSImageLsRequiresProjectCode(t *testing.T) {
	f := newFakeAPI(t)
	env := f.env(t)
	delete(env, config.EnvProjectCode)
	code, _, stderr := runCLI(t, env, "", "vcs", "image", "ls")
	assert.Equal(t, ExitConfig, code)
	assert.Contains(t, stderr, "--project")
}
