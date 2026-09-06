package testutil

import (
	"bytes"
	"crypto/md5" //nolint:gosec // 僅用於產生假 S3 server 的 ETag，非安全用途
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// FakeS3 是記憶體版的 S3 相容假伺服器，供 internal/twai/cos 與 internal/cli 的測試共用。
// 只實作 twaictl 用得到的 API 子集：bucket CRUD、versioning、物件 CRUD、
// ListObjects（V1，marker 分頁）、ListObjectsV2（list-type=2，供舊測試相容用）、
// ListObjectVersions（?versions）、GetBucketVersioning（?versioning）、
// 以及基本的 multipart upload（CreateMultipartUpload/UploadPart/CompleteMultipartUpload）。
// 支援 versioning（每個 key 保留多個版本、delete marker）、對不存在的 bucket 一律回 404、
// 所有寫進回應 XML 的使用者字串都經過 XML escape。不驗證 SigV4 簽章，只記錄 Authorization
// header 是否存在，供測試斷言請求確實有簽章。
type FakeS3 struct {
	srv *httptest.Server

	mu       sync.Mutex
	buckets  map[string]*fakeS3Bucket
	uploads  map[string]*fakeS3Upload // uploadId -> 進行中的 multipart upload
	requests []Recorded

	// MaxKeys 是 ListObjects(V1)／ListObjectsV2 沒有指定 max-keys 查詢參數時使用的
	// 預設分頁大小；測試可降低此值（例如 2）以觸發分頁流程。
	MaxKeys int

	// EmitNextMarker 為 true 時，V1 ListObjects 分頁未結束時回應會多帶 <NextMarker>
	// （模擬 Ceph RGW 的行為）；預設 false，模擬 AWS S3 沒有 delimiter 時不回 NextMarker
	// 的嚴格行為，讓呼叫端必須自行以本頁最後一個 key 當下一頁 marker 的退路能被測到。
	EmitNextMarker bool

	// StuckMarker 為 true 時，V1 ListObjects 與 ListObjectVersions 一律回傳 IsTruncated=true、
	// 內容固定為第一頁、NextMarker／NextKeyMarker 固定為第一頁的第一個 key（忽略請求帶的 marker），
	// 模擬「分頁 marker 永遠沒有進展」的異常伺服器行為，供測試驗證呼叫端的無限迴圈防呆。
	StuckMarker bool

	// FailVersioningError 非空時，PutBucketVersioning 一律回傳 500 與此錯誤碼，
	// 供測試模擬「bucket 已建立，但設定版本控制失敗」這種第二步失敗的情境。
	FailVersioningError string

	// BeforeRequest 非 nil 時，在每個請求被記錄進 requests 之後、實際處理之前呼叫一次
	// （呼叫時不持有 f.mu，hook 內可以安全呼叫 PutObject 等會自行上鎖的方法，不會與
	// handle() 互相等待）。供測試模擬「兩段式刪除的 pass 1 與 pass 2 之間 bucket 內容
	// 有變動」：呼叫端可依 *http.Request 的 Method／已經送出的請求數，在特定時機插入
	// 額外的物件。呼叫端須自行確保並行安全（例如用 sync/atomic），這裡不提供內建的
	// 「只呼叫一次」語意。
	BeforeRequest func(r *http.Request)
}

// allUsersGroupURI 是 S3 ACL 中代表「所有人（匿名）」的 grantee 群組 URI，
// 與 internal/twai/cos.allUsersGroupURI 相同（測試用假 server 不依賴業務套件，故各自定義一份）。
const allUsersGroupURI = "http://acs.amazonaws.com/groups/global/AllUsers"

// trackedHeaders 是 SetContentType（CopyObject REPLACE）需要一併保留／替換的系統 metadata
// header：這些欄位在真實 S3 的 REPLACE 語意下，凡沒有在請求中重新帶入的都會被清空，
// 因此 FakeS3 必須記住並在 HEAD/GET 回傳，供 internal/twai/cos.SetContentType 的
// 「保留系統 metadata」邏輯與對應測試驗證。
var trackedHeaders = []string{
	"Cache-Control", "Content-Disposition", "Content-Encoding",
	"Content-Language", "Expires", "X-Amz-Website-Redirect-Location",
}

// extractHeaders 從請求 header 取出 trackedHeaders 中有值的欄位（canonical 大小寫），
// 沒有值的欄位不放進 map（Header accessor 對不存在的 key 回傳空字串，語意等價於「未設定」）。
func extractHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(trackedHeaders))
	for _, name := range trackedHeaders {
		if v := h.Get(name); v != "" {
			out[name] = v
		}
	}
	return out
}

// cloneHeaders 回傳 headers 的淺複本，避免多個物件版本共用同一個 map 底層陣列
// （例如 copyObject 的「不 REPLACE 時沿用來源」情境）。
func cloneHeaders(headers map[string]string) map[string]string {
	out := make(map[string]string, len(headers))
	for k, v := range headers {
		out[k] = v
	}
	return out
}

// objectSize 回傳物件的邏輯大小：sizeOverride 非零時優先採用（供 PutObjectWithSize
// 模擬超大物件，不必真的配置對應大小的記憶體），否則回傳實際內容長度。
func objectSize(obj *fakeS3Object) int64 {
	if obj.sizeOverride > 0 {
		return obj.sizeOverride
	}
	return int64(len(obj.body))
}

// fakeS3Object 是記憶體內單一物件版本的內容與中繼資料。
type fakeS3Object struct {
	body         []byte
	etag         string
	mod          time.Time
	versionID    string
	deleteMarker bool
	contentType  string            // 預設 "application/octet-stream"
	acl          string            // 預設 "private"
	headers      map[string]string // trackedHeaders 中有值的系統 metadata（見 extractHeaders）
	sizeOverride int64             // > 0 時取代 len(body) 作為回報的大小（見 PutObjectWithSize）
}

// fakeS3Bucket 是記憶體內單一 bucket 的物件集合與版本控制狀態。
// versions 保存每個 key 依時間「舊 → 新」排序的所有版本（含 delete marker），
// 最後一個元素即該 key 目前最新的版本。
type fakeS3Bucket struct {
	versions    map[string][]*fakeS3Object
	versioning  string // "" | "Enabled" | "Suspended"
	nextVersion int    // 下一個版本號（versioning Enabled 時用來組出遞增的 versionID）
}

// current 回傳 key 目前最新的版本；若最新版本是 delete marker 或 key 完全不存在，ok 為 false。
func (b *fakeS3Bucket) current(key string) (obj *fakeS3Object, ok bool) {
	vs := b.versions[key]
	if len(vs) == 0 {
		return nil, false
	}
	latest := vs[len(vs)-1]
	if latest.deleteMarker {
		return nil, false
	}
	return latest, true
}

// fakeS3Upload 是進行中的 multipart upload：part 編號對應該分片的內容。
// contentType 是 CreateMultipartUpload 請求帶的 Content-Type，CompleteMultipartUpload
// 時套用到最終合併出的物件（真實 S3 的 Content-Type 是在 initiate 階段決定，
// UploadPart／CompleteMultipartUpload 都不會再帶）。
type fakeS3Upload struct {
	bucket      string
	key         string
	parts       map[int][]byte
	contentType string
}

// NewFakeS3 啟動一個假 S3 server；呼叫端可用 PutBucket / PutObject 預先塞資料，
// server 會在 t 結束時自動關閉。
func NewFakeS3(t *testing.T) *FakeS3 {
	t.Helper()
	f := &FakeS3{
		buckets: map[string]*fakeS3Bucket{},
		uploads: map[string]*fakeS3Upload{},
		MaxKeys: 1000,
	}
	f.srv = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.srv.Close)
	return f
}

// URL 回傳假 S3 server 的網址（作為 cos.S3Options.Endpoint）。
func (f *FakeS3) URL() string {
	return f.srv.URL
}

// PutBucket 預先建立一個空 bucket（測試準備情境，不透過 HTTP，已存在則不動作）。
func (f *FakeS3) PutBucket(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ensureBucketLocked(name)
}

// PutObject 預先塞入一筆物件資料（測試準備情境，不透過 HTTP；bucket 不存在時自動建立）。
func (f *FakeS3) PutObject(bucket, key string, body []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b := f.ensureBucketLocked(bucket)
	b.putLocked(key, newFakeS3Object(body))
}

// PutObjectWithETag 同 PutObject，但可指定 ETag（供測試模擬 multipart 上傳產生的 "<md5>-<n>" ETag；
// 測試準備情境，不透過 HTTP；bucket 不存在時自動建立）。
func (f *FakeS3) PutObjectWithETag(bucket, key string, body []byte, etag string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b := f.ensureBucketLocked(bucket)
	obj := newFakeS3Object(body)
	obj.etag = etag
	b.putLocked(key, obj)
}

// PutObjectWithSize 預先塞入一筆「假裝有 size 位元組」的物件（測試準備情境，不透過 HTTP；
// bucket 不存在時自動建立）：內容本體是空的，只記錄 size 供 HEAD/GET/ListObjects 回報，
// 用於模擬超過 SetContentType 單次 CopyObject 上限（5 GiB）的大型物件，
// 不必真的配置對應大小的記憶體。
func (f *FakeS3) PutObjectWithSize(bucket, key string, size int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b := f.ensureBucketLocked(bucket)
	obj := newFakeS3Object(nil)
	obj.sizeOverride = size
	b.putLocked(key, obj)
}

// newFakeS3Object 建立一個帶預設中繼資料的新物件版本：etag（由內容算出）、mod（單調遞增的
// 目前時間）、contentType 與 acl 皆為預設值。versionID 不在此設定，由呼叫端透過
// fakeS3Bucket.putLocked 依 bucket 目前的版本控制狀態決定。
func newFakeS3Object(body []byte) *fakeS3Object {
	return &fakeS3Object{
		body: append([]byte(nil), body...), etag: etagFor(body), mod: monotonicNow(),
		contentType: "application/octet-stream", acl: "private",
	}
}

// SetVersioning 直接設定 bucket 的版本控制狀態（測試準備情境，不透過 HTTP）。
func (f *FakeS3) SetVersioning(bucket, status string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b := f.ensureBucketLocked(bucket)
	b.versioning = status
}

// FakeS3Version 是 Versions 回傳的單一版本摘要。
type FakeS3Version struct {
	VersionID    string
	DeleteMarker bool
	Body         []byte
}

// Versions 回傳 bucket/key 目前保留的所有版本（含 delete marker），依時間舊 → 新排序。
func (f *FakeS3) Versions(bucket, key string) []FakeS3Version {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.buckets[bucket]
	if !ok {
		return nil
	}
	vs := b.versions[key]
	out := make([]FakeS3Version, 0, len(vs))
	for _, v := range vs {
		out = append(out, FakeS3Version{
			VersionID: v.versionID, DeleteMarker: v.deleteMarker,
			Body: append([]byte(nil), v.body...),
		})
	}
	return out
}

// ensureBucketLocked 取得（必要時建立）bucket；呼叫端須已持有 f.mu。
func (f *FakeS3) ensureBucketLocked(name string) *fakeS3Bucket {
	b, ok := f.buckets[name]
	if !ok {
		b = &fakeS3Bucket{versions: map[string][]*fakeS3Object{}}
		f.buckets[name] = b
	}
	return b
}

// putLocked 依 bucket 目前的版本控制狀態寫入一個新版本；呼叫端須已持有 f.mu。
//   - Enabled：附加一個新版本，versionID 遞增（例如 "v3"）。
//   - Suspended：曾經開過版本控制，帶真正 version id 的既有版本必須保留；新版本的
//     versionID 固定為 "null"，並取代（而非疊加）既有的 null 版本——S3 對同一 key
//     同時最多只有一個 null 版本。
//   - ""（從未設定過版本控制）：以單一版本覆蓋整個歷史記錄，versionID 固定為 "null"。
func (b *fakeS3Bucket) putLocked(key string, obj *fakeS3Object) {
	switch b.versioning {
	case "Enabled":
		obj.versionID = fmt.Sprintf("v%d", b.nextVersion)
		b.nextVersion++
		b.versions[key] = append(b.versions[key], obj)
	case "Suspended":
		obj.versionID = "null"
		b.versions[key] = append(removeNullVersionLocked(b.versions[key]), obj)
	default:
		obj.versionID = "null"
		b.versions[key] = []*fakeS3Object{obj}
	}
}

// removeNullVersionLocked 回傳移除掉既有 "null" 版本後的版本切片（其餘版本保留原本順序）；
// 呼叫端須已持有 f.mu。用於 Suspended 狀態下 PUT／無 versionId 的 DELETE 取代既有 null 版本。
func removeNullVersionLocked(vs []*fakeS3Object) []*fakeS3Object {
	filtered := vs[:0:0]
	for _, v := range vs {
		if v.versionID != "null" {
			filtered = append(filtered, v)
		}
	}
	return filtered
}

// Object 回傳 bucket/key 目前的內容；ok 為 false 代表 bucket 或物件不存在（或已被 delete marker 蓋住）。
func (f *FakeS3) Object(bucket, key string) ([]byte, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.buckets[bucket]
	if !ok {
		return nil, false
	}
	obj, ok := b.current(key)
	if !ok {
		return nil, false
	}
	return append([]byte(nil), obj.body...), true
}

// Buckets 回傳目前所有 bucket 名稱（未排序，呼叫端自行排序）。
func (f *FakeS3) Buckets() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	names := make([]string, 0, len(f.buckets))
	for name := range f.buckets {
		names = append(names, name)
	}
	return names
}

// Versioning 回傳 bucket 目前的版本控制狀態（"" 代表從未設定過、"Enabled"、"Suspended"）。
func (f *FakeS3) Versioning(bucket string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.buckets[bucket]
	if !ok {
		return ""
	}
	return b.versioning
}

// ACL 回傳 bucket/key 目前最新版本的 canned ACL（"private" | "public-read"）；
// 物件不存在時回傳 "private"（與 fakeS3Object 的預設值一致）。
func (f *FakeS3) ACL(bucket, key string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.buckets[bucket]
	if !ok {
		return "private"
	}
	obj, ok := b.current(key)
	if !ok {
		return "private"
	}
	return obj.acl
}

// ContentType 回傳 bucket/key 目前最新版本的 Content-Type；物件不存在時回傳預設值
// "application/octet-stream"（與 fakeS3Object 的預設值一致）。
func (f *FakeS3) ContentType(bucket, key string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.buckets[bucket]
	if !ok {
		return "application/octet-stream"
	}
	obj, ok := b.current(key)
	if !ok {
		return "application/octet-stream"
	}
	return obj.contentType
}

// Header 回傳 bucket/key 目前最新版本的指定系統 metadata header（trackedHeaders 之一，
// 例如 "Content-Encoding"、"Cache-Control"）；物件不存在或該 header 未設定時回傳空字串。
func (f *FakeS3) Header(bucket, key, name string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.buckets[bucket]
	if !ok {
		return ""
	}
	obj, ok := b.current(key)
	if !ok {
		return ""
	}
	return obj.headers[http.CanonicalHeaderKey(name)]
}

// Requests 回傳目前為止收到的所有請求（加鎖複本）。
func (f *FakeS3) Requests() []Recorded {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Recorded, len(f.requests))
	copy(out, f.requests)
	return out
}

// etagFor 用內容的 MD5 產生一個帶引號的 ETag（模擬真實 S3 對單一 PUT 物件的行為）。
func etagFor(body []byte) string {
	sum := md5.Sum(body) //nolint:gosec // 只用於產生假 ETag 供測試比對，非安全用途
	return `"` + hex.EncodeToString(sum[:]) + `"`
}

// monotonicClock 保護 monotonicNow 回傳的時間嚴格遞增。
var (
	monotonicMu   sync.Mutex
	monotonicLast time.Time
)

// monotonicNow 回傳目前時間，並保證每次呼叫都比前一次嚴格遞增（以 1 奈秒為最小間隔）。
// 粗時鐘平台（例如部分 Windows CI）上 time.Now() 解析度可能不足以區分快速連續的呼叫，
// 若同一個 key 的多個版本 mod 時間相同，ListObjectVersions 依 (Key, LastModified 由新到舊)
// 排序時順序會不穩定，因此改用單調遞增計數器保證每個版本都有獨一無二、且遞增的時間戳。
func monotonicNow() time.Time {
	monotonicMu.Lock()
	defer monotonicMu.Unlock()
	now := time.Now().UTC()
	if !now.After(monotonicLast) {
		now = monotonicLast.Add(time.Nanosecond)
	}
	monotonicLast = now
	return now
}

// xmlEscape 把字串中的 XML 特殊字元（& < > 等）轉成 entity，用於所有寫進回應 XML 的
// 使用者輸入字串（bucket、key、prefix、ETag、continuation token）。
func xmlEscape(s string) string {
	var buf bytes.Buffer
	// xml.EscapeText 只會回傳寫入 buf 時的 I/O 錯誤；bytes.Buffer 的 Write 不會失敗，可安全忽略。
	_ = xml.EscapeText(&buf, []byte(s))
	return buf.String()
}

// handle 是假 S3 server 的統一入口：記錄請求、（若設定）呼叫 BeforeRequest hook，
// 再依 path-style 路徑與 query string 分派。
func (f *FakeS3) handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	f.requests = append(f.requests, Recorded{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Clone(), body})
	hook := f.BeforeRequest
	f.mu.Unlock()
	if hook != nil {
		hook(r)
	}

	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		f.listBuckets(w)
		return
	}
	bucket, key, hasKey := strings.Cut(path, "/")
	if !hasKey || key == "" {
		f.handleBucket(w, r, bucket, body)
		return
	}
	f.handleObject(w, r, bucket, key, body)
}

// writeXMLError 依 brief 要求的格式回傳 `<Error><Code>...</Code><Message>...</Message></Error>`。
func writeXMLError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>%s</Code><Message>%s</Message></Error>`,
		xmlEscape(code), xmlEscape(message))
}

func (f *FakeS3) listBuckets(w http.ResponseWriter) {
	f.mu.Lock()
	names := make([]string, 0, len(f.buckets))
	for name := range f.buckets {
		names = append(names, name)
	}
	f.mu.Unlock()
	sort.Strings(names)

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	b.WriteString(`<ListAllMyBucketsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Buckets>`)
	now := time.Now().UTC().Format(time.RFC3339)
	for _, name := range names {
		_, _ = fmt.Fprintf(&b, `<Bucket><Name>%s</Name><CreationDate>%s</CreationDate></Bucket>`, xmlEscape(name), now)
	}
	b.WriteString(`</Buckets></ListAllMyBucketsResult>`)

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, b.String())
}

func (f *FakeS3) handleBucket(w http.ResponseWriter, r *http.Request, bucket string, body []byte) {
	query := r.URL.Query()
	switch r.Method {
	case http.MethodGet:
		switch {
		case query.Has("versions"):
			f.listObjectVersions(w, bucket, query)
		case query.Has("versioning"):
			f.getBucketVersioning(w, bucket)
		case query.Get("list-type") == "2":
			f.listObjectsV2(w, bucket, query)
		default:
			f.listObjectsV1(w, bucket, query)
		}
	case http.MethodPut:
		if _, ok := query["versioning"]; ok {
			f.putBucketVersioning(w, bucket, body)
			return
		}
		f.createBucket(w, bucket)
	case http.MethodDelete:
		f.deleteBucket(w, bucket)
	default:
		writeXMLError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "不支援的方法")
	}
}

func (f *FakeS3) createBucket(w http.ResponseWriter, bucket string) {
	f.mu.Lock()
	_, exists := f.buckets[bucket]
	if !exists {
		f.ensureBucketLocked(bucket)
	}
	f.mu.Unlock()

	if exists {
		writeXMLError(w, http.StatusConflict, "BucketAlreadyOwnedByYou", "您已擁有此 bucket")
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (f *FakeS3) deleteBucket(w http.ResponseWriter, bucket string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.buckets[bucket]
	if !ok {
		writeXMLError(w, http.StatusNotFound, "NoSuchBucket", "指定的 bucket 不存在")
		return
	}
	// delete marker 也算非空（與真實 Ceph 行為一致）：只要 versions 底下還留有任何一個 key
	// 的歷史記錄（不論最新版本是不是 delete marker），bucket 就不算空。
	if len(b.versions) > 0 {
		writeXMLError(w, http.StatusConflict, "BucketNotEmpty", "bucket 非空，無法刪除")
		return
	}
	delete(f.buckets, bucket)
	w.WriteHeader(http.StatusNoContent)
}

func (f *FakeS3) putBucketVersioning(w http.ResponseWriter, bucket string, body []byte) {
	if f.FailVersioningError != "" {
		writeXMLError(w, http.StatusInternalServerError, f.FailVersioningError, "模擬設定版本控制失敗")
		return
	}
	status := "Suspended"
	if strings.Contains(string(body), "<Status>Enabled</Status>") {
		status = "Enabled"
	}

	f.mu.Lock()
	b := f.ensureBucketLocked(bucket)
	b.versioning = status
	f.mu.Unlock()

	w.WriteHeader(http.StatusOK)
}

// getBucketVersioning 對應 `GET /{bucket}?versioning`（GetBucketVersioning）：
// 從未設定過版本控制時，S3 回傳沒有 <Status> 子元素的空 VersioningConfiguration。
func (f *FakeS3) getBucketVersioning(w http.ResponseWriter, bucket string) {
	// b.versioning 必須在持鎖期間讀出到區域變數 versioning：先前的寫法解鎖後才透過
	// *fakeS3Bucket 指標讀 b.versioning，與並發的 PutBucketVersioning（setBucketVersioning
	// 在鎖內寫入同一個欄位）是真正的資料競爭，-race 底下可能被抓到。
	f.mu.Lock()
	b, ok := f.buckets[bucket]
	var versioning string
	if ok {
		versioning = b.versioning
	}
	f.mu.Unlock()
	if !ok {
		writeXMLError(w, http.StatusNotFound, "NoSuchBucket", "指定的 bucket 不存在")
		return
	}

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	if versioning == "" {
		_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><VersioningConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"/>`)
		return
	}
	_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><VersioningConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Status>%s</Status></VersioningConfiguration>`,
		xmlEscape(versioning))
}

// fakeS3ObjectEntry 是 listObjectsV2／listObjectsV1 分頁與渲染用的扁平化單一物件快照：
// 只包含渲染 XML <Contents> 需要的欄位值（而非底層 *fakeS3Object 指標），
// 讓 liveObjectEntries 拿到之後可以完全放開鎖，之後所有處理都不再碰共用狀態。
type fakeS3ObjectEntry struct {
	key  string
	mod  time.Time
	etag string
	size int64
}

// liveObjectEntries 在單一次持鎖內完整快照 bucket 底下「目前存在」的物件（已排序、
// 且已經解析出渲染需要的 mod/etag/size），供 listObjectsV2／listObjectsV1 共用。
// 刻意把「列出 key」與「讀取每個 key 的內容」收在同一段鎖裡：先前的寫法是鎖一次取得
// key 清單、解鎖，稍後渲染內容時才又鎖一次讀取——這兩次持鎖之間有一段空窗，
// 若剛好有並發的 PutObject／DeleteObject 落在空窗內，回應可能混雜「已經不是這次
// 快照當下」的內容。改成一次鎖到底之後就不再有這個空窗。
func (f *FakeS3) liveObjectEntries(bucket string) (entries []fakeS3ObjectEntry, ok bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, exists := f.buckets[bucket]
	if !exists {
		return nil, false
	}
	keys := make([]string, 0, len(b.versions))
	for k := range b.versions {
		if _, live := b.current(k); live {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	entries = make([]fakeS3ObjectEntry, 0, len(keys))
	for _, k := range keys {
		obj, _ := b.current(k)
		entries = append(entries, fakeS3ObjectEntry{key: k, mod: obj.mod, etag: obj.etag, size: objectSize(obj)})
	}
	return entries, true
}

func (f *FakeS3) listObjectsV2(w http.ResponseWriter, bucket string, query url.Values) {
	entries, ok := f.liveObjectEntries(bucket)
	if !ok {
		writeXMLError(w, http.StatusNotFound, "NoSuchBucket", "指定的 bucket 不存在")
		return
	}

	prefix := query.Get("prefix")
	if prefix != "" {
		filtered := entries[:0:0]
		for _, e := range entries {
			if strings.HasPrefix(e.key, prefix) {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}

	maxKeys := f.MaxKeys
	if v := query.Get("max-keys"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxKeys = n
		}
	}

	start := 0
	if token := query.Get("continuation-token"); token != "" {
		for i, e := range entries {
			if e.key == token {
				start = i
				break
			}
		}
	}

	end := start + maxKeys
	truncated := end < len(entries)
	if end > len(entries) {
		end = len(entries)
	}
	page := entries[start:end]

	var buf strings.Builder
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	buf.WriteString(`<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">`)
	fmt.Fprintf(&buf, `<Name>%s</Name><Prefix>%s</Prefix><KeyCount>%d</KeyCount><MaxKeys>%d</MaxKeys><IsTruncated>%t</IsTruncated>`,
		xmlEscape(bucket), xmlEscape(prefix), len(page), maxKeys, truncated)
	if truncated {
		fmt.Fprintf(&buf, `<NextContinuationToken>%s</NextContinuationToken>`, xmlEscape(entries[end].key))
	}
	for _, e := range page {
		fmt.Fprintf(&buf, `<Contents><Key>%s</Key><LastModified>%s</LastModified><ETag>%s</ETag><Size>%d</Size><StorageClass>STANDARD</StorageClass></Contents>`,
			xmlEscape(e.key), e.mod.UTC().Format(time.RFC3339Nano), xmlEscape(e.etag), e.size)
	}
	buf.WriteString(`</ListBucketResult>`)

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, buf.String())
}

// listObjectsV1 對應 `GET /{bucket}`（沒有 list-type=2 查詢參數時的 ListObjects V1）：
// 支援 prefix、max-keys、marker（只回傳 key 嚴格大於 marker 的物件，依 key 排序）。
// 模擬 AWS S3 的嚴格行為：預設不回 <NextMarker>（呼叫端必須自行用本頁最後一個 key
// 當下一頁 marker）；EmitNextMarker 為 true 時才多帶 <NextMarker>（模擬 Ceph RGW）。
func (f *FakeS3) listObjectsV1(w http.ResponseWriter, bucket string, query url.Values) {
	entries, ok := f.liveObjectEntries(bucket)
	if !ok {
		writeXMLError(w, http.StatusNotFound, "NoSuchBucket", "指定的 bucket 不存在")
		return
	}

	prefix := query.Get("prefix")
	if prefix != "" {
		filtered := entries[:0:0]
		for _, e := range entries {
			if strings.HasPrefix(e.key, prefix) {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}

	maxKeys := f.MaxKeys
	if v := query.Get("max-keys"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxKeys = n
		}
	}

	start := 0
	if !f.StuckMarker {
		if marker := query.Get("marker"); marker != "" {
			// V1 語意：只回傳 key 嚴格大於 marker 的物件。entries 已排序，用線性找出第一個 > marker 的位置即可。
			for i, e := range entries {
				if e.key > marker {
					start = i
					break
				}
				start = i + 1
			}
		}
	}

	end := start + maxKeys
	truncated := end < len(entries)
	if f.StuckMarker {
		// 模擬異常伺服器：不論請求帶什麼 marker，一律回第一頁且宣稱還沒結束。
		truncated = true
	}
	if end > len(entries) {
		end = len(entries)
	}
	page := entries[start:end]

	var buf strings.Builder
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	buf.WriteString(`<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">`)
	fmt.Fprintf(&buf, `<Name>%s</Name><Prefix>%s</Prefix><Marker>%s</Marker><MaxKeys>%d</MaxKeys><IsTruncated>%t</IsTruncated>`,
		xmlEscape(bucket), xmlEscape(prefix), xmlEscape(query.Get("marker")), maxKeys, truncated)
	if truncated && len(page) > 0 {
		if f.StuckMarker {
			fmt.Fprintf(&buf, `<NextMarker>%s</NextMarker>`, xmlEscape(page[0].key))
		} else if f.EmitNextMarker {
			fmt.Fprintf(&buf, `<NextMarker>%s</NextMarker>`, xmlEscape(page[len(page)-1].key))
		}
	}
	for _, e := range page {
		fmt.Fprintf(&buf, `<Contents><Key>%s</Key><LastModified>%s</LastModified><ETag>%s</ETag><Size>%d</Size><StorageClass>STANDARD</StorageClass></Contents>`,
			xmlEscape(e.key), e.mod.UTC().Format(time.RFC3339Nano), xmlEscape(e.etag), e.size)
	}
	buf.WriteString(`</ListBucketResult>`)

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, buf.String())
}

// fakeS3VersionEntry 是 listObjectVersions 分頁用的扁平化單筆版本／delete marker，
// 已從 fakeS3Bucket.versions 展開成單一有序序列（見 listObjectVersions 註解）。
type fakeS3VersionEntry struct {
	key          string
	versionID    string
	isLatest     bool
	deleteMarker bool
	mod          time.Time
	etag         string
	size         int64
}

// allVersionEntries 在單一次持鎖內完整快照 bucket 底下所有版本（含 delete marker，
// 已依 Key 排序、同 key 內新→舊展開成單一序列，且已解析出渲染需要的欄位），
// 供 listObjectVersions 使用；理由與 liveObjectEntries 相同：避免「先鎖一次列出 key、
// 解鎖、再鎖一次展開版本」之間留下可能被並發寫入影響的空窗。
func (f *FakeS3) allVersionEntries(bucket string) (entries []fakeS3VersionEntry, ok bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, exists := f.buckets[bucket]
	if !exists {
		return nil, false
	}
	keys := make([]string, 0, len(b.versions))
	for k := range b.versions {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		vs := b.versions[k]
		for i := len(vs) - 1; i >= 0; i-- { // 新 → 舊
			v := vs[i]
			entries = append(entries, fakeS3VersionEntry{
				key: k, versionID: v.versionID, isLatest: i == len(vs)-1,
				deleteMarker: v.deleteMarker, mod: v.mod, etag: v.etag, size: objectSize(v),
			})
		}
	}
	return entries, true
}

// listObjectVersions 對應 `GET /{bucket}?versions[&prefix=]`（ListObjectVersions）：
// 依 key 排序，每個 key 底下的版本由新到舊各輸出一筆 <Version> 或 <DeleteMarker>。
// 分頁單位是「版本」而非 key：先把所有版本攤平成單一序列（依 Key 排序、同 key 內新→舊），
// 再用 max-keys（預設 f.MaxKeys）切頁，讓單一 key 的版本數超過一頁時也能正確拆頁
// （真實 Ceph 對超過一頁的單一 key 就是這樣：NextKeyMarker 維持同一個 key、只有
// NextVersionIdMarker 前進，見 internal/twai/cos.S3.ListObjectVersions 的分頁邏輯）。
// key-marker + version-id-marker 一起定位到「上一頁最後一筆」之後接續；未結束時回
// <IsTruncated>true</IsTruncated>，是否附帶 <NextKeyMarker>／<NextVersionIdMarker>
// 依 EmitNextMarker（模擬 AWS 嚴格不回 vs. Ceph 會回）而定；StuckMarker 為 true 時
// 則無視請求 marker、固定回第一頁並附上不會前進的兩個 marker。
//
// key-marker 沒有搭配 version-id-marker 時：依 S3 語意視為「這個 key 已經看完了」，
// 略過該 key 的全部版本、從下一個 key 開始（而不是去比對某個特定版本再接續）。
func (f *FakeS3) listObjectVersions(w http.ResponseWriter, bucket string, query url.Values) {
	entries, ok := f.allVersionEntries(bucket)
	if !ok {
		writeXMLError(w, http.StatusNotFound, "NoSuchBucket", "指定的 bucket 不存在")
		return
	}

	prefix := query.Get("prefix")
	if prefix != "" {
		filtered := entries[:0:0]
		for _, e := range entries {
			if strings.HasPrefix(e.key, prefix) {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}

	maxKeys := f.MaxKeys
	if v := query.Get("max-keys"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxKeys = n
		}
	}

	start := 0
	if !f.StuckMarker {
		if keyMarker := query.Get("key-marker"); keyMarker != "" {
			versionMarker := query.Get("version-id-marker")
			if versionMarker == "" {
				// 只帶 key-marker、沒有 version-id-marker：S3 語意是「這個 key 已經看完
				// 了」，必須略過該 key 的全部版本，從下一個 key 開始；不能沿用下面
				// 「比對特定 (key, versionID)」的邏輯，那種比對方式在沒有
				// version-id-marker（版本 id 永遠是空字串）時只會匹配到「versionID 也剛好
				// 是空字串」的版本，一般情況下找不到任何符合項，等於 key-marker 完全沒生效。
				for i, e := range entries {
					if e.key > keyMarker {
						start = i
						break
					}
					start = i + 1
				}
			} else {
				found := false
				for i, e := range entries {
					if e.key == keyMarker && e.versionID == versionMarker {
						start = i + 1
						found = true
						break
					}
				}
				if !found {
					// 真實 S3／Ceph 對指到一個不存在（或已刪除）的 (key, versionID)
					// 一律回 400 InvalidArgument，不會悄悄從頭開始：呼叫端若把「上一頁最後
					// 一筆」當成下一頁的 version-id-marker，那個版本必須在送出這個請求之前
					// 仍然存在（見 internal/cli 兩段式刪除對這個限制的因應：cosStreamDeleteVersionTargets
					// 用「延後一筆」的 lookbehind，確保被拿來當 marker 的版本在被拿去 list 下一頁
					// 之前不會被刪除）。
					writeXMLError(w, http.StatusBadRequest, "InvalidArgument",
						"指定的 version-id-marker 在目前的物件版本清單中不存在（可能已被刪除）")
					return
				}
			}
		}
	}

	end := start + maxKeys
	truncated := end < len(entries)
	if f.StuckMarker {
		truncated = true
	}
	if end > len(entries) {
		end = len(entries)
	}
	page := entries[start:end]

	var buf strings.Builder
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	buf.WriteString(`<ListVersionsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">`)
	fmt.Fprintf(&buf, `<Name>%s</Name><Prefix>%s</Prefix><KeyMarker>%s</KeyMarker><MaxKeys>%d</MaxKeys><IsTruncated>%t</IsTruncated>`,
		xmlEscape(bucket), xmlEscape(prefix), xmlEscape(query.Get("key-marker")), maxKeys, truncated)
	if truncated && len(page) > 0 {
		if f.StuckMarker {
			fmt.Fprintf(&buf, `<NextKeyMarker>%s</NextKeyMarker><NextVersionIdMarker>%s</NextVersionIdMarker>`,
				xmlEscape(page[0].key), xmlEscape(page[0].versionID))
		} else if f.EmitNextMarker {
			last := page[len(page)-1]
			fmt.Fprintf(&buf, `<NextKeyMarker>%s</NextKeyMarker><NextVersionIdMarker>%s</NextVersionIdMarker>`,
				xmlEscape(last.key), xmlEscape(last.versionID))
		}
	}

	for _, e := range page {
		if e.deleteMarker {
			fmt.Fprintf(&buf, `<DeleteMarker><Key>%s</Key><VersionId>%s</VersionId><IsLatest>%t</IsLatest><LastModified>%s</LastModified></DeleteMarker>`,
				xmlEscape(e.key), xmlEscape(e.versionID), e.isLatest, e.mod.UTC().Format(time.RFC3339Nano))
			continue
		}
		fmt.Fprintf(&buf, `<Version><Key>%s</Key><VersionId>%s</VersionId><IsLatest>%t</IsLatest><LastModified>%s</LastModified><ETag>%s</ETag><Size>%d</Size><StorageClass>STANDARD</StorageClass></Version>`,
			xmlEscape(e.key), xmlEscape(e.versionID), e.isLatest, e.mod.UTC().Format(time.RFC3339Nano), xmlEscape(e.etag), e.size)
	}
	buf.WriteString(`</ListVersionsResult>`)

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, buf.String())
}

// handleObject 依 query string 與是否帶 x-amz-copy-source 分派物件層級的操作。
// 分派順序：?acl（GET/PUT）→ ?uploads → ?uploadId → 帶 x-amz-copy-source 的 PUT
// （CopyObject）→ 一般 PUT/GET/HEAD/DELETE；順序很重要，因為 ?acl／?uploads 等
// query 與一般 PUT/GET 用的是同一組 method，必須先攔截特殊 query 才會落到預設分支。
func (f *FakeS3) handleObject(w http.ResponseWriter, r *http.Request, bucket, key string, body []byte) {
	query := r.URL.Query()
	switch {
	case query.Has("acl") && r.Method == http.MethodGet:
		f.getObjectACL(w, bucket, key)
	case query.Has("acl") && r.Method == http.MethodPut:
		f.putObjectACL(w, bucket, key, r.Header.Get("x-amz-acl"))
	case r.Method == http.MethodPost && query.Has("uploads"):
		f.initiateMultipartUpload(w, bucket, key, r.Header)
	case r.Method == http.MethodPut && query.Has("uploadId") && query.Has("partNumber"):
		f.uploadPart(w, query, body)
	case r.Method == http.MethodPost && query.Has("uploadId"):
		f.completeMultipartUpload(w, bucket, key, query.Get("uploadId"), body)
	case r.Method == http.MethodPut && r.Header.Get("x-amz-copy-source") != "":
		f.copyObject(w, r, bucket, key)
	case r.Method == http.MethodPut:
		f.putObject(w, bucket, key, body, r.Header)
	case r.Method == http.MethodGet:
		f.getObject(w, bucket, key, false)
	case r.Method == http.MethodHead:
		f.getObject(w, bucket, key, true)
	case r.Method == http.MethodDelete:
		f.deleteObject(w, bucket, key, query)
	default:
		writeXMLError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "不支援的方法")
	}
}

// putObject 對應 `PUT /{bucket}/{key}`；與測試準備用的 PutObject（自動建立 bucket）不同，
// HTTP 入口對不存在的 bucket 一律回 404 NoSuchBucket，符合真實 S3 行為。
// Content-Type 為空時使用預設值 "application/octet-stream"（newFakeS3Object 已設定）；
// 同時記錄 trackedHeaders 中請求帶了值的系統 metadata（供 SetContentType 的
// REPLACE 語意測試使用）。
func (f *FakeS3) putObject(w http.ResponseWriter, bucket, key string, body []byte, header http.Header) {
	f.mu.Lock()
	b, ok := f.buckets[bucket]
	if !ok {
		f.mu.Unlock()
		writeXMLError(w, http.StatusNotFound, "NoSuchBucket", "指定的 bucket 不存在")
		return
	}
	obj := newFakeS3Object(body)
	if contentType := header.Get("Content-Type"); contentType != "" {
		obj.contentType = contentType
	}
	obj.headers = extractHeaders(header)
	b.putLocked(key, obj)
	f.mu.Unlock()

	w.Header().Set("ETag", obj.etag)
	w.WriteHeader(http.StatusOK)
}

func (f *FakeS3) getObject(w http.ResponseWriter, bucket, key string, headOnly bool) {
	f.mu.Lock()
	b, ok := f.buckets[bucket]
	var obj *fakeS3Object
	if ok {
		obj, ok = b.current(key)
	}
	f.mu.Unlock()

	if !ok {
		writeXMLError(w, http.StatusNotFound, "NoSuchKey", "指定的 key 不存在")
		return
	}

	// HEAD 用 sizeOverride（若有設定，見 PutObjectWithSize）回報邏輯大小，避免必須真的
	// 配置對應大小的記憶體；GET 一律用實際內容長度，因為緊接著會把 obj.body 整個寫出，
	// Content-Length 必須與實際寫出的位元組數一致。
	size := int64(len(obj.body))
	if headOnly {
		size = objectSize(obj)
	}
	w.Header().Set("ETag", obj.etag)
	w.Header().Set("Last-Modified", obj.mod.UTC().Format(http.TimeFormat))
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	w.Header().Set("Content-Type", obj.contentType)
	for name, value := range obj.headers {
		w.Header().Set(name, value)
	}
	w.WriteHeader(http.StatusOK)
	if !headOnly {
		_, _ = w.Write(obj.body)
	}
}

// getObjectACL 對應 `GET /{bucket}/{key}?acl`（GetObjectAcl）：永遠回傳 owner 的
// FULL_CONTROL grant，物件目前是 public-read 時額外多一個 AllUsers 群組的 READ grant。
func (f *FakeS3) getObjectACL(w http.ResponseWriter, bucket, key string) {
	f.mu.Lock()
	b, ok := f.buckets[bucket]
	var obj *fakeS3Object
	if ok {
		obj, ok = b.current(key)
	}
	f.mu.Unlock()
	if !ok {
		writeXMLError(w, http.StatusNotFound, "NoSuchKey", "指定的 key 不存在")
		return
	}

	var buf strings.Builder
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	buf.WriteString(`<AccessControlPolicy xmlns="http://s3.amazonaws.com/doc/2006-03-01/">`)
	buf.WriteString(`<Owner><ID>owner</ID></Owner><AccessControlList>`)
	buf.WriteString(`<Grant><Grantee xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:type="CanonicalUser"><ID>owner</ID></Grantee><Permission>FULL_CONTROL</Permission></Grant>`)
	if obj.acl == "public-read" {
		fmt.Fprintf(&buf, `<Grant><Grantee xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:type="Group"><URI>%s</URI></Grantee><Permission>READ</Permission></Grant>`,
			xmlEscape(allUsersGroupURI))
	}
	buf.WriteString(`</AccessControlList></AccessControlPolicy>`)

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, buf.String())
}

// putObjectACL 對應 `PUT /{bucket}/{key}?acl`（PutObjectAcl）：以 x-amz-acl header
// 值更新目前最新版本的 canned ACL（缺省或非 "public-read" 一律視為 "private"）。
func (f *FakeS3) putObjectACL(w http.ResponseWriter, bucket, key, xAmzACL string) {
	acl := "private"
	if xAmzACL == "public-read" {
		acl = "public-read"
	}

	f.mu.Lock()
	b, ok := f.buckets[bucket]
	var obj *fakeS3Object
	if ok {
		obj, ok = b.current(key)
	}
	if ok {
		obj.acl = acl
	}
	f.mu.Unlock()

	if !ok {
		writeXMLError(w, http.StatusNotFound, "NoSuchKey", "指定的 key 不存在")
		return
	}
	w.WriteHeader(http.StatusOK)
}

// copyObject 對應帶 `x-amz-copy-source` header 的 `PUT /{bucket}/{key}`（CopyObject）。
// CopySource header 的值可能是 "/src-bucket/src-key" 或 "src-bucket/src-key"，且已
// URL escape（例如 key 中的 "/" 可能寫成 %2F 或原樣保留），因此先 PathUnescape 再去掉
// 開頭的 "/"，以第一個 "/" 切出 bucket 與 key。
func (f *FakeS3) copyObject(w http.ResponseWriter, r *http.Request, dstBucket, dstKey string) {
	raw := r.Header.Get("x-amz-copy-source")
	unescaped, err := url.PathUnescape(raw)
	if err != nil {
		unescaped = raw
	}
	srcBucket, srcKey, hasKey := strings.Cut(strings.TrimPrefix(unescaped, "/"), "/")
	if !hasKey {
		writeXMLError(w, http.StatusBadRequest, "InvalidArgument", "x-amz-copy-source 格式錯誤")
		return
	}

	f.mu.Lock()
	srcB, ok := f.buckets[srcBucket]
	var src *fakeS3Object
	if ok {
		src, ok = srcB.current(srcKey)
	}
	if !ok {
		f.mu.Unlock()
		writeXMLError(w, http.StatusNotFound, "NoSuchKey", "指定的來源 key 不存在")
		return
	}

	dstB, ok := f.buckets[dstBucket]
	if !ok {
		f.mu.Unlock()
		writeXMLError(w, http.StatusNotFound, "NoSuchBucket", "指定的 bucket 不存在")
		return
	}

	obj := newFakeS3Object(src.body)
	obj.contentType = src.contentType
	obj.headers = cloneHeaders(src.headers)
	if r.Header.Get("x-amz-metadata-directive") == "REPLACE" {
		// REPLACE：完全以請求帶的 metadata 取代（不是與來源合併），與真實 S3 一致——
		// 沒有在請求中重新帶入的系統 metadata（Cache-Control、Content-Encoding 等）會消失。
		if ct := r.Header.Get("Content-Type"); ct != "" {
			obj.contentType = ct
		}
		obj.headers = extractHeaders(r.Header)
	}
	obj.acl = "private"
	if r.Header.Get("x-amz-acl") == "public-read" {
		obj.acl = "public-read"
	}
	dstB.putLocked(dstKey, obj)
	f.mu.Unlock()

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><CopyObjectResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><ETag>%s</ETag><LastModified>%s</LastModified></CopyObjectResult>`,
		xmlEscape(obj.etag), obj.mod.UTC().Format(time.RFC3339Nano))
}

// deleteObject 對應 `DELETE /{bucket}/{key}[?versionId=]`：
//   - 帶 versionId：從該 key 的版本歷史中移除指定版本（找不到也回 204，與 S3 行為一致），
//     移除後該 key 若沒有剩餘版本則整筆從 versions 移除。
//   - 不帶 versionId：versioning Enabled／Suspended 時透過 putLocked 附加（Enabled）或取代既有
//     null 版本（Suspended）一個 delete marker，帶真正 version id 的既有版本不受影響；
//     從未設定過版本控制時直接清除該 key 的所有版本歷史。bucket 不存在一律回 404。
func (f *FakeS3) deleteObject(w http.ResponseWriter, bucket, key string, query url.Values) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.buckets[bucket]
	if !ok {
		writeXMLError(w, http.StatusNotFound, "NoSuchBucket", "指定的 bucket 不存在")
		return
	}

	if versionID := query.Get("versionId"); versionID != "" {
		vs := b.versions[key]
		filtered := vs[:0:0]
		for _, v := range vs {
			if v.versionID != versionID {
				filtered = append(filtered, v)
			}
		}
		if len(filtered) == 0 {
			delete(b.versions, key)
		} else {
			b.versions[key] = filtered
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	switch b.versioning {
	case "Enabled", "Suspended":
		marker := &fakeS3Object{mod: monotonicNow(), deleteMarker: true}
		b.putLocked(key, marker)
	default:
		delete(b.versions, key)
	}
	w.WriteHeader(http.StatusNoContent)
}

// initiateMultipartUpload 對應 `POST /{bucket}/{key}?uploads`（CreateMultipartUpload）；
// contentType 是請求帶的 Content-Type header，套用到 CompleteMultipartUpload 合併出的物件。
func (f *FakeS3) initiateMultipartUpload(w http.ResponseWriter, bucket, key string, header http.Header) {
	f.mu.Lock()
	if _, ok := f.buckets[bucket]; !ok {
		f.mu.Unlock()
		writeXMLError(w, http.StatusNotFound, "NoSuchBucket", "指定的 bucket 不存在")
		return
	}
	uploadID := fmt.Sprintf("upload-%d", len(f.uploads)+1)
	f.uploads[uploadID] = &fakeS3Upload{bucket: bucket, key: key, parts: map[int][]byte{}, contentType: header.Get("Content-Type")}
	f.mu.Unlock()

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><InitiateMultipartUploadResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Bucket>%s</Bucket><Key>%s</Key><UploadId>%s</UploadId></InitiateMultipartUploadResult>`,
		xmlEscape(bucket), xmlEscape(key), xmlEscape(uploadID))
}

// uploadPart 對應 `PUT /{bucket}/{key}?partNumber=&uploadId=`（UploadPart）。
func (f *FakeS3) uploadPart(w http.ResponseWriter, query url.Values, body []byte) {
	uploadID := query.Get("uploadId")
	partNumber, err := strconv.Atoi(query.Get("partNumber"))
	if err != nil {
		writeXMLError(w, http.StatusBadRequest, "InvalidArgument", "partNumber 必須是整數")
		return
	}

	f.mu.Lock()
	upload, ok := f.uploads[uploadID]
	if ok {
		upload.parts[partNumber] = append([]byte(nil), body...)
	}
	f.mu.Unlock()

	if !ok {
		writeXMLError(w, http.StatusNotFound, "NoSuchUpload", "指定的 multipart upload 不存在")
		return
	}
	w.Header().Set("ETag", etagFor(body))
	w.WriteHeader(http.StatusOK)
}

// completeMultipartUpload 對應 `POST /{bucket}/{key}?uploadId=`（CompleteMultipartUpload）：
// 依 part number 排序後串接所有分片內容，寫入為一個一般物件。
func (f *FakeS3) completeMultipartUpload(w http.ResponseWriter, bucket, key, uploadID string, _ []byte) {
	f.mu.Lock()
	upload, ok := f.uploads[uploadID]
	if !ok {
		f.mu.Unlock()
		writeXMLError(w, http.StatusNotFound, "NoSuchUpload", "指定的 multipart upload 不存在")
		return
	}
	delete(f.uploads, uploadID)

	b, ok := f.buckets[bucket]
	if !ok {
		f.mu.Unlock()
		writeXMLError(w, http.StatusNotFound, "NoSuchBucket", "指定的 bucket 不存在")
		return
	}

	numbers := make([]int, 0, len(upload.parts))
	for n := range upload.parts {
		numbers = append(numbers, n)
	}
	sort.Ints(numbers)

	var combined []byte
	for _, n := range numbers {
		combined = append(combined, upload.parts[n]...)
	}

	obj := newFakeS3Object(combined)
	if upload.contentType != "" {
		obj.contentType = upload.contentType
	}
	b.putLocked(key, obj)
	f.mu.Unlock()

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><CompleteMultipartUploadResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Bucket>%s</Bucket><Key>%s</Key><ETag>%s</ETag></CompleteMultipartUploadResult>`,
		xmlEscape(bucket), xmlEscape(key), xmlEscape(obj.etag))
}
