package twai

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fakeResponse(method, rawURL string, status int) *http.Response {
	u, _ := url.Parse(rawURL)
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Request:    &http.Request{Method: method, URL: u},
	}
}

// fakeRedirectResponse 建立一個帶 Location header 的 3xx *http.Response，
// 供下面測試 NewAPIError 如何把 Location 併進 Message。
func fakeRedirectResponse(status int, location string) *http.Response {
	resp := fakeResponse("GET", "https://gw/x", status)
	resp.Header = http.Header{"Location": []string{location}}
	return resp
}

// TestNewAPIErrorLocationOpaqueFormNeverLeaks 是 Codex review P2 的迴歸測試：opaque
// 形式的 Location（scheme 後面不是 "//" 而是直接接內容，例如
// "https:user:pw@example.com/path?token=SECRET"）經 url.Parse 後，
// "user:pw@example.com/path" 整段會被放進 u.Opaque，不是 u.User／u.Host／u.Path——
// 先前只清空 User/RawQuery/Fragment 的寫法完全擋不住 Opaque，會把使用者名稱、密碼、
// token 原樣印出來。stripQuery 遇到 u.Opaque != "" 時回傳空字串，NewAPIError 改印
// 「Location: （略）」，敏感內容完全不會出現在訊息裡。
func TestNewAPIErrorLocationOpaqueFormNeverLeaks(t *testing.T) {
	e := NewAPIError(fakeRedirectResponse(302, "https:user:pw@example.com/path?token=SECRET"), nil)
	assert.NotContains(t, e.Message, "pw")
	assert.NotContains(t, e.Message, "SECRET")
	assert.NotContains(t, e.Message, "user:pw")
	assert.Contains(t, e.Message, "（略）")
}

// TestNewAPIErrorLocationRelativePathStillRenders 驗證相對路徑形式的 Location
// （例如 "/login?next=x"，沒有 scheme/host）仍然正常顯示 path，只是去掉 query
// string——這種情況 u.Opaque 為空、u.Path 非空，stripQuery 不會回傳空字串。
func TestNewAPIErrorLocationRelativePathStillRenders(t *testing.T) {
	e := NewAPIError(fakeRedirectResponse(302, "/login?next=x"), nil)
	assert.Contains(t, e.Message, "/login")
	assert.NotContains(t, e.Message, "next=x")
}

func TestNewAPIErrorExtractsMessageFromCommonJSONKeys(t *testing.T) {
	cases := []struct{ body, want string }{
		{`{"message":"Project not found"}`, "Project not found"},
		{`{"detail":"Invalid token"}`, "Invalid token"},
		{`{"error":"boom"}`, "boom"},
		{`{"unknown":"x"}`, `{"unknown":"x"}`},
		{`<html>gateway</html>`, `<html>gateway</html>`},
	}
	for _, c := range cases {
		e := NewAPIError(fakeResponse("GET", "https://gw/api/v3/x/projects/", 404), []byte(c.body))
		assert.Equal(t, c.want, e.Message, c.body)
	}
}

func TestNewAPIErrorTruncatesLongBodyMessage(t *testing.T) {
	long := strings.Repeat("a", 500)
	e := NewAPIError(fakeResponse("GET", "https://gw/x", 500), []byte(long))
	// Truncate 在超過 maxMessageLength 時會補上 "…"（多 3 bytes），故訊息的位元組長度
	// 是 maxMessageLength（純 ASCII "a"）加上 "…" 的位元組數，而非剛好 200。
	assert.Equal(t, strings.Repeat("a", 200)+"…", e.Message)
	assert.Equal(t, long, string(e.Body), "Body 保留完整內容")
}

// TestNewAPIErrorTrimsSurroundingWhitespaceBeforeTruncating 驗證 body 前後有空白、
// 且無法解析出常見訊息欄位時，訊息以「先 TrimSpace 過的內容」為準做截斷：
// 結果應與直接對已去除頭尾空白的內容截斷一致，不受外圍空白影響長度判斷。
func TestNewAPIErrorTrimsSurroundingWhitespaceBeforeTruncating(t *testing.T) {
	long := strings.Repeat("a", 500)
	padded := "  \n" + long + "  \n"
	e := NewAPIError(fakeResponse("GET", "https://gw/x", 500), []byte(padded))
	assert.Equal(t, strings.Repeat("a", 200)+"…", e.Message)
}

func TestNewAPIErrorStripsQueryString(t *testing.T) {
	e := NewAPIError(fakeResponse("GET", "https://gw/x?api_key=abc", 401), nil)
	assert.Equal(t, "https://gw/x", e.URL)
	assert.NotContains(t, e.Error(), "abc")
}

func TestAPIErrorString(t *testing.T) {
	e := NewAPIError(fakeResponse("DELETE", "https://gw/api/v3/openstack-taichung-default-2/sites/1/", 403), []byte(`{"detail":"forbidden"}`))
	assert.Equal(t, "API 回應 403 Forbidden: forbidden (DELETE https://gw/api/v3/openstack-taichung-default-2/sites/1/)", e.Error())
}

func TestAPIErrorStringWithoutMessage(t *testing.T) {
	e := NewAPIError(fakeResponse("GET", "https://gw/api/v3/projects/", 502), nil)
	assert.Equal(t, "API 回應 502 Bad Gateway (GET https://gw/api/v3/projects/)", e.Error())

	blank := NewAPIError(fakeResponse("GET", "https://gw/x", 500), []byte("   \n"))
	assert.Equal(t, "", blank.Message, "只有空白的 body 視為沒有訊息")
	assert.Equal(t, "API 回應 500 Internal Server Error (GET https://gw/x)", blank.Error())
}

func TestIsNetworkError(t *testing.T) {
	assert.True(t, IsNetworkError(context.DeadlineExceeded))
	assert.True(t, IsNetworkError(&url.Error{Op: "Get", URL: "x", Err: errors.New("refused")}))
	assert.True(t, IsNetworkError(&net.DNSError{Err: "no such host", IsNotFound: true}))
	assert.True(t, IsNetworkError(fmt.Errorf("wrap: %w", context.DeadlineExceeded)))
	assert.False(t, IsNetworkError(errors.New("plain")))
	assert.False(t, IsNetworkError(NewAPIError(fakeResponse("GET", "https://x", 500), nil)))
	assert.False(t, IsNetworkError(nil))
}

// TestStripQueryKeepsPercentEncodedPath 驗證 stripQuery 保留 path 中的百分號編碼
// （`a%2Fb` 不應被 url.URL.String() 正規化成 `a/b`），讓錯誤訊息中的路徑與實際送出的
// 一致。
func TestStripQueryKeepsPercentEncodedPath(t *testing.T) {
	u, err := url.Parse("https://gw/api/v3/x/a%2Fb/?token=1#frag")
	require.NoError(t, err)
	assert.Equal(t, "https://gw/api/v3/x/a%2Fb/", stripQuery(u))
}

// TestIsNetworkErrorExcludesFsPathError 是 vcs keypair create --out 寫檔失敗
// （開發本 task 時發現的案例）的迴歸測試：*fs.PathError（一般本機檔案系統操作失敗，例如
// O_EXCL 開檔遇到既有檔案）底層包的 syscall.Errno 剛好也實作了 net.Error 介面的
// Timeout()/Temporary() 方法（原意是給 os 套件自己判斷是否可重試，與網路完全無關），
// 若不特別排除，errors.As(err, &netErr) 會誤判成網路錯誤（見 IsNetworkError 註解）。
// 用真正的檔案系統操作重現，確保涵蓋各平台實際回傳的底層錯誤型別。
func TestIsNetworkErrorExcludesFsPathError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))

	_, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	require.Error(t, err)
	var pathErr *fs.PathError
	require.ErrorAs(t, err, &pathErr, "此測試假設作業系統對 O_EXCL 撞到既有檔案回傳 *fs.PathError")

	assert.False(t, IsNetworkError(err))
	assert.False(t, IsNetworkError(fmt.Errorf("寫入 %s 失敗: %w", path, err)))
}
