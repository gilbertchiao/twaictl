package twai

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilbertchiao/twaictl/internal/config"
	"github.com/gilbertchiao/twaictl/internal/gen/common"
	"github.com/gilbertchiao/twaictl/internal/gen/vcs"
)

func TestBaseURL(t *testing.T) {
	assert.Equal(t, "https://gw/api/v3/openstack-taichung-default-2/", BaseURL("https://gw", "openstack-taichung-default-2"))
	assert.Equal(t, "https://gw/api/v3/openstack-taichung-default-2/", BaseURL("https://gw/", "openstack-taichung-default-2"), "gateway 尾端斜線要正規化")
	assert.Equal(t, "https://gw/api/v3/", BaseURL("https://gw", ""), "Common API 沒有 ApiHost 段")
}

func TestNewClientRoutesToServicePathsWithHeaders(t *testing.T) {
	type seen struct {
		path, apiKey, apiHost, userAgent string
	}
	var requests []seen
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, seen{r.URL.Path, r.Header.Get(HeaderAPIKey), r.Header.Get(HeaderAPIHost), r.Header.Get("User-Agent")})
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v3/version/":
			_, _ = w.Write([]byte(`{"version":"1.2.1"}`))
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	defer srv.Close()

	settings := &config.Settings{
		APIKey: "secret", APIGateway: srv.URL,
		APIHosts: config.APIHosts{VCS: "my-vcs", Common: "goc"},
	}
	client, err := NewClient(settings, ClientOptions{
		Version: "v9.9.9", Timeout: 5 * time.Second,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	require.NoError(t, err)

	ctx := context.Background()
	projects, err := client.VCS.GetProjectsWithResponse(ctx, &vcs.GetProjectsParams{XApiHost: settings.APIHosts.VCS})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, projects.StatusCode())

	ver, err := client.Common.GetVersionWithResponse(ctx, &common.GetVersionParams{XApiHost: settings.APIHosts.Common})
	require.NoError(t, err)
	require.NotNil(t, ver.JSON200)
	assert.Equal(t, "1.2.1", ver.JSON200.Version)

	require.Len(t, requests, 2)
	assert.Equal(t, seen{"/api/v3/my-vcs/projects/", "secret", "my-vcs", "twaictl/v9.9.9 (" + runtimeOSArch() + ")"}, requests[0])
	assert.Equal(t, "/api/v3/version/", requests[1].path)
	assert.Equal(t, "goc", requests[1].apiHost)
}

func TestNewClientRejectsInvalidGateway(t *testing.T) {
	cases := []struct {
		name    string
		gateway string
	}{
		{"不合法的 URL", "://bad"},
		{"缺少 scheme", "apigateway.twcc.ai"},
		{"空字串", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := NewClient(&config.Settings{APIKey: "k", APIGateway: c.gateway}, ClientOptions{})
			var cfgErr *config.Error
			require.ErrorAs(t, err, &cfgErr)
			assert.Contains(t, cfgErr.Msg, "apigateway 必須是 http(s):// 開頭的完整 URL")
			assert.NotContains(t, cfgErr.Msg, "k")
		})
	}
}

func runtimeOSArch() string {
	ua := UserAgent("x")
	// "twaictl/x (linux/amd64)" → "linux/amd64"
	return ua[len("twaictl/x (") : len(ua)-1]
}

// TestNewClientDoesNotFollowRedirect 驗證型別化 client 對 3xx 不跟隨（A26）：
// 伺服器只應收到一次請求，且 CheckResponse 回傳含 Location（去 query）的 *APIError。
func TestNewClientDoesNotFollowRedirect(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Location", "https://192.0.2.10/login?next=/api/v3/x/projects/")
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()
	settings := &config.Settings{APIGateway: srv.URL, APIKey: "k", APIHosts: config.APIHosts{VCS: "openstack-taichung-default-2", COS: "ceph-taichung-default", Common: "goc"}}
	client, err := NewClient(settings, ClientOptions{Version: "test", Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	require.NoError(t, err)

	resp, err := client.VCS.GetProjectsWithResponse(context.Background(), &vcs.GetProjectsParams{XApiHost: settings.APIHosts.VCS})
	require.NoError(t, err)
	assert.Equal(t, http.StatusFound, resp.StatusCode())
	assert.Equal(t, int32(1), hits.Load(), "不得跟隨重導向再打第二次")

	err = CheckResponse(resp.HTTPResponse, resp.Body)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusFound, apiErr.StatusCode)
	assert.Contains(t, apiErr.Message, "https://192.0.2.10/login")
	assert.NotContains(t, apiErr.Message, "next=")
}
