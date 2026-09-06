package cli

import (
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilbertchiao/twaictl/internal/testutil"
)

// TestCosLsGoldenAndPrefix 驗證 `cos ls <bucket>` 列出所有物件（golden，時間戳記已正規化）；
// 也驗證 `cos://bucket/prefix` 與 `<bucket> <prefix>` 兩種 prefix 指定方式，
// 以及兩者同時指定時為錯誤。
func TestCosLsGoldenAndPrefix(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutObject("b", "a.txt", []byte("hello"))
	fs3.PutObject("b", "dir/b.txt", []byte("world!"))

	code, stdout, stderr := runCLI(t, fs3.env(t), "", "cos", "ls", "b")
	require.Equal(t, ExitOK, code, stderr)
	testutil.AssertGolden(t, "cos_ls.txt", []byte(normalizeTimestamps(stdout)))

	code, stdout, stderr = runCLI(t, fs3.env(t), "", "cos", "ls", "cos://b/dir/")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stdout, "dir/b.txt")
	assert.NotContains(t, stdout, "a.txt")

	code, stdout, stderr = runCLI(t, fs3.env(t), "", "cos", "ls", "b", "dir/")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stdout, "dir/b.txt")
	assert.NotContains(t, stdout, "a.txt")

	code, _, stderr = runCLI(t, fs3.env(t), "", "cos", "ls", "cos://b/dir/", "dir/")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "prefix")
}

// TestCosLsNonURLBucketSlashPrefix 驗證第一個參數不是 `cos://` URL、但含 `/` 時
// （例如 `cos ls b/dir`），會用 `/` 切成 bucket 與 prefix，而不是把整個 "b/dir"
// 當成不存在的 bucket 名稱去查；與第二個參數同時指定 prefix 時仍是錯誤。
func TestCosLsNonURLBucketSlashPrefix(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutObject("b", "dir/x.txt", []byte("X"))
	fs3.PutObject("b", "other.txt", []byte("O"))

	code, stdout, stderr := runCLI(t, fs3.env(t), "", "cos", "ls", "b/dir")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stdout, "dir/x.txt")
	assert.NotContains(t, stdout, "other.txt")

	code, _, stderr = runCLI(t, fs3.env(t), "", "cos", "ls", "b/dir", "dir/")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "prefix")
}

// TestCosLsJSON 驗證 -o json 輸出 [{"key","size","last_modified","etag"}] 形狀。
func TestCosLsJSON(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutObject("b", "a.txt", []byte("hello"))

	code, stdout, stderr := runCLI(t, fs3.env(t), "", "cos", "ls", "b", "-o", "json")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stdout, `"key": "a.txt"`)
	assert.Contains(t, stdout, `"size": 5`)
	assert.Contains(t, stdout, `"last_modified"`)
	assert.Contains(t, stdout, `"etag"`)
}

// TestCosLsJSONEmptyBucketIsEmptyArray 驗證空 bucket 的 `-o json` 輸出是 `[]`，
// 而不是 Go nil slice 被 json.Marshal 成的 `null`（後者對消費 JSON 的腳本較不友善，
// 通常得額外處理 null 才能當陣列疊代）。
func TestCosLsJSONEmptyBucketIsEmptyArray(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutBucket("b")

	code, stdout, stderr := runCLI(t, fs3.env(t), "", "cos", "ls", "b", "-o", "json")
	require.Equal(t, ExitOK, code, stderr)
	assert.Equal(t, "[]\n", stdout)
}

// TestCosRmSingleAndRecursive 驗證單一完整 key 刪除，以及 --recursive 依前綴列出後逐一刪除。
func TestCosRmSingleAndRecursive(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutObject("b", "a.txt", []byte("A"))
	fs3.PutObject("b", "dir/x.txt", []byte("X"))
	fs3.PutObject("b", "dir/y.txt", []byte("Y"))

	code, _, stderr := runCLI(t, fs3.env(t), "", "cos", "rm", "cos://b/a.txt", "-y")
	require.Equal(t, ExitOK, code, stderr)
	_, ok := fs3.Object("b", "a.txt")
	assert.False(t, ok)

	code, _, stderr = runCLI(t, fs3.env(t), "", "cos", "rm", "cos://b/dir/", "--recursive", "-y")
	require.Equal(t, ExitOK, code, stderr)
	_, ok = fs3.Object("b", "dir/x.txt")
	assert.False(t, ok)
	_, ok = fs3.Object("b", "dir/y.txt")
	assert.False(t, ok)
}

// TestCosRmRecursiveDoesNotDeleteSiblingPrefix 驗證 `cos rm cos://b/dir --recursive`
// （注意：src 沒有尾端 `/`）不會因為字串前綴比對而誤刪 "directory/other.txt"
// 這種手足前綴的物件——prefix 必須先正規化補上 `/` 才拿去 ListObjects。
func TestCosRmRecursiveDoesNotDeleteSiblingPrefix(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutObject("b", "dir/a.txt", []byte("A"))
	fs3.PutObject("b", "directory/other.txt", []byte("O"))

	code, _, stderr := runCLI(t, fs3.env(t), "", "cos", "rm", "cos://b/dir", "--recursive", "-y")
	require.Equal(t, ExitOK, code, stderr)

	_, ok := fs3.Object("b", "dir/a.txt")
	assert.False(t, ok)
	_, ok = fs3.Object("b", "directory/other.txt")
	assert.True(t, ok, "不應刪除手足前綴（directory/）的物件")
}

// TestCosRmRequiresRecursiveForPrefix 驗證沒有 --recursive 時，key 為空或以 `/` 結尾
// 視為一般錯誤，且不送出任何請求。
func TestCosRmRequiresRecursiveForPrefix(t *testing.T) {
	fs3 := newFakeS3(t)
	code, _, stderr := runCLI(t, fs3.env(t), "", "cos", "rm", "cos://b/dir/", "-y")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "--recursive")
	assert.Empty(t, fs3.Requests())
}

// TestCosRmConfirm 驗證取消時不送出 DELETE。
func TestCosRmConfirm(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutObject("b", "a.txt", []byte("A"))

	code, _, stderr := runCLI(t, fs3.env(t), "n\n", "cos", "rm", "cos://b/a.txt")
	assert.Equal(t, ExitGeneral, code, stderr)
	_, ok := fs3.Object("b", "a.txt")
	assert.True(t, ok, "取消後物件應仍存在")
}

// TestCosRmJSONOutput 驗證 -o json 輸出 [{"bucket","key"}] 形狀。
func TestCosRmJSONOutput(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutObject("b", "k", []byte("x"))

	code, stdout, stderr := runCLI(t, fs3.env(t), "", "cos", "rm", "cos://b/k", "-y", "-o", "json")
	require.Equal(t, ExitOK, code, stderr)
	assert.JSONEq(t, `[{"bucket":"b","key":"k"}]`, stdout)
}

// TestCosRmYAMLOutput 驗證 renderSummary 補上 -o yaml 後，`cos rm` 也能輸出 YAML 摘要
// （欄位名稱與 json 一致，走 output.RenderRaw 轉換）。
func TestCosRmYAMLOutput(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutObject("b", "k", []byte("x"))

	code, stdout, stderr := runCLI(t, fs3.env(t), "", "cos", "rm", "cos://b/k", "-y", "-o", "yaml")
	require.Equal(t, ExitOK, code, stderr)
	assert.Equal(t, "- bucket: b\n  key: k\n", stdout)
}

// TestCosRmRecursiveJSONOutput 驗證 `cos rm --recursive -o json` 也會累積摘要
// （cosRmRecursive 的 collect 分支：collect := summaryWanted(a) 為 true 時才會把
// 每筆刪除結果 append 進 summaries；-o table／未指定 -o 時不會累積，見
// TestCosRmRecursiveTwoPassListsTwiceAndStreams 的常數記憶體說明），輸出形狀與非
// --recursive 的 TestCosRmJSONOutput 一致。
func TestCosRmRecursiveJSONOutput(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutObject("b", "dir/a.txt", []byte("A"))
	fs3.PutObject("b", "dir/b.txt", []byte("B"))

	code, stdout, stderr := runCLI(t, fs3.env(t), "", "cos", "rm", "cos://b/dir/", "--recursive", "-y", "-o", "json")
	require.Equal(t, ExitOK, code, stderr)
	assert.JSONEq(t, `[{"bucket":"b","key":"dir/a.txt"},{"bucket":"b","key":"dir/b.txt"}]`, stdout)
}

// TestCosRmRecursiveYAMLOutput 是 TestCosRmRecursiveJSONOutput 的 -o yaml 版本。
func TestCosRmRecursiveYAMLOutput(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutObject("b", "dir/a.txt", []byte("A"))

	code, stdout, stderr := runCLI(t, fs3.env(t), "", "cos", "rm", "cos://b/dir/", "--recursive", "-y", "-o", "yaml")
	require.Equal(t, ExitOK, code, stderr)
	assert.Equal(t, "- bucket: b\n  key: dir/a.txt\n", stdout)
}

// TestCosRmDryRun 驗證非 --recursive 的 --dry-run 完全不送出任何請求。
func TestCosRmDryRun(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutObject("b", "a.txt", []byte("A"))

	code, stdout, stderr := runCLI(t, fs3.env(t), "", "--dry-run", "cos", "rm", "cos://b/a.txt")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stdout, "dry-run: 刪除 cos://b/a.txt")
	assert.Empty(t, fs3.Requests())

	_, ok := fs3.Object("b", "a.txt")
	assert.True(t, ok)
}

// TestCosRmRecursiveConfirmShowsRealCount 驗證 --recursive 的確認訊息顯示的是
// 實際 ListObjects 找到的物件數（而不是命令列參數個數），且訊息含前綴；
// 取消時（回答 n）不送出任何 DELETE。
func TestCosRmRecursiveConfirmShowsRealCount(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutObject("b", "dir/a.txt", []byte("A"))
	fs3.PutObject("b", "dir/b.txt", []byte("B"))
	fs3.PutObject("b", "dir/c.txt", []byte("C"))

	code, _, stderr := runCLI(t, fs3.env(t), "n\n", "cos", "rm", "cos://b/dir", "--recursive")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "3 個物件")
	assert.Contains(t, stderr, "cos://b/dir/")

	for _, k := range []string{"dir/a.txt", "dir/b.txt", "dir/c.txt"} {
		_, ok := fs3.Object("b", k)
		assert.True(t, ok, "取消後 %s 應仍存在", k)
	}
	for _, r := range fs3.Requests() {
		assert.NotEqual(t, "DELETE", r.Method, "取消後不應送出 DELETE")
	}
}

// TestCosRmRecursiveEmptyPrefixSaysWholeBucket 驗證 prefix 為空（整個 bucket）時，
// 確認訊息改用「整個 bucket」字樣，而不是空字串前綴。
func TestCosRmRecursiveEmptyPrefixSaysWholeBucket(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutObject("b", "a.txt", []byte("A"))

	code, _, stderr := runCLI(t, fs3.env(t), "n\n", "cos", "rm", "cos://b/", "--recursive")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "整個 bucket")

	_, ok := fs3.Object("b", "a.txt")
	assert.True(t, ok, "取消後物件應仍存在")
}

// TestCosRmRecursiveNoMatch 驗證前綴沒有符合的物件時，直接印出提示並 exit 0，
// 不會印出確認提示（不會卡在等待輸入）。
func TestCosRmRecursiveNoMatch(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutBucket("b")

	code, _, stderr := runCLI(t, fs3.env(t), "", "cos", "rm", "cos://b/nothing/", "--recursive")
	assert.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stderr, "沒有符合")
	assert.NotContains(t, stderr, "確定嗎")
}

// TestCosRmRecursiveDryRun 驗證 --recursive --dry-run 仍會呼叫 ListObjects（read-only），
// 但不會真的刪除任何物件；兩段式刪除下 pass 1（計數）與 pass 2（列出並印出 dry-run 行）
// 各自呼叫一次 ListObjects，因此共 2 個 GET 請求，皆為唯讀。
func TestCosRmRecursiveDryRun(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutObject("b", "dir/a.txt", []byte("A"))

	code, stdout, stderr := runCLI(t, fs3.env(t), "", "--dry-run", "cos", "rm", "cos://b/dir/", "--recursive")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stdout, "dry-run: 刪除 cos://b/dir/a.txt")

	reqs := fs3.Requests()
	require.Len(t, reqs, 2, "兩段式刪除：pass 1 計數 + pass 2 列出，皆為唯讀 GET")
	for _, r := range reqs {
		assert.Equal(t, "GET", r.Method)
	}

	_, ok := fs3.Object("b", "dir/a.txt")
	assert.True(t, ok, "dry-run 不應真的刪除")
}

// TestCosLsVersionsGolden 驗證 `cos ls b --versions` 列出物件版本與 delete marker（golden，時間戳記正規化），
// -o json 帶 version_id / is_latest / delete_marker 欄位。
func TestCosLsVersionsGolden(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutBucket("b")
	fs3.SetVersioning("b", "Enabled")
	fs3.PutObject("b", "a.txt", []byte("v1"))
	fs3.PutObject("b", "a.txt", []byte("v2-longer"))
	fs3.PutObject("b", "gone.txt", []byte("x"))
	code, _, stderr := runCLI(t, fs3.env(t), "", "cos", "rm", "cos://b/gone.txt", "-y")
	require.Equal(t, ExitOK, code, stderr)

	code, stdout, stderr := runCLI(t, fs3.env(t), "", "cos", "ls", "b", "--versions")
	require.Equal(t, ExitOK, code, stderr)
	testutil.AssertGolden(t, "cos_ls_versions.txt", []byte(normalizeTimestamps(stdout)))

	code, stdout, stderr = runCLI(t, fs3.env(t), "", "cos", "ls", "b", "--versions", "-o", "json")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stdout, `"delete_marker": true`)
	assert.Contains(t, stdout, `"is_latest": true`)
	assert.Contains(t, stdout, `"version_id"`)
}

// TestCosLsWithoutVersionsHidesDeleted 驗證沒有 --versions 時，被 delete marker 蓋住的 key 不會出現。
func TestCosLsWithoutVersionsHidesDeleted(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutBucket("b")
	fs3.SetVersioning("b", "Enabled")
	fs3.PutObject("b", "gone.txt", []byte("x"))
	code, _, stderr := runCLI(t, fs3.env(t), "", "cos", "rm", "cos://b/gone.txt", "-y")
	require.Equal(t, ExitOK, code, stderr)

	code, stdout, stderr := runCLI(t, fs3.env(t), "", "cos", "ls", "b")
	require.Equal(t, ExitOK, code, stderr)
	assert.NotContains(t, stdout, "gone.txt")
}

// TestCosRmAllVersionsExactKey 驗證非遞迴的 --all-versions 只刪除該 key 的所有版本與 delete marker，
// 不會誤刪以該 key 為前綴的手足（a.txt.bak）；-o json 帶 version_id。
func TestCosRmAllVersionsExactKey(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutBucket("b")
	fs3.SetVersioning("b", "Enabled")
	fs3.PutObject("b", "a.txt", []byte("v1"))
	fs3.PutObject("b", "a.txt", []byte("v2"))
	fs3.PutObject("b", "a.txt.bak", []byte("bak"))

	code, stdout, stderr := runCLI(t, fs3.env(t), "", "cos", "rm", "cos://b/a.txt", "--all-versions", "-y", "-o", "json")
	require.Equal(t, ExitOK, code, stderr)
	assert.Empty(t, fs3.Versions("b", "a.txt"))
	assert.Len(t, fs3.Versions("b", "a.txt.bak"), 1)
	assert.Contains(t, stdout, `"version_id"`)
	assert.NotContains(t, stdout, "a.txt.bak")
}

// TestCosRmAllVersionsRecursiveIncludesDeleteMarkers 驗證 --recursive --all-versions 會把 delete marker 一併刪除，
// 確認訊息顯示版本數量；取消時不送出任何 DELETE。
func TestCosRmAllVersionsRecursiveIncludesDeleteMarkers(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutBucket("b")
	fs3.SetVersioning("b", "Enabled")
	fs3.PutObject("b", "dir/a", []byte("a"))
	fs3.PutObject("b", "dir/a", []byte("a2"))
	code, _, stderr := runCLI(t, fs3.env(t), "", "cos", "rm", "cos://b/dir/a", "-y")
	require.Equal(t, ExitOK, code, stderr)
	require.Len(t, fs3.Versions("b", "dir/a"), 3, "兩個版本 + 一個 delete marker")

	code, _, stderr = runCLI(t, fs3.env(t), "n\n", "cos", "rm", "cos://b/dir/", "--recursive", "--all-versions")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "3 個物件版本")
	assert.Len(t, fs3.Versions("b", "dir/a"), 3)

	code, _, stderr = runCLI(t, fs3.env(t), "", "cos", "rm", "cos://b/dir/", "--recursive", "--all-versions", "-y")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stderr, "delete marker")
	assert.Empty(t, fs3.Versions("b", "dir/a"))
}

// TestCosRmAllVersionsDryRunAndNoMatch 驗證 --dry-run 只 list 不刪除；沒有符合的版本時直接提示並 exit 0。
func TestCosRmAllVersionsDryRunAndNoMatch(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutObject("b", "k", []byte("x"))

	code, stdout, stderr := runCLI(t, fs3.env(t), "", "--dry-run", "cos", "rm", "cos://b/k", "--all-versions")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stdout, "dry-run: 刪除 cos://b/k（版本 null）")
	_, ok := fs3.Object("b", "k")
	assert.True(t, ok)

	code, _, stderr = runCLI(t, fs3.env(t), "", "cos", "rm", "cos://b/nothing", "--all-versions")
	assert.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stderr, "沒有符合")
}

// TestCosRmRecursiveTwoPassListsTwiceAndStreams 驗證 --recursive 兩段式刪除：
// pass 1（cosRmRecursive 的計數迴圈）只計數、不蒐集任何 key，MaxKeys=2、5 個物件因此
// 分 3 頁 GET；確認後 pass 2 重新呼叫 ListObjects，每一頁物件一到就立即 DeleteObject，
// 因此 DELETE 會交錯在 pass 2 的 list 分頁之間（GET → 該頁的 DELETE → 下一頁 GET → …），
// 而不是等 pass 2 全部列完才開始刪——以 Requests() 的完整順序斷言。
func TestCosRmRecursiveTwoPassListsTwiceAndStreams(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.MaxKeys = 2
	keys := []string{"dir/1", "dir/2", "dir/3", "dir/4", "dir/5"}
	for _, k := range keys {
		fs3.PutObject("b", k, []byte(k))
	}

	code, _, stderr := runCLI(t, fs3.env(t), "", "cos", "rm", "cos://b/dir/", "--recursive", "-y")
	require.Equal(t, ExitOK, code, stderr)

	for _, k := range keys {
		_, ok := fs3.Object("b", k)
		assert.False(t, ok, "%s 應已刪除", k)
	}

	var methods []string
	for _, r := range fs3.Requests() {
		methods = append(methods, r.Method)
	}
	assert.Equal(t, []string{
		"GET", "GET", "GET", // pass 1：只計數（5 個物件、MaxKeys=2 → 3 頁）
		"GET", "DELETE", "DELETE", // pass 2 第 1 頁（2 筆）：list 後立即刪除，交錯進行
		"GET", "DELETE", "DELETE", // pass 2 第 2 頁（2 筆）
		"GET", "DELETE", // pass 2 第 3 頁（1 筆）
	}, methods, "GET list 請求數應為 2 段 × 3 頁，DELETE 應交錯於 pass 2 的 list 之間")
}

// TestCosRmAllVersionsTwoPassListsTwiceAndStreams 鏡射
// TestCosRmRecursiveTwoPassListsTwiceAndStreams，驗證 --all-versions 走
// ListObjectVersions/DeleteObjectVersion 時同樣是兩段式：pass 1 只計數，
// pass 2 重新列出並邊列邊刪、與 list 分頁交錯進行。
//
// 與 cosRmRecursive（V1 物件、Marker 是純字串）不同，版本路徑用的是 lookbehind：
// 每一頁最後一筆版本要延後到「下一頁的 list 請求已經送出成功」之後才真正刪除，
// 因為它可能被拿去當下一頁的 version-id-marker（真實 S3／Ceph 對已刪除的
// version-id-marker 回 400，見 internal/testutil/fakes3.go 的嚴格檢查與
// internal/cli/cos_object.go 的 cosStreamDeleteVersionTargets 說明）。因此 DELETE
// 序列比「邊列邊刪」直觀預期的晚一拍，最後一筆也要在整個 ListObjectVersions 呼叫
// 結束後才補刪。
func TestCosRmAllVersionsTwoPassListsTwiceAndStreams(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.MaxKeys = 2
	fs3.PutBucket("b")
	fs3.SetVersioning("b", "Enabled")
	keys := []string{"dir/1", "dir/2", "dir/3", "dir/4", "dir/5"}
	for _, k := range keys {
		fs3.PutObject("b", k, []byte(k))
	}

	code, _, stderr := runCLI(t, fs3.env(t), "", "cos", "rm", "cos://b/dir/", "--recursive", "--all-versions", "-y")
	require.Equal(t, ExitOK, code, stderr)

	for _, k := range keys {
		assert.Empty(t, fs3.Versions("b", k), "%s 的所有版本應已刪除", k)
	}

	var methods []string
	for _, r := range fs3.Requests() {
		methods = append(methods, r.Method)
	}
	assert.Equal(t, []string{
		"GET", "GET", "GET", // pass 1：只計數（5 個版本、MaxKeys=2 → 3 頁）
		"GET", "DELETE", // pass 2 第 1 頁（2 筆）：第 1 筆延後（lookbehind），第 2 筆的
		// callback 觸發刪除第 1 筆
		"GET", "DELETE", "DELETE", // pass 2 第 2 頁（2 筆）：先補刪上一頁最後一筆，
		// 再延後本頁第 1 筆、刪本頁第 1 筆（即上一輪的 pending）
		"GET", "DELETE", "DELETE", // pass 2 第 3 頁（1 筆）：補刪上一頁最後一筆，
		// 迴圈結束後再補刪最後一筆 pending
	}, methods)
}

// TestCosRmRecursiveCountMismatchStillDeletesAndHints 驗證兩段式刪除的「期間有變動」
// 分支：用 FakeS3.BeforeRequest 在 pass 1（第 1 個 GET，用來算確認訊息的計數）完成、
// pass 2（第 2 個 GET）開始之前插入一個新物件，模擬「bucket 內容在兩段之間被改動」。
// 預期：exit code 仍為 0，pass 2 連新增的物件也一併刪除（不會漏掉，也不會報錯），
// 且 stderr 印出「實際刪除數與確認時不同」的提示。
func TestCosRmRecursiveCountMismatchStillDeletesAndHints(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutObject("b", "dir/a.txt", []byte("A"))

	var seenGET int32
	fs3.BeforeRequest = func(r *http.Request) {
		if r.Method != http.MethodGet {
			return
		}
		if atomic.AddInt32(&seenGET, 1) == 2 {
			// 第 2 個 GET 是 pass 2 的第一次 list；在它被處理之前插入新物件，
			// 讓 pass 1 沒看到、但 pass 2 會看到並一併刪除。
			fs3.PutObject("b", "dir/b.txt", []byte("B"))
		}
	}

	code, _, stderr := runCLI(t, fs3.env(t), "", "cos", "rm", "cos://b/dir/", "--recursive", "-y")
	require.Equal(t, ExitOK, code, stderr)

	_, ok := fs3.Object("b", "dir/a.txt")
	assert.False(t, ok, "pass 1 就看到的物件應已刪除")
	_, ok = fs3.Object("b", "dir/b.txt")
	assert.False(t, ok, "pass 2 期間新增的物件也應一併被刪除")

	assert.Contains(t, stderr, "實際刪除 2 個物件")
	assert.Contains(t, stderr, "確認當下為 1 個")
}

// TestCosRmRecursiveDryRunCountMismatchHintsProcessedNotDeleted 是 code review Minor 5
// 的迴歸測試：--dry-run 底下沒有真的刪除任何東西，「期間有變動」提示不可說「實際刪除」，
// 否則會誤導使用者以為 dry-run 也真的刪了東西；改成「實際處理」。情境與上面的
// TestCosRmRecursiveCountMismatchStillDeletesAndHints 相同，差別只在多加 --dry-run。
func TestCosRmRecursiveDryRunCountMismatchHintsProcessedNotDeleted(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutObject("b", "dir/a.txt", []byte("A"))

	var seenGET int32
	fs3.BeforeRequest = func(r *http.Request) {
		if r.Method != http.MethodGet {
			return
		}
		if atomic.AddInt32(&seenGET, 1) == 2 {
			fs3.PutObject("b", "dir/b.txt", []byte("B"))
		}
	}

	code, _, stderr := runCLI(t, fs3.env(t), "", "cos", "rm", "cos://b/dir/", "--recursive", "-y", "--dry-run")
	require.Equal(t, ExitOK, code, stderr)

	_, ok := fs3.Object("b", "dir/a.txt")
	assert.True(t, ok, "--dry-run 不應真的刪除任何物件")
	_, ok = fs3.Object("b", "dir/b.txt")
	assert.True(t, ok, "--dry-run 不應真的刪除任何物件")

	assert.Contains(t, stderr, "實際處理 2 個物件")
	assert.Contains(t, stderr, "確認當下為 1 個")
	assert.NotContains(t, stderr, "實際刪除", "dry-run 不可宣稱「實際刪除」")
}
