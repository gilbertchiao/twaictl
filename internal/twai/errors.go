// Package twai 是 TWCC API 的存取層：自訂 transport、錯誤型別與各服務 client 的建構。
package twai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// maxMessageLength 是 body 無法解析時，塞進 Message 的最大字元數。
const maxMessageLength = 200

// ErrWaitTimeout 代表 --wait 輪詢逾時（exit code 5）。
var ErrWaitTimeout = errors.New("等待操作完成逾時")

// APIError 代表 API 回傳的非 2xx 回應。
type APIError struct {
	StatusCode int
	Method     string
	URL        string // 已去除 query string，避免洩漏敏感資料
	Message    string // 從 body 解析出的人類可讀訊息；解析失敗則為 body 前 200 字
	Body       []byte
}

func (e *APIError) Error() string {
	statusText := http.StatusText(e.StatusCode)
	if e.Message == "" {
		return fmt.Sprintf("API 回應 %d %s (%s %s)", e.StatusCode, statusText, e.Method, e.URL)
	}
	return fmt.Sprintf("API 回應 %d %s: %s (%s %s)", e.StatusCode, statusText, e.Message, e.Method, e.URL)
}

// NewAPIError 由 http.Response 與已讀出的 body 建立 APIError。
func NewAPIError(resp *http.Response, body []byte) *APIError {
	apiErr := &APIError{
		StatusCode: resp.StatusCode,
		Body:       body,
		Message:    extractMessage(body),
	}
	if resp.Request != nil {
		apiErr.Method = resp.Request.Method
		if resp.Request.URL != nil {
			apiErr.URL = stripQuery(resp.Request.URL)
		}
	}
	// 3xx 回應（見 raw.go 的 CheckRedirect：RawClient 不再自動跟隨重導向，3xx 會落到
	// 這裡）額外把 Location header 併進 Message，讓使用者不必自己重送一次帶
	// -v/-i 的請求就知道目標在哪。只取 Location 這一個 header，不把整組回應 header
	// 都印出來，避免意外洩漏其他內部資訊；Location 本身也只取 scheme+host+path
	// （用 stripQuery 去除 query string／fragment／userinfo／opaque 內容）——
	// 重導向目標的 URL 可能夾帶 SSO token、簽章過的參數，或 user:pass@ 這類
	// userinfo，這些都不該原樣出現在 stderr／log（Codex review 發現）。Location
	// 不是合法 URL、或 stripQuery 判斷無法安全重組時（opaque 形式，見下方說明）
	// 改印「Location: （略）」，不含任何原始內容。
	if apiErr.StatusCode >= 300 && apiErr.StatusCode < 400 {
		if loc := resp.Header.Get("Location"); loc != "" {
			locNote := "Location 無法解析"
			if parsedLoc, err := url.Parse(loc); err == nil {
				display := stripQuery(parsedLoc)
				if display == "" {
					display = "（略）"
				}
				locNote = fmt.Sprintf("Location: %s", display)
			}
			if apiErr.Message != "" {
				apiErr.Message = fmt.Sprintf("%s（%s）", apiErr.Message, locNote)
			} else {
				apiErr.Message = locNote
			}
		}
	}
	return apiErr
}

// stripQuery 只用 scheme／host／path 三個允許出現的欄位重組 URL 字串，用來去除
// query string、fragment、userinfo。兩個呼叫端都需要這個範圍——APIError.URL
// （避免記錄請求的敏感查詢參數）、NewAPIError 把 3xx 回應的 Location header
// 併入 Message 時（見上方說明）。
//
// 刻意用 url.URL{Scheme, Host, Path}.String() 重組，而不是在原本的 *url.URL 上
// 個別清空 RawQuery/Fragment/User：opaque 形式的 URL（例如
// Location: `https:user:pw@example.com/path?token=x`）經 url.Parse 後，
// `user:pw@example.com/path` 整段會被放進 u.Opaque，而不是 u.User／u.Host／
// u.Path——只清空 User/RawQuery/Fragment 完全擋不住，Opaque 欄位本身就可能夾帶
// userinfo 或敏感路徑。只從允許清單裡的三個欄位重組，Opaque 的內容天然就不會
// 出現在結果裡；u.Opaque 非空時直接回傳空字串，呼叫端據此顯示「（略）」。
//
// 帶上 RawPath 讓 String() 走 EscapedPath()，`%2F` 這類編碼保留原樣，錯誤訊息
// 中的路徑才與實際送出的一致。
func stripQuery(u *url.URL) string {
	if u.Opaque != "" {
		return ""
	}
	clean := url.URL{Scheme: u.Scheme, Host: u.Host, Path: u.Path, RawPath: u.RawPath}
	return clean.String()
}

// extractMessage 嘗試從 JSON body 的常見欄位取出訊息；失敗時回傳 body 前 200 字。
// TWCC 各服務的錯誤格式在 OpenAPI 中未定義，故採寬鬆策略（見 docs/api-notes.md）。
// 最後一步的 fallback 傳入已經 TrimSpace 過的 trimmed（而非原始 body），
// 避免 Truncate 內部又對同一段內容重複做一次 TrimSpace。
func extractMessage(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return ""
	}
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err == nil {
		for _, key := range []string{"message", "detail", "error", "msg"} {
			if value, ok := parsed[key].(string); ok && value != "" {
				return value
			}
		}
	}
	return Truncate([]byte(trimmed), maxMessageLength)
}

// Truncate 回傳 body 前 n 個字元（以 rune 計，超過時補 "…"），供錯誤訊息引用回應片段而不至於太長。
func Truncate(body []byte, n int) string {
	runes := []rune(strings.TrimSpace(string(body)))
	if len(runes) > n {
		return string(runes[:n]) + "…"
	}
	return string(runes)
}

// IsNetworkError 判斷 err 是否為連線 / DNS / 逾時類錯誤（exit code 4）。
func IsNetworkError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return true
	}
	// *fs.PathError（一般本機檔案系統操作失敗，例如 vcs keypair create --out 寫檔失敗）
	// 在 Unix 上底層包的 syscall.Errno 剛好也實作了 Timeout()/Temporary()（原意是給
	// os 套件自己判斷是否可重試，與網路完全無關），因而會意外滿足下面的 net.Error 介面，
	// 讓本機檔案錯誤被誤判成網路錯誤（exit code 4）。故在檢查 net.Error 前先排除它，
	// 讓這類錯誤落到 exitCodeFor 的預設分支（ExitGeneral）。
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return false
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}
