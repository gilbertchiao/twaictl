package twai

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gilbertchiao/twaictl/internal/config"
)

// RawService 是 `twaictl api` 逃生口可指定的服務別，決定要打到哪一組 BaseURL／
// x-api-host 預設值（見 BaseURL、config.Settings.APIHosts）。
type RawService string

// 支援的服務別。
const (
	RawServiceVCS    RawService = "vcs"
	RawServiceCOS    RawService = "cos"
	RawServiceCommon RawService = "common"
)

// RawClient 是 `twaictl api` 逃生口用的用戶端：重用既有 Transport（header 注入、
// 重試、--dry-run 印 curl 並遮蔽 x-api-key），但不經過 oapi-codegen 生成的型別化
// client，讓使用者可以呼叫任意 method/path——不做任何驗證、不做名稱→id 解析。
type RawClient struct {
	settings *config.Settings
	opts     ClientOptions
}

// NewRawClient 驗證 gateway URL（規則與 NewClient 相同：必須是 http(s):// 開頭且
// 帶 host 的完整 URL，見 client.go 的 validateGateway）後建立 RawClient。
func NewRawClient(settings *config.Settings, opts ClientOptions) (*RawClient, error) {
	if err := validateGateway(settings); err != nil {
		return nil, err
	}
	return &RawClient{settings: settings, opts: opts}, nil
}

// reservedRawHeaders 是 transport 自動注入、呼叫端不可自行指定的 header 名稱
// （比對時忽略大小寫）；User-Agent 沒有對應的 Header* 常數，故直接寫字面值。
var reservedRawHeaders = map[string]bool{
	strings.ToLower(HeaderAPIKey):  true,
	strings.ToLower(HeaderAPIHost): true,
	"user-agent":                   true,
}

// IsReservedRawHeader 判斷 name 是否為 transport 自動注入、`twaictl api --header`
// 不可自訂的 header（x-api-key／x-api-host／User-Agent，不分大小寫）。匯出供 cli 層
// 在組 --header 時提前擋下，讓錯誤訊息在解析階段就出現，而不必等到 Do 才發現。
func IsReservedRawHeader(name string) bool {
	return reservedRawHeaders[strings.ToLower(name)]
}

// RawResponse 是 (*RawClient).Do 的回傳值：保留 HTTP 狀態碼／狀態文字與原始 body，
// 讓呼叫端在 body 為空時仍能印出正確的狀態碼，而不必（錯誤地）籠統宣稱是 204。
//
// Raw 不保證是合法 JSON——逃生口打到的 endpoint 可能回傳純文字錯誤頁或空 body，
// 呼叫端（`twaictl api`）只在需要重新格式化（-o json/yaml）時才嘗試解析，
// 非 JSON 時原樣輸出（見 output.RenderRaw 的 pass-through 行為）。
type RawResponse struct {
	StatusCode int
	Status     string // http.Response.Status，例如 "204 No Content"
	Raw        []byte
}

// Do 對 `{BaseURL(service 對應的 host)}{path}` 送出請求。
//
// path 必須以 "/" 開頭、且不得包含 "://"（避免呼叫端誤傳絕對 URL 打到其他網域）；
// headers 只允許附加自訂 header，x-api-key／x-api-host／User-Agent 由 transport
// 依 settings 自動注入，呼叫端傳入同名 header 一律拒絕（見 IsReservedRawHeader）。
//
// 回傳的是 CheckResponse 之後的原始 body（dry-run → ErrDryRun；非 2xx →
// *APIError）；刻意不透過 Decode 解析 JSON，因為逃生口打到的 endpoint 不保證
// 回應一定是合法 JSON（例如 204 空 body、純文字錯誤頁）。
func (c *RawClient) Do(ctx context.Context, service RawService, method, path string, body []byte, headers map[string]string) (RawResponse, error) {
	var res RawResponse

	if !strings.HasPrefix(path, "/") {
		return res, fmt.Errorf("path 必須以 / 開頭: %q", path)
	}
	if strings.Contains(path, "://") {
		return res, fmt.Errorf("path 不得包含 scheme 或 host（不可傳入絕對 URL）: %q", path)
	}
	for name := range headers {
		if IsReservedRawHeader(name) {
			return res, fmt.Errorf("header %q 由 twaictl 依 settings 自動注入，不可自行指定", name)
		}
	}

	urlHost, headerHost, err := rawServiceHosts(c.settings, service)
	if err != nil {
		return res, err
	}
	fullURL := strings.TrimRight(BaseURL(c.settings.APIGateway, urlHost), "/") + path

	var reqBody io.Reader
	if len(body) > 0 {
		reqBody = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, fullURL, reqBody)
	if err != nil {
		return res, fmt.Errorf("建立 HTTP request 失敗: %w", err)
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}

	httpClient := newServiceHTTPClient(c.settings, headerHost, c.opts)

	resp, err := httpClient.Do(req)
	if err != nil {
		return res, fmt.Errorf("送出請求失敗: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return res, fmt.Errorf("讀取回應失敗: %w", err)
	}

	if err := CheckResponse(resp, respBody); err != nil {
		return res, err
	}
	res.StatusCode = resp.StatusCode
	res.Status = resp.Status
	res.Raw = respBody
	return res, nil
}

// rawServiceHosts 依 service 回傳「組 URL 用的 host 段」與「x-api-host header 預設值」。
// Common API 的 URL 沒有 host 段（urlHost 為空字串），但 header 仍要帶預設值
// （見 internal/twai/client.go NewClient 對 Common client 的說明）。
func rawServiceHosts(settings *config.Settings, service RawService) (urlHost, headerHost string, err error) {
	switch service {
	case RawServiceVCS:
		return settings.APIHosts.VCS, settings.APIHosts.VCS, nil
	case RawServiceCOS:
		return settings.APIHosts.COS, settings.APIHosts.COS, nil
	case RawServiceCommon:
		return "", settings.APIHosts.Common, nil
	default:
		return "", "", fmt.Errorf("不支援的 service %q（可用：vcs、cos、common）", service)
	}
}
