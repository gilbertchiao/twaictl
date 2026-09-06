// Package cos 是 Ceph（S3 相容物件儲存）金鑰管理與 S3 資料面封裝：
// Ceph project 解析與金鑰 CRUD、`cos://` URL 解析、S3 bucket/object 操作。
package cos

import "strings"

// scheme 是 cos:// URL 的協定前綴。
const scheme = "cos://"

// Location 是 `cos://bucket[/key]` URL 解析後的結果。
type Location struct {
	Bucket string
	Key    string
}

// ParseURL 把 `cos://bucket[/key]` 解析成 Location；不是 cos:// 開頭或缺少 bucket 時回傳 false。
// `cos://bucket` 與 `cos://bucket/` 的 Key 皆為空字串；
// 尾端斜線（例如 `cos://bucket/prefix/`）會保留在 Key 中，供呼叫端判斷是否為前綴。
func ParseURL(s string) (Location, bool) {
	if !strings.HasPrefix(s, scheme) {
		return Location{}, false
	}
	rest := strings.TrimPrefix(s, scheme)
	if rest == "" {
		return Location{}, false
	}
	bucket, key, _ := strings.Cut(rest, "/")
	if bucket == "" {
		return Location{}, false
	}
	return Location{Bucket: bucket, Key: key}, true
}

// String 把 Location 還原成 `cos://bucket/key` 形式；Key 為空時只回傳 `cos://bucket`。
func (l Location) String() string {
	if l.Key == "" {
		return scheme + l.Bucket
	}
	return scheme + l.Bucket + "/" + l.Key
}

// IsRemote 判斷 s 是否為 cos:// URL（不驗證 bucket 是否合法，僅檢查協定前綴）。
func IsRemote(s string) bool {
	return strings.HasPrefix(s, scheme)
}
