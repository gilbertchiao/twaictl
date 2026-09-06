package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCosCpUploadSingleFile 驗證上傳到以 `/` 結尾的 cos:// URL 會用檔名補上 key。
func TestCosCpUploadSingleFile(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutBucket("b")
	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	require.NoError(t, os.WriteFile(src, []byte("hello"), 0o644))

	code, _, stderr := runCLI(t, fs3.env(t), "", "cos", "cp", src, "cos://b/dir/")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stderr, "上傳")

	body, ok := fs3.Object("b", "dir/a.txt")
	require.True(t, ok)
	assert.Equal(t, "hello", string(body))
	assert.True(t, strings.HasPrefix(fs3.ContentType("b", "dir/a.txt"), "text/plain"))
}

// TestCosCpUploadSymlinkToFileKeepsLinkName 驗證上傳「指向一般檔案的 symlink」時，
// key 推導沿用使用者輸入的 symlink 檔名（filepath.Base(src)），而不是它指向的實體檔名；
// 這是 I3 修法（解析指向目錄的 symlink 根路徑）的邊界情況：解析動作只能影響
// --recursive 走訪目錄用的根路徑，不能連帶改變單檔上傳既有的 key 推導語意。
func TestCosCpUploadSymlinkToFileKeepsLinkName(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink 測試在 Windows 需要額外權限，略過")
	}
	fs3 := newFakeS3(t)
	fs3.PutBucket("b")

	dir := t.TempDir()
	real := filepath.Join(dir, "real.txt")
	require.NoError(t, os.WriteFile(real, []byte("real content"), 0o644))
	link := filepath.Join(dir, "link.txt")
	require.NoError(t, os.Symlink(real, link))

	code, _, stderr := runCLI(t, fs3.env(t), "", "cos", "cp", link, "cos://b/dir/")
	require.Equal(t, ExitOK, code, stderr)

	body, ok := fs3.Object("b", "dir/link.txt")
	require.True(t, ok, "key 應沿用 symlink 本身的檔名")
	assert.Equal(t, "real content", string(body))

	_, ok = fs3.Object("b", "dir/real.txt")
	assert.False(t, ok, "不應該用 symlink 指向的實體檔名當 key")
}

// TestCosCpUploadRecursive 驗證 --recursive 遞迴走訪目錄，key 一律用 `/` 分隔。
func TestCosCpUploadRecursive(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutBucket("b")
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("A"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "b.txt"), []byte("B"), 0o644))

	code, _, stderr := runCLI(t, fs3.env(t), "", "cos", "cp", dir, "cos://b/backup", "--recursive")
	require.Equal(t, ExitOK, code, stderr)

	bodyA, ok := fs3.Object("b", "backup/a.txt")
	require.True(t, ok)
	assert.Equal(t, "A", string(bodyA))

	bodyB, ok := fs3.Object("b", "backup/sub/b.txt")
	require.True(t, ok)
	assert.Equal(t, "B", string(bodyB))
}

// TestCosCpUploadSymlinkedDirectoryRoot 驗證 `cos cp <symlink-to-dir> cos://... --recursive`
// 會把 symlink 解析成實際目錄後正常走訪上傳，而不是被 WalkDir 對根目錄的 Lstat
// 判斷成「symlink 型別」而整個跳過（先前會靜默上傳 0 個檔案、exit 0）。
func TestCosCpUploadSymlinkedDirectoryRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink 測試在 Windows 需要額外權限，略過")
	}
	fs3 := newFakeS3(t)
	fs3.PutBucket("b")

	base := t.TempDir()
	real := filepath.Join(base, "real")
	require.NoError(t, os.Mkdir(real, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(real, "a.txt"), []byte("A"), 0o644))
	link := filepath.Join(base, "link")
	require.NoError(t, os.Symlink(real, link))

	code, _, stderr := runCLI(t, fs3.env(t), "", "cos", "cp", link, "cos://b/up/", "--recursive")
	require.Equal(t, ExitOK, code, stderr)

	body, ok := fs3.Object("b", "up/a.txt")
	require.True(t, ok, "symlink 指向目錄的根應能正常走訪並上傳")
	assert.Equal(t, "A", string(body))
}

// TestCosCpUploadSkipsInnerSymlinkWithProgress 驗證遞迴上傳時，目錄樹「內部」的 symlink
// 仍然略過（避免符號連結迴圈），但現在會在 stderr 印出提示，讓使用者知道有檔案被跳過，
// 而不是靜默漏掉。
func TestCosCpUploadSkipsInnerSymlinkWithProgress(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink 測試在 Windows 需要額外權限，略過")
	}
	fs3 := newFakeS3(t)
	fs3.PutBucket("b")

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("A"), 0o644))
	outside := filepath.Join(t.TempDir(), "outside.txt")
	require.NoError(t, os.WriteFile(outside, []byte("O"), 0o644))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "link.txt")))

	code, _, stderr := runCLI(t, fs3.env(t), "", "cos", "cp", dir, "cos://b/up/", "--recursive")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stderr, "略過 symlink")

	_, ok := fs3.Object("b", "up/a.txt")
	assert.True(t, ok)
	_, ok = fs3.Object("b", "up/link.txt")
	assert.False(t, ok, "目錄樹內部的 symlink 仍應略過，不上傳")
}

// TestCosCpUploadEmptyDirReportsNothing 驗證遞迴上傳空目錄時，會明確印出「沒有可上傳的檔案」
// 的提示（而不是靜默成功，讓使用者以為上傳了東西但其實什麼都沒發生）。
func TestCosCpUploadEmptyDirReportsNothing(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutBucket("b")
	dir := t.TempDir()

	code, _, stderr := runCLI(t, fs3.env(t), "", "cos", "cp", dir, "cos://b/up/", "--recursive")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stderr, "沒有可上傳")
}

// TestCosCpDirectoryWithoutRecursiveFails 驗證上傳目錄卻沒帶 --recursive 是一般錯誤，
// 且完全不會送出任何 S3 請求（本地檢查先於任何遠端呼叫）。
func TestCosCpDirectoryWithoutRecursiveFails(t *testing.T) {
	fs3 := newFakeS3(t)
	dir := t.TempDir()

	code, _, stderr := runCLI(t, fs3.env(t), "", "cos", "cp", dir, "cos://b/backup/")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "--recursive")
	assert.Empty(t, fs3.Requests())
}

// TestCosCpDownloadSingleAndToDirectory 驗證下載到明確檔名，以及下載到既有目錄
// （以 key 最後一段作檔名）。
func TestCosCpDownloadSingleAndToDirectory(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutObject("b", "k.txt", []byte("content"))
	dir := t.TempDir()

	dst1 := filepath.Join(dir, "out.txt")
	code, _, stderr := runCLI(t, fs3.env(t), "", "cos", "cp", "cos://b/k.txt", dst1)
	require.Equal(t, ExitOK, code, stderr)
	data, err := os.ReadFile(dst1)
	require.NoError(t, err)
	assert.Equal(t, "content", string(data))

	code, _, stderr = runCLI(t, fs3.env(t), "", "cos", "cp", "cos://b/k.txt", dir+string(os.PathSeparator))
	require.Equal(t, ExitOK, code, stderr)
	data2, err := os.ReadFile(filepath.Join(dir, "k.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content", string(data2))
}

// TestCosCpDownloadRecursive 驗證 --recursive 用 prefix 列出物件後逐一下載，
// 且跳過以 `/` 結尾的「目錄標記」物件。
func TestCosCpDownloadRecursive(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutObject("b", "dir/a.txt", []byte("A"))
	fs3.PutObject("b", "dir/sub/b.txt", []byte("B"))
	fs3.PutObject("b", "dir/marker/", []byte(""))
	dir := t.TempDir()

	code, _, stderr := runCLI(t, fs3.env(t), "", "cos", "cp", "cos://b/dir/", dir, "--recursive")
	require.Equal(t, ExitOK, code, stderr)

	dataA, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	require.NoError(t, err)
	assert.Equal(t, "A", string(dataA))

	dataB, err := os.ReadFile(filepath.Join(dir, "sub", "b.txt"))
	require.NoError(t, err)
	assert.Equal(t, "B", string(dataB))

	_, err = os.Stat(filepath.Join(dir, "marker"))
	assert.True(t, os.IsNotExist(err), "目錄標記物件不應被下載")
}

// TestCosCpDownloadRecursiveSiblingPrefix 驗證 `cos cp cos://b/dir <dst> --recursive`
// （注意：src 沒有尾端 `/`）不會因為字串前綴比對而把 "directory/other.txt" 這種
// 手足前綴的物件也下載下來，也不會把 key 切成錯誤的相對路徑（例如 "ectory/other.txt"）。
func TestCosCpDownloadRecursiveSiblingPrefix(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutObject("b", "dir/a.txt", []byte("A"))
	fs3.PutObject("b", "directory/other.txt", []byte("O"))
	dst := t.TempDir()

	code, _, stderr := runCLI(t, fs3.env(t), "", "cos", "cp", "cos://b/dir", dst, "--recursive")
	require.Equal(t, ExitOK, code, stderr)

	entries, err := os.ReadDir(dst)
	require.NoError(t, err)
	require.Len(t, entries, 1, "只應該下載 dir/ 底下的物件")
	assert.Equal(t, "a.txt", entries[0].Name())

	data, err := os.ReadFile(filepath.Join(dst, "a.txt"))
	require.NoError(t, err)
	assert.Equal(t, "A", string(data))
}

// TestCosCpDownloadRecursiveNoMatch 驗證遞迴下載的前綴沒有符合的物件時，
// 印出「沒有符合的物件」提示、exit 0，而不是靜默什麼都不做。
func TestCosCpDownloadRecursiveNoMatch(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutBucket("b")
	dst := t.TempDir()

	code, _, stderr := runCLI(t, fs3.env(t), "", "cos", "cp", "cos://b/nothing/", dst, "--recursive")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stderr, "沒有符合的物件")
}

// TestCosCpDownloadRejectsPathTraversal 驗證遞迴下載遇到會逃出 dst 的 key
// （例如 "dir/../../evil.txt"）時整個命令中止、回一般錯誤，且不會真的把檔案
// 寫到 dst 之外。
func TestCosCpDownloadRejectsPathTraversal(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutObject("b", "dir/../../evil.txt", []byte("evil"))
	dst := t.TempDir()

	code, _, stderr := runCLI(t, fs3.env(t), "", "cos", "cp", "cos://b/dir/", dst, "--recursive")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "逃出")

	_, err := os.Stat(filepath.Join(filepath.Dir(dst), "evil.txt"))
	assert.True(t, os.IsNotExist(err), "不應該把檔案寫到 dst 之外")
	_, err = os.Stat(filepath.Join(filepath.Dir(filepath.Dir(dst)), "evil.txt"))
	assert.True(t, os.IsNotExist(err), "不應該把檔案寫到 dst 之外")
}

// TestCosCpDownloadRejectsSymlinkEscape 驗證即使 key 的字面相對路徑「乾淨」
// （沒有 `..` 之類的片段），若 dst 目錄樹裡已經有 symlink 指到 dst 之外
// （例如 dst/sub -> outside），遞迴下載仍會被 ensureUnderDir 用實體路徑攔下，
// 不會真的把檔案寫到 outside 底下。
func TestCosCpDownloadRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink 測試在 Windows 需要額外權限，略過")
	}
	fs3 := newFakeS3(t)
	fs3.PutObject("b", "dir/sub/evil.txt", []byte("evil"))
	outside := t.TempDir()
	dst := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(dst, "sub")))

	code, _, stderr := runCLI(t, fs3.env(t), "", "cos", "cp", "cos://b/dir/", dst, "--recursive")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "symlink")

	_, err := os.Stat(filepath.Join(outside, "evil.txt"))
	assert.True(t, os.IsNotExist(err), "不應該透過 symlink 把檔案寫到 dst 之外")
}

// TestCosCpDownloadRejectsSymlinkEscapeNested 驗證 key 有多層路徑時
// （去掉前綴後是 "sub/nested/evil.txt"），若 dst/sub 是指向 outside 的 symlink，
// 在建立 "nested" 這一層目錄之前就會先被 ensureUnderDir 擋下——不會先在 outside
// 底下建立空的 "nested" 目錄才發現逃出（檢查必須先於 MkdirAll）。
func TestCosCpDownloadRejectsSymlinkEscapeNested(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink 測試在 Windows 需要額外權限，略過")
	}
	fs3 := newFakeS3(t)
	fs3.PutObject("b", "dir/sub/nested/evil.txt", []byte("evil"))
	outside := t.TempDir()
	dst := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(dst, "sub")))

	code, _, stderr := runCLI(t, fs3.env(t), "", "cos", "cp", "cos://b/dir/", dst, "--recursive")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "symlink")

	_, err := os.Stat(filepath.Join(outside, "nested"))
	assert.True(t, os.IsNotExist(err), "偵測到逃出之前不應該先在 outside 底下建立 nested 目錄")
}

// TestCosCpDownloadRejectsExistingSymlinkTarget 驗證下載目標本身若已經是既存的
// symlink，會被拒絕（避免覆寫 symlink 指向的檔案，而不是建立新檔案）。
func TestCosCpDownloadRejectsExistingSymlinkTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink 測試在 Windows 需要額外權限，略過")
	}
	fs3 := newFakeS3(t)
	fs3.PutObject("b", "file.txt", []byte("new content"))
	outside := t.TempDir()
	dst := t.TempDir()
	outsideTarget := filepath.Join(outside, "target")
	require.NoError(t, os.Symlink(outsideTarget, filepath.Join(dst, "file.txt")))

	code, _, stderr := runCLI(t, fs3.env(t), "", "cos", "cp", "cos://b/file.txt", dst+string(os.PathSeparator))
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "symlink")

	_, err := os.Stat(outsideTarget)
	assert.True(t, os.IsNotExist(err), "不應該透過既存的 symlink 建立/覆寫 outside/target")
}

// TestCosCpDownloadSingleRejectsPathTraversal 驗證單檔下載（下載到目錄）時，
// 若 key 的最後一段本身就是 ".."，一樣要被拒絕，不能被拿去當檔名接在 dst 後面。
func TestCosCpDownloadSingleRejectsPathTraversal(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutObject("b", "..", []byte("x"))
	dst := t.TempDir()

	code, _, stderr := runCLI(t, fs3.env(t), "", "cos", "cp", "cos://b/..", dst+string(os.PathSeparator))
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "逃出")
}

// TestCosCpBothLocalOrBothRemoteFails 驗證 src/dst 皆本地或皆遠端時是一般錯誤，
// 且不送出任何 S3 請求。
func TestCosCpBothLocalOrBothRemoteFails(t *testing.T) {
	fs3 := newFakeS3(t)
	dir := t.TempDir()

	code, _, stderr := runCLI(t, fs3.env(t), "", "cos", "cp", dir, dir)
	assert.Equal(t, ExitGeneral, code, stderr)

	code, _, stderr = runCLI(t, fs3.env(t), "", "cos", "cp", "cos://a/x", "cos://b/y")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Empty(t, fs3.Requests())
}

// TestCosCpJSONOutput 驗證 -o json 輸出 [{"src","dst","size"}] 形狀。
func TestCosCpJSONOutput(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutBucket("b")
	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	require.NoError(t, os.WriteFile(src, []byte("hello"), 0o644))

	code, stdout, stderr := runCLI(t, fs3.env(t), "", "cos", "cp", src, "cos://b/a.txt", "-o", "json")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stdout, `"src"`)
	assert.Contains(t, stdout, `"dst": "cos://b/a.txt"`)
	assert.Contains(t, stdout, `"size": 5`)
}

// TestCosCpUploadDryRun 驗證 --dry-run 上傳單一檔案不會建立 S3 client、不送出任何請求。
func TestCosCpUploadDryRun(t *testing.T) {
	fs3 := newFakeS3(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	require.NoError(t, os.WriteFile(src, []byte("hello"), 0o644))

	code, stdout, stderr := runCLI(t, fs3.env(t), "", "--dry-run", "cos", "cp", src, "cos://b/dir/")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stdout, "dry-run: 上傳")
	assert.Contains(t, stdout, "cos://b/dir/a.txt")
	assert.Empty(t, fs3.Requests())
}

// TestCosCpDownloadRecursiveDryRun 驗證 --recursive --dry-run 下載仍會呼叫 ListObjects
// （read-only），但不會真的下載任何檔案。
func TestCosCpDownloadRecursiveDryRun(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutObject("b", "dir/a.txt", []byte("A"))
	dir := t.TempDir()

	code, stdout, stderr := runCLI(t, fs3.env(t), "", "--dry-run", "cos", "cp", "cos://b/dir/", dir, "--recursive")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stdout, "dry-run: 下載")
	assert.Contains(t, stdout, "dir/a.txt")

	reqs := fs3.Requests()
	require.Len(t, reqs, 1, "recursive dry-run 仍需要 list（read-only）")
	assert.Equal(t, "GET", reqs[0].Method)

	_, err := os.Stat(filepath.Join(dir, "a.txt"))
	assert.True(t, os.IsNotExist(err), "dry-run 不應真的下載")
}
