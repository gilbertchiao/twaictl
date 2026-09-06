package cos

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithy "github.com/aws/smithy-go"

	"github.com/gilbertchiao/twaictl/internal/twai"
)

// defaultPartSize 是 S3Options.PartSize 為 0 時的預設分片大小（16 MiB），
// 交由 transfermanager 決定何時改用 multipart 上傳。
const defaultPartSize = 16 * 1024 * 1024

// minPartSize 是 S3 / Ceph RGW 對 multipart 分片大小的下限（5 MiB）：
// 除最後一片外，每個分片都必須至少這麼大，否則 CompleteMultipartUpload 會失敗。
// 見 https://docs.aws.amazon.com/AmazonS3/latest/userguide/qfacts.html。
const minPartSize = 5 * 1024 * 1024

// tempSuffix 是 Download 下載中暫存檔的副檔名；完成後才 rename 成正式檔名，
// 避免下載到一半失敗時留下不完整的檔案。
const tempSuffix = ".twaictl-part"

// s3Region 是呼叫 Ceph S3 相容端點時固定填入的 region；Ceph 不驗證此值，
// 但 SigV4 簽章計算需要一個非空字串。
const s3Region = "us-east-1"

// retryMaxAttempts 是 NewS3 明確設定的 SDK 重試次數上限。
//
// S3Options.Timeout 只涵蓋單一次嘗試的逾時（http.Client.Timeout 是每個 RoundTrip 各自計時，
// 不是整個呼叫的總時限）；aws-sdk-go-v2 預設在可重試錯誤（連線失敗、逾時、5xx 等）時最多
// 重試到「預設重試次數」，若不在這裡固定住，使用者從 --timeout 推算出的等待時間上限
// 會不準確（例如 --timeout 1s 實際可能等到約 5s 才真正失敗）。固定為 3 次後，
// 一次呼叫最長可能耗時約 3×Timeout 加上重試之間的退避時間。
const retryMaxAttempts = 3

// S3Options 是 NewS3 的參數。
type S3Options struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	// Timeout 是每次嘗試（單一次 HTTP RoundTrip）的逾時；NewS3 固定重試最多 3 次
	// （見 retryMaxAttempts），因此單次呼叫最長可能耗時約 3×Timeout 加上重試間的退避時間，
	// 並非單純的 Timeout。
	Timeout  time.Duration
	PartSize int64 // 0 時使用 defaultPartSize（16 MiB）
}

// S3 封裝 S3 相容資料面操作（bucket / object）：SigV4 簽章、path-style 定址。
type S3 struct {
	Client   *s3.Client
	Transfer *transfermanager.Client
}

// NewS3 依 opts 建立 S3 封裝；endpoint 必須是 http(s):// 開頭的完整 URL，access/secret key 不可為空。
func NewS3(opts S3Options) (*S3, error) {
	u, err := url.Parse(opts.Endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("cos endpoint 必須是 http(s):// 開頭的完整 URL")
	}
	if opts.AccessKey == "" || opts.SecretKey == "" {
		return nil, fmt.Errorf("cos access key / secret key 不可為空")
	}
	partSize := opts.PartSize
	if partSize == 0 {
		partSize = defaultPartSize
	} else if partSize < minPartSize {
		return nil, fmt.Errorf("PartSize 至少需為 5 MiB（目前 %d bytes）", partSize)
	}

	client := s3.New(s3.Options{
		Region:                     s3Region,
		Credentials:                credentials.NewStaticCredentialsProvider(opts.AccessKey, opts.SecretKey, ""),
		BaseEndpoint:               aws.String(opts.Endpoint),
		UsePathStyle:               true,
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
		// 用 SDK 的 BuildableClient 而非裸 http.Client：BuildableClient 帶 SDK 預設 transport 與
		// limitedRedirect（只跟隨 307/308，301/302 一律不跟隨、且不會把簽章帶到別的 host）；
		// 裸 http.Client 會沿用 Go 預設「跟隨最多 10 次」的政策。Timeout 語意不變
		// （每次嘗試各自計時，見 retryMaxAttempts 說明）。
		HTTPClient:       awshttp.NewBuildableClient().WithTimeout(opts.Timeout),
		RetryMaxAttempts: retryMaxAttempts,
	})
	transfer := transfermanager.New(client, func(o *transfermanager.Options) {
		o.PartSizeBytes = partSize
		// transfermanager 的 multipart 門檻（MultipartUploadThreshold）與分片大小是兩個獨立設定，
		// 預設門檻固定 16 MiB、不會跟著 PartSizeBytes 調整。S3Options 只暴露一個 PartSize 給
		// 呼叫端，因此這裡讓門檻等於分片大小：只要檔案超過一個分片，就會走 multipart。
		o.MultipartUploadThreshold = partSize
		// transfermanager.Options 有自己的一份 RequestChecksumCalculation，預設值
		// WhenSupported（會自動加上 CRC32 trailer），不會沿用上面 s3.Options 設的
		// WhenRequired；兩邊不一致的話，一般的 s3.Client 呼叫（ListBuckets 等）不送
		// checksum，但 Upload 卻會送，Ceph RGW 不一定支援 CRC32 trailer。這裡明確設成
		// WhenRequired，讓兩個 client 的行為一致。
		o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
	})
	return &S3{Client: client, Transfer: transfer}, nil
}

// Bucket 是簡化後的 bucket 中繼資料。
type Bucket struct {
	Name    string
	Created time.Time
}

// ListBuckets 列出所有 bucket。
func (c *S3) ListBuckets(ctx context.Context) ([]Bucket, error) {
	out, err := c.Client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return nil, ClassifyS3Error(err)
	}
	buckets := make([]Bucket, 0, len(out.Buckets))
	for _, b := range out.Buckets {
		var bucket Bucket
		if b.Name != nil {
			bucket.Name = *b.Name
		}
		if b.CreationDate != nil {
			bucket.Created = *b.CreationDate
		}
		buckets = append(buckets, bucket)
	}
	return buckets, nil
}

// CreateBucket 建立 bucket；已存在時回傳 *twai.APIError（409 BucketAlreadyOwnedByYou）。
func (c *S3) CreateBucket(ctx context.Context, name string) error {
	if _, err := c.Client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: &name}); err != nil {
		return ClassifyS3Error(err)
	}
	return nil
}

// DeleteBucket 刪除 bucket；bucket 非空時回傳 *twai.APIError（409 BucketNotEmpty）。
func (c *S3) DeleteBucket(ctx context.Context, name string) error {
	if _, err := c.Client.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: &name}); err != nil {
		return ClassifyS3Error(err)
	}
	return nil
}

// SetVersioning 開關 bucket 的版本控制。
func (c *S3) SetVersioning(ctx context.Context, name string, enabled bool) error {
	status := types.BucketVersioningStatusSuspended
	if enabled {
		status = types.BucketVersioningStatusEnabled
	}
	_, err := c.Client.PutBucketVersioning(ctx, &s3.PutBucketVersioningInput{
		Bucket:                  &name,
		VersioningConfiguration: &types.VersioningConfiguration{Status: status},
	})
	if err != nil {
		return ClassifyS3Error(err)
	}
	return nil
}

// Object 是簡化後的物件中繼資料。
type Object struct {
	Key          string
	Size         int64
	LastModified time.Time
	ETag         string
}

// ListObjects 用 V1 ListObjects（`GET /{bucket}?marker=`）搭配 Marker 手動分頁列出 bucket 底下
// （可選 prefix）的物件，依序逐一呼叫 fn；fn 回傳錯誤時立即中止並回傳該錯誤（不再取下一頁）。
//
// 不用 V2 ListObjectsV2（及其 s3.NewListObjectsV2Paginator）的原因：2026-08-27 對
// cos.twcc.ai 真實環境實測發現，Ceph RGW 對 ListObjectsV2 回應 IsTruncated=true 但
// NextContinuationToken 為空、KeyCount=0，導致 aws-sdk-go-v2 的 paginator 誤判為已到最後一頁，
// 只取得第一頁（1000 筆）就停止；四個真實 bucket（5.7 萬～1,242 萬個物件）都因此只列出 1000 筆。
// V1 ListObjects 的分頁在同一組 bucket 上驗證正常（IsTruncated=true 且 NextMarker 有值），
// 詳見 docs/api-notes.md「Phase 2a 實測」。
//
// 下一頁的 Marker 依序嘗試：(1) 本頁回應的 NextMarker（Ceph 會回，AWS S3 在沒有 delimiter
// 時不回）；(2) 本頁最後一個 Contents[].Key（AWS S3 的標準退路）。若計算出的 Marker 與上一頁
// 相同（表示分頁沒有前進），視為異常並回傳錯誤，避免無限迴圈。
func (c *S3) ListObjects(ctx context.Context, bucket, prefix string, fn func(Object) error) error {
	var marker *string
	for {
		input := &s3.ListObjectsInput{Bucket: &bucket, Marker: marker}
		if prefix != "" {
			input.Prefix = &prefix
		}
		page, err := c.Client.ListObjects(ctx, input)
		if err != nil {
			return ClassifyS3Error(err)
		}
		if len(page.Contents) == 0 {
			return nil
		}
		for _, obj := range page.Contents {
			var o Object
			if obj.Key != nil {
				o.Key = *obj.Key
			}
			if obj.Size != nil {
				o.Size = *obj.Size
			}
			if obj.LastModified != nil {
				o.LastModified = *obj.LastModified
			}
			if obj.ETag != nil {
				o.ETag = *obj.ETag
			}
			if err := fn(o); err != nil {
				return err
			}
		}
		if !aws.ToBool(page.IsTruncated) {
			return nil
		}
		nextMarker := aws.ToString(page.NextMarker)
		if nextMarker == "" {
			nextMarker = aws.ToString(page.Contents[len(page.Contents)-1].Key)
		}
		if marker != nil && nextMarker == *marker {
			return fmt.Errorf("ListObjects 分頁沒有進展（marker %q）", nextMarker)
		}
		marker = &nextMarker
	}
}

// Upload 把本地檔案上傳到 bucket/key；超過 S3Options.PartSize 時 transfermanager 自動改用 multipart
// （見套件文件關於 multipart 假 server 的說明：Phase 1b 已驗證真正的 multipart 上傳流程）。
func (c *S3) Upload(ctx context.Context, bucket, key, localPath string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("開啟檔案 %s 失敗: %w", localPath, err)
	}
	defer func() { _ = f.Close() }()

	contentType := ContentTypeForPath(localPath)
	if _, err := c.Transfer.UploadObject(ctx, &transfermanager.UploadObjectInput{
		Bucket:      &bucket,
		Key:         &key,
		Body:        f,
		ContentType: aws.String(contentType),
	}); err != nil {
		return ClassifyS3Error(err)
	}
	return nil
}

// Download 把 bucket/key 下載到 localPath：先寫入 localPath+".twaictl-part"，成功後才 rename
// 成正式檔名，避免下載到一半失敗時留下不完整的檔案；localPath 的父目錄不存在時會自動建立。
// 暫存檔以 O_EXCL 建立，若該路徑已存在（不論是一般檔案還是 symlink）一律直接失敗，
// 不會跟隨既有 symlink 寫入／截斷它指向的檔案。
func (c *S3) Download(ctx context.Context, bucket, key, localPath string) error {
	out, err := c.Client.GetObject(ctx, &s3.GetObjectInput{Bucket: &bucket, Key: &key})
	if err != nil {
		return ClassifyS3Error(err)
	}
	defer func() { _ = out.Body.Close() }()

	if dir := filepath.Dir(localPath); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("建立目錄 %s 失敗: %w", dir, err)
		}
	}

	// 用 O_EXCL 建立暫存檔：若 tmpPath 已存在（不管是一般檔案還是 symlink）都會直接失敗，
	// 不會跟隨既有的 symlink 寫入／截斷它指向的檔案。os.Create 底層是 O_TRUNC，若 tmpPath
	// 剛好是攻擊者（或殘留）預先放置、指向系統外部檔案的 symlink，會直接截斷該檔案。
	tmpPath := localPath + tempSuffix
	tmp, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("暫存檔 %s 已存在（可能是上次中斷的下載），請先移除", tmpPath)
		}
		return fmt.Errorf("建立暫存檔 %s 失敗: %w", tmpPath, err)
	}

	if _, err := io.Copy(tmp, out.Body); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("下載 cos://%s/%s 失敗: %w", bucket, key, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("關閉暫存檔 %s 失敗: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, localPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("重新命名暫存檔 %s 失敗: %w", tmpPath, err)
	}
	return nil
}

// DeleteObject 刪除單一物件。
func (c *S3) DeleteObject(ctx context.Context, bucket, key string) error {
	if _, err := c.Client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: &bucket, Key: &key}); err != nil {
		return ClassifyS3Error(err)
	}
	return nil
}

// ObjectVersion 是 ListObjectVersions 回傳的單一物件版本或 delete marker。
// unversioned bucket 的每個物件 VersionID 固定為 "null"（S3 語意）。
type ObjectVersion struct {
	Key          string
	VersionID    string
	IsLatest     bool
	DeleteMarker bool // true 時 Size、ETag 為零值
	Size         int64
	LastModified time.Time
	ETag         string
}

// ListObjectVersions 手動以 KeyMarker／VersionIdMarker 分頁列出 bucket 底下（可選 prefix）的
// 所有物件版本與 delete marker，依 S3 回傳順序（同一 key 由新到舊）逐一呼叫 fn；
// fn 回傳錯誤時立即中止並回傳該錯誤。未開啟 versioning 的 bucket 也能列出
// （每個物件一筆、VersionID 為 "null"），因此 `cos bucket rm --purge` 對兩種 bucket 都能用同一條路徑清空。
//
// 不用 s3.NewListObjectVersionsPaginator：Ceph RGW 在 IsTruncated=true 但 NextKeyMarker 為空
// 時會導致該 paginator 無限重送第一頁（同 ListObjects 改手動迴圈的原因，見 handwritten_paginators.go），
// 因此改為手動迴圈並比照 ListObjects 加上「沒有進展」防呆。
func (c *S3) ListObjectVersions(ctx context.Context, bucket, prefix string, fn func(ObjectVersion) error) error {
	var keyMarker, versionIDMarker *string
	for {
		input := &s3.ListObjectVersionsInput{Bucket: &bucket, KeyMarker: keyMarker, VersionIdMarker: versionIDMarker}
		if prefix != "" {
			input.Prefix = &prefix
		}
		page, err := c.Client.ListObjectVersions(ctx, input)
		if err != nil {
			return ClassifyS3Error(err)
		}
		if len(page.Versions) == 0 && len(page.DeleteMarkers) == 0 {
			return nil
		}
		// 同一頁內 S3 會把 Versions 與 DeleteMarkers 分成兩個陣列回傳，各自已依 key / 新舊排序；
		// 這裡先合併再依 (Key, LastModified 由新到舊) 排序，讓呼叫端拿到單一有序序列。
		versions := make([]ObjectVersion, 0, len(page.Versions)+len(page.DeleteMarkers))
		for _, v := range page.Versions {
			versions = append(versions, ObjectVersion{
				Key: aws.ToString(v.Key), VersionID: aws.ToString(v.VersionId),
				IsLatest: aws.ToBool(v.IsLatest), Size: aws.ToInt64(v.Size),
				LastModified: aws.ToTime(v.LastModified), ETag: aws.ToString(v.ETag),
			})
		}
		for _, m := range page.DeleteMarkers {
			versions = append(versions, ObjectVersion{
				Key: aws.ToString(m.Key), VersionID: aws.ToString(m.VersionId),
				IsLatest: aws.ToBool(m.IsLatest), DeleteMarker: true,
				LastModified: aws.ToTime(m.LastModified),
			})
		}
		sort.SliceStable(versions, func(i, j int) bool {
			if versions[i].Key != versions[j].Key {
				return versions[i].Key < versions[j].Key
			}
			return versions[i].LastModified.After(versions[j].LastModified)
		})
		for _, v := range versions {
			if err := fn(v); err != nil {
				return err
			}
		}
		if !aws.ToBool(page.IsTruncated) {
			return nil
		}
		nextKeyMarker := aws.ToString(page.NextKeyMarker)
		nextVersionIDMarker := aws.ToString(page.NextVersionIdMarker)
		if nextKeyMarker == "" || nextVersionIDMarker == "" {
			// Ceph 有時不回 NextKeyMarker／NextVersionIdMarker（實測甚至只回前者、後者固定空字串）：
			// 兩者分別獨立退回本頁（已排序）最後一筆的 Key／VersionID 當下一頁 marker，
			// 與 ListObjects 對 NextMarker 的退路邏輯一致。
			last := versions[len(versions)-1]
			if nextKeyMarker == "" {
				nextKeyMarker = last.Key
			}
			if nextVersionIDMarker == "" {
				nextVersionIDMarker = last.VersionID
			}
		}
		// 單一 key 的版本數超過一頁時，NextKeyMarker 在後續每一頁都會維持同一個 key 不變，
		// 只有 NextVersionIdMarker 會前進；因此「沒有進展」必須同時比較兩個 marker，
		// 只比較 key marker 會在這種情境下把正常分頁誤判為卡住。
		if keyMarker != nil && versionIDMarker != nil && nextKeyMarker == *keyMarker && nextVersionIDMarker == *versionIDMarker {
			return fmt.Errorf("ListObjectVersions 分頁沒有進展（key marker %q, version marker %q）", nextKeyMarker, nextVersionIDMarker)
		}
		keyMarker = &nextKeyMarker
		versionIDMarker = &nextVersionIDMarker
	}
}

// DeleteObjectVersion 刪除指定版本（versionID 為 "null" 時刪除 unversioned 物件本身；
// 對 delete marker 呼叫則移除該 marker，讓前一版本重新成為最新）。
func (c *S3) DeleteObjectVersion(ctx context.Context, bucket, key, versionID string) error {
	_, err := c.Client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: &bucket, Key: &key, VersionId: &versionID})
	if err != nil {
		return ClassifyS3Error(err)
	}
	return nil
}

// allUsersGroupURI 是 S3 ACL 中代表「所有人（匿名）」的 grantee 群組 URI。
const allUsersGroupURI = "http://acs.amazonaws.com/groups/global/AllUsers"

// SetObjectACL 以 canned ACL 設定物件：public 為 public-read（匿名可讀），否則 private。
func (c *S3) SetObjectACL(ctx context.Context, bucket, key string, public bool) error {
	acl := types.ObjectCannedACLPrivate
	if public {
		acl = types.ObjectCannedACLPublicRead
	}
	if _, err := c.Client.PutObjectAcl(ctx, &s3.PutObjectAclInput{Bucket: &bucket, Key: &key, ACL: acl}); err != nil {
		return ClassifyS3Error(err)
	}
	return nil
}

// IsObjectPublic 以 GetObjectAcl 判斷物件是否對 AllUsers 群組授予 READ（或 FULL_CONTROL）。
func (c *S3) IsObjectPublic(ctx context.Context, bucket, key string) (bool, error) {
	out, err := c.Client.GetObjectAcl(ctx, &s3.GetObjectAclInput{Bucket: &bucket, Key: &key})
	if err != nil {
		return false, ClassifyS3Error(err)
	}
	for _, g := range out.Grants {
		if g.Grantee == nil || aws.ToString(g.Grantee.URI) != allUsersGroupURI {
			continue
		}
		if g.Permission == types.PermissionRead || g.Permission == types.PermissionFullControl {
			return true, nil
		}
	}
	return false, nil
}

// maxCopyObjectSize 是 S3 單次 CopyObject（非 multipart）允許的最大來源物件大小（5 GiB）；
// 超過此大小必須改用 multipart copy（UploadPartCopy），目前 SetContentType 未實作，
// 直接回一般錯誤讓使用者知道限制所在，而不是讓 CopyObject 呼叫失敗才得知。
const maxCopyObjectSize = 5 * 1024 * 1024 * 1024

// SetContentType 就地修改物件的 Content-Type：S3 沒有「只改 metadata」的 API，必須用 CopyObject
// 把物件複製到自己身上並以 MetadataDirective=REPLACE 帶入新的 Content-Type。CopyObject 在
// REPLACE 模式下會清掉所有沒有在請求中重送的 HTTP metadata（使用者自訂 metadata、
// Cache-Control、Content-Encoding 等系統 metadata 一併歸零），也會把 ACL 重設為 private，
// 因此先讀取原本的 ACL 與這些欄位再一併帶回，只有 Content-Type 是真正被改變的值。
// 單次 CopyObject 有 5 GiB 上限（超過需 multipart copy，此函式未實作），超過時直接回錯誤。
func (c *S3) SetContentType(ctx context.Context, bucket, key, contentType string) error {
	public, err := c.IsObjectPublic(ctx, bucket, key)
	if err != nil {
		return err
	}
	head, err := c.Client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &bucket, Key: &key})
	if err != nil {
		return ClassifyS3Error(err)
	}
	if size := aws.ToInt64(head.ContentLength); size > maxCopyObjectSize {
		return fmt.Errorf("物件 cos://%s/%s 大小 %.2f GiB 超過單次 CopyObject 上限 5 GiB，目前不支援修改其 Content-Type",
			bucket, key, float64(size)/(1024*1024*1024))
	}
	acl := types.ObjectCannedACLPrivate
	if public {
		acl = types.ObjectCannedACLPublicRead
	}
	// CopySource 格式為 "<bucket>/<key>"，key 需 URL escape（保留 "/"）。
	copySource := bucket + "/" + strings.ReplaceAll(url.PathEscape(key), "%2F", "/")
	_, err = c.Client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket: &bucket, Key: &key, CopySource: &copySource,
		ContentType: &contentType, Metadata: head.Metadata,
		MetadataDirective:       types.MetadataDirectiveReplace,
		ACL:                     acl,
		CacheControl:            head.CacheControl,
		ContentDisposition:      head.ContentDisposition,
		ContentEncoding:         head.ContentEncoding,
		ContentLanguage:         head.ContentLanguage,
		Expires:                 parseExpires(head.ExpiresString),
		WebsiteRedirectLocation: head.WebsiteRedirectLocation,
	})
	if err != nil {
		return ClassifyS3Error(err)
	}
	return nil
}

// parseExpires 把 HeadObjectOutput.ExpiresString（HTTP-date 格式的原始 Expires header 值，
// SDK 建議優先使用此欄位而非已標記為 deprecated 的 HeadObjectOutput.Expires）解析回
// CopyObjectInput.Expires 需要的 *time.Time；s 為 nil、空字串，或解析失敗（來源值本就不合法）
// 時回傳 nil，等同「不帶這個欄位」，不會讓整個 SetContentType 因為這種邊角情況而失敗。
func parseExpires(s *string) *time.Time {
	if s == nil || *s == "" {
		return nil
	}
	t, err := http.ParseTime(*s)
	if err != nil {
		return nil
	}
	return &t
}

// ClassifyS3Error 把 S3 SDK 回傳的錯誤分類成 twaictl 統一的錯誤型別，判斷順序很重要：
//   - 網路 / DNS / 逾時錯誤（twai.IsNetworkError）→ 原樣回傳（cli 對應 exit 4）。必須先判斷，
//     因為請求根本沒送出（例如連線被拒絕）時，SDK 仍會用 *awshttp.ResponseError 包裝底層錯誤，
//     只是 HTTPStatusCode() 為 0；若先判斷 ResponseError 會把這種網路錯誤誤判成 API 錯誤。
//   - 有 HTTP 狀態碼（≥400，透過 *awshttp.ResponseError 判斷）→ *twai.APIError（cli 對應 exit 3）；
//     Message 為 "<ErrorCode>: <ErrorMessage>"（由 smithy.APIError 取得，取不到則退回 err.Error()）；
//     Method 為 "S3 <Operation>"（由 *smithy.OperationError 取得，取不到則退回 "S3"）；
//     URL 只放請求路徑（path-style 下即 "<bucket>/<key>"），不含 endpoint host、query string，
//     因此不會洩漏 SigV4 簽章或憑證。
//   - 其餘錯誤 → 包裝成一般錯誤（cli 對應 exit 1）。
//
// 注意：任何情況下都不會把 access key / secret key 放進回傳的錯誤訊息。
func ClassifyS3Error(err error) error {
	if err == nil {
		return nil
	}

	if twai.IsNetworkError(err) {
		return err
	}

	var respErr *awshttp.ResponseError
	if errors.As(err, &respErr) && respErr.HTTPStatusCode() >= 400 {
		message := err.Error()
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) {
			message = fmt.Sprintf("%s: %s", apiErr.ErrorCode(), apiErr.ErrorMessage())
		}
		method := "S3"
		var opErr *smithy.OperationError
		if errors.As(err, &opErr) {
			method = "S3 " + opErr.Operation()
		}
		return &twai.APIError{
			StatusCode: respErr.HTTPStatusCode(),
			Method:     method,
			URL:        requestPathFromResponseError(respErr),
			Message:    message,
		}
	}

	return fmt.Errorf("S3 操作失敗: %w", err)
}

// requestPathFromResponseError 盡量從 SDK 錯誤中取出原始 request 的 URL path。
// path-style 定址下這正好是 "/bucket/key"，去除開頭斜線後即為 brief 要求的
// "<bucket>/<key>" 格式；不含 host（因此不含 endpoint）也不含 query string
// （因此不含任何簽章或憑證資訊）。若底層沒有保留 *http.Request（理論上不會發生，
// 但避免因 SDK 內部實作變動而 panic），回傳空字串。
func requestPathFromResponseError(respErr *awshttp.ResponseError) string {
	if respErr == nil || respErr.Response == nil || respErr.Response.Response == nil {
		return ""
	}
	req := respErr.Response.Request
	if req == nil || req.URL == nil {
		return ""
	}
	return strings.TrimPrefix(req.URL.Path, "/")
}
