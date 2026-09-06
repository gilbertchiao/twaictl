package cli

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gilbertchiao/twaictl/internal/config"
)

// TestExitCodeMatrix 是 docs/DESIGN.md 第 3.5 節 exit code 定義的整合驗證：table-driven，
// 每一列對應一個 exit code（0～5），透過 runCLI 實際執行一次命令、確認 process exit code
// 與該節定義相符。個別行為（訊息內容、細節分支）已有專屬測試涵蓋，這裡只驗證「哪一類
// 情境對應哪一個 exit code」這個總覽層級的事實，避免日後改動不小心打亂分類。
func TestExitCodeMatrix(t *testing.T) {
	cases := []struct {
		name     string
		wantCode int
		setup    func(t *testing.T) (env map[string]string, args []string)
	}{
		{
			// DESIGN 3.5 code 0：成功。--dry-run 一律視為成功（transport 攔截請求、
			// 合成 204，不論底下命令原本是讀取還是寫入）。
			name:     "code 0 成功：--dry-run vcs ls",
			wantCode: ExitOK,
			setup: func(t *testing.T) (map[string]string, []string) {
				f := newFakeAPI(t)
				env := f.env(t)
				// project code 用數字，projectID() 不必查 API 就能解析；--dry-run 會在
				// 真正送出 GET /sites/ 之前被 transport 攔截，因此完全不需要註冊任何回應。
				env[config.EnvProjectCode] = "101"
				return env, []string{"--dry-run", "vcs", "ls"}
			},
		},
		{
			// DESIGN 3.5 code 1：一般錯誤（使用者輸入、找不到資源）。
			name:     "code 1 一般錯誤：vcs get 找不到 name",
			wantCode: ExitGeneral,
			setup: func(t *testing.T) (map[string]string, []string) {
				f := newFakeAPI(t)
				f.onFixture("GET", vcsBase+"/sites/", 200, "vcs/sites")
				return f.env(t), []string{"vcs", "get", "nosuch"}
			},
		},
		{
			// DESIGN 3.5 code 2：設定錯誤（無 profile、缺 API key）。
			name:     "code 2 設定錯誤：vcs ls 缺 API key",
			wantCode: ExitConfig,
			setup: func(t *testing.T) (map[string]string, []string) {
				env := map[string]string{"XDG_CONFIG_HOME": t.TempDir()}
				return env, []string{"vcs", "ls"}
			},
		},
		{
			// DESIGN 3.5 code 3：API 錯誤（HTTP 4xx / 5xx）。
			name:     "code 3 API 錯誤：vcs ls 遇到 500",
			wantCode: ExitAPI,
			setup: func(t *testing.T) (map[string]string, []string) {
				f := newFakeAPI(t)
				f.on("GET", vcsBase+"/sites/", 500, `{"message":"internal error"}`)
				return f.env(t), []string{"vcs", "ls"}
			},
		},
		{
			// DESIGN 3.5 code 4：網路 / 逾時。apigateway 指向一個已經關閉的 port，
			// 連線在 TCP 層就被拒絕，屬於網路錯誤而非 API 錯誤。
			name:     "code 4 網路錯誤：apigateway 指向已關閉的 port",
			wantCode: ExitNetwork,
			setup: func(t *testing.T) (map[string]string, []string) {
				srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
				endpoint := srv.URL
				srv.Close()
				env := map[string]string{
					"XDG_CONFIG_HOME":     t.TempDir(),
					config.EnvAPIKey:      "test-key",
					config.EnvAPIGateway:  endpoint,
					config.EnvProjectCode: "101",
				}
				return env, []string{"vcs", "ls"}
			},
		},
		{
			// DESIGN 3.5 code 5：--wait 逾時。site 一直維持 Ready（從未變成 Deleted 或
			// 404），--wait-timeout 遠短於預設值，讓測試快速觸發逾時；--wait-interval
			// （隱藏旗標）調快輪詢間隔，避免測試因為預設 5s 輪詢間隔而變慢。
			name:     "code 5 --wait 逾時：vcs rm --wait 但 site 永遠 Ready",
			wantCode: ExitWaitTimeout,
			setup: func(t *testing.T) (map[string]string, []string) {
				f := newFakeAPI(t)
				f.on("DELETE", vcsBase+"/sites/7/", 204, "")
				f.onFixture("GET", vcsBase+"/sites/7/", 200, "vcs/site_ready")
				return f.env(t), []string{
					"vcs", "rm", "7", "-y", "--wait",
					"--wait-timeout", "30ms", "--wait-interval", "1ms",
				}
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env, args := c.setup(t)
			code, _, stderr := runCLI(t, env, "", args...)
			require.Equal(t, c.wantCode, code, "stderr=%s", stderr)
		})
	}
}
