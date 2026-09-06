package twai

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestTransport 建立不會真的 sleep 的 transport，並記錄 sleep 時間。
func newTestTransport(opts TransportOptions, slept *[]time.Duration) *Transport {
	opts.Sleep = func(d time.Duration) { *slept = append(*slept, d) }
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return NewTransport(opts)
}

func TestTransportInjectsHeaders(t *testing.T) {
	var gotHeader http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var slept []time.Duration
	client := &http.Client{Transport: newTestTransport(TransportOptions{
		APIKey: "secret", APIHost: "openstack-taichung-default-2", UserAgent: "twaictl/test (linux/amd64)",
	}, &slept)}

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/sites/", strings.NewReader(`{}`))
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, "secret", gotHeader.Get(HeaderAPIKey))
	assert.Equal(t, "openstack-taichung-default-2", gotHeader.Get(HeaderAPIHost))
	assert.Equal(t, "twaictl/test (linux/amd64)", gotHeader.Get("User-Agent"))
	assert.Equal(t, "application/json", gotHeader.Get("Content-Type"))
}

func TestTransportDoesNotOverrideExistingAPIHost(t *testing.T) {
	var gotHost string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Header.Get(HeaderAPIHost)
	}))
	defer srv.Close()

	var slept []time.Duration
	client := &http.Client{Transport: newTestTransport(TransportOptions{APIKey: "k", APIHost: "default-host"}, &slept)}
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	req.Header.Set(HeaderAPIHost, "explicit-host")
	resp, err := client.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()

	assert.Equal(t, "explicit-host", gotHost)
}

func TestTransportRetriesGetOnRetryableStatus(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`ok`))
	}))
	defer srv.Close()

	var slept []time.Duration
	client := &http.Client{Transport: newTestTransport(TransportOptions{APIKey: "k"}, &slept)}
	resp, err := client.Get(srv.URL)
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "ok", string(body))
	assert.Equal(t, int32(3), atomic.LoadInt32(&calls))
	assert.Equal(t, []time.Duration{500 * time.Millisecond, time.Second}, slept)
}

func TestTransportGivesUpAfterMaxRetries(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"detail":"upstream down"}`))
	}))
	defer srv.Close()

	var slept []time.Duration
	client := &http.Client{Transport: newTestTransport(TransportOptions{APIKey: "k"}, &slept)}
	resp, err := client.Get(srv.URL)
	require.NoError(t, err)
	body, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	assert.Equal(t, http.StatusBadGateway, resp.StatusCode)
	require.NoError(t, readErr, "重試耗盡後回傳的 response body 不可已被關閉")
	assert.Equal(t, `{"detail":"upstream down"}`, string(body))
	assert.Equal(t, int32(4), atomic.LoadInt32(&calls), "1 次原始 + 3 次重試")
	assert.Equal(t, []time.Duration{500 * time.Millisecond, time.Second, 2 * time.Second}, slept)
}

func TestTransportDoesNotRetryNonGet(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	var slept []time.Duration
	client := &http.Client{Transport: newTestTransport(TransportOptions{APIKey: "k"}, &slept)}
	resp, err := client.Post(srv.URL, "application/json", strings.NewReader(`{}`))
	require.NoError(t, err)
	_ = resp.Body.Close()

	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))
	assert.Empty(t, slept)
}

func TestTransportDoesNotRetryGetWithNonRewindableBody(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	var slept []time.Duration
	client := &http.Client{Transport: newTestTransport(TransportOptions{APIKey: "k"}, &slept)}
	req, _ := http.NewRequest(http.MethodGet, srv.URL, io.NopCloser(strings.NewReader(`{"q":1}`)))
	req.GetBody = nil // 明確表示 body 不可重繞
	resp, err := client.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()

	assert.Equal(t, int32(1), atomic.LoadInt32(&calls), "body 不可重繞的 GET 不可重試")
	assert.Empty(t, slept)
}

func TestTransportRetriesGetWithRewindableBody(t *testing.T) {
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(data))
		if len(bodies) < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var slept []time.Duration
	client := &http.Client{Transport: newTestTransport(TransportOptions{APIKey: "k"}, &slept)}
	// http.NewRequest 對 *strings.Reader 會自動設定 GetBody
	req, _ := http.NewRequest(http.MethodGet, srv.URL, strings.NewReader(`{"q":1}`))
	resp, err := client.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, []string{`{"q":1}`, `{"q":1}`}, bodies, "重試時 body 必須完整重送")
}

func TestTransportDoesNotRetryNonRetryableStatus(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	var slept []time.Duration
	client := &http.Client{Transport: newTestTransport(TransportOptions{APIKey: "k"}, &slept)}
	resp, err := client.Get(srv.URL)
	require.NoError(t, err)
	_ = resp.Body.Close()

	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))
}

func TestTransportRetriesGetOnNetworkError(t *testing.T) {
	// 先關掉 server 讓連線失敗
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	addr := srv.URL
	srv.Close()

	var slept []time.Duration
	client := &http.Client{Transport: newTestTransport(TransportOptions{APIKey: "k", MaxRetries: 2}, &slept)}
	_, err := client.Get(addr)
	require.Error(t, err)
	assert.True(t, IsNetworkError(err))
	assert.Len(t, slept, 2)
}

func TestTransportStopsRetryingWhenContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	var slept []time.Duration
	tr := newTestTransport(TransportOptions{APIKey: "k"}, &slept)
	tr.Sleep = func(time.Duration) { cancel() } // 第一次退避時取消

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	_, err := (&http.Client{Transport: tr}).Do(req)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestTransportDryRunPrintsCurlAndReturns204(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
	}))
	defer srv.Close()

	var out bytes.Buffer
	var slept []time.Duration
	client := &http.Client{Transport: newTestTransport(TransportOptions{
		APIKey: "top-secret", APIHost: "openstack-taichung-default-2", DryRun: true, DryRunWriter: &out,
	}, &slept)}
	resp, err := client.Post(srv.URL+"/sites/", "application/json", strings.NewReader(`{"name":"x"}`))
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Empty(t, body)
	assert.Equal(t, int32(0), atomic.LoadInt32(&calls), "dry-run 不可送出請求")
	assert.Contains(t, out.String(), "curl -X POST")
	assert.Contains(t, out.String(), `-d '{"name":"x"}'`)
	assert.Contains(t, out.String(), "x-api-key: ***")
	assert.NotContains(t, out.String(), "top-secret")
	assert.True(t, strings.HasSuffix(out.String(), "\n"))
	assert.True(t, IsDryRunResponse(resp), "合成的 204 回應要能被 IsDryRunResponse 辨識")
}

func TestIsDryRunResponseFalseForNormalResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var slept []time.Duration
	client := &http.Client{Transport: newTestTransport(TransportOptions{APIKey: "k"}, &slept)}
	resp, err := client.Get(srv.URL)
	require.NoError(t, err)
	_ = resp.Body.Close()

	assert.False(t, IsDryRunResponse(resp), "一般 200 回應不可被誤判為 dry-run")
}

func TestTransportLogsNeverContainAPIKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"detail":"boom"}`))
	}))
	defer srv.Close()

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	var slept []time.Duration
	client := &http.Client{Transport: newTestTransport(TransportOptions{
		APIKey: "top-secret", Verbose: true, Logger: logger, MaxRetries: -1,
	}, &slept)}
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/x?token=top-secret", nil)
	resp, err := client.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()

	logs := logBuf.String()
	assert.Contains(t, logs, "status=500")
	assert.Contains(t, logs, "boom", "verbose 且非 2xx 時記錄 body")
	assert.NotContains(t, logs, "top-secret")
}

func TestTransportVerboseLogBodyIsReadableByCaller(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"detail":"bad"}`))
	}))
	defer srv.Close()

	var slept []time.Duration
	client := &http.Client{Transport: newTestTransport(TransportOptions{APIKey: "k", Verbose: true}, &slept)}
	resp, err := client.Get(srv.URL)
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	assert.Equal(t, `{"detail":"bad"}`, string(body), "記錄 body 後仍要讓 caller 讀得到完整內容")
}
