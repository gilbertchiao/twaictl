package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilbertchiao/twaictl/internal/testutil"
)

func TestKeypairLsGolden(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/keypairs/", 200, "vcs/keypairs")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "keypair", "ls")
	require.Equal(t, ExitOK, code, stderr)
	testutil.AssertGolden(t, "vcs_keypair_ls.txt", []byte(stdout))
}

func TestKeypairLsJSONIsRawArray(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/keypairs/", 200, "vcs/keypairs")
	code, stdout, _ := runCLI(t, f.env(t), "", "vcs", "keypair", "ls", "-o", "json")
	require.Equal(t, ExitOK, code)
	assert.JSONEq(t, fixture(t, "vcs/keypairs"), stdout)
}

func TestKeypairGetGolden(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/keypairs/mykey/", 200, "vcs/keypair_detail")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "keypair", "get", "mykey")
	require.Equal(t, ExitOK, code, stderr)
	testutil.AssertGolden(t, "vcs_keypair_get.txt", []byte(stdout))
}

func TestKeypairGetJSONIsRawObject(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/keypairs/mykey/", 200, "vcs/keypair_detail")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "keypair", "get", "mykey", "-o", "json")
	require.Equal(t, ExitOK, code, stderr)
	assert.JSONEq(t, fixture(t, "vcs/keypair_detail"), stdout)
}

func TestKeypairCreatePrintsPEMToStdout(t *testing.T) {
	f := newFakeAPI(t)
	f.onText("POST", vcsBase+"/keypairs/", 200, "-----BEGIN RSA PRIVATE KEY-----\nabc\n-----END RSA PRIVATE KEY-----\n")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "keypair", "create", "mykey")
	require.Equal(t, ExitOK, code, stderr)
	assert.Equal(t, "-----BEGIN RSA PRIVATE KEY-----\nabc\n-----END RSA PRIVATE KEY-----\n", stdout)
	assert.NotContains(t, stderr, "abc")
}

func TestKeypairCreateOutFileIs0600AndNotOverwritten(t *testing.T) {
	f := newFakeAPI(t)
	f.onText("POST", vcsBase+"/keypairs/", 200, "-----BEGIN RSA PRIVATE KEY-----\nabc\n-----END RSA PRIVATE KEY-----\n")
	dir := t.TempDir()
	out := filepath.Join(dir, "mykey.pem")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "keypair", "create", "mykey", "--out", out)
	require.Equal(t, ExitOK, code, stderr)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "已將 private key 寫入 "+out+"（權限 0600）")
	assert.NotContains(t, stderr, "abc", "private key 內容不可出現在 stderr")

	data, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Equal(t, "-----BEGIN RSA PRIVATE KEY-----\nabc\n-----END RSA PRIVATE KEY-----\n", string(data))

	if runtime.GOOS != "windows" {
		info, err := os.Stat(out)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}

	// 第二次以同路徑建立 → 不可覆寫，exit 1，檔案內容不變；
	// 且因為檔案保留（reserve）發生在呼叫 API 之前，不可再送出第二次 POST
	// （否則會在遠端多建立一個永遠拿不到 private key 的金鑰對）。
	postCountBefore := len(f.Requests())
	code, stdout, stderr = runCLI(t, f.env(t), "", "vcs", "keypair", "create", "mykey", "--out", out)
	assert.Equal(t, ExitGeneral, code)
	assert.Empty(t, stdout)
	assert.NotContains(t, stderr, "abc")
	assert.Equal(t, postCountBefore, len(f.Requests()), "檔案已存在時不可送出任何新請求")

	data, err = os.ReadFile(out)
	require.NoError(t, err)
	assert.Equal(t, "-----BEGIN RSA PRIVATE KEY-----\nabc\n-----END RSA PRIVATE KEY-----\n", string(data), "檔案內容不可被第二次呼叫覆寫")
}

func TestKeypairCreateOutExistingFileFailsBeforeAPI(t *testing.T) {
	f := newFakeAPI(t)
	f.onText("POST", vcsBase+"/keypairs/", 200, "-----BEGIN RSA PRIVATE KEY-----\nabc\n-----END RSA PRIVATE KEY-----\n")
	dir := t.TempDir()
	out := filepath.Join(dir, "mykey.pem")
	require.NoError(t, os.WriteFile(out, []byte("既有內容"), 0o600))

	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "keypair", "create", "mykey", "--out", out)
	assert.Equal(t, ExitGeneral, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "已存在")
	assert.Nil(t, f.find("POST", vcsBase+"/keypairs/"), "檔案已存在時，不可在檢查失敗前就先呼叫 API 建立遠端金鑰對")

	data, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Equal(t, "既有內容", string(data), "既有檔案內容不可被覆寫")
}

func TestKeypairCreateOutRemovedWhenAPIFails(t *testing.T) {
	f := newFakeAPI(t)
	f.on("POST", vcsBase+"/keypairs/", 500, `{"message":"boom"}`)
	dir := t.TempDir()
	out := filepath.Join(dir, "mykey.pem")

	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "keypair", "create", "mykey", "--out", out)
	assert.Equal(t, ExitAPI, code, stderr)
	assert.Empty(t, stdout)

	_, err := os.Stat(out)
	assert.True(t, os.IsNotExist(err), "API 失敗時，事先保留的檔案必須被清除，否則會留下一個永遠沒有內容的空檔")
}

func TestKeypairCreateOutWithJSONIsError(t *testing.T) {
	f := newFakeAPI(t)
	dir := t.TempDir()
	out := filepath.Join(dir, "mykey.pem")

	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "keypair", "create", "mykey", "--out", out, "-o", "json")
	assert.Equal(t, ExitGeneral, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "--out")
	assert.Contains(t, stderr, "json")
	assert.Empty(t, f.Requests(), "--out 與 -o json 併用時，格式檢查必須在任何 API 呼叫之前就擋下")

	_, err := os.Stat(out)
	assert.True(t, os.IsNotExist(err), "--out 與 -o json 併用時不可建立檔案")
}

func TestKeypairCreateWithPublicKeyFile(t *testing.T) {
	f := newFakeAPI(t)
	f.onText("POST", vcsBase+"/keypairs/", 200, "")
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "id_rsa.pub")
	require.NoError(t, os.WriteFile(keyFile, []byte("ssh-rsa AAAA...  \n"), 0o600))

	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "keypair", "create", "mykey", "--public-key-file", keyFile)
	require.Equal(t, ExitOK, code, stderr)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "金鑰對 mykey 已建立")

	post := f.find("POST", vcsBase+"/keypairs/")
	require.NotNil(t, post)
	assert.Contains(t, string(post.Body), `"public_key":"ssh-rsa AAAA..."`, "public key 內容需經 TrimSpace")
}

func TestKeypairCreateJSONOutputsStructuredObject(t *testing.T) {
	f := newFakeAPI(t)
	f.onText("POST", vcsBase+"/keypairs/", 200, "-----BEGIN RSA PRIVATE KEY-----\nabc\n-----END RSA PRIVATE KEY-----\n")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "keypair", "create", "mykey", "-o", "json")
	require.Equal(t, ExitOK, code, stderr)
	assert.JSONEq(t, `{"name":"mykey","private_key":"-----BEGIN RSA PRIVATE KEY-----\nabc\n-----END RSA PRIVATE KEY-----\n"}`, stdout)
}

func TestKeypairCreateYAMLOutput(t *testing.T) {
	f := newFakeAPI(t)
	f.onText("POST", vcsBase+"/keypairs/", 200, "-----BEGIN RSA PRIVATE KEY-----\nabc\n-----END RSA PRIVATE KEY-----\n")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "keypair", "create", "mykey", "-o", "yaml")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stdout, "name: mykey")
	assert.Contains(t, stdout, "private_key:")
	assert.Contains(t, stdout, "BEGIN RSA PRIVATE KEY")
}

func TestKeypairRmConfirm(t *testing.T) {
	f := newFakeAPI(t)
	f.on("DELETE", vcsBase+"/keypairs/mykey/", 204, "")
	code, stdout, stderr := runCLI(t, f.env(t), "n\n", "vcs", "keypair", "rm", "mykey")
	assert.Equal(t, ExitGeneral, code)
	assert.Contains(t, stderr, "確定嗎")
	assert.Contains(t, stderr, "已取消")
	assert.Empty(t, stdout)
	assert.Nil(t, f.find("DELETE", vcsBase+"/keypairs/mykey/"), "取消後不可送出 DELETE")

	code, _, stderr = runCLI(t, f.env(t), "", "vcs", "keypair", "rm", "mykey", "-y")
	require.Equal(t, ExitOK, code, stderr)
	assert.NotNil(t, f.find("DELETE", vcsBase+"/keypairs/mykey/"))
	assert.Contains(t, stderr, "已刪除金鑰對：mykey")
}

func TestKeypairAPIErrorIsExitAPI(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/keypairs/", 403, `{"message":"forbidden"}`)
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "keypair", "ls")
	assert.Equal(t, ExitAPI, code)
	assert.Contains(t, stderr, "403")
	assert.Empty(t, stdout)
}
