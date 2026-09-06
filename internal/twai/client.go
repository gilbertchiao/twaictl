package twai

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gilbertchiao/twaictl/internal/config"
	"github.com/gilbertchiao/twaictl/internal/gen/ceph"
	"github.com/gilbertchiao/twaictl/internal/gen/common"
	"github.com/gilbertchiao/twaictl/internal/gen/vcs"
)

// ClientOptions 是 NewClient 的執行期參數（多數來自全域 flag）。
type ClientOptions struct {
	Version      string        // 放進 User-Agent
	Timeout      time.Duration // 單次 HTTP 請求逾時，0 代表不限制
	DryRun       bool
	DryRunWriter io.Writer
	Verbose      bool
	Logger       *slog.Logger
}

// Client 聚合各服務的生成 client，共享同一組設定與 transport 行為。
type Client struct {
	Settings *config.Settings
	VCS      *vcs.ClientWithResponses
	Ceph     *ceph.ClientWithResponses
	Common   *common.ClientWithResponses
}

// BaseURL 組出 `{gateway}/api/v3/{apiHost}/`；apiHost 為空時（Common API）為 `{gateway}/api/v3/`。
func BaseURL(gateway, apiHost string) string {
	base := strings.TrimRight(gateway, "/") + "/api/v3/"
	if apiHost == "" {
		return base
	}
	return base + apiHost + "/"
}

// validateGateway 確認 apigateway 是 http(s):// 開頭的完整 URL（NewClient 與
// NewRawClient 共用）。
func validateGateway(settings *config.Settings) error {
	u, err := url.Parse(settings.APIGateway)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return &config.Error{Msg: "apigateway 必須是 http(s):// 開頭的完整 URL"}
	}
	return nil
}

// newServiceHTTPClient 建立單一服務用的 http.Client：注入 Transport（header、
// retry、dry-run）與 Timeout。CheckRedirect 一律回 http.ErrUseLastResponse：
// Go 只在跨網域時剝除 Authorization/Cookie，x-api-key 不在清單內，跟隨重導向會把
// API key 送到重導向目標（例如 apigateway 對不存在路徑回的內部登入頁，見
// docs/api-notes.md A26）；spec 的所有 path 都以 / 結尾、正常流程不依賴 redirect，
// 3xx 一律交給 CheckResponse 變成 *APIError（exit code 3）。
func newServiceHTTPClient(settings *config.Settings, apiHost string, opts ClientOptions) *http.Client {
	transport := NewTransport(TransportOptions{
		APIKey:       settings.APIKey,
		APIHost:      apiHost,
		UserAgent:    UserAgent(opts.Version),
		DryRun:       opts.DryRun,
		DryRunWriter: opts.DryRunWriter,
		Logger:       opts.Logger,
		Verbose:      opts.Verbose,
	})
	return &http.Client{
		Transport: transport,
		Timeout:   opts.Timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// NewClient 依 settings 建立所有服務的 client。
// 每個服務有自己的 http.Client，因為 x-api-host 的預設值不同。
func NewClient(settings *config.Settings, opts ClientOptions) (*Client, error) {
	if err := validateGateway(settings); err != nil {
		return nil, err
	}

	vcsClient, err := vcs.NewClientWithResponses(
		BaseURL(settings.APIGateway, settings.APIHosts.VCS),
		vcs.WithHTTPClient(newServiceHTTPClient(settings, settings.APIHosts.VCS, opts)))
	if err != nil {
		return nil, fmt.Errorf("建立 VCS client 失敗: %w", err)
	}
	cephClient, err := ceph.NewClientWithResponses(
		BaseURL(settings.APIGateway, settings.APIHosts.COS),
		ceph.WithHTTPClient(newServiceHTTPClient(settings, settings.APIHosts.COS, opts)))
	if err != nil {
		return nil, fmt.Errorf("建立 Ceph client 失敗: %w", err)
	}
	// Common API 的 URL 沒有 ApiHost 段，但 header 仍要帶（spec 預設 goc）
	commonClient, err := common.NewClientWithResponses(
		BaseURL(settings.APIGateway, ""),
		common.WithHTTPClient(newServiceHTTPClient(settings, settings.APIHosts.Common, opts)))
	if err != nil {
		return nil, fmt.Errorf("建立 Common client 失敗: %w", err)
	}

	return &Client{
		Settings: settings,
		VCS:      vcsClient,
		Ceph:     cephClient,
		Common:   commonClient,
	}, nil
}
