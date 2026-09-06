package testutil

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// Recorded 是 FakeAPI 收到的一次請求記錄。
type Recorded struct {
	Method string
	Path   string
	Query  string
	Header http.Header
	Body   []byte
}

// Seq 是 OnSequence 的單一步回應。
type Seq struct {
	Status int
	Body   []byte
}

// FakeAPI 以 method+path 對應固定回應，並記錄每個收到的請求，供 cli 與
// twai/vcs 的測試共用。所有讀寫都受 mu 保護，可安全在 -race 下平行使用。
type FakeAPI struct {
	srv      *httptest.Server
	mu       sync.Mutex
	routes   map[string]http.HandlerFunc
	requests []Recorded
}

// NewFakeAPI 啟動一個假 API server；呼叫端透過 On / OnText / OnSequence 註冊回應，
// server 會在 t 結束時自動關閉。
func NewFakeAPI(t *testing.T) *FakeAPI {
	t.Helper()
	f := &FakeAPI{routes: map[string]http.HandlerFunc{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.srv.Close)
	return f
}

// handle 是假 API server 的統一入口：記錄請求後轉交給對應 route 的 handler；
// 沒有註冊過的 method+path 一律回應 404。
func (f *FakeAPI) handle(w http.ResponseWriter, r *http.Request) {
	var body []byte
	if r.Body != nil {
		body, _ = io.ReadAll(r.Body)
	}
	f.mu.Lock()
	f.requests = append(f.requests, Recorded{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Clone(), body})
	handler := f.routes[r.Method+" "+r.URL.Path]
	f.mu.Unlock()

	if handler == nil {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"no route in fake api: ` + r.Method + " " + r.URL.Path + `"}`))
		return
	}
	handler(w, r)
}

// On 註冊固定狀態碼與 JSON body 的回應。
func (f *FakeAPI) On(method, path string, status int, body []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.routes[method+" "+path] = func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}
}

// OnText 註冊固定狀態碼與純文字 body 的回應（例如 CreateKeypair 回傳的 PEM）。
func (f *FakeAPI) OnText(method, path string, status int, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.routes[method+" "+path] = func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

// OnSequence 依呼叫次序回傳不同回應；用完最後一個之後持續重複最後一個
// （供 --wait 輪詢測試模擬狀態從 Initializing 變成 Ready）。
func (f *FakeAPI) OnSequence(method, path string, responses ...Seq) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var callMu sync.Mutex
	count := 0
	f.routes[method+" "+path] = func(w http.ResponseWriter, _ *http.Request) {
		callMu.Lock()
		idx := count
		if idx >= len(responses) {
			idx = len(responses) - 1
		}
		count++
		callMu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(responses[idx].Status)
		_, _ = w.Write(responses[idx].Body)
	}
}

// Find 回傳最後一次符合 method+path 的請求；沒有則回傳 nil。
func (f *FakeAPI) Find(method, path string) *Recorded {
	f.mu.Lock()
	defer f.mu.Unlock()
	var found *Recorded
	for i := range f.requests {
		if f.requests[i].Method == method && f.requests[i].Path == path {
			r := f.requests[i]
			found = &r
		}
	}
	return found
}

// Requests 回傳目前為止收到的所有請求（加鎖複本，呼叫端可安全讀取）。
func (f *FakeAPI) Requests() []Recorded {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Recorded, len(f.requests))
	copy(out, f.requests)
	return out
}

// URL 回傳假 API server 的網址。
func (f *FakeAPI) URL() string {
	return f.srv.URL
}
