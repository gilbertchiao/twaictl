//go:build integration

// 本檔改用外部測試套件（package twai_test），因為 TestIntegrationCOSReadOnly 需要同時
// import internal/twai 與 internal/twai/cos；cos 套件本身已 import twai，若本檔仍宣告
// package twai，會構成 import cycle（twai -> cos -> twai）。twai 對外用到的識別字
// （NewClient、ClientOptions 等）都已匯出，改成外部測試套件不影響可測試性。
package twai_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilbertchiao/twaictl/internal/config"
	"github.com/gilbertchiao/twaictl/internal/gen/common"
	"github.com/gilbertchiao/twaictl/internal/gen/vcs"
	"github.com/gilbertchiao/twaictl/internal/twai"
	"github.com/gilbertchiao/twaictl/internal/twai/cos"
	vcssvc "github.com/gilbertchiao/twaictl/internal/twai/vcs"
)

// TestIntegrationTypedClientDoesNotFollowRedirect 對真實 apigateway 驗證 A26：型別化
// client 必須停在 302（不跟隨、不外洩 x-api-key），CheckResponse 得到 *APIError。
//
// 實測發現：brief 原先設計的「把 nosuch-prefix 接在 settings.APIHosts.VCS 後面」
// （即 /api/v3/<host>/nosuch-prefix/projects/）在真實環境會被 apigateway 轉發給 VCS
// 後端，落到後端自己「找不到路由」的處理（回 200，body 是與預期陣列型別不符的 JSON
// 物件，導致型別化 client 在 JSON unmarshal 階段就失敗），不會重現 A26 的 302；
// 而 brief 括號內給的另一個變體——把 `/api/v3/<host>/nosuch-prefix` 整段接在
// settings.APIGateway 後面（讓 host 段重複出現、形成 apigateway 自己都無法辨識的
// 路徑），才會被 apigateway 外層的 catch-all 導向登入頁，實測穩定重現 302，與
// A26 原始（用 RawClient 打 /nosuch-path/）觀察到的現象一致。詳見
// work/phase3a-sdd/progress.md「Task 1」。
func TestIntegrationTypedClientDoesNotFollowRedirect(t *testing.T) {
	apiKey := os.Getenv(config.EnvAPIKey)
	if apiKey == "" {
		t.Skip("未設定 TWAI_API_KEY，略過真實 API 測試")
	}
	settings, err := config.Resolve(&config.Config{}, "", os.Getenv, config.Overrides{})
	require.NoError(t, err)
	settings.APIGateway = settings.APIGateway + "/api/v3/" + settings.APIHosts.VCS + "/nosuch-prefix"
	client, err := twai.NewClient(settings, twai.ClientOptions{Version: "integration", Timeout: 30 * time.Second})
	require.NoError(t, err)
	resp, err := client.VCS.GetProjectsWithResponse(context.Background(), &vcs.GetProjectsParams{XApiHost: settings.APIHosts.VCS})
	require.NoError(t, err)
	require.True(t, resp.StatusCode() >= 300 && resp.StatusCode() < 400, "預期 3xx，實際 %d body=%s", resp.StatusCode(), string(resp.Body))
	location := resp.HTTPResponse.Header.Get("Location")
	require.NotEmpty(t, location, "3xx 應帶 Location")
	var apiErr *twai.APIError
	require.ErrorAs(t, twai.CheckResponse(resp.HTTPResponse, resp.Body), &apiErr)
	assert.Equal(t, resp.StatusCode(), apiErr.StatusCode)
	assert.Contains(t, apiErr.Message, "Location:")
	assert.NotContains(t, apiErr.Message, "next=")
	t.Logf("redirect 未被跟隨：%s", apiErr.Message)
}

// TestIntegrationReadOnly 對真實 apigateway 做兩個 GET，驗證認證 header 與 v3 路徑。
// 執行：TWAI_API_KEY=... go test -tags integration ./internal/twai/ -run Integration -v
func TestIntegrationReadOnly(t *testing.T) {
	apiKey := os.Getenv(config.EnvAPIKey)
	if apiKey == "" {
		t.Skip("未設定 TWAI_API_KEY，略過真實 API 測試")
	}
	settings, err := config.Resolve(&config.Config{}, "", os.Getenv, config.Overrides{})
	require.NoError(t, err)

	client, err := twai.NewClient(settings, twai.ClientOptions{Version: "integration", Timeout: 30 * time.Second})
	require.NoError(t, err)
	ctx := context.Background()

	ver, err := client.Common.GetVersionWithResponse(ctx, &common.GetVersionParams{XApiHost: settings.APIHosts.Common})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, ver.StatusCode(), "Common /version/ body: %s", string(ver.Body))
	if ver.JSON200 != nil {
		t.Logf("Common API version: %s", ver.JSON200.Version)
	}

	projects, err := client.VCS.GetProjectsWithResponse(ctx, &vcs.GetProjectsParams{XApiHost: settings.APIHosts.VCS})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, projects.StatusCode(), "VCS /projects/ body: %s", string(projects.Body))
	if projects.JSON200 != nil {
		t.Logf("VCS projects 數量: %d", len(*projects.JSON200))
	}
}

// newIntegrationCOSService 是 COS 相關整合測試共用的前置步驟：確認有可用的
// TWAI_API_KEY / TWAI_PROJECT_CODE、建立 twai.Client、建立 cos.Service 並解析出
// Ceph 的數字 project id（與 VCS 的 project id 是各自獨立的編號空間，見 A14，
// 不能沿用 TestIntegrationReadOnly 解析出的 VCS project id）。
// 沒有設定 TWAI_API_KEY 時直接 t.Skip，呼叫端不需要再檢查。
func newIntegrationCOSService(t *testing.T) (*cos.Service, int64) {
	t.Helper()
	apiKey := os.Getenv(config.EnvAPIKey)
	if apiKey == "" {
		t.Skip("未設定 TWAI_API_KEY，略過真實 API 測試")
	}
	settings, err := config.Resolve(&config.Config{}, "", os.Getenv, config.Overrides{})
	require.NoError(t, err)
	require.NotEmpty(t, settings.ProjectCode, "需要 TWAI_PROJECT_CODE 才能解析 Ceph project id")

	client, err := twai.NewClient(settings, twai.ClientOptions{Version: "integration", Timeout: 30 * time.Second})
	require.NoError(t, err)

	cosService := cos.New(client)
	projectID, err := cosService.ResolveProjectID(context.Background(), settings.ProjectCode)
	require.NoError(t, err)
	return cosService, projectID
}

// newIntegrationCOSS3 用 public S3 金鑰建立 S3 client，供需要直接呼叫 S3 相容 API 的
// 整合測試使用（例如 ListBuckets、ListObjects）。全程只印 bucket/物件數量等中繼資料，
// 絕不印出金鑰內容（access key / secret key）。
func newIntegrationCOSS3(t *testing.T, cosService *cos.Service, projectID int64) *cos.S3 {
	t.Helper()
	settings, err := config.Resolve(&config.Config{}, "", os.Getenv, config.Overrides{})
	require.NoError(t, err)

	ctx := context.Background()
	keysResp, err := cosService.GetKeys(ctx, projectID)
	require.NoError(t, err)
	require.NotNil(t, keysResp.Value.Public, "project 應至少有一組 public S3 金鑰")

	s3Client, err := cos.NewS3(cos.S3Options{
		Endpoint:  settings.COS.Endpoint,
		AccessKey: keysResp.Value.Public.AccessKey,
		SecretKey: keysResp.Value.Public.SecretKey,
		Timeout:   30 * time.Second,
	})
	require.NoError(t, err)
	return s3Client
}

// TestIntegrationCOSReadOnly 對真實 Ceph API 與 S3 相容端點做唯讀驗證：
// 取得目前 project 的 public S3 金鑰後，用它建立 S3 client 並呼叫 ListBuckets。
// 全程只印 bucket 數量，絕不印出金鑰內容（access key / secret key）。
// 執行：TWAI_API_KEY=... TWAI_PROJECT_CODE=... go test -tags integration ./internal/twai/ -run IntegrationCOS -v
func TestIntegrationCOSReadOnly(t *testing.T) {
	cosService, projectID := newIntegrationCOSService(t)
	s3Client := newIntegrationCOSS3(t, cosService, projectID)

	buckets, err := s3Client.ListBuckets(context.Background())
	require.NoError(t, err)
	t.Logf("COS ListBuckets 數量: %d", len(buckets))
}

// TestIntegrationCOSUsage 對真實 Ceph API 呼叫 GET /projects/{id}/buckets_util/ 與
// GET /projects/{id}/buckets/（read-only），驗證以 patches/Ceph-buckets-flat.patch
// 修正後的扁平結構（見 docs/api-notes.md A16）能正確解析。結果數字只印到 log，
// 不做斷言（每個 project 的用量與 bucket 數不同）。
func TestIntegrationCOSUsage(t *testing.T) {
	svc, projectID := newIntegrationCOSService(t)
	ctx := context.Background()

	usage, err := svc.GetUsage(ctx, projectID, "")
	require.NoError(t, err)
	t.Logf("total_used_space=%d total_objects=%d key_name=%q",
		usage.Value.TotalUsedSpace, usage.Value.TotalObjects, usage.Value.KeyName)

	buckets, err := svc.ListBucketUsage(ctx, projectID, "")
	require.NoError(t, err)
	for _, b := range buckets.Value {
		t.Logf("bucket=%s used=%d objects=%d modified=%q", b.Name, b.UsedSpace, b.NumObjects, b.LastModified)
	}
}

// errStopListing 是 TestIntegrationCOSListObjectsPagination 用來提前中止 S3.ListObjects 的
// sentinel error：一旦計數跨過第一頁（1000 筆）的邊界就回傳它讓 ListObjects 立即停止，
// 不需要把整顆 bucket（實測環境最大達 1,242 萬個物件）分頁列完，避免對真實 API 發出
// 上萬次請求。
var errStopListing = errors.New("stop")

// TestIntegrationCOSListObjectsPagination 對一個已知物件數 > 1000 的 bucket 做「有界」的
// S3.ListObjects 驗證（Task 7 修正後改用 V1 Marker 分頁的版本，見 docs/api-notes.md A18）：
// 先用 ListBucketUsage（read-only）找出第一個 NumObjects > 1000 的 bucket，再對它呼叫
// ListObjects，數到第 1001 筆就用 errStopListing 中止並斷言確實數到了 1001——這證明分頁
// 修正後成功跨過了「第一頁只有 1000 筆」的邊界，不會像修正前的 ListObjectsV2 那樣被 Ceph
// 誤判為只有一頁而卡住。全程最多發出 2 次 ListObjects 請求（第一頁 + 觸發第二頁分頁的
// 那一次），不完整列舉整顆 bucket。
//
// 分頁邏輯本身的完整性（多頁、Marker 進度、錯誤處理等）已由
// internal/twai/cos/s3_test.go 的 TestListObjectsPaginatesWithMarker（FakeS3）用假資料
// 覆蓋；這裡只需要在真實環境驗證「確實有跨頁」這件事，因此刻意把呼叫量控制在最小必要範圍
// （CLAUDE.md：不對真實 TWCC API 做大量呼叫）。
func TestIntegrationCOSListObjectsPagination(t *testing.T) {
	cosService, projectID := newIntegrationCOSService(t)
	ctx := context.Background()

	buckets, err := cosService.ListBucketUsage(ctx, projectID, "")
	require.NoError(t, err)

	var bucket string
	var numObjects int64
	for _, b := range buckets.Value {
		if b.NumObjects > 1000 {
			bucket, numObjects = b.Name, b.NumObjects
			break
		}
	}
	if bucket == "" {
		t.Skip("沒有超過 1000 個物件的 bucket，無法驗證分頁")
	}
	t.Logf("bucket=%s num_objects=%d", bucket, numObjects)

	s3Client := newIntegrationCOSS3(t, cosService, projectID)
	var count int
	err = s3Client.ListObjects(ctx, bucket, "", func(cos.Object) error {
		count++
		if count >= 1001 {
			return errStopListing
		}
		return nil
	})
	require.ErrorIs(t, err, errStopListing)
	require.Equal(t, 1001, count, "應成功跨過第一頁（1000 筆）的邊界")
}

// newIntegrationVCSService 是 VCS 相關整合測試共用的前置步驟：確認有可用的
// TWAI_API_KEY / TWAI_PROJECT_CODE、建立 twai.Client、建立 vcssvc.Service 並解析出
// VCS 的數字 project id（與 Ceph 的 project id 是各自獨立的編號空間，見 A14，
// 不能沿用 newIntegrationCOSService 解析出的 Ceph project id）。
// 沒有設定 TWAI_API_KEY 時直接 t.Skip，呼叫端不需要再檢查。
func newIntegrationVCSService(t *testing.T) (*vcssvc.Service, int64) {
	t.Helper()
	apiKey := os.Getenv(config.EnvAPIKey)
	if apiKey == "" {
		t.Skip("未設定 TWAI_API_KEY，略過真實 API 測試")
	}
	settings, err := config.Resolve(&config.Config{}, "", os.Getenv, config.Overrides{})
	require.NoError(t, err)
	require.NotEmpty(t, settings.ProjectCode, "需要 TWAI_PROJECT_CODE 才能解析 VCS project id")

	client, err := twai.NewClient(settings, twai.ClientOptions{Version: "integration", Timeout: 30 * time.Second})
	require.NoError(t, err)

	vcsService := vcssvc.New(client)
	projectID, err := vcsService.ResolveProjectID(context.Background(), settings.ProjectCode)
	require.NoError(t, err)
	return vcsService, projectID
}

// TestIntegrationVCSReadOnly 對真實 VCS API 做一輪 read-only 驗證，涵蓋 Phase 2b 新增的
// server／network／eip／volume／snapshot／quota／solutions 查詢，以及（若有資料）
// site 的事件紀錄、security group 規則、metrics。全程只 t.Logf 筆數／摘要，不對回應內容
// 做斷言（真實環境的資料量會隨時間變動），也不印出任何金鑰或敏感資訊。
// 執行：TWAI_API_KEY=... TWAI_PROJECT_CODE=... go test -tags integration ./internal/twai/ -run IntegrationVCS -v
func TestIntegrationVCSReadOnly(t *testing.T) {
	svc, projectID := newIntegrationVCSService(t)
	ctx := context.Background()

	servers, err := svc.ListServers(ctx, projectID)
	require.NoError(t, err)
	t.Logf("VCS servers 數量: %d", len(servers.Value))

	networks, err := svc.ListNetworks(ctx, projectID)
	require.NoError(t, err)
	t.Logf("VCS networks 數量: %d", len(networks.Value))

	ips, err := svc.ListIPs(ctx, projectID, "")
	require.NoError(t, err)
	t.Logf("VCS ips 數量: %d", len(ips.Value))

	volumes, err := svc.ListVolumes(ctx, projectID)
	require.NoError(t, err)
	t.Logf("VCS volumes 數量: %d", len(volumes.Value))

	snapshots, err := svc.ListSnapshots(ctx, projectID, "")
	require.NoError(t, err)
	t.Logf("VCS snapshots 數量: %d", len(snapshots.Value))

	quota, err := svc.ProjectQuota(ctx, projectID)
	require.NoError(t, err)
	t.Logf("VCS project quota: cpu=%+v gpu=%+v memory=%+v", quota.Value.Cpu, quota.Value.Gpu, quota.Value.Memory)

	userQuotas, err := svc.UserQuotas(ctx, projectID)
	require.NoError(t, err)
	t.Logf("VCS user quotas 數量: %d", len(userQuotas.Value))

	solutionIDs, err := svc.ProjectSolutionIDs(ctx, projectID)
	require.NoError(t, err)
	t.Logf("VCS project solutions 數量: %d", len(solutionIDs.Value))

	sites, err := svc.ListSites(ctx, projectID, false)
	require.NoError(t, err)
	if len(sites.Value) > 0 {
		siteID := sites.Value[0].Id
		events, err := svc.SiteEventLogs(ctx, siteID)
		require.NoError(t, err)
		t.Logf("VCS site %d 事件紀錄數量: %d", siteID, len(events.Value))
	} else {
		t.Log("VCS 目前無 site，略過 SiteEventLogs")
	}

	if len(servers.Value) > 0 {
		serverID := servers.Value[0].Id
		secGroups, err := svc.ListSecurityGroups(ctx, projectID, serverID)
		require.NoError(t, err)
		t.Logf("VCS server %d security group 規則數量: %d", serverID, len(secGroups.Value))

		// 帶最近一小時的 begin/end，避免像 A22 實測時那樣不帶時間範圍會回傳全部歷史資料
		// （net-out 曾實測回 1657 筆），對真實 API 造成不必要的大量資料傳輸。
		now := time.Now().UTC()
		begin := now.Add(-time.Hour).Format(time.RFC3339)
		end := now.Format(time.RFC3339)
		metrics, err := svc.Metrics(ctx, serverID, "cpu", begin, end)
		require.NoError(t, err)
		t.Logf("VCS server %d cpu metrics（近一小時）筆數: %d", serverID, len(metrics.Value))
	} else {
		t.Log("VCS 目前無 server，略過 ListSecurityGroups／Metrics")
	}
}
