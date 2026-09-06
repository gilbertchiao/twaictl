package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	genvcs "github.com/gilbertchiao/twaictl/internal/gen/vcs"
	"github.com/gilbertchiao/twaictl/internal/testutil"
)

// TestEipColumnsStatusHandlesNil 驗證 eipColumns／eipDetailColumns 的 STATUS 欄位
// （移除 ipStatus 包裝、改直接呼叫 output.Str 之後）status 為 nil 時仍顯示空字串，
// 而不是 panic 或印出 "<nil>"。
func TestEipColumnsStatusHandlesNil(t *testing.T) {
	ip := genvcs.IPSerializer{Status: nil}
	for _, col := range eipColumns {
		if col.Name == "STATUS" {
			assert.Empty(t, col.Value(ip))
		}
	}
	for _, col := range eipDetailColumns {
		if col.Name == "Status" {
			assert.Empty(t, col.Value(ip))
		}
	}
}

func TestVCSEipLsGoldenAndAddressFilter(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/ips/", 200, "vcs/ips")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "eip", "ls")
	require.Equal(t, ExitOK, code, stderr)
	testutil.AssertGolden(t, "vcs_eip_ls.txt", []byte(stdout))
	assert.Contains(t, stdout, "network/328291")

	_, _, _ = runCLI(t, f.env(t), "", "vcs", "eip", "ls", "--address", "203.0.113.219")
	assert.Equal(t, "project=101&address=203.0.113.219", f.Requests()[len(f.Requests())-1].Query)
}

func TestVCSEipGetByAddress(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/ips/", 200, "vcs/ips")
	f.onFixture("GET", vcsBase+"/ips/1393847/", 200, "vcs/ip_detail")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "eip", "get", "203.0.113.219")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stdout, "STATIC")
}

func TestVCSEipCreateSendsProject(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("POST", vcsBase+"/ips/", 200, "vcs/ip_detail")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "eip", "create")
	require.Equal(t, ExitOK, code, stderr)
	assert.JSONEq(t, `{"project":101}`, string(f.find("POST", vcsBase+"/ips/").Body))
	assert.Contains(t, stdout, "203.0.113.219")
}

func TestVCSEipUpdateSendsPatch(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("PATCH", vcsBase+"/ips/1393847/", 200, "vcs/ip_detail")
	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "eip", "update", "1393847", "--desc", "for web")
	require.Equal(t, ExitOK, code, stderr)
	assert.JSONEq(t, `{"desc":"for web"}`, string(f.find("PATCH", vcsBase+"/ips/1393847/").Body))
}

// TestVCSEipUpdateAllowsEmptyDescAndRequiresFlag 驗證 `--desc ""`（明確指定為空字串）
// 視為合法輸入、用來清除描述，會送出 PATCH `{"desc":""}`；完全不帶 `--desc` 則是缺參數
// 的一般錯誤，不送任何請求（見 Codex review fix round 1 Finding 1）。
func TestVCSEipUpdateAllowsEmptyDescAndRequiresFlag(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("PATCH", vcsBase+"/ips/1393847/", 200, "vcs/ip_detail")
	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "eip", "update", "1393847", "--desc", "")
	require.Equal(t, ExitOK, code, stderr)
	assert.JSONEq(t, `{"desc":""}`, string(f.find("PATCH", vcsBase+"/ips/1393847/").Body))

	f2 := newFakeAPI(t)
	code, _, stderr = runCLI(t, f2.env(t), "", "vcs", "eip", "update", "1393847")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "--desc")
	assert.Nil(t, f2.find("PATCH", vcsBase+"/ips/1393847/"))
}

func TestVCSEipRmConfirm(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/ips/", 200, "vcs/ips")
	f.on("DELETE", vcsBase+"/ips/1393847/", 204, "")
	code, _, stderr := runCLI(t, f.env(t), "n\n", "vcs", "eip", "rm", "203.0.113.219")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Nil(t, f.find("DELETE", vcsBase+"/ips/1393847/"))
	code, _, stderr = runCLI(t, f.env(t), "", "vcs", "eip", "rm", "203.0.113.219", "-y")
	require.Equal(t, ExitOK, code, stderr)
	assert.NotNil(t, f.find("DELETE", vcsBase+"/ips/1393847/"))
}
