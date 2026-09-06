package twai

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilbertchiao/twaictl/internal/config"
)

// rawTestSettings 建立一組指向 srv 的 settings，服務別的 host 沿用 config.DefaultAPIHosts。
func rawTestSettings(gatewayURL string) *config.Settings {
	return &config.Settings{
		APIKey:     "secret",
		APIGateway: gatewayURL,
		APIHosts: config.APIHosts{
			VCS:    "openstack-taichung-default-2",
			COS:    "ceph-taichung-default",
			Common: "goc",
		},
	}
}

func rawTestOpts() ClientOptions {
	return ClientOptions{Version: "v9.9.9", Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func TestRawClientDoJoinsBaseAndInjectsHeaders(t *testing.T) {
	var gotURI, apiKey, apiHost string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURI = r.URL.RequestURI()
		apiKey = r.Header.Get(HeaderAPIKey)
		apiHost = r.Header.Get(HeaderAPIHost)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	client, err := NewRawClient(rawTestSettings(srv.URL), rawTestOpts())
	require.NoError(t, err)

	res, err := client.Do(context.Background(), RawServiceVCS, http.MethodGet, "/sites/?project=1", nil, nil)
	require.NoError(t, err)

	assert.Equal(t, "/api/v3/openstack-taichung-default-2/sites/?project=1", gotURI)
	assert.Equal(t, "secret", apiKey)
	assert.Equal(t, "openstack-taichung-default-2", apiHost)
	assert.JSONEq(t, `{"ok":true}`, string(res.Raw))
}

func TestRawClientDoCommonHasNoHostSegment(t *testing.T) {
	var gotPath, apiHost string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		apiHost = r.Header.Get(HeaderAPIHost)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client, err := NewRawClient(rawTestSettings(srv.URL), rawTestOpts())
	require.NoError(t, err)

	_, err = client.Do(context.Background(), RawServiceCommon, http.MethodGet, "/solutions/", nil, nil)
	require.NoError(t, err)

	assert.Equal(t, "/api/v3/solutions/", gotPath, "Common API 的 URL 不含 host 段")
	assert.Equal(t, "goc", apiHost, "header 仍要帶 Common 的預設 x-api-host")
}

func TestRawClientRejectsAbsoluteURLAndReservedHeaders(t *testing.T) {
	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, err := NewRawClient(rawTestSettings(srv.URL), rawTestOpts())
	require.NoError(t, err)

	_, err = client.Do(context.Background(), RawServiceVCS, http.MethodGet, "sites/", nil, nil)
	assert.Error(t, err, "path 不是 / 開頭應被拒絕")

	_, err = client.Do(context.Background(), RawServiceVCS, http.MethodGet, "/https://evil.example/sites/", nil, nil)
	assert.Error(t, err, "含 scheme/host 的 path 應被拒絕")

	_, err = client.Do(context.Background(), RawServiceVCS, http.MethodGet, "/sites/", nil, map[string]string{HeaderAPIKey: "hijack"})
	assert.Error(t, err, "呼叫端不可自訂 x-api-key")

	_, err = client.Do(context.Background(), RawServiceVCS, http.MethodGet, "/sites/", nil, map[string]string{"User-Agent": "evil"})
	assert.Error(t, err, "呼叫端不可自訂 User-Agent")

	assert.False(t, called, "驗證失敗時不應送出任何請求")
}

func TestRawClientNon2xxIsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"not found"}`))
	}))
	defer srv.Close()

	client, err := NewRawClient(rawTestSettings(srv.URL), rawTestOpts())
	require.NoError(t, err)

	_, err = client.Do(context.Background(), RawServiceVCS, http.MethodGet, "/sites/999/", nil, nil)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 404, apiErr.StatusCode)
	assert.Equal(t, "not found", apiErr.Message)
}

// TestRawClientDoesNotFollowRedirects 是 code review Important 1 的迴歸測試：3xx 回應
// 不可被自動跟隨（否則 x-api-key 會被複製到重導向目標，見 raw.go CheckRedirect 的說明
// 與 docs/api-notes.md A26）。用兩個各自獨立的 httptest server 模擬「第一個回 302 導向
// 第二個」，斷言第二個完全沒收到任何請求，且錯誤是 *APIError（狀態碼 302）、訊息含
// Location 的 host/path。
//
// Location 刻意帶上 userinfo（user:pw@）、query string（?token=SECRET）與 fragment
// （#frag）：這是 Codex review 發現的問題（NewAPIError 先前把 Location 原封不動塞進
// Message），驗證 NewAPIError 只會保留 scheme+host+path，敏感資訊不會外洩到
// stderr／log。
func TestRawClientDoesNotFollowRedirects(t *testing.T) {
	var secondCalled bool
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		secondCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer second.Close()

	secondURL, err := url.Parse(second.URL)
	require.NoError(t, err)
	secondURL.User = url.UserPassword("user", "pw")
	secondURL.Path = "/redirected/"
	secondURL.RawQuery = "token=SECRET"
	secondURL.Fragment = "frag"
	locationWithSecrets := secondURL.String()
	locationHostPath := second.URL + "/redirected/" // 預期訊息只保留這一段

	var firstRequests int
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstRequests++
		w.Header().Set("Location", locationWithSecrets)
		w.WriteHeader(http.StatusFound)
	}))
	defer first.Close()

	client, err := NewRawClient(rawTestSettings(first.URL), rawTestOpts())
	require.NoError(t, err)

	_, err = client.Do(context.Background(), RawServiceVCS, http.MethodGet, "/sites/", nil, nil)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusFound, apiErr.StatusCode)
	assert.Contains(t, apiErr.Message, locationHostPath, "錯誤訊息應含 Location 的 host/path")
	assert.NotContains(t, apiErr.Message, "SECRET", "不可洩漏 Location 的 query string")
	assert.NotContains(t, apiErr.Message, "frag", "不可洩漏 Location 的 fragment")
	assert.NotContains(t, apiErr.Message, "user:pw", "不可洩漏 Location 的 userinfo")
	assert.NotContains(t, apiErr.Message, "pw@", "不可洩漏 Location 的 userinfo")

	assert.Equal(t, 1, firstRequests, "只應送出一次請求，不可重試或跟隨")
	assert.False(t, secondCalled, "不可跟隨重導向送到 Location 指定的目標")
}

// TestRawClientDoReturnsStatusCodeOnEmptyBody 是 Important 2 的迴歸測試：body 為空時
// Do 仍要回傳真實的狀態碼／狀態文字，讓呼叫端（cli 層）不必籠統地宣稱 204。
func TestRawClientDoReturnsStatusCodeOnEmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK) // 200 但沒有寫任何 body
	}))
	defer srv.Close()

	client, err := NewRawClient(rawTestSettings(srv.URL), rawTestOpts())
	require.NoError(t, err)

	res, err := client.Do(context.Background(), RawServiceVCS, http.MethodGet, "/sites/", nil, nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, "200 OK", res.Status)
	assert.Empty(t, res.Raw)
}

func TestRawClientDryRunReturnsErrDryRun(t *testing.T) {
	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var dryRunOut bytes.Buffer
	opts := rawTestOpts()
	opts.DryRun = true
	opts.DryRunWriter = &dryRunOut

	client, err := NewRawClient(rawTestSettings(srv.URL), opts)
	require.NoError(t, err)

	_, err = client.Do(context.Background(), RawServiceVCS, http.MethodPost, "/sites/", []byte(`{"name":"x"}`), nil)
	assert.True(t, errors.Is(err, ErrDryRun))
	assert.False(t, called, "dry-run 不應真的送出請求")
	assert.Contains(t, dryRunOut.String(), "-H 'x-api-key: ***'")
	assert.NotContains(t, dryRunOut.String(), "secret")
}
