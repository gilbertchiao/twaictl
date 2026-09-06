package vcs

import (
	"bytes"
	"context"
	"encoding/json"
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

// fixture 讀取 testdata/fixtures/vcs/<name>.json。
func fixture(t *testing.T, name string) []byte {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "testdata", "fixtures", "vcs", name+".json")
	data, err := os.ReadFile(path)
	require.NoError(t, err, "讀取 fixture %s", name)
	return data
}

// seq 是 onSequence 的單一步回應，型別對齊 testutil.Seq 以維持現有測試呼叫端寫法。
type seq = testutil.Seq

// apiHarness 是 testutil.FakeAPI 的薄包裝，補上 onSequence 的呼叫端相容寫法。
type apiHarness struct {
	*testutil.FakeAPI
}

func newFakeAPI(t *testing.T) *apiHarness {
	return &apiHarness{FakeAPI: testutil.NewFakeAPI(t)}
}

// on 註冊固定狀態碼與 body 的回應。
func (f *apiHarness) on(method, path string, status int, body []byte) {
	f.On(method, path, status, body)
}

// onText 註冊固定狀態碼與純文字 body 的回應（例如 CreateKeypair 回傳的 PEM）。
func (f *apiHarness) onText(method, path string, status int, body string) {
	f.OnText(method, path, status, body)
}

// onSequence 依呼叫次序回傳不同回應（最後一個之後重複最後一個），供 --wait 輪詢測試。
func (f *apiHarness) onSequence(method, path string, responses ...seq) {
	f.OnSequence(method, path, responses...)
}

func newService(t *testing.T, f *apiHarness) *Service {
	settings := &config.Settings{APIKey: "k", APIGateway: f.URL(), APIHosts: config.DefaultAPIHosts}
	client, err := twai.NewClient(settings, twai.ClientOptions{Version: "test"})
	require.NoError(t, err)
	return New(client)
}

const vcsBase = "/api/v3/openstack-taichung-default-2"

func TestListProjectsReturnsValueAndRaw(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/projects/", 200, fixture(t, "projects"))
	s := newService(t, f)

	res, err := s.ListProjects(context.Background())
	require.NoError(t, err)
	require.Len(t, res.Value, 2)
	assert.Equal(t, "ENT1", res.Value[0].Name)
	assert.JSONEq(t, string(fixture(t, "projects")), string(res.Raw))
	assert.Equal(t, "openstack-taichung-default-2", f.Requests()[0].Header.Get("x-api-host"))
}

func TestGetProjectReturnsDetail(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/projects/101/", 200, fixture(t, "project_detail"))
	s := newService(t, f)

	res, err := s.GetProject(context.Background(), 101)
	require.NoError(t, err)
	assert.Equal(t, "ENT1", res.Value.Name)
	assert.Equal(t, "企業專案", res.Value.Desc)
}

func TestListSitesSendsProjectAndAllUsers(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/sites/", 200, fixture(t, "sites"))
	s := newService(t, f)

	_, err := s.ListSites(context.Background(), 101, true)
	require.NoError(t, err)
	// 生成碼依 GetSitesParams 欄位宣告順序（Project 先於 AllUsers）組 query string，
	// 而非 url.Values.Encode() 的字母序，故順序為 project 在前。
	assert.Equal(t, "project=101&all_users=1", f.Requests()[0].Query)

	_, err = s.ListSites(context.Background(), 101, false)
	require.NoError(t, err)
	assert.Equal(t, "project=101", f.Requests()[1].Query, "未指定 --all 時不送 all_users")
}

func TestGetSiteReturnsSingle(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/sites/7/", 200, fixture(t, "site_ready"))
	s := newService(t, f)

	res, err := s.GetSite(context.Background(), 7)
	require.NoError(t, err)
	assert.Equal(t, "web-01", res.Value.Name)
	assert.Equal(t, SiteStatusReady, string(res.Value.Status))
}

func TestCreateSiteSendsBodyAndOnlySetHeaders(t *testing.T) {
	f := newFakeAPI(t)
	f.on("POST", vcsBase+"/sites/", 201, fixture(t, "site_initializing"))
	s := newService(t, f)

	res, err := s.CreateSite(context.Background(), CreateSiteInput{
		Name: "web-01", ProjectID: 101, SolutionID: 11, Image: "Ubuntu 22.04", Flavor: "v.2xsmall",
		Keypair: "mykey", FloatingIP: true, VolumeSize: 100, VolumeType: "ssd",
	})
	require.NoError(t, err)
	assert.Equal(t, int64(7), res.Value.Id)

	req := f.Requests()[0]
	var body map[string]any
	require.NoError(t, json.Unmarshal(req.Body, &body))
	assert.Equal(t, map[string]any{"name": "web-01", "project": float64(101), "solution": float64(11)}, body, "desc 為空時不送")
	assert.Equal(t, "Ubuntu 22.04", req.Header.Get("x-extra-property-image"))
	assert.Equal(t, "v.2xsmall", req.Header.Get("x-extra-property-flavor"))
	assert.Equal(t, "mykey", req.Header.Get("x-extra-property-keypair"))
	assert.Equal(t, "floating", req.Header.Get("x-extra-property-floating-ip"))
	assert.Equal(t, "100", req.Header.Get("x-extra-property-volume-size"))
	assert.Equal(t, "ssd", req.Header.Get("x-extra-property-volume-type"))
	assert.Empty(t, req.Header.Get("x-extra-property-password"), "未指定的 header 不送")
	assert.Empty(t, req.Header.Get("x-extra-property-private-network"))
	assert.Empty(t, req.Header.Get("x-extra-property-system-volume-size"))
}

func TestCreateSiteNoFloatingIPSendsNofloating(t *testing.T) {
	f := newFakeAPI(t)
	f.on("POST", vcsBase+"/sites/", 201, fixture(t, "site_initializing"))
	s := newService(t, f)

	_, err := s.CreateSite(context.Background(), CreateSiteInput{
		Name: "web-01", ProjectID: 101, SolutionID: 11, FloatingIP: false,
	})
	require.NoError(t, err)
	assert.Equal(t, "nofloating", f.Requests()[0].Header.Get("x-extra-property-floating-ip"))
}

func TestDeleteSiteAndAPIError(t *testing.T) {
	f := newFakeAPI(t)
	f.on("DELETE", vcsBase+"/sites/7/", 204, nil)
	f.on("DELETE", vcsBase+"/sites/8/", 404, []byte(`{"message":"Site not found"}`))
	s := newService(t, f)

	require.NoError(t, s.DeleteSite(context.Background(), 7))
	err := s.DeleteSite(context.Background(), 8)
	var apiErr *twai.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 404, apiErr.StatusCode)
	assert.Equal(t, "Site not found", apiErr.Message)
}

func TestListFlavorsOptionalProject(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/flavors/", 200, fixture(t, "flavors"))
	s := newService(t, f)

	_, err := s.ListFlavors(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, f.Requests()[0].Query, "projectID 為 nil 時不送 project")

	projectID := int64(101)
	_, err = s.ListFlavors(context.Background(), &projectID)
	require.NoError(t, err)
	assert.Equal(t, "project=101", f.Requests()[1].Query)
}

func TestListImagesRequiresProject(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/images/", 200, fixture(t, "images"))
	s := newService(t, f)

	res, err := s.ListImages(context.Background(), 101)
	require.NoError(t, err)
	require.Len(t, res.Value, 2)
	assert.Equal(t, "project=101", f.Requests()[0].Query)
}

func TestListKeypairs(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/keypairs/", 200, fixture(t, "keypairs"))
	s := newService(t, f)

	res, err := s.ListKeypairs(context.Background())
	require.NoError(t, err)
	require.Len(t, res.Value, 2)
	assert.Equal(t, "mykey", res.Value[0].Name)
}

func TestKeypairCreateReturnsRawText(t *testing.T) {
	f := newFakeAPI(t)
	f.onText("POST", vcsBase+"/keypairs/", 200, "-----BEGIN RSA PRIVATE KEY-----\nabc\n-----END RSA PRIVATE KEY-----\n")
	s := newService(t, f)
	pem, err := s.CreateKeypair(context.Background(), "mykey", "")
	require.NoError(t, err)
	assert.Contains(t, pem, "BEGIN RSA PRIVATE KEY")
	var body map[string]any
	require.NoError(t, json.Unmarshal(f.Requests()[0].Body, &body))
	assert.Equal(t, map[string]any{"name": "mykey"}, body, "未提供 public_key 時不送該欄位")
}

func TestKeypairCreateWithPublicKey(t *testing.T) {
	f := newFakeAPI(t)
	f.onText("POST", vcsBase+"/keypairs/", 200, "")
	s := newService(t, f)
	_, err := s.CreateKeypair(context.Background(), "mykey", "ssh-rsa AAAA...")
	require.NoError(t, err)

	var body map[string]any
	require.NoError(t, json.Unmarshal(f.Requests()[0].Body, &body))
	assert.Equal(t, map[string]any{"name": "mykey", "public_key": "ssh-rsa AAAA..."}, body)
}

func TestGetKeypairParsesDetail(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/keypairs/mykey/", 200, fixture(t, "keypair_detail"))
	s := newService(t, f)

	res, err := s.GetKeypair(context.Background(), "mykey")
	require.NoError(t, err)
	require.NotNil(t, res.Value.PublicKey)
	assert.Contains(t, *res.Value.PublicKey, "ssh-rsa")
}

func TestDeleteKeypair(t *testing.T) {
	f := newFakeAPI(t)
	f.on("DELETE", vcsBase+"/keypairs/mykey/", 204, nil)
	s := newService(t, f)
	require.NoError(t, s.DeleteKeypair(context.Background(), "mykey"))
}

func TestDryRunReturnsErrDryRun(t *testing.T) {
	f := newFakeAPI(t)
	settings := &config.Settings{APIKey: "k", APIGateway: f.URL(), APIHosts: config.DefaultAPIHosts}
	var curl bytes.Buffer
	client, err := twai.NewClient(settings, twai.ClientOptions{Version: "test", DryRun: true, DryRunWriter: &curl})
	require.NoError(t, err)
	s := New(client)
	_, err = s.ListProjects(context.Background())
	assert.ErrorIs(t, err, twai.ErrDryRun)
	assert.Contains(t, curl.String(), "curl -X GET")
	assert.Empty(t, f.Requests(), "dry-run 不可送出請求")
}
