package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"

	"github.com/gilbertchiao/twaictl/internal/config"
)

func newConfigCmd(a *app) *cobra.Command {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "管理設定檔與 profile",
	}
	configCmd.AddCommand(newConfigInitCmd(a))
	configCmd.AddCommand(newConfigShowCmd(a))
	configCmd.AddCommand(newConfigWhoamiCmd(a))
	return configCmd
}

// configInitFlags 是 config init 可用 flag 指定的欄位；未指定且必填者會互動詢問。
type configInitFlags struct {
	apiKey        string
	projectCode   string
	apigateway    string
	apiHostVCS    string
	apiHostCOS    string
	apiHostCommon string
	cosEndpoint   string
	cosAccessKey  string
	cosSecretKey  string
}

func newConfigInitCmd(a *app) *cobra.Command {
	var f configInitFlags
	cmd := &cobra.Command{
		Use:   "init",
		Short: "建立或更新 profile（API key、project code、選填的 apigateway / ApiHost）",
		Long: "互動式建立 profile。以 --profile 指定名稱；未指定時為目前生效的 profile\n" +
			"（TWAI_PROFILE 或設定檔 current_profile，皆未設定時為 default）。\n" +
			"API key 可在 TWCC 網站「使用者資訊 → API 金鑰」取得；學研用戶（iService）與企業用戶的 key 用法相同。\n" +
			"API key、project code 未以 flag 指定時，依序改用環境變數（TWAI_API_KEY / TWAI_PROJECT_CODE，\n" +
			"或舊版 _TWCC_API_KEY_ / _TWCC_PROJECT_CODE_）、再互動詢問。\n" +
			"提示：互動輸入 API key 時不會回顯（非終端機環境例如 pipe 則以一般方式讀取一行）；\n" +
			"若在意，可改用 --api-key flag 或 TWAI_API_KEY 環境變數。",
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return a.runConfigInit(f)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&f.apiKey, "api-key", "", "API key（未指定則互動詢問）")
	flags.StringVar(&f.projectCode, "project-code", "", "預設 project code（未指定則互動詢問）")
	flags.StringVar(&f.apigateway, "apigateway", "", "覆寫 apigateway URL（預設 "+config.DefaultAPIGateway+"）")
	flags.StringVar(&f.apiHostVCS, "api-host-vcs", "", "覆寫 VCS 的 x-api-host（預設 "+config.DefaultAPIHosts.VCS+"）")
	flags.StringVar(&f.apiHostCOS, "api-host-cos", "", "覆寫 COS 的 x-api-host（預設 "+config.DefaultAPIHosts.COS+"）")
	flags.StringVar(&f.apiHostCommon, "api-host-common", "", "覆寫 Common API 的 x-api-host（預設 "+config.DefaultAPIHosts.Common+"）")
	flags.StringVar(&f.cosEndpoint, "cos-endpoint", "", "覆寫 COS S3 endpoint（預設 "+config.DefaultCOSEndpoint+"）")
	flags.StringVar(&f.cosAccessKey, "cos-access-key", "", "COS S3 access key（選填）")
	flags.StringVar(&f.cosSecretKey, "cos-secret-key", "", "COS S3 secret key（選填）")
	return cmd
}

// runConfigInit 合併 flag 與互動輸入後寫入設定檔。
func (a *app) runConfigInit(f configInitFlags) error {
	reader := bufio.NewReader(a.stdin)
	profileName := a.settings.ProfileName

	apiKey := f.apiKey
	if apiKey == "" {
		var source string
		apiKey, source = config.EnvValueWithLegacy(a.getenv, config.EnvAPIKey, config.LegacyEnvAPIKey)
		if apiKey != "" {
			_, _ = fmt.Fprintf(a.stderr, "twaictl: 未指定 --api-key，改用環境變數 %s 的值寫入設定檔\n", source)
		}
	}
	if apiKey == "" {
		var err error
		apiKey, err = a.promptSecret(reader, "API key: ")
		if err != nil {
			return err
		}
	}
	if apiKey == "" {
		return &config.Error{Msg: "API key 不可為空"}
	}

	projectCode := f.projectCode
	if projectCode == "" {
		var source string
		projectCode, source = config.EnvValueWithLegacy(a.getenv, config.EnvProjectCode, config.LegacyEnvProjectCode)
		if projectCode != "" {
			_, _ = fmt.Fprintf(a.stderr, "twaictl: 未指定 --project-code，改用環境變數 %s 的值寫入設定檔\n", source)
		}
	}
	if projectCode == "" {
		var err error
		projectCode, err = a.prompt(reader, "Project code（例如 GOV123456，可留空稍後以 --project 指定）: ")
		if err != nil {
			return err
		}
	}

	profile := a.config.Profiles[profileName] // 不存在時為零值，保留既有欄位再覆寫
	profile.APIKey = apiKey
	profile.ProjectCode = projectCode
	overwriteIfSet(&profile.APIGateway, f.apigateway)
	overwriteIfSet(&profile.APIHosts.VCS, f.apiHostVCS)
	overwriteIfSet(&profile.APIHosts.COS, f.apiHostCOS)
	overwriteIfSet(&profile.APIHosts.Common, f.apiHostCommon)
	overwriteIfSet(&profile.COS.Endpoint, f.cosEndpoint)
	overwriteIfSet(&profile.COS.AccessKey, f.cosAccessKey)
	overwriteIfSet(&profile.COS.SecretKey, f.cosSecretKey)

	if a.config.Profiles == nil {
		a.config.Profiles = make(map[string]config.Profile)
	}
	a.config.Profiles[profileName] = profile
	a.config.CurrentProfile = profileName

	if err := config.Save(a.configPath, a.config); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(a.stderr, "twaictl: 已寫入 profile %q 到 %s（權限 0600）\n", profileName, a.configPath)
	_, _ = fmt.Fprintf(a.stderr, "twaictl: 建議接著執行 `twaictl config whoami` 驗證 API key\n")
	return nil
}

// prompt 在 stderr 顯示提示並從 stdin 讀一行（去除前後空白）。EOF 視為輸入結束：
// 若已讀到內容則視為有效輸入，沒有內容則視為空輸入。任何非 EOF 的讀取錯誤一律視為
// 失敗並回傳 error——即使 err 同時帶有已讀到的部分資料（io.Reader 的合法行為，
// bufio.Reader 內部也確實會發生：fill() 先把資料寫入緩衝區、才記錄 error），
// 也不可把這段殘缺資料當成使用者輸入採用，避免把截斷的 API key／project code 寫入設定檔。
func (a *app) prompt(reader *bufio.Reader, label string) (string, error) {
	_, _ = fmt.Fprint(a.stderr, label)
	line, err := reader.ReadString('\n')
	if err != nil {
		if !errors.Is(err, io.EOF) {
			return "", fmt.Errorf("讀取輸入失敗: %w", err)
		}
		if line == "" {
			// EOF 且沒有內容：回傳空字串讓 caller 決定是否為錯誤
			_, _ = fmt.Fprintln(a.stderr)
			return "", nil
		}
		// EOF 但已讀到內容（輸入沒有結尾換行）：視為有效輸入
	}
	return strings.TrimSpace(line), nil
}

// promptSecret 用於輸入 API key 這類敏感資料：若 stdin 是連到終端機（TTY），
// 用 term.ReadPassword 直接讀 fd，輸入時不回顯；否則（pipe、測試用的 buffer、
// 一般檔案）退回 prompt 的逐行讀取，行為與既有測試完全一致。
//
// 注意：term.ReadPassword 是直接對 fd 做系統呼叫讀取，會繞過 bufio.Reader
// 已經緩衝、尚未取用的資料。之所以安全，是因為只有在偵測到 TTY 時才會走這條路，
// 而在互動情境下 API key 一定是本次 session 第一個提示——reader 在呼叫本函式
// 之前不會被讀過、緩衝區必為空，因此不會遺失或跳過任何已緩衝的輸入。若日後要
// 在 API key 提示之前插入其他也會讀 reader 的提示，需要重新檢視這個前提。
func (a *app) promptSecret(reader *bufio.Reader, label string) (string, error) {
	if f, ok := a.stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		_, _ = fmt.Fprint(a.stderr, label)
		data, err := term.ReadPassword(int(f.Fd()))
		_, _ = fmt.Fprintln(a.stderr)
		if err != nil {
			return "", fmt.Errorf("讀取 API key 失敗: %w", err)
		}
		return strings.TrimSpace(string(data)), nil
	}
	// 非終端機環境（pipe、測試用的 strings.Reader/bytes.Buffer、一般檔案）：
	// 無法隱藏回顯，退回既有的逐行讀取。
	return a.prompt(reader, label)
}

// overwriteIfSet 只在 value 非空時覆寫 target。
func overwriteIfSet(target *string, value string) {
	if value != "" {
		*target = value
	}
}

func newConfigShowCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "顯示目前生效的設定（API key 與 secret 遮蔽）",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			masked := a.settings.Masked()
			switch a.opts.output {
			case "json":
				data, err := json.MarshalIndent(masked, "", "  ")
				if err != nil {
					return fmt.Errorf("序列化設定為 JSON 失敗: %w", err)
				}
				_, _ = fmt.Fprintln(a.stdout, string(data))
			default:
				// table 與 yaml 都以 YAML 呈現：設定是巢狀結構，表格反而不易讀
				data, err := yaml.Marshal(masked)
				if err != nil {
					return fmt.Errorf("序列化設定為 YAML 失敗: %w", err)
				}
				_, _ = fmt.Fprint(a.stdout, string(data))
			}
			return nil
		},
	}
}
