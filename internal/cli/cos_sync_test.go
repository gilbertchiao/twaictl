package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeFile 在 dir 底下建立 rel（可含子目錄）並寫入 content。
func writeFile(t *testing.T, dir, rel, content string) string {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

// TestCosSyncUploadsOnlyChangedFiles 驗證：新檔案上傳、內容相同略過、大小不同上傳、大小相同但內容不同（MD5）上傳、
// 大小相同且內容相同但遠端 ETag 為大寫 hex 時仍能正確判定略過（不分大小寫比對）；
// 子目錄 key 用 `/` 分隔；stderr 有總結。
func TestCosSyncUploadsOnlyChangedFiles(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutBucket("b")
	fs3.PutObject("b", "site/same.txt", []byte("same"))
	fs3.PutObject("b", "site/size.txt", []byte("old"))
	fs3.PutObject("b", "site/md5.txt", []byte("aaaa"))
	// "same" 的 MD5 是 51037a4a37730f52c8732586d3aaa316（printf 'same' | md5sum 驗證過）；
	// 用大寫 ETag 模擬部分 S3 相容服務回傳大寫 hex 的情境。
	fs3.PutObjectWithETag("b", "site/upper.txt", []byte("same"), `"51037A4A37730F52C8732586D3AAA316"`)

	dir := t.TempDir()
	writeFile(t, dir, "same.txt", "same")
	writeFile(t, dir, "size.txt", "longer")
	writeFile(t, dir, "md5.txt", "bbbb")
	writeFile(t, dir, "sub/new.txt", "new")
	writeFile(t, dir, "upper.txt", "same")

	code, _, stderr := runCLI(t, fs3.env(t), "", "cos", "sync", dir, "cos://b/site")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stderr, "sync 完成：上傳 3、略過 2、刪除 0")
	assert.Contains(t, stderr, "大小不同")
	assert.Contains(t, stderr, "內容不同")
	assert.Contains(t, stderr, "新檔案")

	body, _ := fs3.Object("b", "site/size.txt")
	assert.Equal(t, "longer", string(body))
	body, _ = fs3.Object("b", "site/md5.txt")
	assert.Equal(t, "bbbb", string(body))
	body, _ = fs3.Object("b", "site/sub/new.txt")
	assert.Equal(t, "new", string(body))

	// same.txt、upper.txt 都沒有被重新上傳：只有 3 個 PUT
	puts := 0
	for _, r := range fs3.Requests() {
		if r.Method == "PUT" {
			puts++
		}
	}
	assert.Equal(t, 3, puts)
}

// TestCosSyncDeleteRemovesRemoteOnly 驗證 --delete 刪除遠端多出的物件（需確認；取消時整個命令不動作），
// 不會動到 prefix 以外的物件。
func TestCosSyncDeleteRemovesRemoteOnly(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutBucket("b")
	fs3.PutObject("b", "site/old.txt", []byte("old"))
	fs3.PutObject("b", "other/keep.txt", []byte("keep"))
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "a")

	code, _, stderr := runCLI(t, fs3.env(t), "n\n", "cos", "sync", dir, "cos://b/site/", "--delete")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "1 個本地不存在的遠端物件")
	assert.Contains(t, stderr, "cos://b/site/", "確認訊息要說明目標前綴")
	_, ok := fs3.Object("b", "site/a.txt")
	assert.False(t, ok, "取消後不應上傳")

	code, stdout, stderr := runCLI(t, fs3.env(t), "", "cos", "sync", dir, "cos://b/site/", "--delete", "-y", "-o", "json")
	require.Equal(t, ExitOK, code, stderr)
	_, ok = fs3.Object("b", "site/old.txt")
	assert.False(t, ok)
	_, ok = fs3.Object("b", "other/keep.txt")
	assert.True(t, ok)
	assert.Contains(t, stdout, `"action": "delete"`)
	assert.Contains(t, stdout, `"action": "upload"`)
	assert.Contains(t, stdout, `"reason": "新檔案"`)
}

// TestCosSyncDeleteConfirmMessageWholeBucket 驗證未帶前綴（sync 到 cos://b/）時，
// --delete 的確認訊息用「整個 bucket」描述目標，而不是誤寫成空前綴。
func TestCosSyncDeleteConfirmMessageWholeBucket(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutBucket("b")
	fs3.PutObject("b", "old.txt", []byte("old"))
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "a")

	code, _, stderr := runCLI(t, fs3.env(t), "n\n", "cos", "sync", dir, "cos://b/", "--delete")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "整個 bucket cos://b")
}

// TestCosSyncDryRun 驗證 --dry-run 只 list 不上傳不刪除，並印出每個動作與總結。
func TestCosSyncDryRun(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutBucket("b")
	fs3.PutObject("b", "old.txt", []byte("old"))
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "a")

	code, stdout, stderr := runCLI(t, fs3.env(t), "", "--dry-run", "cos", "sync", dir, "cos://b/", "--delete")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stdout, "dry-run: 上傳 ")
	assert.Contains(t, stdout, "cos://b/a.txt（新檔案）")
	assert.Contains(t, stdout, "dry-run: 刪除 cos://b/old.txt")
	assert.Contains(t, stdout, "dry-run: 上傳 1、略過 0、刪除 1")
	for _, r := range fs3.Requests() {
		assert.Equal(t, "GET", r.Method)
	}
}

// TestCosSyncMultipartETagUsesMtime 驗證遠端 ETag 為 multipart 形式（無法比對 MD5）時，
// 大小相同且本地 mtime 不晚於遠端 → 略過；本地較新 → 上傳。
func TestCosSyncMultipartETagUsesMtime(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutBucket("b")
	fs3.PutObjectWithETag("b", "big.bin", []byte("1234"), `"abc-2"`)
	dir := t.TempDir()
	path := writeFile(t, dir, "big.bin", "5678")
	old := time.Now().Add(-time.Hour)
	require.NoError(t, os.Chtimes(path, old, old))

	code, _, stderr := runCLI(t, fs3.env(t), "", "cos", "sync", dir, "cos://b/")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stderr, "上傳 0、略過 1")

	future := time.Now().Add(time.Hour)
	require.NoError(t, os.Chtimes(path, future, future))
	code, _, stderr = runCLI(t, fs3.env(t), "", "cos", "sync", dir, "cos://b/")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stderr, "本地較新")
	assert.Contains(t, stderr, "上傳 1、略過 0")
}

// TestCosSyncRejectsBadArgs 驗證來源不存在、來源不是目錄、目的地不是 cos:// 都是一般錯誤且不送出請求。
func TestCosSyncRejectsBadArgs(t *testing.T) {
	fs3 := newFakeS3(t)
	file := writeFile(t, t.TempDir(), "f.txt", "x")

	code, _, stderr := runCLI(t, fs3.env(t), "", "cos", "sync", filepath.Join(t.TempDir(), "missing"), "cos://b/")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "不是目錄")

	code, _, stderr = runCLI(t, fs3.env(t), "", "cos", "sync", file, "cos://b/")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "不是目錄")

	code, _, stderr = runCLI(t, fs3.env(t), "", "cos", "sync", filepath.Dir(file), "./out")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "cos://")
	assert.Empty(t, fs3.Requests())
}

// TestCosSyncEmptyResultJSONOutputsEmptyArray 驗證本地目錄底下沒有檔案、遠端也沒東西可刪時，
// -o json 仍輸出 `[]`（與 cos cp 同情境一致），而不是留空白 stdout。
func TestCosSyncEmptyResultJSONOutputsEmptyArray(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutBucket("b")
	dir := t.TempDir()

	code, stdout, stderr := runCLI(t, fs3.env(t), "", "cos", "sync", dir, "cos://b/", "-o", "json")
	require.Equal(t, ExitOK, code, stderr)
	assert.Equal(t, "[]\n", stdout)
}
