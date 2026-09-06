package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilbertchiao/twaictl/internal/config"
	"github.com/gilbertchiao/twaictl/internal/twai"
)

func TestExitCodeFor(t *testing.T) {
	u, _ := url.Parse("https://gw/x")
	apiErr := twai.NewAPIError(&http.Response{StatusCode: 404, Request: &http.Request{Method: "GET", URL: u}}, nil)

	cases := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, ExitOK},
		{"一般錯誤", errors.New("x"), ExitGeneral},
		{"設定錯誤", &config.Error{Msg: "no key"}, ExitConfig},
		{"包裝後的設定錯誤", fmt.Errorf("wrap: %w", &config.Error{Msg: "no key"}), ExitConfig},
		{"API 錯誤", apiErr, ExitAPI},
		{"網路錯誤", &url.Error{Op: "Get", URL: "x", Err: errors.New("refused")}, ExitNetwork},
		{"逾時", context.DeadlineExceeded, ExitNetwork},
		{"wait 逾時", fmt.Errorf("wrap: %w", twai.ErrWaitTimeout), ExitWaitTimeout},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, exitCodeFor(c.err), c.name)
	}
}

func TestExitCodeForDryRunIsZero(t *testing.T) {
	assert.Equal(t, ExitOK, exitCodeFor(twai.ErrDryRun))
	assert.Equal(t, ExitOK, exitCodeFor(fmt.Errorf("wrap: %w", twai.ErrDryRun)))
}

func TestUnknownCommandIsGeneralErrorOnStderr(t *testing.T) {
	code, stdout, stderr := runCLI(t, nil, "", "no-such-command")
	assert.Equal(t, ExitGeneral, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "twaictl: ")
}

func TestLegacyEnvWarningPrintedOnce(t *testing.T) {
	env := map[string]string{
		"XDG_CONFIG_HOME":      t.TempDir(),
		config.LegacyEnvAPIKey: "legacy",
	}
	_, _, stderr := runCLI(t, env, "", "config", "show")
	assert.Equal(t, 1, countOccurrences(stderr, "_TWCC_API_KEY_"))
	assert.Contains(t, stderr, "twaictl: 警告:")
	assert.NotContains(t, stderr, "legacy")
}

func countOccurrences(s, sub string) int {
	count := 0
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			count++
		}
	}
	return count
}

// TestWaitFlagsMustBePositive 驗證 --wait-interval / --wait-timeout 非正值會在
// PersistentPreRunE 就被擋下（exit 1），而不是留到輪詢時才 panic 或立即逾時。
// 用 version 命令當測試載體：它不需要設定檔，是最便宜的觸發方式。
func TestWaitFlagsMustBePositive(t *testing.T) {
	code, _, stderr := runCLI(t, nil, "", "version", "--wait-interval", "0")
	assert.Equal(t, ExitGeneral, code)
	assert.Contains(t, stderr, "--wait-interval")

	code, _, stderr = runCLI(t, nil, "", "version", "--wait-timeout", "-1s")
	assert.Equal(t, ExitGeneral, code)
	assert.Contains(t, stderr, "--wait-timeout")
}

func TestCompletionCommandExists(t *testing.T) {
	code, stdout, _ := runCLI(t, nil, "", "completion", "bash")
	assert.Equal(t, ExitOK, code)
	assert.Contains(t, stdout, "twaictl")
}

// TestCommandsWithoutSettingsSurviveBrokenConfig 驗證診斷 / shell 整合命令
// （version、completion、help、shell 補全背後的 __complete）在設定檔損毀時仍可用；
// 對照 config show 仍應因設定檔損毀而回傳 ExitConfig。
func TestCommandsWithoutSettingsSurviveBrokenConfig(t *testing.T) {
	xdg := t.TempDir()
	configDir := filepath.Join(xdg, "twaictl")
	require.NoError(t, os.MkdirAll(configDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte("profiles: [broken"), 0o600))
	env := map[string]string{"XDG_CONFIG_HOME": xdg}

	cases := [][]string{
		{"version"},
		{"completion", "bash"},
		{"help"},
		{"__complete", "con"},
	}
	for _, args := range cases {
		code, _, stderr := runCLI(t, env, "", args...)
		assert.Equal(t, ExitOK, code, "args=%v stderr=%s", args, stderr)
		assert.NotContains(t, stderr, "格式錯誤", "args=%v", args)
	}

	code, _, stderr := runCLI(t, env, "", "config", "show")
	assert.Equal(t, ExitConfig, code, stderr)
}

// 以下兩個測試補上 (*app).apiClient 的覆蓋率——brief 的 CLI 整合測試都還沒有
// Phase 1 的 API 命令會呼叫它，先在 cli 套件內直接測試這個共用方法本身。

func TestAppAPIClientRequiresAPIKey(t *testing.T) {
	a := &app{
		opts:     &globalOptions{},
		settings: &config.Settings{ProfileName: "default", ConfigPath: "/tmp/config.yaml"},
	}
	_, err := a.apiClient()
	var cfgErr *config.Error
	require.ErrorAs(t, err, &cfgErr, "沒有 API key 時應回傳 *config.Error，對應 exit code 2")
}

func TestAppAPIClientBuildsClientWithAPIKey(t *testing.T) {
	a := &app{
		opts:   &globalOptions{},
		stdout: io.Discard,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		settings: &config.Settings{
			ProfileName: "default",
			ConfigPath:  "/tmp/config.yaml",
			APIKey:      "k",
			APIGateway:  config.DefaultAPIGateway,
			APIHosts:    config.DefaultAPIHosts,
		},
	}
	client, err := a.apiClient()
	require.NoError(t, err)
	assert.NotNil(t, client)
}
