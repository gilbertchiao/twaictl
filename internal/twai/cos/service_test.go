package cos

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilbertchiao/twaictl/internal/config"
	"github.com/gilbertchiao/twaictl/internal/testutil"
	"github.com/gilbertchiao/twaictl/internal/twai"
)

// cephBase 是 Ceph API 在 fake API server 上的路徑前綴（見 config.DefaultAPIHosts.COS）。
const cephBase = "/api/v3/ceph-taichung-default"

// fixture 讀取 testdata/fixtures/ceph/<name>.json。
func fixture(t *testing.T, name string) []byte {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "testdata", "fixtures", "ceph", name+".json")
	data, err := os.ReadFile(path)
	require.NoError(t, err, "讀取 fixture %s", name)
	return data
}

func newService(t *testing.T, f *testutil.FakeAPI) *Service {
	settings := &config.Settings{APIKey: "k", APIGateway: f.URL(), APIHosts: config.DefaultAPIHosts}
	client, err := twai.NewClient(settings, twai.ClientOptions{Version: "test"})
	require.NoError(t, err)
	return New(client)
}

func newDryRunService(t *testing.T, f *testutil.FakeAPI) *Service {
	settings := &config.Settings{APIKey: "k", APIGateway: f.URL(), APIHosts: config.DefaultAPIHosts}
	client, err := twai.NewClient(settings, twai.ClientOptions{Version: "test", DryRun: true, DryRunWriter: httptest.NewRecorder().Body})
	require.NoError(t, err)
	return New(client)
}

func TestResolveProjectIDByCodeAndNumber(t *testing.T) {
	f := testutil.NewFakeAPI(t)
	f.On("GET", cephBase+"/projects/", 200, fixture(t, "projects"))
	s := newService(t, f)

	id, err := s.ResolveProjectID(context.Background(), "ENT1")
	require.NoError(t, err)
	assert.Equal(t, int64(60178), id)
	require.Len(t, f.Requests(), 1)
	assert.Equal(t, "name=ENT1", f.Requests()[0].Query)
	assert.Equal(t, "ceph-taichung-default", f.Requests()[0].Header.Get("x-api-host"))

	id, err = s.ResolveProjectID(context.Background(), "999")
	require.NoError(t, err)
	assert.Equal(t, int64(999), id, "數字 id 直接視為 id")
	assert.Len(t, f.Requests(), 1, "數字 id 不查 API")

	_, err = s.ResolveProjectID(context.Background(), "NOPE")
	assert.ErrorContains(t, err, "找不到 project")
}

func TestGetKeysReturnsValueAndRaw(t *testing.T) {
	f := testutil.NewFakeAPI(t)
	f.On("GET", cephBase+"/projects/60178/key/", 200, fixture(t, "key"))
	s := newService(t, f)

	res, err := s.GetKeys(context.Background(), 60178)
	require.NoError(t, err)
	require.NotNil(t, res.Value.Public)
	assert.Equal(t, "AKIAPUBLIC", res.Value.Public.AccessKey)
	assert.Equal(t, "SECRETPUBLIC", res.Value.Public.SecretKey)
	require.Len(t, res.Value.Private, 1)
	assert.Equal(t, "default", res.Value.Private[0].Name)
	assert.Equal(t, "AKIAPRIV", res.Value.Private[0].AccessKey)
	assert.JSONEq(t, string(fixture(t, "key")), string(res.Raw))
}

func TestCreateKeySendsName(t *testing.T) {
	f := testutil.NewFakeAPI(t)
	f.On("POST", cephBase+"/projects/60178/key/", 200, fixture(t, "key_after_create"))
	s := newService(t, f)

	res, err := s.CreateKey(context.Background(), 60178, "backup")
	require.NoError(t, err)
	require.Len(t, res.Value.Private, 2)

	req := f.Requests()[0]
	var body map[string]any
	require.NoError(t, json.Unmarshal(req.Body, &body))
	assert.Equal(t, map[string]any{"name": "backup"}, body)
}

func TestRenewKeyAllOmitsNameWhenEmpty(t *testing.T) {
	f := testutil.NewFakeAPI(t)
	f.On("PUT", cephBase+"/projects/60178/key/", 200, fixture(t, "key"))
	s := newService(t, f)

	_, err := s.RenewKey(context.Background(), 60178, "", true)
	require.NoError(t, err)

	req := f.Requests()[0]
	var body map[string]any
	require.NoError(t, json.Unmarshal(req.Body, &body))
	assert.Equal(t, map[string]any{"all": true}, body, "name 為空時不送 name 欄位")
}

func TestRenewKeySpecificName(t *testing.T) {
	f := testutil.NewFakeAPI(t)
	f.On("PUT", cephBase+"/projects/60178/key/", 200, fixture(t, "key"))
	s := newService(t, f)

	_, err := s.RenewKey(context.Background(), 60178, "default", false)
	require.NoError(t, err)

	req := f.Requests()[0]
	var body map[string]any
	require.NoError(t, json.Unmarshal(req.Body, &body))
	assert.Equal(t, map[string]any{"name": "default"}, body, "all 為 false 時不送 all 欄位")
}

func TestDeleteKeySendsNameInBody(t *testing.T) {
	f := testutil.NewFakeAPI(t)
	f.On("DELETE", cephBase+"/projects/60178/key/", 200, fixture(t, "key"))
	s := newService(t, f)

	_, err := s.DeleteKey(context.Background(), 60178, "default")
	require.NoError(t, err)

	req := f.Requests()[0]
	var body map[string]any
	require.NoError(t, json.Unmarshal(req.Body, &body))
	assert.Equal(t, map[string]any{"name": "default"}, body)
}

func TestGetKeysNotFoundReturnsAPIError(t *testing.T) {
	f := testutil.NewFakeAPI(t)
	f.On("GET", cephBase+"/projects/60178/key/", 404, []byte(`{"message":"not found"}`))
	s := newService(t, f)

	_, err := s.GetKeys(context.Background(), 60178)
	var apiErr *twai.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusNotFound, apiErr.StatusCode)
}

func TestGetKeysDryRunSendsNoRequest(t *testing.T) {
	f := testutil.NewFakeAPI(t)
	s := newDryRunService(t, f)

	_, err := s.GetKeys(context.Background(), 60178)
	assert.ErrorIs(t, err, twai.ErrDryRun)
	assert.Empty(t, f.Requests(), "dry-run 不應送出任何請求")
}

// TestGetUsageDecodesFlatResponse 驗證 /buckets_util/ 的扁平回應解析，keyName 非空時帶 key_name 查詢參數。
func TestGetUsageDecodesFlatResponse(t *testing.T) {
	fake := testutil.NewFakeAPI(t)
	fake.On("GET", cephBase+"/projects/60178/buckets_util/", 200,
		[]byte(`{"total_used_space": 123456789, "total_objects": 42, "key_name": "backup"}`))
	svc := newService(t, fake)

	res, err := svc.GetUsage(context.Background(), 60178, "backup")
	require.NoError(t, err)
	assert.Equal(t, Usage{KeyName: "backup", TotalUsedSpace: 123456789, TotalObjects: 42}, res.Value)
	req := fake.Find("GET", cephBase+"/projects/60178/buckets_util/")
	require.NotNil(t, req)
	assert.Equal(t, "key_name=backup", req.Query)

	_, err = svc.GetUsage(context.Background(), 60178, "")
	require.NoError(t, err)
	req = fake.Find("GET", cephBase+"/projects/60178/buckets_util/")
	assert.Empty(t, req.Query, "keyName 為空時不送 key_name")
}

// TestListBucketUsageDecodesFlatResponse 驗證 /buckets/ 的扁平回應解析。
func TestListBucketUsageDecodesFlatResponse(t *testing.T) {
	fake := testutil.NewFakeAPI(t)
	fake.On("GET", cephBase+"/projects/60178/buckets/", 200,
		[]byte(`{"key_name": "public", "buckets": [{"bucket_name": "b1", "last_modified_time": "2026-08-01T00:00:00", "bucket_used_space": 1024, "num_objects": 3}]}`))
	svc := newService(t, fake)

	res, err := svc.ListBucketUsage(context.Background(), 60178, "")
	require.NoError(t, err)
	require.Len(t, res.Value, 1)
	assert.Equal(t, BucketUsage{Name: "b1", LastModified: "2026-08-01T00:00:00", UsedSpace: 1024, NumObjects: 3}, res.Value[0])
}

// TestGetKeysRejectsResponseWithoutKeys 驗證 200 但 body 不含 public/private（例如 gateway 回了別的 JSON）時報錯，
// 而不是靜默回傳空的 Keys。
func TestGetKeysRejectsResponseWithoutKeys(t *testing.T) {
	fake := testutil.NewFakeAPI(t)
	fake.On("GET", cephBase+"/projects/60178/key/", 200, []byte(`{"message":"no Route matched with those values"}`))
	svc := newService(t, fake)

	_, err := svc.GetKeys(context.Background(), 60178)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "不含金鑰")
}

// TestGetUsageRejectsResponseWithoutFields 驗證 200 但 body 不含 total_used_space/total_objects
// （例如 gateway 對未知路徑回了別的 JSON）時報錯，而不是靜默顯示「用量為 0」。
func TestGetUsageRejectsResponseWithoutFields(t *testing.T) {
	fake := testutil.NewFakeAPI(t)
	fake.On("GET", cephBase+"/projects/60178/buckets_util/", 200, []byte(`{"message":"no Route matched"}`))
	svc := newService(t, fake)

	_, err := svc.GetUsage(context.Background(), 60178, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "缺少欄位")

	// 欄位存在但值為 JSON null（同樣不能被 json.Unmarshal 默默解讀成 0）也要視同缺少。
	fake.On("GET", cephBase+"/projects/60178/buckets_util/", 200, []byte(`{"total_used_space":null,"total_objects":null,"key_name":"x"}`))
	_, err = svc.GetUsage(context.Background(), 60178, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "缺少欄位")
}

// TestListBucketUsageRejectsResponseWithoutBuckets 驗證 200 但 body 不含 buckets 欄位時報錯；
// 欄位存在但為空陣列（真的沒有 bucket）則視為成功，回傳長度 0。
func TestListBucketUsageRejectsResponseWithoutBuckets(t *testing.T) {
	fake := testutil.NewFakeAPI(t)
	fake.On("GET", cephBase+"/projects/60178/buckets/", 200, []byte(`{"key_name":"public"}`))
	svc := newService(t, fake)

	_, err := svc.ListBucketUsage(context.Background(), 60178, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "缺少欄位")

	// 欄位存在但值為 JSON null（同樣不能被 json.Unmarshal 默默解讀成 nil slice）也要視同缺少。
	fake.On("GET", cephBase+"/projects/60178/buckets/", 200, []byte(`{"key_name":"public","buckets":null}`))
	_, err = svc.ListBucketUsage(context.Background(), 60178, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "缺少欄位")

	fake.On("GET", cephBase+"/projects/60178/buckets/", 200, []byte(`{"key_name":"public","buckets":[]}`))
	res, err := svc.ListBucketUsage(context.Background(), 60178, "")
	require.NoError(t, err)
	assert.Len(t, res.Value, 0)
}
