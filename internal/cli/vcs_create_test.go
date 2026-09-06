package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilbertchiao/twaictl/internal/config"
)

// TestReadPasswordFromStdin 是 readPasswordFromStdin 的純函式單元測試，直接餵不同的
// stdin 內容，涵蓋幾個邊界情境：沒有結尾換行就 EOF（ReadString 回傳 io.EOF 但仍帶有
// 資料，不應被當成錯誤）、多行輸入只取第一行、純空白（不做 TrimSpace，保留原樣）、
// 以及完全空白（視為使用者忘了接管線的錯誤）。
func TestReadPasswordFromStdin(t *testing.T) {
	tests := []struct {
		name    string
		stdin   string
		want    string
		wantErr string
	}{
		{name: "沒有結尾換行就 EOF", stdin: "abc", want: "abc"},
		{name: "多行輸入只取第一行", stdin: "line1\nline2\n", want: "line1"},
		{name: "純空白保留、不做 TrimSpace", stdin: "  \n", want: "  "},
		{name: "只有換行視為沒有密碼", stdin: "\n", wantErr: "stdin 沒有密碼"},
		{name: "完全沒有輸入視為沒有密碼", stdin: "", wantErr: "stdin 沒有密碼"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &app{stdin: strings.NewReader(tt.stdin)}
			got, err := readPasswordFromStdin(a)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// errStdin 是一個一讀就出錯的 io.Reader，用來證明「--password 與 --password-stdin
// 同時指定」時完全不會去讀 stdin：若程式碼真的呼叫了 Read（回歸成只看 password 值、
// 沒看 Changed），這裡就會冒出「test: 不應該讀取 stdin」這個非預期的錯誤，而不是
// 期望的互斥訊息，測試就會失敗。
type errStdin struct{}

func (errStdin) Read([]byte) (int, error) {
	return 0, errors.New("test: 不應該讀取 stdin")
}

func TestVCSCreateColumnsValidatedBeforeRequest(t *testing.T) {
	// vcs create 最後用 renderOne(..., siteDetailColumns) 輸出，也接受 --columns，
	// 但原本沒有像其他命令一樣提前驗證：--columns 打錯字要等到成功建立（且開始計費）
	// 的實例之後、render 階段才會發現，使用者會平白建立一個沒必要的實例。
	f := newFakeAPI(t)
	env := f.env(t)
	env[config.EnvProjectCode] = "101"
	code, stdout, stderr := runCLI(t, env, "", "vcs", "create", "--name", "x", "--solution", "11",
		"--image", "i", "--flavor", "f", "--password", "pw", "--columns", "nope")
	assert.Equal(t, ExitGeneral, code)
	assert.Contains(t, stderr, "不支援的欄位")
	assert.Empty(t, stdout)
	assert.Empty(t, f.Requests(), "欄位不合法時不可送出任何請求（不可先建立實例）")
}

func TestVCSCreateHelpListsColumns(t *testing.T) {
	f := newFakeAPI(t)
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "create", "--help")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stdout, "可用欄位")
}

func TestVCSCreateSendsHeadersAndRendersSite(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", "/api/v3/solutions/", 200, "vcs/solutions")
	f.onFixture("POST", vcsBase+"/sites/", 201, "vcs/site_initializing")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "create",
		"--name", "web-01", "--solution", "Ubuntu 22.04", "--image", "Ubuntu 22.04", "--flavor", "v.2xsmall",
		"--keypair", "mykey", "--eip", "--volume-size", "100", "--volume-type", "ssd")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stdout, "ID:")
	assert.Contains(t, stdout, "Initializing")
	post := f.find("POST", vcsBase+"/sites/")
	require.NotNil(t, post)
	assert.Equal(t, "floating", post.Header.Get("x-extra-property-floating-ip"))
	assert.Equal(t, "ssd", post.Header.Get("x-extra-property-volume-type"))
	assert.Contains(t, string(post.Body), `"solution":11`)
	// 生成碼依 GetSolutionsParams 欄位宣告順序（Project 先於 Category）組 query string，
	// 而非 brief 節錄中的字母序，故實際順序為 project 在前（與
	// internal/twai/vcs/resolve_test.go 的 TestResolveSolutionIDQueriesCommonWithCategoryOS 一致）。
	assert.Equal(t, "project=101&category=os", f.find("GET", "/api/v3/solutions/").Query)
}

func TestVCSCreateRequiresKeypairOrPassword(t *testing.T) {
	f := newFakeAPI(t)
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "create",
		"--name", "web-01", "--solution", "11", "--image", "img", "--flavor", "fl")
	assert.Equal(t, ExitGeneral, code)
	assert.Contains(t, stderr, "--keypair 或 --password")
	assert.Empty(t, stdout)
	assert.Nil(t, f.find("GET", "/api/v3/solutions/"), "未指定 --keypair/--password 時不可解析 solution")
	assert.Nil(t, f.find("POST", vcsBase+"/sites/"), "未指定 --keypair/--password 時不可送出 POST")
}

func TestVCSCreateRejectsBadVolumeType(t *testing.T) {
	f := newFakeAPI(t)
	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "create",
		"--name", "web-01", "--solution", "11", "--image", "img", "--flavor", "fl",
		"--keypair", "mykey", "--volume-type", "nvme")
	assert.Equal(t, ExitGeneral, code)
	assert.Contains(t, stderr, "hdd")
	assert.Contains(t, stderr, "ssd")
	assert.Contains(t, stderr, "LUKS-hdd")
	assert.Contains(t, stderr, "LUKS-ssd")
	assert.Nil(t, f.find("POST", vcsBase+"/sites/"), "enum 不合法時不可送出 POST")
}

func TestVCSCreateRejectsBadSystemVolumeType(t *testing.T) {
	f := newFakeAPI(t)
	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "create",
		"--name", "web-01", "--solution", "11", "--image", "img", "--flavor", "fl",
		"--keypair", "mykey", "--system-volume-type", "nvme")
	assert.Equal(t, ExitGeneral, code)
	assert.Contains(t, stderr, "local_disk")
	assert.Contains(t, stderr, "block_storage-hdd")
	assert.Nil(t, f.find("POST", vcsBase+"/sites/"), "enum 不合法時不可送出 POST")
}

func TestVCSCreateWaitUntilReady(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", "/api/v3/solutions/", 200, "vcs/solutions")
	f.onFixture("POST", vcsBase+"/sites/", 201, "vcs/site_initializing")
	f.onSequence("GET", vcsBase+"/sites/7/", seq{200, fixture(t, "vcs/site_initializing")}, seq{200, fixture(t, "vcs/site_ready")})
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "create", "--name", "web-01", "--solution", "11",
		"--image", "img", "--flavor", "fl", "--password", "pw", "--wait", "--wait-interval", "1ms")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stderr, "狀態：Ready")
	assert.Contains(t, stdout, "Ready")
	assert.NotContains(t, stderr, "pw", "密碼不可出現在任何輸出")
	assert.NotContains(t, stdout, "pw", "密碼不可出現在任何輸出")
}

func TestVCSCreateWaitTimeoutIsExit5(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", "/api/v3/solutions/", 200, "vcs/solutions")
	f.onFixture("POST", vcsBase+"/sites/", 201, "vcs/site_initializing")
	f.onFixture("GET", vcsBase+"/sites/7/", 200, "vcs/site_initializing")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "create", "--name", "web-01", "--solution", "11",
		"--image", "img", "--flavor", "fl", "--password", "pw", "--wait",
		"--wait-timeout", "20ms", "--wait-interval", "1ms")
	assert.Equal(t, ExitWaitTimeout, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "7", "逾時錯誤訊息必須附上已建立（且正在計費）的實例 id")
	assert.NotContains(t, stderr, "pw", "密碼不可出現在任何輸出")

	// -o json 時 progress（stderr 的「VCS 實例 7 狀態：...」）會被靜音，
	// 若 WaitSiteReady 的錯誤本身沒有附上 id，使用者會完全不知道是哪個實例逾時。
	f2 := newFakeAPI(t)
	f2.onFixture("GET", "/api/v3/solutions/", 200, "vcs/solutions")
	f2.onFixture("POST", vcsBase+"/sites/", 201, "vcs/site_initializing")
	f2.onFixture("GET", vcsBase+"/sites/7/", 200, "vcs/site_initializing")
	code2, stdout2, stderr2 := runCLI(t, f2.env(t), "", "vcs", "create", "--name", "web-01", "--solution", "11",
		"--image", "img", "--flavor", "fl", "--password", "pw", "--wait", "-o", "json",
		"--wait-timeout", "20ms", "--wait-interval", "1ms")
	assert.Equal(t, ExitWaitTimeout, code2)
	assert.Empty(t, stdout2)
	assert.NotContains(t, stderr2, "狀態：", "-o json 應靜音 progress")
	assert.Contains(t, stderr2, "7", "即使靜音 progress，逾時錯誤本身仍須附上實例 id")
}

func TestVCSCreateDryRunWithNumericIDsPrintsPost(t *testing.T) {
	f := newFakeAPI(t)
	env := f.env(t)
	env[config.EnvProjectCode] = "101"
	code, stdout, _ := runCLI(t, env, "", "vcs", "create", "--name", "x", "--solution", "11",
		"--image", "i", "--flavor", "f", "--password", "pw", "--dry-run")
	assert.Equal(t, ExitOK, code)
	assert.Contains(t, stdout, "curl -X POST")
	assert.NotContains(t, stdout, "pw", "密碼不可出現在 dry-run 輸出（貼進 issue/log 的風險）")
	assert.Contains(t, stdout, "x-extra-property-password: ***")
	assert.Empty(t, f.Requests())
}

func TestVCSCreatePasswordStdin(t *testing.T) {
	f := newFakeAPI(t)
	env := f.env(t)
	env[config.EnvProjectCode] = "101"
	code, stdout, _ := runCLI(t, env, "s3cret\n", "vcs", "create", "--name", "x", "--solution", "11",
		"--image", "i", "--flavor", "f", "--password-stdin", "--dry-run")
	assert.Equal(t, ExitOK, code)
	assert.Contains(t, stdout, "x-extra-property-password: ***", "dry-run 輸出仍須遮蔽密碼")
	assert.NotContains(t, stdout, "s3cret", "密碼不可出現在 dry-run 輸出")
	assert.Empty(t, f.Requests(), "dry-run 不應真的送出任何請求")

	f2 := newFakeAPI(t)
	f2.onFixture("GET", "/api/v3/solutions/", 200, "vcs/solutions")
	f2.onFixture("POST", vcsBase+"/sites/", 201, "vcs/site_initializing")
	code, _, stderr := runCLI(t, f2.env(t), "s3cret\n", "vcs", "create",
		"--name", "web-01", "--solution", "Ubuntu 22.04", "--image", "Ubuntu 22.04", "--flavor", "v.2xsmall",
		"--password-stdin")
	require.Equal(t, ExitOK, code, stderr)
	post := f2.find("POST", vcsBase+"/sites/")
	require.NotNil(t, post)
	assert.Equal(t, "s3cret", post.Header.Get("x-extra-property-password"))
}

func TestVCSCreatePasswordStdinExclusiveAndEmpty(t *testing.T) {
	f := newFakeAPI(t)
	code, stdout, stderr := runCLI(t, f.env(t), "s3cret\n", "vcs", "create",
		"--name", "web-01", "--solution", "11", "--image", "img", "--flavor", "fl",
		"--password", "pw", "--password-stdin")
	assert.Equal(t, ExitGeneral, code)
	assert.Contains(t, stderr, "--password 與 --password-stdin 不可同時使用")
	assert.Empty(t, stdout)
	assert.Nil(t, f.find("POST", vcsBase+"/sites/"), "同時指定 --password 與 --password-stdin 不可送出請求")

	code, stdout, stderr = runCLI(t, f.env(t), "\n", "vcs", "create",
		"--name", "web-01", "--solution", "11", "--image", "img", "--flavor", "fl",
		"--password-stdin")
	assert.Equal(t, ExitGeneral, code)
	assert.Contains(t, stderr, "stdin 沒有密碼")
	assert.Empty(t, stdout)
	assert.Nil(t, f.find("POST", vcsBase+"/sites/"), "stdin 密碼為空時不可送出請求")

	// --password ""（明確指定但值為空字串）仍要視為「有指定 --password」而觸發互斥錯誤：
	// 用一讀就出錯的 errStdin 證明這個分支完全不會去讀 stdin。
	env := withIsolatedConfigHome(t, f.env(t))
	getenv := func(key string) string { return env[key] }
	var stdoutBuf, stderrBuf bytes.Buffer
	code = Execute([]string{"vcs", "create",
		"--name", "web-01", "--solution", "11", "--image", "img", "--flavor", "fl",
		"--password", "", "--password-stdin"}, errStdin{}, &stdoutBuf, &stderrBuf, getenv)
	assert.Equal(t, ExitGeneral, code)
	assert.Contains(t, stderrBuf.String(), "--password 與 --password-stdin 不可同時使用")
	assert.Empty(t, stdoutBuf.String())
	assert.Nil(t, f.find("POST", vcsBase+"/sites/"), "--password \"\" 與 --password-stdin 同時指定不可送出請求")
}

func TestVCSCreateMissingRequiredFlagIsChineseError(t *testing.T) {
	f := newFakeAPI(t)
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "create",
		"--solution", "11", "--image", "img", "--flavor", "fl", "--keypair", "mykey")
	assert.Equal(t, ExitGeneral, code)
	assert.Contains(t, stderr, "--name")
	assert.NotContains(t, stderr, "required flag", "錯誤訊息必須是繁體中文，不可洩漏 cobra 英文訊息")
	assert.Empty(t, stdout)
	assert.Nil(t, f.find("GET", "/api/v3/solutions/"))
	assert.Nil(t, f.find("POST", vcsBase+"/sites/"))

	code, stdout, stderr = runCLI(t, f.env(t), "", "vcs", "create",
		"--name", "", "--solution", "11", "--image", "img", "--flavor", "fl", "--keypair", "mykey")
	assert.Equal(t, ExitGeneral, code, "空字串視同未指定")
	assert.Contains(t, stderr, "--name")
	assert.Empty(t, stdout)
}

func TestVCSCreateRejectsNegativeSizes(t *testing.T) {
	f := newFakeAPI(t)
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "create",
		"--name", "web-01", "--solution", "11", "--image", "img", "--flavor", "fl",
		"--keypair", "mykey", "--volume-size", "-1")
	assert.Equal(t, ExitGeneral, code)
	assert.Contains(t, stderr, "--volume-size")
	assert.Empty(t, stdout)
	assert.Empty(t, f.Requests(), "負數容量不可送出任何請求")

	f = newFakeAPI(t)
	code, stdout, stderr = runCLI(t, f.env(t), "", "vcs", "create",
		"--name", "web-01", "--solution", "11", "--image", "img", "--flavor", "fl",
		"--keypair", "mykey", "--system-volume-size", "-5")
	assert.Equal(t, ExitGeneral, code)
	assert.Contains(t, stderr, "--system-volume-size")
	assert.Empty(t, stdout)
	assert.Empty(t, f.Requests(), "負數容量不可送出任何請求")
}

func TestVCSCreateAllOptionalFlagsSendExpectedHeaders(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("POST", vcsBase+"/sites/", 201, "vcs/site_initializing")
	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "create",
		"--name", "web-01", "--solution", "11", "--image", "img", "--flavor", "fl",
		"--keypair", "mykey", "--network", "priv-net", "--az", "tw-az1", "--desc", "測試用",
		"--system-volume-size", "50", "--system-volume-type", "local_disk")
	require.Equal(t, ExitOK, code, stderr)
	post := f.find("POST", vcsBase+"/sites/")
	require.NotNil(t, post)
	assert.Equal(t, "nofloating", post.Header.Get("x-extra-property-floating-ip"), "未指定 --eip 時預設 nofloating")
	assert.Equal(t, "priv-net", post.Header.Get("x-extra-property-private-network"))
	assert.Equal(t, "tw-az1", post.Header.Get("x-extra-property-availability-zone"))
	assert.Equal(t, "local_disk", post.Header.Get("x-extra-property-system-volume-type"))
	assert.Contains(t, string(post.Body), `"desc":"測試用"`)
}
