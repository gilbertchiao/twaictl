package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilbertchiao/twaictl/internal/testutil"
)

func TestVCSVolumeLsGolden(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/volumes/", 200, "vcs/volumes")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "volume", "ls")
	require.Equal(t, ExitOK, code, stderr)
	assert.Equal(t, "project=101", f.find("GET", vcsBase+"/volumes/").Query)
	testutil.AssertGolden(t, "vcs_volume_ls.txt", []byte(stdout))
	assert.Contains(t, stdout, "web-01-vm")
}

func TestVCSVolumeGetByNameShowsMountpoint(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/volumes/", 200, "vcs/volumes")
	f.onFixture("GET", vcsBase+"/volumes/1436398/", 200, "vcs/volume_detail")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "volume", "get", "data-01")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stdout, "/dev/vdb")
	assert.Contains(t, stdout, "97738afd")
}

func TestVCSVolumeCreateSendsBody(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("POST", vcsBase+"/volumes/", 200, "vcs/volume_created")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "volume", "create", "--name", "v1", "--size", "50", "--type", "ssd")
	require.Equal(t, ExitOK, code, stderr)
	assert.JSONEq(t, `{"name":"v1","project":101,"size":50,"volume_type":"ssd"}`, string(f.find("POST", vcsBase+"/volumes/").Body))
	assert.Contains(t, stdout, "1436400")
}

func TestVCSVolumeCreateRejectsBadTypeAndSize(t *testing.T) {
	f := newFakeAPI(t)
	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "volume", "create", "--name", "v1", "--size", "0")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "--size")
	code, _, stderr = runCLI(t, f.env(t), "", "vcs", "volume", "create", "--name", "v1", "--size", "5", "--type", "floppy")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "ssd")
	assert.Nil(t, f.find("POST", vcsBase+"/volumes/"))
}

func TestVCSVolumeActionAttachResolvesSiteServer(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/volumes/", 200, "vcs/volumes")
	f.onFixture("GET", vcsBase+"/sites/", 200, "vcs/sites")
	f.onFixture("GET", vcsBase+"/servers/", 200, "vcs/servers")
	f.onFixture("PUT", vcsBase+"/volumes/1436399/action/", 200, "vcs/volume_action")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "volume", "action", "scratch", "attach", "--site", "web-01")
	require.Equal(t, ExitOK, code, stderr)
	assert.JSONEq(t, `{"status":"attach","server":3658462}`, string(f.find("PUT", vcsBase+"/volumes/1436399/action/").Body))
	assert.Contains(t, stdout, "/dev/vdc")
	assert.Contains(t, stderr, "已對 volume 1436399 執行 attach")
}

func TestVCSVolumeActionRequiresTargetFlags(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/volumes/", 200, "vcs/volumes")
	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "volume", "action", "scratch", "attach")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "--site")
	code, _, stderr = runCLI(t, f.env(t), "", "vcs", "volume", "action", "scratch", "extend")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "--size")
	code, _, stderr = runCLI(t, f.env(t), "", "vcs", "volume", "action", "scratch", "shrink")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "extend")
	assert.Nil(t, f.find("PUT", vcsBase+"/volumes/1436399/action/"))
}

// TestVCSVolumeActionRejectsBadServerAndConflict 驗證 --server 的合法性檢查（正整數）
// 與 --site/--server 擇一檢查，都在送出任何 API 請求（含解析 project）之前完成；
// 每個子案例各用一個新的假 API server，才能準確斷言「完全沒有任何 HTTP 請求」
// （newFakeAPI 預先註冊的 GET /projects/ 不會被呼叫）。
func TestVCSVolumeActionRejectsBadServerAndConflict(t *testing.T) {
	f := newFakeAPI(t)
	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "volume", "action", "1436398", "attach", "--server", "0")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "正整數")
	assert.Empty(t, f.Requests())

	f2 := newFakeAPI(t)
	code, _, stderr = runCLI(t, f2.env(t), "", "vcs", "volume", "action", "1436398", "attach", "--server", "-1")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "正整數")
	assert.Empty(t, f2.Requests())

	f3 := newFakeAPI(t)
	code, _, stderr = runCLI(t, f3.env(t), "", "vcs", "volume", "action", "1436398", "attach", "--site", "web-01", "--server", "3658462")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "擇一")
	assert.Empty(t, f3.Requests())
}

// TestVCSVolumeActionDetachEmptyBodyOutputsSummary 驗證 detach 成功但 API 回 200 空 body（A38）時
// table 模式只印 progress、-o json 輸出 {volume_id, action} 摘要，不報「解析 API 回應失敗」。
func TestVCSVolumeActionDetachEmptyBodyOutputsSummary(t *testing.T) {
	f := newFakeAPI(t)
	f.on("PUT", vcsBase+"/volumes/1436398/action/", 200, "")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "volume", "action", "1436398", "detach", "--server", "3658462", "-y")
	require.Equal(t, ExitOK, code, stderr)
	assert.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, stderr, "已對 volume 1436398 執行 detach")

	code, stdout, stderr = runCLI(t, f.env(t), "", "vcs", "volume", "action", "1436398", "detach", "--server", "3658462", "-y", "-o", "json")
	require.Equal(t, ExitOK, code, stderr)
	assert.JSONEq(t, `{"volume_id":1436398,"action":"detach"}`, stdout)
}

func TestVCSVolumeActionDetachConfirmsAndExtendSendsSize(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("PUT", vcsBase+"/volumes/1436398/action/", 200, "vcs/volume_action")
	code, _, stderr := runCLI(t, f.env(t), "n\n", "vcs", "volume", "action", "1436398", "detach", "--server", "3658462")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Nil(t, f.find("PUT", vcsBase+"/volumes/1436398/action/"))

	code, _, stderr = runCLI(t, f.env(t), "", "vcs", "volume", "action", "1436398", "extend", "--size", "600")
	require.Equal(t, ExitOK, code, stderr)
	assert.JSONEq(t, `{"status":"extend","size":600}`, string(f.find("PUT", vcsBase+"/volumes/1436398/action/").Body))
}

func TestVCSVolumeRmConfirm(t *testing.T) {
	f := newFakeAPI(t)
	f.on("DELETE", vcsBase+"/volumes/1436399/", 204, "")
	code, _, stderr := runCLI(t, f.env(t), "n\n", "vcs", "volume", "rm", "1436399")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Nil(t, f.find("DELETE", vcsBase+"/volumes/1436399/"))
	code, _, stderr = runCLI(t, f.env(t), "", "vcs", "volume", "rm", "1436399", "-y")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stderr, "已刪除 volume 1436399")
}
