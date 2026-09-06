package twai

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"time"
)

const (
	defaultMaxRetries = 3
	// logBodyLimit 是 --verbose 時記錄 response body 的上限。
	logBodyLimit = 2 * 1024
)

// retryBackoffs 是每次重試前的等待時間（指數退避）。
var retryBackoffs = []time.Duration{500 * time.Millisecond, time.Second, 2 * time.Second}

// TransportOptions 是 NewTransport 的參數。
type TransportOptions struct {
	APIKey       string
	APIHost      string // request 未帶 x-api-host 時的預設值
	UserAgent    string
	DryRun       bool
	DryRunWriter io.Writer           // dry-run 時 curl 命令的輸出目的地，預設 os.Stdout
	Logger       *slog.Logger        // 預設 slog.Default()
	Verbose      bool                // true 時對非 2xx 回應記錄 body 前 2KB
	MaxRetries   int                 // 預設 3；設為負數代表不重試
	Sleep        func(time.Duration) // 退避用，測試可注入；預設 time.Sleep
	Base         http.RoundTripper   // 預設 http.DefaultTransport
}

// Transport 是 twaictl 所有 API 呼叫共用的 http.RoundTripper。
type Transport struct {
	opts TransportOptions
	// Sleep 公開給測試在建立後替換。
	Sleep func(time.Duration)
}

// NewTransport 建立 Transport 並填入預設值。
func NewTransport(opts TransportOptions) *Transport {
	if opts.Base == nil {
		opts.Base = http.DefaultTransport
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.DryRunWriter == nil {
		opts.DryRunWriter = os.Stdout
	}
	if opts.MaxRetries == 0 {
		opts.MaxRetries = defaultMaxRetries
	}
	if opts.MaxRetries < 0 {
		opts.MaxRetries = 0
	}
	sleep := opts.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	return &Transport{opts: opts, Sleep: sleep}
}

// RoundTrip 實作 http.RoundTripper。
//
// 限制：帶 body 的 GET（型別上允許但實務上少見）只有在 req.GetBody 可用時才會重試，
// 因為重試需要重新取得完整 body——同一個 io.Reader 在第一次嘗試時已被消耗，
// 若不確認可重繞就重試，第二次送出的會是空 body。
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	// 依 RoundTripper 契約不可修改原 request，先複製
	req = req.Clone(req.Context())
	t.injectHeaders(req)

	if t.opts.DryRun {
		return t.dryRun(req)
	}

	var lastResp *http.Response
	var lastErr error
	for attempt := 0; attempt <= t.opts.MaxRetries; attempt++ {
		if attempt > 0 {
			if err := t.waitBeforeRetry(req, attempt); err != nil {
				return nil, err
			}
			if req.GetBody != nil {
				body, err := req.GetBody()
				if err != nil {
					return nil, fmt.Errorf("重新取得 request body 失敗: %w", err)
				}
				req.Body = body
			}
		}
		lastResp, lastErr = t.doOnce(req, attempt)
		isLastAttempt := attempt == t.opts.MaxRetries
		if isLastAttempt || !t.shouldRetry(req, lastResp, lastErr) {
			break
		}
		// 只有還會再試時才丟棄 body；若這是最後一次嘗試，body 要完整保留給 caller 讀取
		if lastResp != nil {
			_, _ = io.Copy(io.Discard, lastResp.Body)
			_ = lastResp.Body.Close()
		}
	}
	return lastResp, lastErr
}

// injectHeaders 補上 API 需要的 header；已存在的值不覆寫。
func (t *Transport) injectHeaders(req *http.Request) {
	if req.Header.Get(HeaderAPIKey) == "" && t.opts.APIKey != "" {
		req.Header.Set(HeaderAPIKey, t.opts.APIKey)
	}
	if req.Header.Get(HeaderAPIHost) == "" && t.opts.APIHost != "" {
		req.Header.Set(HeaderAPIHost, t.opts.APIHost)
	}
	if req.Header.Get("User-Agent") == "" && t.opts.UserAgent != "" {
		req.Header.Set("User-Agent", t.opts.UserAgent)
	}
	if req.Body != nil && req.Body != http.NoBody && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
}

// dryRun 印出 curl 命令並回傳合成的 204 回應。
func (t *Transport) dryRun(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil && req.Body != http.NoBody {
		var err error
		body, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("讀取 request body 失敗: %w", err)
		}
		_ = req.Body.Close()
	}
	if _, err := fmt.Fprintln(t.opts.DryRunWriter, BuildCurl(req, body)); err != nil {
		return nil, fmt.Errorf("輸出 dry-run 命令失敗: %w", err)
	}
	header := make(http.Header)
	header.Set(HeaderDryRun, "true")
	return &http.Response{
		StatusCode: http.StatusNoContent,
		Status:     "204 No Content (dry-run)",
		Header:     header,
		Body:       io.NopCloser(bytes.NewReader(nil)),
		Request:    req,
	}, nil
}

// IsDryRunResponse 判斷 resp 是否為 dryRun() 合成的回應。
// Phase 1 每個命令呼叫後應先判斷 dry-run 再 render（合成回應的 JSONxxx 皆為 nil）。
func IsDryRunResponse(resp *http.Response) bool {
	return resp != nil && resp.Header.Get(HeaderDryRun) == "true"
}

// doOnce 送出一次請求並記錄 debug log。
func (t *Transport) doOnce(req *http.Request, attempt int) (*http.Response, error) {
	start := time.Now()
	resp, err := t.opts.Base.RoundTrip(req)
	elapsed := time.Since(start)

	safeURL := stripQuery(req.URL)
	if err != nil {
		t.opts.Logger.Debug("http request failed",
			"method", req.Method, "url", safeURL, "attempt", attempt,
			"duration_ms", elapsed.Milliseconds(), "error", err.Error())
		return nil, err
	}

	attrs := []any{
		"method", req.Method, "url", safeURL, "status", resp.StatusCode,
		"attempt", attempt, "duration_ms", elapsed.Milliseconds(),
	}
	if t.opts.Verbose && !isSuccess(resp.StatusCode) {
		snippet, restoreErr := peekBody(resp)
		if restoreErr != nil {
			return nil, restoreErr
		}
		attrs = append(attrs, "body", snippet)
	}
	t.opts.Logger.Debug("http request", attrs...)
	return resp, nil
}

// peekBody 讀出 body 前 logBodyLimit bytes 供記錄，並把完整 body 放回 resp 讓 caller 仍可讀取。
func peekBody(resp *http.Response) (string, error) {
	data, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return "", fmt.Errorf("讀取 response body 失敗: %w", err)
	}
	resp.Body = io.NopCloser(bytes.NewReader(data))
	if len(data) > logBodyLimit {
		return string(data[:logBodyLimit]) + "...(truncated)", nil
	}
	return string(data), nil
}

// shouldRetry 只對冪等的 GET、且為暫時性失敗時回 true。
func (t *Transport) shouldRetry(req *http.Request, resp *http.Response, err error) bool {
	if req.Method != http.MethodGet {
		return false
	}
	// 帶 body 的 GET 只有在提供 GetBody（可重新取得 body）時才重試，否則第二次送出的 body 會是空的
	if req.Body != nil && req.Body != http.NoBody && req.GetBody == nil {
		return false
	}
	if err != nil {
		// 網路層錯誤（連線失敗、逾時）；context 已取消則不重試
		return req.Context().Err() == nil
	}
	switch resp.StatusCode {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

// waitBeforeRetry 依退避表等待；等待後若 context 已取消則回傳其錯誤。
func (t *Transport) waitBeforeRetry(req *http.Request, attempt int) error {
	backoff := retryBackoffs[len(retryBackoffs)-1]
	if attempt-1 < len(retryBackoffs) {
		backoff = retryBackoffs[attempt-1]
	}
	t.opts.Logger.Debug("http retry", "method", req.Method, "url", stripQuery(req.URL),
		"attempt", attempt, "backoff", backoff.String())
	t.Sleep(backoff)
	if err := req.Context().Err(); err != nil {
		return &url.Error{Op: req.Method, URL: stripQuery(req.URL), Err: err}
	}
	return nil
}

func isSuccess(status int) bool {
	return status >= 200 && status < 300
}
