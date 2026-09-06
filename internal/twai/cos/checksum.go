package cos

import (
	"crypto/md5" //nolint:gosec // 只用於與 S3 ETag（本就是 MD5）比對，非安全用途
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
)

// IsPlainMD5ETag 判斷 S3 ETag 是否為單一 PUT 產生的純 MD5（32 個 hex 字元，可帶引號）。
// multipart 上傳的 ETag 形如 "<md5>-<分片數>"，不是內容的 MD5，無法拿來比對本地檔案。
func IsPlainMD5ETag(etag string) bool {
	s := strings.Trim(etag, `"`)
	if len(s) != 32 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

// FileMD5 計算檔案內容的 MD5，回傳小寫 hex（不含引號），供與 S3 ETag 比對。
func FileMD5(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("開啟檔案 %s 失敗: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	h := md5.New() //nolint:gosec // 同上
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("讀取檔案 %s 失敗: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// builtinContentTypes 是常見 Web 副檔名的 Content-Type 對照表，優先於作業系統的 mime 表
// 使用：標準庫 mime.TypeByExtension 讀取的是本機系統設定（Linux 讀 /etc/mime.types、
// macOS/Windows 各自的登錄資料），同一個副檔名在不同平台可能得到不同結果（甚至完全沒有
// 登錄），對 `cos cp` 上傳這種需要跨平台一致行為的場景不適合依賴系統設定。
//
// charset 後綴的決定：只有 "text/" 開頭的型別（html/css/js/txt/md）才加上
// "; charset=utf-8"；其餘型別（json、svg+xml、image/*、font/*、wasm、xml、pdf）不加，
// 沿用各自 IANA 登錄的慣例（例如 RFC 8259 規定 JSON 一律 UTF-8，不需要在 Content-Type
// 另外標註 charset）。
var builtinContentTypes = map[string]string{
	".html":  "text/html; charset=utf-8",
	".css":   "text/css; charset=utf-8",
	".js":    "text/javascript; charset=utf-8",
	".json":  "application/json",
	".svg":   "image/svg+xml",
	".png":   "image/png",
	".jpg":   "image/jpeg",
	".jpeg":  "image/jpeg",
	".gif":   "image/gif",
	".webp":  "image/webp",
	".woff2": "font/woff2",
	".wasm":  "application/wasm",
	".txt":   "text/plain; charset=utf-8",
	".md":    "text/markdown; charset=utf-8",
	".xml":   "application/xml",
	".pdf":   "application/pdf",
}

// ContentTypeForPath 依副檔名判斷 Content-Type：常見 Web 副檔名優先查 builtinContentTypes
// （跨平台行為一致），其餘才退回標準庫 mime 表；都沒有登錄則回傳 application/octet-stream。
func ContentTypeForPath(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if ct, ok := builtinContentTypes[ext]; ok {
		return ct
	}
	if ct := mime.TypeByExtension(ext); ct != "" {
		return ct
	}
	return "application/octet-stream"
}
