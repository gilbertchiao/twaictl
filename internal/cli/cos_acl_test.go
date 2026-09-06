package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCosAclPublicRequiresConfirmAndRecursive 驗證 --public 需確認；--recursive 以前綴展開，不誤命中手足前綴。
func TestCosAclPublicRequiresConfirmAndRecursive(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutObject("b", "site/a", []byte("a"))
	fs3.PutObject("b", "site/b", []byte("b"))
	fs3.PutObject("b", "sitemap", []byte("s"))

	code, _, stderr := runCLI(t, fs3.env(t), "n\n", "cos", "acl", "cos://b/site", "--public", "--recursive")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "2 個物件設為公開")
	assert.Equal(t, "private", fs3.ACL("b", "site/a"))

	code, stdout, stderr := runCLI(t, fs3.env(t), "", "cos", "acl", "cos://b/site", "--public", "--recursive", "-y", "-o", "json")
	require.Equal(t, ExitOK, code, stderr)
	assert.Equal(t, "public-read", fs3.ACL("b", "site/a"))
	assert.Equal(t, "public-read", fs3.ACL("b", "site/b"))
	assert.Equal(t, "private", fs3.ACL("b", "sitemap"))
	assert.Contains(t, stdout, `"acl": "public-read"`)
}

// TestCosAclYAMLOutput 驗證 renderSummary 補上 -o yaml 後，`cos acl` 也能輸出 YAML 摘要。
func TestCosAclYAMLOutput(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutObject("b", "k", []byte("x"))

	code, stdout, stderr := runCLI(t, fs3.env(t), "", "cos", "acl", "cos://b/k", "--private", "-o", "yaml")
	require.Equal(t, ExitOK, code, stderr)
	// jsonToYAML 先解成 map[string]any 再交給 yaml.Marshal，key 依字母序排列
	// （與 JSON 原本的欄位宣告順序無關）。
	assert.Equal(t, "- acl: private\n  bucket: b\n  key: k\n", stdout)
}

// TestCosAclPrivateNoConfirmAndFlagValidation 驗證 --private 不需確認；未指定或同時指定 --public/--private 是錯誤。
func TestCosAclPrivateNoConfirmAndFlagValidation(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutObject("b", "k", []byte("x"))
	code, _, stderr := runCLI(t, fs3.env(t), "", "cos", "acl", "cos://b/k", "--public", "-y")
	require.Equal(t, ExitOK, code, stderr)

	code, _, stderr = runCLI(t, fs3.env(t), "", "cos", "acl", "cos://b/k", "--private")
	require.Equal(t, ExitOK, code, stderr)
	assert.NotContains(t, stderr, "確定嗎")
	assert.Equal(t, "private", fs3.ACL("b", "k"))

	code, _, stderr = runCLI(t, fs3.env(t), "", "cos", "acl", "cos://b/k")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "擇一")
	code, _, stderr = runCLI(t, fs3.env(t), "", "cos", "acl", "cos://b/k", "--public", "--private")
	assert.Equal(t, ExitGeneral, code, stderr)
}

// TestCosAclDryRun 驗證 --dry-run 不送出 PUT ?acl；非遞迴時甚至不需要 COS 憑證
// （沒有 client 也能印出 dry-run 行，因為目標就是 locs 本身，不需要展開）。
func TestCosAclDryRun(t *testing.T) {
	noCredsEnv := map[string]string{"XDG_CONFIG_HOME": t.TempDir()}
	code, stdout, stderr := runCLI(t, noCredsEnv, "", "--dry-run", "cos", "acl", "cos://b/k", "--public")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stdout, "dry-run: 設定 cos://b/k 為 public-read")
}

// TestCosContentType 驗證 content-type 命令更新 Content-Type、保留內容與公開狀態；--dry-run 不送請求。
func TestCosContentType(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutObject("b", "page", []byte("<p/>"))
	code, _, stderr := runCLI(t, fs3.env(t), "", "cos", "acl", "cos://b/page", "--public", "-y")
	require.Equal(t, ExitOK, code, stderr)

	code, stdout, stderr := runCLI(t, fs3.env(t), "", "--dry-run", "cos", "content-type", "cos://b/page", "text/html")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stdout, "dry-run: 設定 cos://b/page 的 Content-Type 為 text/html")
	assert.Equal(t, "application/octet-stream", fs3.ContentType("b", "page"))

	code, stdout, stderr = runCLI(t, fs3.env(t), "", "cos", "content-type", "cos://b/page", "text/html", "-o", "json")
	require.Equal(t, ExitOK, code, stderr)
	assert.Equal(t, "text/html", fs3.ContentType("b", "page"))
	assert.Equal(t, "public-read", fs3.ACL("b", "page"))
	assert.JSONEq(t, `[{"bucket":"b","key":"page","content_type":"text/html"}]`, stdout)

	code, _, stderr = runCLI(t, fs3.env(t), "", "cos", "content-type", "cos://b/dir/", "text/html")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "完整的 key")
}
