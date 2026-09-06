package cli

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilbertchiao/twaictl/internal/config"
	"github.com/gilbertchiao/twaictl/internal/testutil"
)

// failingWriter 是一個第一次 Write 就回傳錯誤的 io.Writer，用來模擬 stdout
// 已關閉的 pipe、磁碟已滿等情境，驗證 renderCOSKeys 失敗時 --save 仍會執行。
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("模擬寫入失敗（例如已關閉的 pipe）")
}

// TestCosKeyGetGoldenMasksSecret 驗證 `cos key get` 預設輸出（table）遮蔽所有 secret，
// 且直式版面與 golden 檔一致。
func TestCosKeyGetGoldenMasksSecret(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", cephBase+"/projects/60178/key/", 200, "ceph/key")
	code, stdout, stderr := runCLI(t, f.env(t), "", "cos", "key", "get")
	require.Equal(t, ExitOK, code, stderr)
	assert.NotContains(t, stdout, "SECRETPUBLIC")
	assert.NotContains(t, stdout, "SECRETPRIV")
	testutil.AssertGolden(t, "cos_key_get.txt", []byte(stdout))
}

// TestCosKeyGetShowSecret 驗證 --show-secret 會顯示 public 與 private 的 secret。
func TestCosKeyGetShowSecret(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", cephBase+"/projects/60178/key/", 200, "ceph/key")
	code, stdout, stderr := runCLI(t, f.env(t), "", "cos", "key", "get", "--show-secret")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stdout, "SECRETPUBLIC")
	assert.Contains(t, stdout, "SECRETPRIV")
}

// TestCosKeyGetJSONIsRawAndWarns 驗證 -o json 直接輸出 API 原始回應（含 secret），
// 並在 stderr 印出警告，避免使用者在不知情下外流 secret。
func TestCosKeyGetJSONIsRawAndWarns(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", cephBase+"/projects/60178/key/", 200, "ceph/key")
	code, stdout, stderr := runCLI(t, f.env(t), "", "cos", "key", "get", "-o", "json")
	require.Equal(t, ExitOK, code, stderr)
	assert.JSONEq(t, fixture(t, "ceph/key"), stdout)
	assert.Contains(t, stderr, "secret")
}

// TestCosKeyGetSaveWritesProfile 驗證 --save 把 public 金鑰寫入目前 profile，
// 且 current_profile 不受影響，stdout 不含 secret。
func TestCosKeyGetSaveWritesProfile(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", cephBase+"/projects/60178/key/", 200, "ceph/key")
	env := f.env(t)

	code, _, stderr := runCLI(t, env, "", "config", "init", "--api-key", "k", "--project-code", "ENT1")
	require.Equal(t, ExitOK, code, stderr)

	code, stdout, stderr := runCLI(t, env, "", "cos", "key", "get", "--save")
	require.Equal(t, ExitOK, code, stderr)
	assert.NotContains(t, stdout, "SECRETPUBLIC")
	assert.Contains(t, stderr, "已將 COS S3 金鑰寫入 profile default")

	cfg, err := config.Load(filepath.Join(env["XDG_CONFIG_HOME"], "twaictl", "config.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "AKIAPUBLIC", cfg.Profiles["default"].COS.AccessKey)
	assert.Equal(t, "SECRETPUBLIC", cfg.Profiles["default"].COS.SecretKey)
	assert.Equal(t, "default", cfg.CurrentProfile)
}

// TestCosKeyGetSaveCreatesProfileWhenMissing 驗證 --save 在目前 profile 尚未存在
// 於設定檔（只靠環境變數生效）時，會建立一個最小 profile 而不是失敗。
func TestCosKeyGetSaveCreatesProfileWhenMissing(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", cephBase+"/projects/60178/key/", 200, "ceph/key")
	env := f.env(t)

	code, _, stderr := runCLI(t, env, "", "cos", "key", "get", "--save")
	require.Equal(t, ExitOK, code, stderr)

	cfg, err := config.Load(filepath.Join(env["XDG_CONFIG_HOME"], "twaictl", "config.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "AKIAPUBLIC", cfg.Profiles["default"].COS.AccessKey)
	assert.Equal(t, "SECRETPUBLIC", cfg.Profiles["default"].COS.SecretKey)
	assert.Empty(t, cfg.CurrentProfile, "--save 不應設定 current_profile")
}

// TestCosKeyGetSaveCreatesExplicitNewProfile 驗證 --save 在 --profile / TWAI_PROFILE
// 明確指向「尚未存在」的 profile 時，仍會建立該 profile（不會被 PersistentPreRunE
// 的「找不到 profile」擋下），且不影響 current_profile 與既有的 default profile。
func TestCosKeyGetSaveCreatesExplicitNewProfile(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", cephBase+"/projects/60178/key/", 200, "ceph/key")
	env := f.env(t)

	code, _, stderr := runCLI(t, env, "", "config", "init", "--api-key", "k", "--project-code", "ENT1")
	require.Equal(t, ExitOK, code, stderr)

	envNew := make(map[string]string, len(env)+1)
	for k, v := range env {
		envNew[k] = v
	}
	envNew[config.EnvProfile] = "newp"

	code, _, stderr = runCLI(t, envNew, "", "cos", "key", "get", "--save")
	require.Equal(t, ExitOK, code, stderr)

	cfg, err := config.Load(filepath.Join(env["XDG_CONFIG_HOME"], "twaictl", "config.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "AKIAPUBLIC", cfg.Profiles["newp"].COS.AccessKey)
	assert.Equal(t, "SECRETPUBLIC", cfg.Profiles["newp"].COS.SecretKey)
	assert.Equal(t, "default", cfg.CurrentProfile, "不應改變 current_profile")
	assert.Equal(t, "k", cfg.Profiles["default"].APIKey, "既有 default profile 應完整保留")
}

// TestCosKeyGetWithoutSaveStillRequiresExistingProfile 驗證只有帶 --save 的
// get/renew 才允許 --profile 指向不存在的 profile；沒有 --save 時維持原本規則
// （設定錯誤，exit code 2），確認 isCOSKeySaveCmd 沒有把例外放寬到整個 cos 命令樹。
func TestCosKeyGetWithoutSaveStillRequiresExistingProfile(t *testing.T) {
	env := map[string]string{"XDG_CONFIG_HOME": t.TempDir()}
	code, _, stderr := runCLI(t, env, "", "--profile", "ghost", "cos", "key", "get")
	assert.Equal(t, ExitConfig, code, stderr)
	assert.Contains(t, stderr, "ghost")
}

// TestCosKeyMutatingCommandsValidateColumnsFirst 驗證 create/renew/rm 在任何
// confirm 或 API 呼叫之前就先驗證 --columns，避免不合法的 --columns 要等到
// 建立 / 輪替 / 刪除這些不可逆操作執行完才失敗。
func TestCosKeyMutatingCommandsValidateColumnsFirst(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"create", []string{"cos", "key", "create", "--name", "x", "--columns", "nope"}},
		{"renew", []string{"cos", "key", "renew", "--all", "-y", "--columns", "nope"}},
		{"rm", []string{"cos", "key", "rm", "--name", "x", "-y", "--columns", "nope"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeAPI(t)
			code, _, stderr := runCLI(t, f.env(t), "", tc.args...)
			assert.Equal(t, ExitGeneral, code, stderr)
			assert.Contains(t, stderr, "不支援的欄位")
			assert.Empty(t, f.Requests(), "不合法的 --columns 不應送出任何請求")
		})
	}
}

// TestCosKeyCreateSendsName 驗證 --name 送進 POST body，且新建 key 的 secret
// 預設不顯示，並提示如何取得。
func TestCosKeyCreateSendsName(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("POST", cephBase+"/projects/60178/key/", 200, "ceph/key_after_create")
	code, stdout, stderr := runCLI(t, f.env(t), "", "cos", "key", "create", "--name", "backup")
	require.Equal(t, ExitOK, code, stderr)

	req := f.find("POST", cephBase+"/projects/60178/key/")
	require.NotNil(t, req)
	assert.JSONEq(t, `{"name":"backup"}`, string(req.Body))

	assert.Contains(t, stdout, "backup")
	assert.NotContains(t, stdout, "SECRETBACKUP")
	assert.Contains(t, stderr, "--show-secret")
}

// TestCosKeyCreateRequiresName 驗證未指定 --name 時不送出請求。
func TestCosKeyCreateRequiresName(t *testing.T) {
	f := newFakeAPI(t)
	code, _, stderr := runCLI(t, f.env(t), "", "cos", "key", "create")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Empty(t, f.Requests())
}

// TestCosKeyRenewRequiresConfirm 驗證取消時不送出請求；-y --all 送出 PUT body {"all":true}。
func TestCosKeyRenewRequiresConfirm(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("PUT", cephBase+"/projects/60178/key/", 200, "ceph/key")

	code, _, stderr := runCLI(t, f.env(t), "n\n", "cos", "key", "renew", "--all")
	assert.Equal(t, ExitGeneral, code)
	assert.Nil(t, f.find("PUT", cephBase+"/projects/60178/key/"))
	assert.Contains(t, stderr, "確定嗎")

	code, _, stderr = runCLI(t, f.env(t), "", "cos", "key", "renew", "--all", "-y")
	require.Equal(t, ExitOK, code, stderr)
	req := f.find("PUT", cephBase+"/projects/60178/key/")
	require.NotNil(t, req)
	assert.JSONEq(t, `{"all":true}`, string(req.Body))
}

// TestCosKeyRenewShowSecretAndSave 驗證 renew 的 --show-secret 顯示輪替後的新 secret，
// --save 把（輪替後的）public 金鑰寫回目前 profile；不帶 --show-secret 時
// stdout/stderr 都不含 secret（沿用 get 的預設遮蔽規則）。
func TestCosKeyRenewShowSecretAndSave(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("PUT", cephBase+"/projects/60178/key/", 200, "ceph/key")
	env := f.env(t)

	code, _, stderr := runCLI(t, env, "", "config", "init", "--api-key", "k", "--project-code", "ENT1")
	require.Equal(t, ExitOK, code, stderr)

	code, stdout, stderr := runCLI(t, env, "", "cos", "key", "renew", "--all", "-y", "--show-secret", "--save")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stdout, "SECRETPUBLIC")
	assert.Contains(t, stdout, "SECRETPRIV")

	cfg, err := config.Load(filepath.Join(env["XDG_CONFIG_HOME"], "twaictl", "config.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "AKIAPUBLIC", cfg.Profiles["default"].COS.AccessKey)
	assert.Equal(t, "SECRETPUBLIC", cfg.Profiles["default"].COS.SecretKey)

	code, stdout, stderr = runCLI(t, f.env(t), "", "cos", "key", "renew", "--all", "-y")
	require.Equal(t, ExitOK, code, stderr)
	assert.NotContains(t, stdout, "SECRETPUBLIC")
	assert.NotContains(t, stderr, "SECRETPUBLIC")
}

// TestCosKeyRenewSaveFailureStillPrintsResponse 驗證 renew --save 在寫入設定檔失敗時
// （舊 key 已在遠端失效、新 key 已經送出）仍然把回應（含新 secret）印出來，
// 只讓 exit code 反映「儲存失敗」；不能因為儲存這個附加動作失敗，就讓使用者
// 完全看不到剛剛才拿到、且不會再有第二次機會（同樣方式）取得的新 secret。
func TestCosKeyRenewSaveFailureStillPrintsResponse(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 不遵循 Unix 目錄權限，無法以 0500 模擬設定檔寫入失敗")
	}
	f := newFakeAPI(t)
	f.onFixture("PUT", cephBase+"/projects/60178/key/", 200, "ceph/key")
	env := f.env(t)

	code, _, stderr := runCLI(t, env, "", "config", "init", "--api-key", "k", "--project-code", "ENT1")
	require.Equal(t, ExitOK, code, stderr)

	// 把設定目錄改成唯讀＋可執行（0500）：config.Load 讀取既有 config.yaml 不受影響
	// （開啟已知檔名只需要目錄的 execute/search 權限），但 config.Save 在目錄內建立
	// 暫存檔（os.CreateTemp）需要目錄的 write 權限，因而失敗。
	configDir := filepath.Join(env["XDG_CONFIG_HOME"], "twaictl")
	require.NoError(t, os.Chmod(configDir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(configDir, 0o700) }) // 讓 t.TempDir() 的清理仍能刪除目錄

	code, stdout, stderr := runCLI(t, env, "", "cos", "key", "renew", "--all", "-y", "--save", "--show-secret")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stdout, "SECRETPUBLIC", "即使儲存失敗，回應（含新 secret）仍應輸出")
	assert.Contains(t, stderr, "儲存失敗")

	req := f.find("PUT", cephBase+"/projects/60178/key/")
	assert.NotNil(t, req, "PUT 應該已經送出（金鑰已在遠端輪替）")
}

// TestCosKeyRenewSaveRunsEvenIfRenderFails 驗證 renew --save 在 renderCOSKeys
// 失敗（例如 stdout 是已關閉的 pipe、磁碟已滿）時仍會嘗試儲存：舊 key 在請求
// 送出當下就已經在遠端立即失效，若因為輸出失敗就跳過儲存，新 key 會「既沒
// 顯示、也沒存檔」，使用者將永久失去它。
func TestCosKeyRenewSaveRunsEvenIfRenderFails(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("PUT", cephBase+"/projects/60178/key/", 200, "ceph/key")
	env := f.env(t)

	code, _, stderr := runCLI(t, env, "", "config", "init", "--api-key", "k", "--project-code", "ENT1")
	require.Equal(t, ExitOK, code, stderr)

	getenv := func(key string) string { return env[key] }
	var stderrBuf bytes.Buffer
	code = Execute([]string{"cos", "key", "renew", "--all", "-y", "--save"}, strings.NewReader(""), failingWriter{}, &stderrBuf, getenv)

	assert.NotEqual(t, ExitOK, code)
	assert.Contains(t, stderrBuf.String(), "已將 COS S3 金鑰寫入 profile")

	req := f.find("PUT", cephBase+"/projects/60178/key/")
	assert.NotNil(t, req, "PUT 應該已經送出（金鑰已在遠端輪替）")

	cfg, err := config.Load(filepath.Join(env["XDG_CONFIG_HOME"], "twaictl", "config.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "AKIAPUBLIC", cfg.Profiles["default"].COS.AccessKey, "即使輸出失敗，金鑰仍應寫入設定檔")
	assert.Equal(t, "SECRETPUBLIC", cfg.Profiles["default"].COS.SecretKey)
}

// TestCosKeyGetSaveFailureHintsShowSecretWhenSecretHidden 驗證 --save 儲存失敗、
// 且使用者這次的輸出本來就沒看到 secret（沒有 --show-secret、table 格式）時，
// stderr 會額外提示改用 --show-secret 重新執行才能拿到金鑰。
func TestCosKeyGetSaveFailureHintsShowSecretWhenSecretHidden(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 不遵循 Unix 目錄權限，無法以 0500 模擬設定檔寫入失敗")
	}
	f := newFakeAPI(t)
	f.onFixture("GET", cephBase+"/projects/60178/key/", 200, "ceph/key")
	env := f.env(t)

	code, _, stderr := runCLI(t, env, "", "config", "init", "--api-key", "k", "--project-code", "ENT1")
	require.Equal(t, ExitOK, code, stderr)

	configDir := filepath.Join(env["XDG_CONFIG_HOME"], "twaictl")
	require.NoError(t, os.Chmod(configDir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(configDir, 0o700) })

	code, stdout, stderr := runCLI(t, env, "", "cos", "key", "get", "--save")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.NotContains(t, stdout, "SECRETPUBLIC", "沒有 --show-secret 時，輸出本就不含 secret")
	assert.Contains(t, stderr, "儲存失敗")
	assert.Contains(t, stderr, "--show-secret")
}

// TestCosKeySaveFalseDoesNotRelaxProfileCheck 驗證 --save=false（使用者明確關閉）
// 不會被誤判成「有指定 --save」，profile 必須存在的規則維持不變。
func TestCosKeySaveFalseDoesNotRelaxProfileCheck(t *testing.T) {
	f := newFakeAPI(t)
	env := f.env(t)
	code, _, stderr := runCLI(t, env, "", "--profile", "ghost", "cos", "key", "get", "--save=false")
	assert.Equal(t, ExitConfig, code, stderr)
	assert.Contains(t, stderr, "ghost")
	assert.Empty(t, f.Requests())
}

// TestCosKeyRenewRequiresNameOrAll 驗證未指定 --name 也未指定 --all 時是一般錯誤，不送出請求。
func TestCosKeyRenewRequiresNameOrAll(t *testing.T) {
	f := newFakeAPI(t)
	code, _, stderr := runCLI(t, f.env(t), "", "cos", "key", "renew")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Empty(t, f.Requests())
}

// TestCosKeyRmSendsBody 驗證 --name 送進 DELETE body。
func TestCosKeyRmSendsBody(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("DELETE", cephBase+"/projects/60178/key/", 200, "ceph/key")
	code, _, stderr := runCLI(t, f.env(t), "", "cos", "key", "rm", "--name", "default", "-y")
	require.Equal(t, ExitOK, code, stderr)

	req := f.find("DELETE", cephBase+"/projects/60178/key/")
	require.NotNil(t, req)
	assert.JSONEq(t, `{"name":"default"}`, string(req.Body))
}

// TestCosKeyRmRequiresName 驗證未指定 --name 時是一般錯誤，不送出任何請求
// （包含不會先去解析 project id）。
func TestCosKeyRmRequiresName(t *testing.T) {
	f := newFakeAPI(t)
	code, _, stderr := runCLI(t, f.env(t), "", "cos", "key", "rm")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Empty(t, f.Requests())
}

// TestCosProjectIDUsesCephProjects 驗證 cos 命令只打 Ceph 的 /projects/ 與
// /projects/{id}/key/，完全不打 VCS 的 /projects/（兩者是各自獨立的 project id 編號空間）。
func TestCosProjectIDUsesCephProjects(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", cephBase+"/projects/60178/key/", 200, "ceph/key")
	code, _, stderr := runCLI(t, f.env(t), "", "cos", "key", "get")
	require.Equal(t, ExitOK, code, stderr)

	assert.NotNil(t, f.find("GET", cephBase+"/projects/"))
	assert.NotNil(t, f.find("GET", cephBase+"/projects/60178/key/"))
	assert.Nil(t, f.find("GET", vcsBase+"/projects/"))
}

// TestAppS3ClientRequiresCredentials 驗證缺少 COS S3 access/secret key 時
// 回傳 *config.Error（對應 exit code 2）。
func TestAppS3ClientRequiresCredentials(t *testing.T) {
	a := &app{
		opts:     &globalOptions{},
		settings: &config.Settings{ProfileName: "default", ConfigPath: "/tmp/config.yaml", COS: config.COSSettings{Endpoint: config.DefaultCOSEndpoint}},
	}
	_, err := a.s3Client()
	var cfgErr *config.Error
	require.ErrorAs(t, err, &cfgErr, "缺少 COS S3 金鑰時應回傳 *config.Error")
}

// TestAppS3ClientBuildsClient 驗證有 access/secret key 時能成功建立 S3 client。
func TestAppS3ClientBuildsClient(t *testing.T) {
	a := &app{
		opts:   &globalOptions{timeout: 30 * time.Second},
		stdout: io.Discard,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		settings: &config.Settings{
			COS: config.COSSettings{Endpoint: config.DefaultCOSEndpoint, AccessKey: "ak", SecretKey: "sk"},
		},
	}
	client, err := a.s3Client()
	require.NoError(t, err)
	assert.NotNil(t, client)
}
