package cli

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilbertchiao/twaictl/internal/config"
	"github.com/gilbertchiao/twaictl/internal/testutil"
)

// TestCosBucketLsGolden 驗證 `cos bucket ls` 依名稱排序列出所有 bucket，
// 欄位為 NAME、CREATED；CREATED 來自假 S3 的 time.Now()，無法固定，
// 比對前先用 normalizeTimestamps 取代成固定字串。
func TestCosBucketLsGolden(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutBucket("zeta")
	fs3.PutBucket("alpha")

	code, stdout, stderr := runCLI(t, fs3.env(t), "", "cos", "bucket", "ls")
	require.Equal(t, ExitOK, code, stderr)
	testutil.AssertGolden(t, "cos_bucket_ls.txt", []byte(normalizeTimestamps(stdout)))
}

// TestCosBucketLsJSON 驗證 -o json 輸出 [{"name","created"}] 形狀（snake_case 欄位名稱）。
func TestCosBucketLsJSON(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutBucket("b")

	code, stdout, stderr := runCLI(t, fs3.env(t), "", "cos", "bucket", "ls", "-o", "json")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stdout, `"name": "b"`)
	assert.Contains(t, stdout, `"created"`)
}

// TestCosBucketCreateWithVersioning 驗證 create 會先 PUT bucket，再（因為 --versioning on）
// PUT ?versioning 設為 Enabled。
func TestCosBucketCreateWithVersioning(t *testing.T) {
	fs3 := newFakeS3(t)

	code, _, stderr := runCLI(t, fs3.env(t), "", "cos", "bucket", "create", "b", "--versioning", "on")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stderr, "已建立 bucket b")
	assert.Contains(t, fs3.Buckets(), "b")
	assert.Equal(t, "Enabled", fs3.Versioning("b"))
}

// TestCosBucketCreateInvalidVersioning 驗證 --versioning 只接受 on/off，不合法值不送出請求。
func TestCosBucketCreateInvalidVersioning(t *testing.T) {
	fs3 := newFakeS3(t)
	code, _, stderr := runCLI(t, fs3.env(t), "", "cos", "bucket", "create", "b", "--versioning", "maybe")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Empty(t, fs3.Requests())
}

// TestCosBucketRmConfirmAndNotEmpty 驗證：取消時不送出 DELETE；
// 非空 bucket 刪除失敗（409 BucketNotEmpty）時 exit code 為 3 且 stderr 含 BucketNotEmpty。
func TestCosBucketRmConfirmAndNotEmpty(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutBucket("empty")
	fs3.PutObject("full", "k", []byte("x"))

	code, _, stderr := runCLI(t, fs3.env(t), "n\n", "cos", "bucket", "rm", "empty")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, fs3.Buckets(), "empty", "取消後 bucket 應仍存在")

	code, _, stderr = runCLI(t, fs3.env(t), "", "cos", "bucket", "rm", "full", "-y")
	assert.Equal(t, ExitAPI, code, stderr)
	assert.Contains(t, stderr, "BucketNotEmpty")

	code, _, stderr = runCLI(t, fs3.env(t), "", "cos", "bucket", "rm", "empty", "-y")
	require.Equal(t, ExitOK, code, stderr)
	assert.NotContains(t, fs3.Buckets(), "empty")
}

// TestCosBucketLsWithoutCredentialsIsExitConfig 驗證缺少 COS S3 access/secret key 時
// exit code 為 2，stderr 提示 `cos key get --save`。
func TestCosBucketLsWithoutCredentialsIsExitConfig(t *testing.T) {
	fs3 := newFakeS3(t)
	env := map[string]string{"XDG_CONFIG_HOME": t.TempDir()}
	_ = fs3 // 假 S3 server 本身不需要被打到；只是為了讓測試意圖清楚（沒有金鑰不會送任何請求）

	code, _, stderr := runCLI(t, env, "", "cos", "bucket", "ls")
	assert.Equal(t, ExitConfig, code, stderr)
	assert.Contains(t, stderr, "cos key get --save")
}

// TestCosBucketCreateDryRun 驗證 --dry-run 印出 dry-run 訊息、exit 0，且假 S3 完全沒收到請求。
func TestCosBucketCreateDryRun(t *testing.T) {
	fs3 := newFakeS3(t)
	code, stdout, stderr := runCLI(t, fs3.env(t), "", "--dry-run", "cos", "bucket", "create", "b")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stdout, "dry-run: 建立 bucket b")
	assert.Empty(t, fs3.Requests())
}

// TestCosBucketRmDryRun 驗證 --dry-run 對多個 bucket 各印一行，且不送出任何請求
// （也不會被 confirm 卡住等待輸入）。
func TestCosBucketRmDryRun(t *testing.T) {
	fs3 := newFakeS3(t)
	code, stdout, stderr := runCLI(t, fs3.env(t), "", "--dry-run", "cos", "bucket", "rm", "a", "b")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stdout, "dry-run: 刪除 bucket a")
	assert.Contains(t, stdout, "dry-run: 刪除 bucket b")
	assert.Empty(t, fs3.Requests())
}

// TestCosBucketCreateVersioningFailureMessage 驗證 `create --versioning` 的第二步
// （PutBucketVersioning）失敗時，錯誤訊息會說明 bucket 其實已經建立成功，
// 只是版本控制設定失敗；避免使用者誤以為整個 create 都沒生效而重複建立。
func TestCosBucketCreateVersioningFailureMessage(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.FailVersioningError = "InternalError"

	code, _, stderr := runCLI(t, fs3.env(t), "", "cos", "bucket", "create", "b", "--versioning", "on")
	assert.Equal(t, ExitAPI, code, stderr)
	assert.Contains(t, stderr, "已建立")
	assert.Contains(t, stderr, "設定版本控制失敗")
	assert.Contains(t, fs3.Buckets(), "b", "bucket 應該已經建立成功")
}

// TestCosBucketCreateDryRunVersioning 驗證 --dry-run 搭配 --versioning 時，
// 除了「dry-run: 建立 bucket」還會多印一行「dry-run: 設定 bucket ... 版本控制為 ...」，
// 讓 --dry-run 的輸出完整反映會發生的兩個步驟。
func TestCosBucketCreateDryRunVersioning(t *testing.T) {
	fs3 := newFakeS3(t)
	code, stdout, stderr := runCLI(t, fs3.env(t), "", "--dry-run", "cos", "bucket", "create", "b", "--versioning", "on")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stdout, "dry-run: 建立 bucket b")
	assert.Contains(t, stdout, "dry-run: 設定 bucket b 版本控制為 on")
	assert.Empty(t, fs3.Requests())
}

// TestCosBucketLsNetworkErrorIsExit4 驗證端點連線被拒絕（請求根本沒送出）時，
// exit code 為 4（網路錯誤），且 stderr 不會出現誤判成 API 錯誤時的
// 「API 回應 0」字樣（DESIGN 3.5：網路錯誤與 API 錯誤要用不同 exit code）。
func TestCosBucketLsNetworkErrorIsExit4(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	endpoint := srv.URL
	srv.Close()

	env := map[string]string{
		"XDG_CONFIG_HOME":      t.TempDir(),
		config.EnvCOSEndpoint:  endpoint,
		config.EnvCOSAccessKey: "AKIATEST",
		config.EnvCOSSecretKey: "SECRETTEST",
	}
	code, _, stderr := runCLI(t, env, "", "cos", "bucket", "ls")
	assert.Equal(t, ExitNetwork, code, stderr)
	assert.NotContains(t, stderr, "API 回應 0")
}

// TestCosBucketRmPurgeVersionedBucket 驗證 --purge 會刪除所有物件版本與 delete marker 後刪除 bucket；
// 確認訊息含版本數；-y 略過。
func TestCosBucketRmPurgeVersionedBucket(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutBucket("b")
	fs3.SetVersioning("b", "Enabled")
	fs3.PutObject("b", "k", []byte("1"))
	fs3.PutObject("b", "k", []byte("2"))
	code, _, stderr := runCLI(t, fs3.env(t), "", "cos", "rm", "cos://b/k", "-y")
	require.Equal(t, ExitOK, code, stderr)

	code, _, stderr = runCLI(t, fs3.env(t), "n\n", "cos", "bucket", "rm", "b", "--purge")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Contains(t, stderr, "b（3 個物件版本）")
	assert.Contains(t, fs3.Buckets(), "b")

	code, _, stderr = runCLI(t, fs3.env(t), "", "cos", "bucket", "rm", "b", "--purge", "-y")
	require.Equal(t, ExitOK, code, stderr)
	assert.NotContains(t, fs3.Buckets(), "b")
	assert.Contains(t, stderr, "已刪除 bucket b")
}

// TestCosBucketRmPurgeUnversionedAndDryRun 驗證未開 versioning 的 bucket 也能 --purge；
// --dry-run 列出每個版本與 bucket 但不刪除。
func TestCosBucketRmPurgeUnversionedAndDryRun(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutObject("b", "k", []byte("1"))

	code, stdout, stderr := runCLI(t, fs3.env(t), "", "--dry-run", "cos", "bucket", "rm", "b", "--purge")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stdout, "dry-run: 刪除 cos://b/k（版本 null）")
	assert.Contains(t, stdout, "dry-run: 刪除 bucket b")
	assert.Contains(t, fs3.Buckets(), "b")

	code, _, stderr = runCLI(t, fs3.env(t), "", "cos", "bucket", "rm", "b", "--purge", "-y")
	require.Equal(t, ExitOK, code, stderr)
	assert.NotContains(t, fs3.Buckets(), "b")
}

// TestCosBucketRmNotEmptyHintsPurge 驗證不帶 --purge 遇到 BucketNotEmpty 時，錯誤訊息提示 --purge，且 exit code 仍為 3。
func TestCosBucketRmNotEmptyHintsPurge(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.PutObject("b", "k", []byte("1"))

	code, _, stderr := runCLI(t, fs3.env(t), "", "cos", "bucket", "rm", "b", "-y")
	assert.Equal(t, ExitAPI, code, stderr)
	assert.Contains(t, stderr, "BucketNotEmpty")
	assert.Contains(t, stderr, "--purge")
}

// TestCosBucketRmPurgeTwoPass 驗證 --purge 兩段式清空：pass 1（cosCountVersionTargets）
// 只計數該 bucket 的物件版本數，MaxKeys=2、5 個物件因此分 3 頁 GET；確認後 pass 2
// （cosStreamDeleteVersionTargets）重新列出並邊列邊刪，與 list 分頁交錯進行；最後才
// 刪除 bucket 本身——以 Requests() 的完整順序斷言。
//
// pass 2 的 DELETE 用「延後一筆」的 lookbehind（每一頁最後一筆版本要等下一頁的 list
// 請求已經送出成功才真正刪除，因為它可能被拿去當下一頁的 version-id-marker；已刪除
// 的 version-id-marker 真實 S3／Ceph 會回 400，見 internal/testutil/fakes3.go 的嚴格檢查
// 與 internal/cli/cos_object.go 的 cosStreamDeleteVersionTargets 說明），因此 DELETE
// 序列比「邊列邊刪」直觀預期的晚一拍。
func TestCosBucketRmPurgeTwoPass(t *testing.T) {
	fs3 := newFakeS3(t)
	fs3.MaxKeys = 2
	for _, k := range []string{"1", "2", "3", "4", "5"} {
		fs3.PutObject("b", k, []byte(k))
	}

	code, _, stderr := runCLI(t, fs3.env(t), "", "cos", "bucket", "rm", "b", "--purge", "-y")
	require.Equal(t, ExitOK, code, stderr)
	assert.NotContains(t, fs3.Buckets(), "b")

	var methods []string
	for _, r := range fs3.Requests() {
		methods = append(methods, r.Method)
	}
	assert.Equal(t, []string{
		"GET", "GET", "GET", // pass 1：只計數（5 個版本、MaxKeys=2 → 3 頁）
		"GET", "DELETE", // pass 2 第 1 頁（2 筆）：第 2 筆的 callback 觸發刪除第 1 筆（lookbehind）
		"GET", "DELETE", "DELETE", // pass 2 第 2 頁（2 筆）：先補刪上一頁最後一筆，再刪本頁第 1 筆
		"GET", "DELETE", "DELETE", // pass 2 第 3 頁（1 筆）：先補刪上一頁最後一筆，
		// 迴圈結束後再補刪本頁唯一一筆（最後一筆 pending）
		"DELETE", // 最後刪除 bucket 本身
	}, methods, "GET list 請求數應為 2 段 × 3 頁，DELETE 應交錯於 pass 2 的 list 之間（lookbehind 下延後一拍）")
}
