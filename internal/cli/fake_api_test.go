package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gilbertchiao/twaictl/internal/config"
	"github.com/gilbertchiao/twaictl/internal/testutil"
)

// vcsBase 是 VCS 服務的 API 路徑前綴（x-api-host = openstack-taichung-default-2）。
const vcsBase = "/api/v3/openstack-taichung-default-2"

// cephBase 是 Ceph（COS）服務的 API 路徑前綴（x-api-host = ceph-taichung-default）。
// Ceph 的 project id 與 VCS 是各自獨立的編號空間，因此 cos 命令一律走這個前綴，
// 不會沿用 vcsBase 已解析出的 project id（見 internal/twai/cos.Service.ResolveProjectID）。
const cephBase = "/api/v3/ceph-taichung-default"

// seq 是 onSequence 的單一步回應（body 為字串，方便測試內嵌 JSON 字面值）。
type seq struct {
	status int
	body   string
}

// apiHarness 是 testutil.FakeAPI 的薄包裝，補上 cli 命令整合測試常用、與設定相關
// 的輔助方法（讀 fixture 檔、預先註冊 project 清單、組測試用環境變數）。
type apiHarness struct {
	*testutil.FakeAPI
	t *testing.T
}

// newFakeAPI 啟動假 API server，並預先註冊 GET /projects/ 讓 projectID() 能把
// ENT1 解析成 101（大多數命令測試都需要先解析 project code）；同時預先註冊 Ceph 的
// GET /projects/ 讓 cosProjectID() 能把 ENT1 解析成 60178（cos 命令的測試共用）。
func newFakeAPI(t *testing.T) *apiHarness {
	t.Helper()
	f := &apiHarness{FakeAPI: testutil.NewFakeAPI(t), t: t}
	f.onFixture("GET", vcsBase+"/projects/", 200, "vcs/projects")
	f.onFixture("GET", cephBase+"/projects/", 200, "ceph/projects")
	return f
}

// on 註冊固定狀態碼與 JSON body 的回應。
func (f *apiHarness) on(method, path string, status int, body string) {
	f.On(method, path, status, []byte(body))
}

// onText 註冊固定狀態碼與純文字 body 的回應（例如 CreateKeypair 回傳的 PEM）。
func (f *apiHarness) onText(method, path string, status int, body string) {
	f.OnText(method, path, status, body)
}

// onFixture 註冊回應 body 為 testdata/fixtures/<fixtureName>.json 的內容。
func (f *apiHarness) onFixture(method, path string, status int, fixtureName string) {
	f.on(method, path, status, fixture(f.t, fixtureName))
}

// onSequence 依呼叫次序回傳不同回應；用完最後一個之後持續重複最後一個
// （供 --wait 輪詢測試模擬狀態從 Initializing 變成 Ready）。
func (f *apiHarness) onSequence(method, path string, responses ...seq) {
	converted := make([]testutil.Seq, len(responses))
	for i, r := range responses {
		converted[i] = testutil.Seq{Status: r.status, Body: []byte(r.body)}
	}
	f.OnSequence(method, path, converted...)
}

// find 回傳最後一次符合 method+path 的請求；沒有則回傳 nil。
func (f *apiHarness) find(method, path string) *testutil.Recorded {
	return f.Find(method, path)
}

// env 回傳讓 runCLI 直接呼叫 API 的環境變數：獨立的 XDG_CONFIG_HOME、固定 API key
// 與指向假 API server 的 apigateway。
func (f *apiHarness) env(t *testing.T) map[string]string {
	t.Helper()
	return map[string]string{
		"XDG_CONFIG_HOME":     t.TempDir(),
		config.EnvAPIKey:      "test-key",
		config.EnvAPIGateway:  f.URL(),
		config.EnvProjectCode: "ENT1",
	}
}

// fakeS3Harness 是 testutil.FakeS3 的薄包裝，補上 cli 整合測試慣用的 env() 輔助方法。
// 假 S3 server 與假 Ceph 管理 API（apiHarness）是各自獨立的 httptest server：
// `cos bucket`/`cos ls`/`cos cp`/`cos rm` 只呼叫 S3 資料面，不需要（也不應該）
// 先解析 Ceph project id，因此不與 apiHarness 共用同一組 env。
type fakeS3Harness struct {
	*testutil.FakeS3
}

// newFakeS3 啟動假 S3 server，供 `cos bucket`/`cos ls`/`cos cp`/`cos rm` 的整合測試使用。
func newFakeS3(t *testing.T) *fakeS3Harness {
	t.Helper()
	return &fakeS3Harness{FakeS3: testutil.NewFakeS3(t)}
}

// env 回傳讓 runCLI 直接對假 S3 server 送出請求的環境變數：獨立的 XDG_CONFIG_HOME、
// 固定的 access/secret key，以及指向假 S3 server 的 endpoint。
func (f *fakeS3Harness) env(t *testing.T) map[string]string {
	t.Helper()
	return map[string]string{
		"XDG_CONFIG_HOME":      t.TempDir(),
		config.EnvCOSEndpoint:  f.URL(),
		config.EnvCOSAccessKey: "AKIATEST",
		config.EnvCOSSecretKey: "SECRETTEST",
	}
}

// timestampPattern 比對 output.FormatTime 產生的 "YYYY-MM-DD HH:MM:SS" 字串。
var timestampPattern = regexp.MustCompile(`\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}`)

// normalizeTimestamps 把輸出中所有時間戳記取代成固定字串，供 `cos bucket ls`／`cos ls`
// 這類「時間戳記來自 time.Now()、無法在假 S3 server 上固定」的 golden 測試使用；
// 取代字串與原始格式等長（19 個字元），tabwriter 的欄位對齊不受影響。
func normalizeTimestamps(s string) string {
	return timestampPattern.ReplaceAllString(s, "2024-01-01 00:00:00")
}

// fixture 讀取 testdata/fixtures/<name>.json，回傳原始 JSON 字串。
func fixture(t *testing.T, name string) string {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(thisFile), "..", "..", "testdata", "fixtures", name+".json")
	data, err := os.ReadFile(path)
	require.NoError(t, err, "讀取 fixture %s", name)
	return string(data)
}
