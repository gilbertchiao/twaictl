package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilbertchiao/twaictl/internal/testutil"
)

func TestProjectLsGolden(t *testing.T) {
	f := newFakeAPI(t)
	code, stdout, stderr := runCLI(t, f.env(t), "", "project", "ls")
	require.Equal(t, ExitOK, code, stderr)
	testutil.AssertGolden(t, "project_ls.txt", []byte(stdout))
}

func TestProjectLsJSONIsRawArray(t *testing.T) {
	f := newFakeAPI(t)
	code, stdout, _ := runCLI(t, f.env(t), "", "project", "ls", "-o", "json")
	require.Equal(t, ExitOK, code)
	assert.JSONEq(t, fixture(t, "vcs/projects"), stdout)
}

func TestProjectLsColumnsAndNoHeader(t *testing.T) {
	f := newFakeAPI(t)
	code, stdout, _ := runCLI(t, f.env(t), "", "project", "ls", "--columns", "name,id", "--no-header")
	require.Equal(t, ExitOK, code)
	assert.Equal(t, "ENT1  101\nGOV2  202\n", stdout)
}

func TestProjectGetByCodeAndByID(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", "/api/v3/openstack-taichung-default-2/projects/101/", 200, "vcs/project_detail")
	code, stdout, stderr := runCLI(t, f.env(t), "", "project", "get", "ENT1")
	require.Equal(t, ExitOK, code, stderr)
	testutil.AssertGolden(t, "project_get.txt", []byte(stdout))

	code, stdout2, _ := runCLI(t, f.env(t), "", "project", "get", "101")
	require.Equal(t, ExitOK, code)
	assert.Equal(t, stdout, stdout2)
}

func TestProjectGetColumns(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", "/api/v3/openstack-taichung-default-2/projects/101/", 200, "vcs/project_detail")
	code, stdout, stderr := runCLI(t, f.env(t), "", "project", "get", "ENT1", "--columns", "name,status")
	require.Equal(t, ExitOK, code, stderr)
	assert.Equal(t, "Name:    ENT1\nStatus:  Ready\n", stdout)

	code, _, stderr = runCLI(t, f.env(t), "", "project", "get", "ENT1", "--columns", "nope")
	assert.Equal(t, ExitGeneral, code)
	assert.Contains(t, stderr, "不支援的欄位")
}

func TestProjectGetUnknownCodeIsExitGeneral(t *testing.T) {
	f := newFakeAPI(t)
	code, _, stderr := runCLI(t, f.env(t), "", "project", "get", "NOPE")
	assert.Equal(t, ExitGeneral, code)
	assert.Contains(t, stderr, "找不到 project")
}

func TestProjectLsColumnsValidatedBeforeDryRun(t *testing.T) {
	// --columns 打錯字是純使用者輸入錯誤，應在送出任何請求（含 --dry-run 印出的 curl 命令）
	// 之前就先被擋下，而不是等到 render 階段才發現——原本 --dry-run 會讓 ListProjects 直接
	// 回傳 ErrDryRun（exit 0），--columns nope 這個錯誤永遠不會被檢查到。
	f := newFakeAPI(t)
	code, stdout, stderr := runCLI(t, f.env(t), "", "project", "ls", "--columns", "nope", "--dry-run")
	assert.Equal(t, ExitGeneral, code)
	assert.Contains(t, stderr, "不支援的欄位")
	assert.Empty(t, stdout)
	assert.Empty(t, f.Requests(), "欄位不合法時不可送出任何請求")
}

func TestProjectLsDryRunPrintsCurlAndExitsZero(t *testing.T) {
	f := newFakeAPI(t)
	code, stdout, stderr := runCLI(t, f.env(t), "", "project", "ls", "--dry-run")
	assert.Equal(t, ExitOK, code)
	assert.Contains(t, stdout, "curl -X GET")
	assert.Contains(t, stdout, "x-api-key: ***")
	assert.Empty(t, stderr)
	assert.Empty(t, f.Requests(), "dry-run 不可送出請求")
}
