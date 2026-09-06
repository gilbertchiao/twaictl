package cli

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilbertchiao/twaictl/internal/config"
)

func TestAPIGetPrintsPrettyJSON(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/whatever/", 200, `{"a":1}`)

	code, stdout, stderr := runCLI(t, f.env(t), "", "api", "GET", "/whatever/")
	require.Equal(t, ExitOK, code, stderr)
	assert.Equal(t, "{\n  \"a\": 1\n}\n", stdout)
}

func TestAPIPostSendsDataFromFile(t *testing.T) {
	f := newFakeAPI(t)
	f.on("POST", vcsBase+"/whatever/", 200, `{"ok":true}`)
	path := filepath.Join(t.TempDir(), "body.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"name":"demo"}`), 0o600))

	code, stdout, stderr := runCLI(t, f.env(t), "", "api", "POST", "/whatever/", "--data", "@"+path)
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stdout, `"ok": true`)

	req := f.find("POST", vcsBase+"/whatever/")
	require.NotNil(t, req)
	assert.JSONEq(t, `{"name":"demo"}`, string(req.Body))
	assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
}

func TestAPIDataFromStdin(t *testing.T) {
	f := newFakeAPI(t)
	f.on("POST", vcsBase+"/whatever/", 200, `{}`)

	code, _, stderr := runCLI(t, f.env(t), `{"name":"demo"}`, "api", "POST", "/whatever/", "--data", "@-")
	require.Equal(t, ExitOK, code, stderr)

	req := f.find("POST", vcsBase+"/whatever/")
	require.NotNil(t, req)
	assert.JSONEq(t, `{"name":"demo"}`, string(req.Body))
}

func TestAPIServiceCosUsesCephBase(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", cephBase+"/usage/", 200, `{}`)

	code, _, stderr := runCLI(t, f.env(t), "", "api", "GET", "/usage/", "--service", "cos")
	require.Equal(t, ExitOK, code, stderr)
	assert.NotNil(t, f.find("GET", cephBase+"/usage/"))
}

// TestAPIRejectsBadMethodAndDataOnGet 驗證不合法的 method、以及 GET 帶 --data
// 都在送出任何請求之前就被擋下（純使用者輸入錯誤，exit 1）。
func TestAPIRejectsBadMethodAndDataOnGet(t *testing.T) {
	f := newFakeAPI(t)

	code, _, stderr := runCLI(t, f.env(t), "", "api", "FOO", "/whatever/")
	assert.Equal(t, ExitGeneral, code)
	assert.Contains(t, stderr, "FOO")

	code, _, stderr = runCLI(t, f.env(t), "", "api", "GET", "/whatever/", "--data", `{"a":1}`)
	assert.Equal(t, ExitGeneral, code)
	assert.Contains(t, stderr, "--data")

	assert.Empty(t, f.Requests(), "驗證失敗時不應送出任何請求")
}

// TestAPI404IsExitAPI 驗證非 2xx 回應對應 exit code 3，且 stderr 含 body 解析出的訊息。
func TestAPI404IsExitAPI(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/missing/", 404, `{"message":"not found"}`)

	code, stdout, stderr := runCLI(t, f.env(t), "", "api", "GET", "/missing/")
	assert.Equal(t, ExitAPI, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "not found")
}

// TestAPIDryRunMasksKey 驗證 --dry-run 印出等價 curl 命令、x-api-key 遮蔽，且不真的送出請求。
func TestAPIDryRunMasksKey(t *testing.T) {
	f := newFakeAPI(t)

	code, stdout, stderr := runCLI(t, f.env(t), "", "--dry-run", "api", "POST", "/whatever/", "--data", `{"name":"demo"}`)
	assert.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stdout, "curl -X POST")
	assert.Contains(t, stdout, "-H 'x-api-key: ***'")
	assert.NotContains(t, stdout, "test-key")
	assert.Empty(t, f.Requests(), "dry-run 不應真的送出請求")
}

func TestAPIYamlOutput(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/whatever/", 200, `{"a":1}`)

	code, stdout, stderr := runCLI(t, f.env(t), "", "api", "GET", "/whatever/", "-o", "yaml")
	require.Equal(t, ExitOK, code, stderr)
	assert.Equal(t, "a: 1\n", stdout)
}

// TestAPI204NoContentPrintsNotice 驗證 204／空 body 不印 stdout，改在 stderr 顯示提示。
func TestAPI204NoContentPrintsNotice(t *testing.T) {
	f := newFakeAPI(t)
	f.on("DELETE", vcsBase+"/whatever/", 204, ``)

	code, stdout, stderr := runCLI(t, f.env(t), "", "api", "DELETE", "/whatever/")
	assert.Equal(t, ExitOK, code, stderr)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "twaictl: HTTP 204 No Content（回應沒有內容）")
}

// TestAPI200EmptyBodyPrintsRealStatus 是 code review Important 2 的迴歸測試：
// 200（或任何非 204 的狀態碼）搭配空 body 時，stderr 要印出真實的狀態碼，
// 而不是像修正前那樣籠統宣稱「HTTP 204 No Content」（誤導只看 stderr 的使用者）。
func TestAPI200EmptyBodyPrintsRealStatus(t *testing.T) {
	f := newFakeAPI(t)
	f.on("POST", vcsBase+"/whatever/", 200, ``)

	code, stdout, stderr := runCLI(t, f.env(t), "", "api", "POST", "/whatever/", "--data", "{}")
	assert.Equal(t, ExitOK, code, stderr)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "twaictl: HTTP 200 OK（回應沒有內容）")
	assert.NotContains(t, stderr, "204")
}

// TestAPIDataEmptyOnGetOrDeleteIsRejected 驗證 --data 空字串（明確指定但值為空字串）在
// GET/DELETE 上仍要被擋下，而不是被誤判成「沒指定 --data」而放行
// （用 c.Flags().Changed("data") 判斷是否有指定，而非只看值是否為空字串）。
func TestAPIDataEmptyOnGetOrDeleteIsRejected(t *testing.T) {
	f := newFakeAPI(t)

	code, _, stderr := runCLI(t, f.env(t), "", "api", "GET", "/whatever/", "--data", "")
	assert.Equal(t, ExitGeneral, code)
	assert.Contains(t, stderr, "--data")

	code, _, stderr = runCLI(t, f.env(t), "", "api", "DELETE", "/whatever/", "--data", "")
	assert.Equal(t, ExitGeneral, code)
	assert.Contains(t, stderr, "--data")

	assert.Empty(t, f.Requests(), "驗證失敗時不應送出任何請求")
}

// TestAPIExplicitEmptyDataSendsContentType 驗證 POST 明確指定空 body
// （--data 空字串／空檔案／空 stdin）時仍會送出 Content-Type: application/json——
// Transport.injectHeaders 只在 req.Body 非 nil 時才補預設值，空 body 不會建立
// io.Reader，因此需要在 cli 層直接補上，讓行為與非空 body 一致。
func TestAPIExplicitEmptyDataSendsContentType(t *testing.T) {
	f := newFakeAPI(t)
	f.on("POST", vcsBase+"/whatever/", 200, `{}`)

	code, _, stderr := runCLI(t, f.env(t), "", "api", "POST", "/whatever/", "--data", "")
	require.Equal(t, ExitOK, code, stderr)
	req := f.find("POST", vcsBase+"/whatever/")
	require.NotNil(t, req)
	assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
	assert.Empty(t, req.Body)

	emptyFile := filepath.Join(t.TempDir(), "empty.json")
	require.NoError(t, os.WriteFile(emptyFile, nil, 0o600))
	code, _, stderr = runCLI(t, f.env(t), "", "api", "POST", "/whatever/", "--data", "@"+emptyFile)
	require.Equal(t, ExitOK, code, stderr)
	req = f.find("POST", vcsBase+"/whatever/")
	require.NotNil(t, req)
	assert.Equal(t, "application/json", req.Header.Get("Content-Type"))

	code, _, stderr = runCLI(t, f.env(t), "", "api", "POST", "/whatever/", "--data", "@-")
	require.Equal(t, ExitOK, code, stderr)
	req = f.find("POST", vcsBase+"/whatever/")
	require.NotNil(t, req)
	assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
}

// TestAPIRedirectIsExitAPIWithLocation 是 code review Important 1 的 CLI 層迴歸測試：
// apigateway 對不存在的 path 回 302（見 docs/api-notes.md A26）時，twaictl api 不可
// 跟隨重導向（會把 x-api-key 帶到重導向目標），而是回報 exit code 3，
// stderr 提及 302 與 Location 的 host/path。Location 刻意帶 userinfo／query
// string／fragment，驗證 stderr 不會洩漏這些敏感片段（Codex review 發現，
// NewAPIError 現在只保留 Location 的 scheme+host+path）。
func TestAPIRedirectIsExitAPIWithLocation(t *testing.T) {
	var secondCalled bool
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		secondCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer second.Close()

	secondURL, err := url.Parse(second.URL)
	require.NoError(t, err)
	secondURL.User = url.UserPassword("user", "pw")
	secondURL.Path = "/login/"
	secondURL.RawQuery = "token=SECRET"
	secondURL.Fragment = "frag"
	locationWithSecrets := secondURL.String()
	locationHostPath := second.URL + "/login/"

	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", locationWithSecrets)
		w.WriteHeader(http.StatusFound)
	}))
	defer first.Close()

	env := map[string]string{
		"XDG_CONFIG_HOME":     t.TempDir(),
		config.EnvAPIKey:      "test-key",
		config.EnvAPIGateway:  first.URL,
		config.EnvProjectCode: "ENT1",
	}

	code, stdout, stderr := runCLI(t, env, "", "api", "GET", "/nosuch-path/")
	assert.Equal(t, ExitAPI, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "302")
	assert.Contains(t, stderr, locationHostPath)
	assert.NotContains(t, stderr, "SECRET", "不可洩漏 Location 的 query string")
	assert.NotContains(t, stderr, "frag", "不可洩漏 Location 的 fragment")
	assert.NotContains(t, stderr, "pw@", "不可洩漏 Location 的 userinfo")
	assert.False(t, secondCalled, "不可跟隨重導向送到 Location 指定的目標")
}

// TestAPICustomHeaderIsSent 驗證 --header 可重複附加自訂 header。
func TestAPICustomHeaderIsSent(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/whatever/", 200, `{}`)

	code, _, stderr := runCLI(t, f.env(t), "", "api", "GET", "/whatever/", "--header", "X-Custom=abc", "--header", "X-Other=def")
	require.Equal(t, ExitOK, code, stderr)

	req := f.find("GET", vcsBase+"/whatever/")
	require.NotNil(t, req)
	assert.Equal(t, "abc", req.Header.Get("X-Custom"))
	assert.Equal(t, "def", req.Header.Get("X-Other"))
}

// TestAPIRejectsReservedHeader 驗證 --header 指定 x-api-key/x-api-host/User-Agent
// 這類由 transport 自動注入的 header 會被拒絕（一般錯誤，且不送出請求）。
func TestAPIRejectsReservedHeader(t *testing.T) {
	f := newFakeAPI(t)

	code, _, stderr := runCLI(t, f.env(t), "", "api", "GET", "/whatever/", "--header", "x-api-key=hijack")
	assert.Equal(t, ExitGeneral, code)
	assert.Contains(t, stderr, "x-api-key")
	assert.Empty(t, f.Requests())
}

// TestAPIHeaderOverridesContentTypeDeterministically 是 code review 發現的迴歸測試：
// --data 會讓 headers map 內同時出現使用者指定的 "content-type"（保留原始大小寫）與
// 內部想補上的 "Content-Type" 預設值，兩者在 http.Header.Set 時會正規化成同一個 key，
// 由誰覆蓋誰取決於 map 疊代順序（Go map 疊代順序是不確定的）。修正後 Content-Type 的
// 預設值改交給 Transport.injectHeaders 處理（不會與使用者的 header 重複），
// parseAPIHeaderFlags 也改用 http.CanonicalHeaderKey 正規化 key，
// 讓「使用者的 --header 覆蓋預設值」在任何 map 疊代順序下都是穩定結果。
// 迴圈跑多次是因為 Go 的 map 疊代順序每次呼叫都可能不同，單跑一次不足以重現舊 bug。
func TestAPIHeaderOverridesContentTypeDeterministically(t *testing.T) {
	f := newFakeAPI(t)
	f.on("POST", vcsBase+"/whatever/", 200, `{}`)

	for i := 0; i < 20; i++ {
		code, _, stderr := runCLI(t, f.env(t), "", "api", "POST", "/whatever/", "--data", "{}", "--header", "content-type=text/plain")
		require.Equal(t, ExitOK, code, stderr)

		req := f.find("POST", vcsBase+"/whatever/")
		require.NotNil(t, req)
		assert.Equal(t, "text/plain", req.Header.Get("Content-Type"), "第 %d 次呼叫", i)
	}
}

// TestAPIDataWithoutHeaderStillDefaultsJSON 驗證沒有 --header 覆寫時，
// --data 仍會預設帶上 Content-Type: application/json（由 Transport.injectHeaders 補上）。
func TestAPIDataWithoutHeaderStillDefaultsJSON(t *testing.T) {
	f := newFakeAPI(t)
	f.on("POST", vcsBase+"/whatever/", 200, `{}`)

	code, _, stderr := runCLI(t, f.env(t), "", "api", "POST", "/whatever/", "--data", "{}")
	require.Equal(t, ExitOK, code, stderr)

	req := f.find("POST", vcsBase+"/whatever/")
	require.NotNil(t, req)
	assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
}
