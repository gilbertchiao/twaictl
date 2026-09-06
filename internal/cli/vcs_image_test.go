package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilbertchiao/twaictl/internal/testutil"
)

// TestVCSImageGetByNameGolden 驗證 name 經 GET /images/?project= 解析成 id 後呼叫
// GET /images/{id}/，直式輸出（golden）。fixture vcs/images 第一筆 id 為 21、
// name 為 "Ubuntu 22.04"（brief 原假設 id 5／img-web 與既有 fixture 不符，已調整）。
func TestVCSImageGetByNameGolden(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/images/", 200, "vcs/images")
	f.onFixture("GET", vcsBase+"/images/21/", 200, "vcs/image_detail")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "image", "get", "Ubuntu 22.04")
	require.Equal(t, ExitOK, code, stderr)
	testutil.AssertGolden(t, "vcs_image_get.txt", []byte(stdout))
	assert.Contains(t, stdout, "2.0 GiB")
}

// TestVCSImageGetByIDSkipsList 驗證數字 id 不呼叫清單；-o json 為 API 原始回應。
func TestVCSImageGetByIDSkipsList(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/images/21/", 200, "vcs/image_detail")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "image", "get", "21", "-o", "json")
	require.Equal(t, ExitOK, code, stderr)
	assert.JSONEq(t, fixture(t, "vcs/image_detail"), stdout)
	assert.Nil(t, f.find("GET", vcsBase+"/images/"))
}

// TestVCSImageRmConfirmAndDelete 驗證取消時不送 DELETE；-y 時逐一 DELETE 並印進度。
func TestVCSImageRmConfirmAndDelete(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/images/", 200, "vcs/images")
	f.on("DELETE", vcsBase+"/images/21/", 204, "")
	code, _, stderr := runCLI(t, f.env(t), "n\n", "vcs", "image", "rm", "Ubuntu 22.04")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Nil(t, f.find("DELETE", vcsBase+"/images/21/"))

	code, _, stderr = runCLI(t, f.env(t), "", "vcs", "image", "rm", "21", "-y")
	require.Equal(t, ExitOK, code, stderr)
	assert.NotNil(t, f.find("DELETE", vcsBase+"/images/21/"))
	assert.Contains(t, stderr, "已刪除 image 21")
}

// TestVCSImageSaveSendsPutBody 驗證 site 名稱 → servers 解析 → PUT
// /images/{server_id}/save/，body 只含指定欄位。
func TestVCSImageSaveSendsPutBody(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/sites/", 200, "vcs/sites")
	f.onFixture("GET", vcsBase+"/servers/", 200, "vcs/servers")
	f.onFixture("PUT", vcsBase+"/images/3658462/save/", 201, "vcs/image_saved")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "image", "save", "web-01", "--name", "snap-1", "--os", "Linux", "--os-version", "Ubuntu 24.04")
	require.Equal(t, ExitOK, code, stderr)
	req := f.find("PUT", vcsBase+"/images/3658462/save/")
	require.NotNil(t, req)
	assert.JSONEq(t, `{"name":"snap-1","os":"Linux","os_version":"Ubuntu 24.04"}`, string(req.Body))
	assert.Contains(t, stdout, "snap-1")
	assert.Contains(t, stderr, "已送出建立 image 請求")
}

// TestVCSImageSaveServerFlagAndRequiresName 驗證 --server 指定時不查 /servers/；
// 缺 --name 為一般錯誤且不送請求。
func TestVCSImageSaveServerFlagAndRequiresName(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("PUT", vcsBase+"/images/42/save/", 201, "vcs/image_saved")
	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "image", "save", "7", "--server", "42", "--name", "x", "--os", "Linux", "--os-version", "Ubuntu 24.04")
	require.Equal(t, ExitOK, code, stderr)
	assert.Nil(t, f.find("GET", vcsBase+"/servers/"))

	code, _, stderr = runCLI(t, f.env(t), "", "vcs", "image", "save", "7", "--server", "42", "--os", "Linux", "--os-version", "Ubuntu 24.04")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "--name")

	// spec 標 os／os_version 選填，但真實 API 缺任一個都回 400（A40）：送出前就擋下。
	code, _, stderr = runCLI(t, f.env(t), "", "vcs", "image", "save", "7", "--server", "42", "--name", "x", "--os-version", "Ubuntu 24.04")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "--os")
	code, _, stderr = runCLI(t, f.env(t), "", "vcs", "image", "save", "7", "--server", "42", "--name", "x", "--os", "Linux")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "--os-version")
}

// TestVCSImageSaveRejectsBadServer 驗證 --server 非正整數在送出任何請求之前就被拒絕。
func TestVCSImageSaveRejectsBadServer(t *testing.T) {
	f := newFakeAPI(t)
	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "image", "save", "7", "--server", "0", "--name", "x", "--os", "Linux", "--os-version", "Ubuntu 24.04")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "正整數")
	assert.Empty(t, f.Requests())
}
