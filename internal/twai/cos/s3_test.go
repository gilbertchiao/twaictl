package cos

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilbertchiao/twaictl/internal/testutil"
	"github.com/gilbertchiao/twaictl/internal/twai"
)

func newTestS3(t *testing.T, fake *testutil.FakeS3) *S3 {
	t.Helper()
	c, err := NewS3(S3Options{
		Endpoint: fake.URL(), AccessKey: "AKIATEST", SecretKey: "SECRETTEST",
		Timeout: 10 * time.Second,
	})
	require.NoError(t, err)
	return c
}

func TestNewS3RejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name string
		opts S3Options
	}{
		{"空 access key", S3Options{Endpoint: "http://x", SecretKey: "sk"}},
		{"空 secret key", S3Options{Endpoint: "http://x", AccessKey: "ak"}},
		{"非法 endpoint（無 scheme）", S3Options{Endpoint: "x.example.com", AccessKey: "ak", SecretKey: "sk"}},
		{"非法 endpoint（空字串）", S3Options{AccessKey: "ak", SecretKey: "sk"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := NewS3(c.opts)
			assert.Error(t, err)
		})
	}
}

func TestNewS3RejectsSmallPartSize(t *testing.T) {
	_, err := NewS3(S3Options{
		Endpoint: "http://x", AccessKey: "ak", SecretKey: "sk",
		PartSize: 1024 * 1024, // 1 MiB，小於 S3/Ceph RGW 要求的 5 MiB 下限
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PartSize 至少需為 5 MiB")
	assert.Contains(t, err.Error(), "1048576 bytes")
}

func TestNewS3AllowsZeroOrMinimumPartSize(t *testing.T) {
	_, err := NewS3(S3Options{Endpoint: "http://x", AccessKey: "ak", SecretKey: "sk"})
	assert.NoError(t, err, "PartSize 為 0（未指定）時使用預設值，不應報錯")

	_, err = NewS3(S3Options{Endpoint: "http://x", AccessKey: "ak", SecretKey: "sk", PartSize: minPartSize})
	assert.NoError(t, err, "PartSize 恰好等於下限應允許")
}

func TestListBucketsSorted(t *testing.T) {
	fake := testutil.NewFakeS3(t)
	fake.PutBucket("zeta")
	fake.PutBucket("alpha")
	c := newTestS3(t, fake)

	buckets, err := c.ListBuckets(context.Background())
	require.NoError(t, err)
	require.Len(t, buckets, 2)
	assert.Equal(t, "alpha", buckets[0].Name)
	assert.Equal(t, "zeta", buckets[1].Name)

	req := fake.Requests()[0]
	assert.True(t, strings.HasPrefix(req.Header.Get("Authorization"), "AWS4-HMAC-SHA256"), "請求須帶 SigV4 簽章")
}

func TestCreateBucketTwiceReturnsAPIError(t *testing.T) {
	fake := testutil.NewFakeS3(t)
	c := newTestS3(t, fake)

	require.NoError(t, c.CreateBucket(context.Background(), "b"))
	err := c.CreateBucket(context.Background(), "b")

	var apiErr *twai.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusConflict, apiErr.StatusCode)
	assert.Contains(t, apiErr.Message, "BucketAlreadyOwnedByYou")
	assert.Equal(t, "b", apiErr.URL, "URL 只放 bucket，不含 endpoint")
}

func TestDeleteBucketNotEmptyReturnsAPIError(t *testing.T) {
	fake := testutil.NewFakeS3(t)
	fake.PutObject("b", "k", []byte("x"))
	c := newTestS3(t, fake)

	err := c.DeleteBucket(context.Background(), "b")
	var apiErr *twai.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusConflict, apiErr.StatusCode)
	assert.Contains(t, apiErr.Message, "BucketNotEmpty")
}

func TestSetVersioningEnabledAndSuspended(t *testing.T) {
	fake := testutil.NewFakeS3(t)
	fake.PutBucket("b")
	c := newTestS3(t, fake)

	require.NoError(t, c.SetVersioning(context.Background(), "b", true))
	assert.Equal(t, "Enabled", fake.Versioning("b"))

	require.NoError(t, c.SetVersioning(context.Background(), "b", false))
	assert.Equal(t, "Suspended", fake.Versioning("b"))
}

func TestListObjectsPaginatesAndFiltersPrefix(t *testing.T) {
	fake := testutil.NewFakeS3(t)
	fake.MaxKeys = 2
	fake.PutObject("b", "a.txt", []byte("1"))
	fake.PutObject("b", "b.txt", []byte("22"))
	fake.PutObject("b", "logs/c.txt", []byte("333"))
	c := newTestS3(t, fake)

	var keys []string
	err := c.ListObjects(context.Background(), "b", "", func(o Object) error {
		keys = append(keys, o.Key)
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"a.txt", "b.txt", "logs/c.txt"}, keys, "依 key 排序，順序正確")

	// 3 個物件、max-keys 2 → 兩次 GET 請求（V1 分頁，不帶 list-type=2）。
	listRequests := 0
	for _, r := range fake.Requests() {
		if r.Method == "GET" && r.Path == "/b" {
			assert.NotContains(t, r.Query, "list-type=2", "應使用 V1 ListObjects，而非 ListObjectsV2")
			listRequests++
		}
	}
	assert.Equal(t, 2, listRequests, "應觸發兩次分頁請求")

	var prefixed []string
	err = c.ListObjects(context.Background(), "b", "logs/", func(o Object) error {
		prefixed = append(prefixed, o.Key)
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"logs/c.txt"}, prefixed)
}

// TestListObjectsPaginatesWithMarker 驗證 ListObjects 以 V1 Marker 翻頁：FakeS3 每頁 2 筆、共 5 筆，
// 不帶 NextMarker（AWS 行為）與帶 NextMarker（Ceph 行為）兩種模式都要列出全部 5 筆且順序正確。
func TestListObjectsPaginatesWithMarker(t *testing.T) {
	for _, emit := range []bool{false, true} {
		t.Run(fmt.Sprintf("EmitNextMarker=%v", emit), func(t *testing.T) {
			fake := testutil.NewFakeS3(t)
			fake.MaxKeys = 2
			fake.EmitNextMarker = emit
			for _, k := range []string{"a", "b", "c", "d", "e"} {
				fake.PutObject("b", k, []byte(k))
			}
			c := newTestS3(t, fake)
			var keys []string
			require.NoError(t, c.ListObjects(context.Background(), "b", "", func(o Object) error {
				keys = append(keys, o.Key)
				return nil
			}))
			assert.Equal(t, []string{"a", "b", "c", "d", "e"}, keys)

			pages := 0
			for _, r := range fake.Requests() {
				if r.Method == "GET" && r.Path == "/b" {
					pages++
					assert.NotContains(t, r.Query, "list-type=2", "應使用 V1 ListObjects")
				}
			}
			assert.Equal(t, 3, pages)
		})
	}
}

// TestListObjectsPrefixWithMarker 驗證 prefix 與分頁併用時只列出前綴下的物件。
func TestListObjectsPrefixWithMarker(t *testing.T) {
	fake := testutil.NewFakeS3(t)
	fake.MaxKeys = 1
	fake.PutObject("b", "dir/1", []byte("1"))
	fake.PutObject("b", "dir/2", []byte("2"))
	fake.PutObject("b", "other", []byte("o"))
	c := newTestS3(t, fake)
	var keys []string
	require.NoError(t, c.ListObjects(context.Background(), "b", "dir/", func(o Object) error {
		keys = append(keys, o.Key)
		return nil
	}))
	assert.Equal(t, []string{"dir/1", "dir/2"}, keys)
}

// TestListObjectsStuckMarker 驗證 FakeS3.StuckMarker 模擬「分頁 marker 永遠沒有進展」的異常伺服器
// （每次都回同一頁且 IsTruncated 恆為 true）時，ListObjects 會偵測到並回傳「沒有進展」錯誤，
// 而不是無限迴圈送出請求。
func TestListObjectsStuckMarker(t *testing.T) {
	fake := testutil.NewFakeS3(t)
	fake.MaxKeys = 2
	fake.StuckMarker = true
	for _, k := range []string{"a", "b", "c"} {
		fake.PutObject("b", k, []byte(k))
	}
	c := newTestS3(t, fake)
	err := c.ListObjects(context.Background(), "b", "", func(Object) error { return nil })
	require.Error(t, err)
	assert.Contains(t, err.Error(), "沒有進展")
}

func TestUploadSmallFile(t *testing.T) {
	fake := testutil.NewFakeS3(t)
	fake.PutBucket("b")
	c := newTestS3(t, fake)

	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	content := []byte("hello world")
	require.NoError(t, os.WriteFile(path, content, 0o644))

	require.NoError(t, c.Upload(context.Background(), "b", "a.txt", path))

	got, ok := fake.Object("b", "a.txt")
	require.True(t, ok)
	assert.Equal(t, content, got)

	var put *testutil.Recorded
	for i, r := range fake.Requests() {
		if r.Method == "PUT" && r.Path == "/b/a.txt" {
			put = &fake.Requests()[i]
		}
	}
	require.NotNil(t, put, "應有一次 PUT /b/a.txt")
	assert.True(t, strings.HasPrefix(put.Header.Get("Authorization"), "AWS4-HMAC-SHA256"), "請求須帶 SigV4 簽章")
	assert.Empty(t, put.Header.Get("x-amz-checksum-crc32"), "transfermanager 的 RequestChecksumCalculation 應與 s3.Client 一致設為 WhenRequired，不應自動加上 CRC32 checksum")
	assert.Empty(t, put.Header.Get("x-amz-trailer"), "WhenRequired 不應加上 checksum trailer header")
}

func TestUploadLargeFileTriggersMultipart(t *testing.T) {
	fake := testutil.NewFakeS3(t)
	fake.PutBucket("b")
	c, err := NewS3(S3Options{
		Endpoint: fake.URL(), AccessKey: "AKIATEST", SecretKey: "SECRETTEST",
		Timeout: 30 * time.Second, PartSize: 5 * 1024 * 1024, // 5 MiB（S3 允許的最小分片大小）
	})
	require.NoError(t, err)

	dir := t.TempDir()
	path := filepath.Join(dir, "big.bin")
	content := bytes.Repeat([]byte("x"), 6*1024*1024) // 6 MiB，超過 5 MiB PartSize
	require.NoError(t, os.WriteFile(path, content, 0o644))

	require.NoError(t, c.Upload(context.Background(), "b", "big.bin", path))

	got, ok := fake.Object("b", "big.bin")
	require.True(t, ok)
	assert.Equal(t, content, got)

	sawInitiate, sawUploadPart, sawComplete := false, false, false
	for _, r := range fake.Requests() {
		switch {
		case r.Method == "POST" && strings.Contains(r.Query, "uploads"):
			sawInitiate = true
		case r.Method == "PUT" && strings.Contains(r.Query, "partNumber"):
			sawUploadPart = true
		case r.Method == "POST" && strings.Contains(r.Query, "uploadId"):
			sawComplete = true
		}
	}
	assert.True(t, sawInitiate, "應呼叫 CreateMultipartUpload")
	assert.True(t, sawUploadPart, "應呼叫 UploadPart")
	assert.True(t, sawComplete, "應呼叫 CompleteMultipartUpload")
}

func TestDownloadToNewSubdirectory(t *testing.T) {
	fake := testutil.NewFakeS3(t)
	content := []byte("download me")
	fake.PutObject("b", "dir/file.txt", content)
	c := newTestS3(t, fake)

	dir := t.TempDir()
	dest := filepath.Join(dir, "sub", "not-yet-existing", "file.txt")

	require.NoError(t, c.Download(context.Background(), "b", "dir/file.txt", dest))

	got, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, content, got)

	_, statErr := os.Stat(dest + tempSuffix)
	assert.True(t, os.IsNotExist(statErr), "暫存檔不應殘留")

	var get *testutil.Recorded
	for i, r := range fake.Requests() {
		if r.Method == "GET" && r.Path == "/b/dir/file.txt" {
			get = &fake.Requests()[i]
		}
	}
	require.NotNil(t, get, "應有一次 GET /b/dir/file.txt（path-style，bucket 在路徑裡）")
	assert.True(t, strings.HasPrefix(get.Header.Get("Authorization"), "AWS4-HMAC-SHA256"), "請求須帶 SigV4 簽章")
}

func TestDownloadNotFoundReturnsAPIError(t *testing.T) {
	fake := testutil.NewFakeS3(t)
	fake.PutBucket("b")
	c := newTestS3(t, fake)

	dir := t.TempDir()
	dest := filepath.Join(dir, "file.txt")

	err := c.Download(context.Background(), "b", "missing.txt", dest)
	var apiErr *twai.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusNotFound, apiErr.StatusCode)
	assert.Contains(t, apiErr.Message, "NoSuchKey")

	_, statErr := os.Stat(dest)
	assert.True(t, os.IsNotExist(statErr), "失敗時不應留下目的檔")
	_, statErr = os.Stat(dest + tempSuffix)
	assert.True(t, os.IsNotExist(statErr), "失敗時不應留下暫存檔")
}

// TestDownloadRejectsExistingTempFileSymlink 驗證暫存檔路徑（localPath+tempSuffix）
// 若已經是指向外部檔案的 symlink，Download 會直接失敗（O_EXCL），不會跟隨該 symlink
// 寫入／截斷它指向的外部檔案。
func TestDownloadRejectsExistingTempFileSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink 測試在 Windows 需要額外權限，略過")
	}
	fake := testutil.NewFakeS3(t)
	fake.PutObject("b", "k", []byte("new content"))
	c := newTestS3(t, fake)

	outside := t.TempDir()
	outsideTarget := filepath.Join(outside, "target")
	require.NoError(t, os.WriteFile(outsideTarget, []byte("original"), 0o644))

	dir := t.TempDir()
	dest := filepath.Join(dir, "file.txt")
	require.NoError(t, os.Symlink(outsideTarget, dest+tempSuffix))

	err := c.Download(context.Background(), "b", "k", dest)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "已存在")

	got, readErr := os.ReadFile(outsideTarget)
	require.NoError(t, readErr)
	assert.Equal(t, "original", string(got), "不應該透過既存的 symlink 截斷／覆寫外部檔案")
}

func TestDeleteObject(t *testing.T) {
	fake := testutil.NewFakeS3(t)
	fake.PutObject("b", "k", []byte("x"))
	c := newTestS3(t, fake)

	require.NoError(t, c.DeleteObject(context.Background(), "b", "k"))

	_, ok := fake.Object("b", "k")
	assert.False(t, ok)
}

func TestClassifyS3ErrorNil(t *testing.T) {
	assert.NoError(t, ClassifyS3Error(nil))
}

func TestClassifyS3ErrorWrapsGenericError(t *testing.T) {
	err := ClassifyS3Error(errors.New("boom"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "S3 操作失敗")
	assert.Contains(t, err.Error(), "boom")
}

// TestClassifyS3ErrorNetworkPassthrough 驗證請求根本沒送出（連線被拒絕，
// *awshttp.ResponseError 的 HTTPStatusCode() 為 0）時，ClassifyS3Error 判斷
// 為網路錯誤（twai.IsNetworkError）原樣回傳，而不是誤判成 *twai.APIError
// （會讓 cli 對應到錯誤的 exit code 3，而非 DESIGN 3.5 要求的 exit code 4）。
func TestClassifyS3ErrorNetworkPassthrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	endpoint := srv.URL
	srv.Close() // 關閉後端口不再有人監聽，之後的請求會是連線被拒絕（網路錯誤）

	c, err := NewS3(S3Options{Endpoint: endpoint, AccessKey: "ak", SecretKey: "sk", Timeout: 2 * time.Second})
	require.NoError(t, err)

	_, err = c.ListBuckets(context.Background())
	require.Error(t, err)

	var apiErr *twai.APIError
	assert.False(t, errors.As(err, &apiErr), "連線被拒絕不應被分類成 *twai.APIError")
	assert.True(t, twai.IsNetworkError(err), "連線被拒絕應被判斷為網路錯誤（exit code 4）")
}

// TestS3ClientDoesNotFollow302 驗證 S3 client 對 302 不跟隨（aws-sdk-go-v2 的 limitedRedirect
// 只跟隨 307/308）：伺服器只應收到一次請求，且呼叫以錯誤結束而非打到 Location。
//
// 用相對路徑 "/redirected/" 當 Location（srv 建立後才知道完整 URL，相對路徑即可指回同一台
// 假伺服器）：handler 對 "/redirected/" 回 200，其餘路徑一律回 302 並指向它；若 SDK 真的
// 跟隨了重導向，hits 會變成 2 且第二次請求的路徑會是 "/redirected/"，據此可與「SDK 正確
// 不跟隨」的情況（hits 恆為 1）明確區分。
func TestS3ClientDoesNotFollow302(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path == "/redirected/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Location", "/redirected/")
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()
	s3c, err := NewS3(S3Options{Endpoint: srv.URL, AccessKey: "ak", SecretKey: "sk", Timeout: 5 * time.Second})
	require.NoError(t, err)
	_, err = s3c.Client.ListBuckets(context.Background(), &s3.ListBucketsInput{})
	require.Error(t, err)
	assert.Equal(t, int32(1), hits.Load(), "不得跟隨重導向再打第二次")
}

// TestNewS3SetsRetryMaxAttempts 驗證 NewS3 明確設定 RetryMaxAttempts 為 3：
// --timeout 是「每次嘗試」的逾時，SDK 預設仍會重試，若不固定重試次數，
// 使用者無法從 --timeout 推算出總等待時間上限。
func TestNewS3SetsRetryMaxAttempts(t *testing.T) {
	c, err := NewS3(S3Options{Endpoint: "http://x", AccessKey: "ak", SecretKey: "sk", Timeout: time.Second})
	require.NoError(t, err)
	assert.Equal(t, 3, c.Client.Options().RetryMaxAttempts)
}

// TestClassifyS3ErrorMethodIncludesOperation 驗證 *twai.APIError.Method 對 S3 錯誤
// 顯示 "S3 <Operation>"（例如 "S3 CreateBucket"），而不是單純的 "S3"，
// 讓錯誤訊息能看出是哪一個 S3 操作失敗。
func TestClassifyS3ErrorMethodIncludesOperation(t *testing.T) {
	fake := testutil.NewFakeS3(t)
	c := newTestS3(t, fake)

	require.NoError(t, c.CreateBucket(context.Background(), "b"))
	err := c.CreateBucket(context.Background(), "b")

	var apiErr *twai.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, "S3 CreateBucket", apiErr.Method)
}

// writeTempFile 把 content 寫進暫存檔並回傳路徑。
func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "f")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

// TestListObjectVersionsAndDeleteVersion 驗證開啟 versioning 的 bucket：同一 key PUT 兩次、
// DELETE 一次後，ListObjectVersions 依新 → 舊回傳 delete marker + 兩個版本；
// 逐一以 DeleteObjectVersion 刪除後 bucket 可被 DeleteBucket。
func TestListObjectVersionsAndDeleteVersion(t *testing.T) {
	fake := testutil.NewFakeS3(t)
	fake.PutBucket("b")
	fake.SetVersioning("b", "Enabled")
	c := newTestS3(t, fake)
	ctx := context.Background()

	require.NoError(t, c.Upload(ctx, "b", "k", writeTempFile(t, "v1")))
	require.NoError(t, c.Upload(ctx, "b", "k", writeTempFile(t, "v2")))
	require.NoError(t, c.DeleteObject(ctx, "b", "k"))

	var got []ObjectVersion
	require.NoError(t, c.ListObjectVersions(ctx, "b", "", func(v ObjectVersion) error {
		got = append(got, v)
		return nil
	}))
	require.Len(t, got, 3)
	assert.True(t, got[0].DeleteMarker)
	assert.True(t, got[0].IsLatest)
	assert.Equal(t, "k", got[0].Key)
	assert.False(t, got[1].DeleteMarker)
	assert.Equal(t, int64(2), got[1].Size)
	assert.NotEmpty(t, got[1].VersionID)
	assert.NotEqual(t, got[1].VersionID, got[2].VersionID)

	// 一般 ListObjects 看不到已被 delete marker 蓋住的 key
	count := 0
	require.NoError(t, c.ListObjects(ctx, "b", "", func(Object) error { count++; return nil }))
	assert.Equal(t, 0, count)

	// bucket 仍非空
	err := c.DeleteBucket(ctx, "b")
	var apiErr *twai.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Contains(t, apiErr.Message, "BucketNotEmpty")

	for _, v := range got {
		require.NoError(t, c.DeleteObjectVersion(ctx, "b", v.Key, v.VersionID))
	}
	assert.Empty(t, fake.Versions("b", "k"))
	require.NoError(t, c.DeleteBucket(ctx, "b"))
}

// TestFakeS3SuspendedKeepsExistingVersions 驗證 bucket 從 Enabled 轉為 Suspended 後，
// 既有（帶真正 version id）的版本必須保留：PUT 只會建立或取代 VersionID 為 "null" 的那個版本；
// 無 versionId 的 DELETE 同理，只會用一個 VersionID 為 "null" 的 delete marker 取代既有的 null
// 版本，不影響其他版本。這與「從未設定過 versioning」（PUT／DELETE 直接覆蓋整個歷史）不同。
func TestFakeS3SuspendedKeepsExistingVersions(t *testing.T) {
	fake := testutil.NewFakeS3(t)
	fake.PutBucket("b")
	fake.SetVersioning("b", "Enabled")
	c := newTestS3(t, fake)
	ctx := context.Background()

	require.NoError(t, c.Upload(ctx, "b", "k", writeTempFile(t, "v1")))
	require.NoError(t, c.Upload(ctx, "b", "k", writeTempFile(t, "v2")))
	fake.SetVersioning("b", "Suspended")
	require.NoError(t, c.Upload(ctx, "b", "k", writeTempFile(t, "v3")))

	var got []ObjectVersion
	require.NoError(t, c.ListObjectVersions(ctx, "b", "", func(v ObjectVersion) error {
		got = append(got, v)
		return nil
	}))
	require.Len(t, got, 3, "Suspended 狀態下的 PUT 只取代既有的 null 版本，帶 id 的舊版本應保留")
	assert.Equal(t, "null", got[0].VersionID, "最新一筆應是 Suspended 期間寫入、VersionID 為 null 的版本")
	assert.True(t, got[0].IsLatest)
	assert.False(t, got[0].DeleteMarker)

	require.NoError(t, c.DeleteObject(ctx, "b", "k"))

	got = nil
	require.NoError(t, c.ListObjectVersions(ctx, "b", "", func(v ObjectVersion) error {
		got = append(got, v)
		return nil
	}))
	require.Len(t, got, 3, "無 versionId 的 DELETE 在 Suspended 狀態下只會用 null delete marker 取代既有 null 版本")
	assert.Equal(t, "null", got[0].VersionID)
	assert.True(t, got[0].DeleteMarker, "最新一筆應變成 delete marker")
	assert.True(t, got[0].IsLatest)
	for _, v := range got[1:] {
		assert.NotEqual(t, "null", v.VersionID, "Enabled 期間寫入、帶真正 version id 的版本應保留")
		assert.False(t, v.DeleteMarker)
	}
}

// TestListObjectVersionsUnversionedBucketReportsNullVersion 驗證未開啟 versioning 的 bucket，
// ListObjectVersions 仍列出每個物件且 VersionID 為 "null"；以 "null" 呼叫 DeleteObjectVersion 可刪除。
func TestListObjectVersionsUnversionedBucketReportsNullVersion(t *testing.T) {
	fake := testutil.NewFakeS3(t)
	fake.PutObject("b", "k", []byte("x"))
	c := newTestS3(t, fake)
	ctx := context.Background()

	var got []ObjectVersion
	require.NoError(t, c.ListObjectVersions(ctx, "b", "", func(v ObjectVersion) error {
		got = append(got, v)
		return nil
	}))
	require.Len(t, got, 1)
	assert.Equal(t, "null", got[0].VersionID)
	assert.True(t, got[0].IsLatest)

	require.NoError(t, c.DeleteObjectVersion(ctx, "b", "k", "null"))
	_, ok := fake.Object("b", "k")
	assert.False(t, ok)
}

// TestListObjectVersionsPrefixAndCallbackError 驗證 prefix 過濾，以及 fn 回傳錯誤時立即中止並原樣回傳。
func TestListObjectVersionsPrefixAndCallbackError(t *testing.T) {
	fake := testutil.NewFakeS3(t)
	fake.PutObject("b", "dir/a", []byte("a"))
	fake.PutObject("b", "other", []byte("o"))
	c := newTestS3(t, fake)
	ctx := context.Background()

	var keys []string
	require.NoError(t, c.ListObjectVersions(ctx, "b", "dir/", func(v ObjectVersion) error {
		keys = append(keys, v.Key)
		return nil
	}))
	assert.Equal(t, []string{"dir/a"}, keys)

	stop := errors.New("stop")
	err := c.ListObjectVersions(ctx, "b", "", func(ObjectVersion) error { return stop })
	assert.ErrorIs(t, err, stop)
}

// TestListObjectVersionsPaginates 驗證 ListObjectVersions 以 KeyMarker 手動分頁：FakeS3 每頁 2 個 key、
// 共 5 個 key 各 1 個版本，不帶 NextKeyMarker（AWS 行為）與帶 NextKeyMarker（Ceph 行為）兩種模式都要
// 列出全部 5 筆、順序正確，且總共觸發 3 次 GET（2+2+1）。
func TestListObjectVersionsPaginates(t *testing.T) {
	for _, emit := range []bool{false, true} {
		t.Run(fmt.Sprintf("EmitNextMarker=%v", emit), func(t *testing.T) {
			fake := testutil.NewFakeS3(t)
			fake.MaxKeys = 2
			fake.EmitNextMarker = emit
			for _, k := range []string{"a", "b", "c", "d", "e"} {
				fake.PutObject("bkt", k, []byte(k))
			}
			c := newTestS3(t, fake)
			var keys []string
			require.NoError(t, c.ListObjectVersions(context.Background(), "bkt", "", func(v ObjectVersion) error {
				keys = append(keys, v.Key)
				return nil
			}))
			assert.Equal(t, []string{"a", "b", "c", "d", "e"}, keys)

			pages := 0
			for _, r := range fake.Requests() {
				if r.Method == "GET" && r.Path == "/bkt" {
					pages++
				}
			}
			assert.Equal(t, 3, pages)
		})
	}
}

// TestListObjectVersionsSingleKeyManyVersions 驗證單一 key 的版本數超過一頁（MaxKeys=2、
// 同一個 key PUT 5 次）時也能正確拆頁：NextKeyMarker 在每一頁之間維持同一個 key 不變，
// 只有 NextVersionIdMarker 前進，ListObjectVersions 的「沒有進展」判斷不能只看 key marker
// （見 internal/twai/cos.S3.ListObjectVersions 的比對邏輯），否則會在第二頁就誤判為卡住。
func TestListObjectVersionsSingleKeyManyVersions(t *testing.T) {
	for _, emit := range []bool{false, true} {
		t.Run(fmt.Sprintf("EmitNextMarker=%v", emit), func(t *testing.T) {
			fake := testutil.NewFakeS3(t)
			fake.MaxKeys = 2
			fake.EmitNextMarker = emit
			fake.PutBucket("b")
			fake.SetVersioning("b", "Enabled")
			c := newTestS3(t, fake)
			ctx := context.Background()
			for i := 0; i < 5; i++ {
				require.NoError(t, c.Upload(ctx, "b", "k", writeTempFile(t, fmt.Sprintf("v%d", i))))
			}

			var versionIDs []string
			require.NoError(t, c.ListObjectVersions(ctx, "b", "", func(v ObjectVersion) error {
				assert.Equal(t, "k", v.Key)
				versionIDs = append(versionIDs, v.VersionID)
				return nil
			}))
			require.Len(t, versionIDs, 5)
			// 依新 → 舊排序，且沒有重複（拆頁時沒有漏掉或重覆任何一筆）。
			seen := map[string]bool{}
			for _, id := range versionIDs {
				assert.False(t, seen[id], "version %s 不應重覆出現", id)
				seen[id] = true
			}
			assert.Len(t, seen, 5)

			pages := 0
			for _, r := range fake.Requests() {
				if r.Method == "GET" && r.Path == "/b" {
					pages++
				}
			}
			assert.Equal(t, 3, pages)
		})
	}
}

// TestListObjectVersionsStuckMarker 驗證 FakeS3.StuckMarker 模擬 ListObjectVersions 分頁
// marker 永遠沒有進展時，ListObjectVersions 回傳「沒有進展」錯誤而不是無限迴圈。
func TestListObjectVersionsStuckMarker(t *testing.T) {
	fake := testutil.NewFakeS3(t)
	fake.MaxKeys = 2
	fake.StuckMarker = true
	for _, k := range []string{"a", "b", "c"} {
		fake.PutObject("b", k, []byte(k))
	}
	c := newTestS3(t, fake)
	err := c.ListObjectVersions(context.Background(), "b", "", func(ObjectVersion) error { return nil })
	require.Error(t, err)
	assert.Contains(t, err.Error(), "沒有進展")
}

// TestFakeS3MissingBucketIs404 驗證對不存在的 bucket 上傳／下載／刪除都回 404 NoSuchBucket（exit 3 對應）。
func TestFakeS3MissingBucketIs404(t *testing.T) {
	fake := testutil.NewFakeS3(t)
	c := newTestS3(t, fake)
	ctx := context.Background()

	var apiErr *twai.APIError
	err := c.Upload(ctx, "missing", "k", writeTempFile(t, "x"))
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 404, apiErr.StatusCode)

	err = c.Download(ctx, "missing", "k", filepath.Join(t.TempDir(), "out"))
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 404, apiErr.StatusCode)

	err = c.DeleteObject(ctx, "missing", "k")
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 404, apiErr.StatusCode)
}

// TestFakeS3EscapesXML 驗證 key 含 XML 特殊字元（& < >）時 ListObjects 仍能正確解析出原始 key。
func TestFakeS3EscapesXML(t *testing.T) {
	fake := testutil.NewFakeS3(t)
	fake.PutObject("b", "a&b<c>.txt", []byte("x"))
	c := newTestS3(t, fake)

	var keys []string
	require.NoError(t, c.ListObjects(context.Background(), "b", "", func(o Object) error {
		keys = append(keys, o.Key)
		return nil
	}))
	assert.Equal(t, []string{"a&b<c>.txt"}, keys)
}

// findRequest 回傳第一個 method+path 相符且帶指定 header 的請求；找不到回傳 nil。
func findRequest(fake *testutil.FakeS3, method, path, header string) *testutil.Recorded {
	for _, req := range fake.Requests() {
		if req.Method == method && req.Path == path && req.Header.Get(header) != "" {
			r := req
			return &r
		}
	}
	return nil
}

// TestSetObjectACLAndIsObjectPublic 驗證 canned ACL 設定與 GetObjectAcl 判斷。
func TestSetObjectACLAndIsObjectPublic(t *testing.T) {
	fake := testutil.NewFakeS3(t)
	fake.PutObject("b", "k", []byte("x"))
	c := newTestS3(t, fake)
	ctx := context.Background()

	public, err := c.IsObjectPublic(ctx, "b", "k")
	require.NoError(t, err)
	assert.False(t, public)

	require.NoError(t, c.SetObjectACL(ctx, "b", "k", true))
	assert.Equal(t, "public-read", fake.ACL("b", "k"))
	public, err = c.IsObjectPublic(ctx, "b", "k")
	require.NoError(t, err)
	assert.True(t, public)

	require.NoError(t, c.SetObjectACL(ctx, "b", "k", false))
	assert.Equal(t, "private", fake.ACL("b", "k"))
}

// TestSetContentTypePreservesContentAndACL 驗證 SetContentType 以 CopyObject 就地改 Content-Type，
// 內容不變、原本公開的物件維持公開。
func TestSetContentTypePreservesContentAndACL(t *testing.T) {
	fake := testutil.NewFakeS3(t)
	fake.PutObject("b", "page", []byte("<h1>hi</h1>"))
	c := newTestS3(t, fake)
	ctx := context.Background()
	require.NoError(t, c.SetObjectACL(ctx, "b", "page", true))

	require.NoError(t, c.SetContentType(ctx, "b", "page", "text/html"))
	assert.Equal(t, "text/html", fake.ContentType("b", "page"))
	assert.Equal(t, "public-read", fake.ACL("b", "page"))
	body, _ := fake.Object("b", "page")
	assert.Equal(t, "<h1>hi</h1>", string(body))

	copyReq := findRequest(fake, "PUT", "/b/page", "x-amz-copy-source")
	require.NotNil(t, copyReq, "應以 CopyObject（帶 x-amz-copy-source）實作")
	assert.Equal(t, "REPLACE", copyReq.Header.Get("x-amz-metadata-directive"))
}

// TestSetContentTypePreservesSystemMetadata 驗證 SetContentType 的 CopyObject REPLACE
// 不會清掉沒有明確重新帶入的系統 metadata（Cache-Control、Content-Encoding 等）——
// CopyObject 在 REPLACE 模式下，所有未在請求中重送的 HTTP metadata 都會被清空，
// 因此 SetContentType 必須先讀出這些欄位再一併帶回。
func TestSetContentTypePreservesSystemMetadata(t *testing.T) {
	fake := testutil.NewFakeS3(t)
	fake.PutBucket("b")
	c := newTestS3(t, fake)
	ctx := context.Background()

	bucket, key := "b", "asset.js.gz"
	body := []byte("gzip-content")
	_, err := c.Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: &bucket, Key: &key, Body: bytes.NewReader(body),
		ContentEncoding: aws.String("gzip"), CacheControl: aws.String("max-age=60"),
	})
	require.NoError(t, err)

	require.NoError(t, c.SetContentType(ctx, bucket, key, "application/javascript"))
	assert.Equal(t, "application/javascript", fake.ContentType(bucket, key))
	assert.Equal(t, "gzip", fake.Header(bucket, key, "Content-Encoding"), "Content-Encoding 不應在 REPLACE 後消失")
	assert.Equal(t, "max-age=60", fake.Header(bucket, key, "Cache-Control"), "Cache-Control 不應在 REPLACE 後消失")
}

// TestSetContentTypeRejectsOversizedObject 驗證超過單次 CopyObject 上限（5 GiB）的物件
// 會直接回一般錯誤，且不會送出 CopyObject 請求（避免呼叫端誤以為已修改成功、
// 或讓假 server／真實 S3 才發現失敗）；同時驗證 FakeS3 的 ListObjects 回報的 Size
// 與 HEAD（HeadObject 依賴的大小判斷）一致，都採用 sizeOverride，而不是各自認定
// 不同大小（先前 listObjectsV2／listObjectVersions 仍用 len(body)，同一物件在
// 清單中會誤報成 0 bytes）。
func TestSetContentTypeRejectsOversizedObject(t *testing.T) {
	fake := testutil.NewFakeS3(t)
	const hugeSize = 6 * 1024 * 1024 * 1024 // 6 GiB，超過 5 GiB 上限
	fake.PutObjectWithSize("b", "huge.bin", hugeSize)
	c := newTestS3(t, fake)
	ctx := context.Background()

	err := c.SetContentType(ctx, "b", "huge.bin", "application/octet-stream")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "超過單次 CopyObject 上限 5 GiB")

	assert.Nil(t, findRequest(fake, "PUT", "/b/huge.bin", "x-amz-copy-source"), "超過上限時不應送出 CopyObject 請求")

	var objs []Object
	require.NoError(t, c.ListObjects(ctx, "b", "", func(o Object) error {
		objs = append(objs, o)
		return nil
	}))
	require.Len(t, objs, 1)
	assert.Equal(t, int64(hugeSize), objs[0].Size, "ListObjects 回報的 Size 應與 sizeOverride 一致，而不是誤報成 0 bytes")
}

// TestUploadSetsContentTypeByExtension 驗證上傳依副檔名帶 Content-Type，未知副檔名為 application/octet-stream。
func TestUploadSetsContentTypeByExtension(t *testing.T) {
	fake := testutil.NewFakeS3(t)
	fake.PutBucket("b")
	c := newTestS3(t, fake)
	ctx := context.Background()

	dir := t.TempDir()
	html := filepath.Join(dir, "index.html")
	require.NoError(t, os.WriteFile(html, []byte("<p/>"), 0o644))
	bin := filepath.Join(dir, "data.unknownext")
	require.NoError(t, os.WriteFile(bin, []byte{0, 1}, 0o644))

	require.NoError(t, c.Upload(ctx, "b", "index.html", html))
	require.NoError(t, c.Upload(ctx, "b", "data", bin))
	assert.True(t, strings.HasPrefix(fake.ContentType("b", "index.html"), "text/html"))
	assert.Equal(t, "application/octet-stream", fake.ContentType("b", "data"))
}

func TestContentTypeForPath(t *testing.T) {
	assert.True(t, strings.HasPrefix(ContentTypeForPath("a/b/c.json"), "application/json"))
	assert.Equal(t, "application/octet-stream", ContentTypeForPath("noext"))
	assert.Equal(t, "application/octet-stream", ContentTypeForPath("x.unknownext"))
}

// TestContentTypeForPathBuiltinTable 驗證常見 Web 副檔名一律採用內建對照表，
// 不受作業系統的 mime 表影響（跨平台行為一致）；.js 固定為 text/javascript，
// .md 固定為 text/markdown（brief 明確要求的兩個值）。
func TestContentTypeForPathBuiltinTable(t *testing.T) {
	cases := []struct{ path, want string }{
		{"a.html", "text/html; charset=utf-8"},
		{"a.css", "text/css; charset=utf-8"},
		{"a.js", "text/javascript; charset=utf-8"},
		{"a.json", "application/json"},
		{"a.svg", "image/svg+xml"},
		{"a.png", "image/png"},
		{"a.jpg", "image/jpeg"},
		{"a.jpeg", "image/jpeg"},
		{"a.gif", "image/gif"},
		{"a.webp", "image/webp"},
		{"a.woff2", "font/woff2"},
		{"a.wasm", "application/wasm"},
		{"a.txt", "text/plain; charset=utf-8"},
		{"a.md", "text/markdown; charset=utf-8"},
		{"a.xml", "application/xml"},
		{"a.pdf", "application/pdf"},
		// 副檔名大小寫不應影響結果。
		{"A.HTML", "text/html; charset=utf-8"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, ContentTypeForPath(c.path), c.path)
	}
}
