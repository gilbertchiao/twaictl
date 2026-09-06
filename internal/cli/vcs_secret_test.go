package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilbertchiao/twaictl/internal/config"
	"github.com/gilbertchiao/twaictl/internal/testutil"
)

func TestVCSSecretLsGolden(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/secrets/", 200, "vcs/secrets")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "secret", "ls")
	require.Equal(t, ExitOK, code, stderr)
	assert.Equal(t, "project=101", f.find("GET", vcsBase+"/secrets/").Query)
	testutil.AssertGolden(t, "vcs_secret_ls.txt", []byte(stdout))
}

func TestVCSSecretGetByName(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/secrets/", 200, "vcs/secrets")
	f.onFixture("GET", vcsBase+"/secrets/7/", 200, "vcs/secret_detail")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "secret", "get", "tls-cert")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stdout, "tls-cert")
	assert.Contains(t, stdout, "2027-01-01 08:00:00")
}

func TestVCSSecretCreateFromFileEncodesBase64(t *testing.T) {
	f := newFakeAPI(t)
	f.on("POST", vcsBase+"/secrets/", 200, fixture(t, "vcs/secret_detail"))
	dir := t.TempDir()
	payloadFile := filepath.Join(dir, "cert.pem")
	require.NoError(t, os.WriteFile(payloadFile, []byte("hello"), 0o644))

	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "secret", "create",
		"--name", "s1", "--payload-file", payloadFile, "--expire", "2027-01-01T00:00:00Z", "--desc", "d")
	require.Equal(t, ExitOK, code, stderr)
	req := f.find("POST", vcsBase+"/secrets/")
	require.NotNil(t, req)
	assert.Equal(t, "project=101", req.Query)
	assert.JSONEq(t, `{"name":"s1","payload":"aGVsbG8=","expire_time":"2027-01-01T00:00:00Z","desc":"d","project":101}`, string(req.Body))
	assert.Contains(t, stdout, "tls-cert")
}

func TestVCSSecretCreateFromStdin(t *testing.T) {
	f := newFakeAPI(t)
	f.on("POST", vcsBase+"/secrets/", 200, fixture(t, "vcs/secret_detail"))
	code, _, stderr := runCLI(t, f.env(t), "hello\n", "vcs", "secret", "create", "--name", "s1", "--payload-stdin")
	require.Equal(t, ExitOK, code, stderr)
	req := f.find("POST", vcsBase+"/secrets/")
	require.NotNil(t, req)
	assert.JSONEq(t, `{"name":"s1","payload":"aGVsbG8K","project":101}`, string(req.Body))
}

func TestVCSSecretCreateRequiresExactlyOnePayloadSource(t *testing.T) {
	f := newFakeAPI(t)
	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "secret", "create", "--name", "s1")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "--payload-file 或 --payload-stdin")
	assert.Nil(t, f.find("POST", vcsBase+"/secrets/"))

	dir := t.TempDir()
	payloadFile := filepath.Join(dir, "cert.pem")
	require.NoError(t, os.WriteFile(payloadFile, []byte("hello"), 0o644))
	code, _, stderr = runCLI(t, f.env(t), "hello\n", "vcs", "secret", "create",
		"--name", "s1", "--payload-file", payloadFile, "--payload-stdin")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "--payload-file 或 --payload-stdin")
	assert.Nil(t, f.find("POST", vcsBase+"/secrets/"))

	code, _, stderr = runCLI(t, f.env(t), "", "vcs", "secret", "create",
		"--name", "s1", "--payload-file", payloadFile, "--expire", "not-a-time")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "--expire")
	assert.Nil(t, f.find("POST", vcsBase+"/secrets/"))
}

// TestVCSSecretCreateEmptyStdinPayloadIsError 驗證 --payload-stdin 讀到空內容
// （沒有接管線、或接了空檔案）時明確報錯，而不是靜默送出空字串 payload
// （base64 編碼空字串仍是合法值，容易被誤判成成功）。
func TestVCSSecretCreateEmptyStdinPayloadIsError(t *testing.T) {
	f := newFakeAPI(t)
	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "secret", "create", "--name", "s1", "--payload-stdin")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "payload 為空")
	assert.Nil(t, f.find("POST", vcsBase+"/secrets/"))
}

// TestVCSSecretCreateEmptyPayloadFileIsError 驗證 --payload-file 讀到空檔案時
// 也明確報錯（與 --payload-stdin 分支的空值檢查一致），而不是靜默送出空字串
// payload——空的 cert.pem（憑證產生步驟失敗、scp 傳了 0 byte）在寫入型的
// create 路徑上會建立一個內容為空、之後掛到 load balancer 才會神祕失敗的 secret。
func TestVCSSecretCreateEmptyPayloadFileIsError(t *testing.T) {
	f := newFakeAPI(t)
	dir := t.TempDir()
	payloadFile := filepath.Join(dir, "empty.pem")
	require.NoError(t, os.WriteFile(payloadFile, []byte{}, 0o644))

	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "secret", "create",
		"--name", "s1", "--payload-file", payloadFile)
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "payload 為空")
	assert.Contains(t, stderr, payloadFile)
	assert.Nil(t, f.find("POST", vcsBase+"/secrets/"))
}

func TestVCSSecretCreateDryRunMasksPayload(t *testing.T) {
	f := newFakeAPI(t)
	dir := t.TempDir()
	payloadFile := filepath.Join(dir, "cert.pem")
	require.NoError(t, os.WriteFile(payloadFile, []byte("hello"), 0o644))

	// project 用數字（略過 ResolveProjectID 的 GET /projects/ 名稱解析），
	// 否則 --dry-run 會讓第一個請求（project 解析）就印出 curl 並中止，
	// 看不到本測試真正要驗證的 POST /secrets/ payload 遮蔽。
	env := f.env(t)
	env[config.EnvProjectCode] = "101"
	code, stdout, stderr := runCLI(t, env, "", "vcs", "secret", "create",
		"--name", "s1", "--payload-file", payloadFile, "--dry-run")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stdout, "curl -X POST")
	assert.Contains(t, stdout, "***")
	assert.NotContains(t, stdout, "aGVsbG8=")
	assert.Empty(t, f.Requests())
}

func TestVCSSecretRmConfirm(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/secrets/", 200, "vcs/secrets")
	f.on("DELETE", vcsBase+"/secrets/7/", 204, "")
	code, _, stderr := runCLI(t, f.env(t), "n\n", "vcs", "secret", "rm", "tls-cert")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Nil(t, f.find("DELETE", vcsBase+"/secrets/7/"))

	code, _, stderr = runCLI(t, f.env(t), "", "vcs", "secret", "rm", "tls-cert", "-y")
	require.Equal(t, ExitOK, code, stderr)
	assert.NotNil(t, f.find("DELETE", vcsBase+"/secrets/7/"))
}
