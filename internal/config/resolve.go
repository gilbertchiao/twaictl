package config

import "fmt"

// 內建預設值（設計文件 2.1 節；Common 的 x-api-host 預設值 goc 取自 Common.yaml）。
const (
	DefaultAPIGateway  = "https://apigateway.twcc.ai"
	DefaultCOSEndpoint = "https://cos.twcc.ai"
	DefaultProfileName = "default"
	MaskedValue        = "***"
)

// DefaultAPIHosts 是各服務預設的平台識別字串。
var DefaultAPIHosts = APIHosts{
	VCS:    "openstack-taichung-default-2",
	COS:    "ceph-taichung-default",
	Common: "goc",
}

// 環境變數名稱。
const (
	EnvAPIKey        = "TWAI_API_KEY"
	EnvProjectCode   = "TWAI_PROJECT_CODE"
	EnvProfile       = "TWAI_PROFILE"
	EnvAPIGateway    = "TWAI_APIGATEWAY"
	EnvAPIHostVCS    = "TWAI_API_HOST_VCS"
	EnvAPIHostCOS    = "TWAI_API_HOST_COS"
	EnvAPIHostCommon = "TWAI_API_HOST_COMMON"
	EnvCOSEndpoint   = "TWAI_COS_ENDPOINT"
	EnvCOSAccessKey  = "TWAI_COS_ACCESS_KEY"
	EnvCOSSecretKey  = "TWAI_COS_SECRET_KEY"

	// 舊版 twccli 的環境變數，為方便遷移而接受，但會提示改用 TWAI_*。
	LegacyEnvAPIKey      = "_TWCC_API_KEY_"
	LegacyEnvProjectCode = "_TWCC_PROJECT_CODE_"
)

// Overrides 是來自命令列 flag 的覆寫值（優先序最高）。
type Overrides struct {
	Profile     string
	ProjectCode string
	// AllowMissingProfile 為 true 時，明確指定但不存在的 profile 不視為錯誤，
	// 改用零值 Profile 繼續解析；供 `config init` 建立新 profile 使用。
	AllowMissingProfile bool
}

// Settings 是解析完優先序後、實際生效的設定。
type Settings struct {
	ProfileName string      `yaml:"profile" json:"profile"`
	ConfigPath  string      `yaml:"config_path" json:"config_path"`
	APIKey      string      `yaml:"api_key" json:"api_key"`
	ProjectCode string      `yaml:"project_code" json:"project_code"`
	APIGateway  string      `yaml:"apigateway" json:"apigateway"`
	APIHosts    APIHosts    `yaml:"api_hosts" json:"api_hosts"`
	COS         COSSettings `yaml:"cos" json:"cos"`
	// Warnings 是解析過程產生、應顯示給使用者一次的提示（例如使用了舊版環境變數）。
	Warnings []string `yaml:"-" json:"-"`
}

// firstNonEmpty 回傳第一個非空字串，用來實作優先序。
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// Resolve 依「flag → 環境變數 → 設定檔 profile → 預設值」的順序合併出生效設定。
func Resolve(cfg *Config, configPath string, getenv func(string) string, ov Overrides) (*Settings, error) {
	profileName := firstNonEmpty(ov.Profile, getenv(EnvProfile), cfg.CurrentProfile, DefaultProfileName)

	profile, found := cfg.Profiles[profileName]
	// 「明確指定」涵蓋 flag、環境變數，以及設定檔的 current_profile；
	// 只有都沒指定、退回硬編碼的 DefaultProfileName 時，該 profile 不存在才不算錯誤
	// （因為 config init 本身就是在建立它）。
	explicitlyRequested := ov.Profile != "" || getenv(EnvProfile) != "" || cfg.CurrentProfile != ""
	if !found && explicitlyRequested && !ov.AllowMissingProfile {
		return nil, &Error{Msg: fmt.Sprintf("設定檔 %s 中找不到 profile %q", configPath, profileName)}
	}

	settings := &Settings{
		ProfileName: profileName,
		ConfigPath:  configPath,
		APIGateway:  firstNonEmpty(getenv(EnvAPIGateway), profile.APIGateway, DefaultAPIGateway),
		APIHosts: APIHosts{
			VCS:    firstNonEmpty(getenv(EnvAPIHostVCS), profile.APIHosts.VCS, DefaultAPIHosts.VCS),
			COS:    firstNonEmpty(getenv(EnvAPIHostCOS), profile.APIHosts.COS, DefaultAPIHosts.COS),
			Common: firstNonEmpty(getenv(EnvAPIHostCommon), profile.APIHosts.Common, DefaultAPIHosts.Common),
		},
		COS: COSSettings{
			Endpoint:  firstNonEmpty(getenv(EnvCOSEndpoint), profile.COS.Endpoint, DefaultCOSEndpoint),
			AccessKey: firstNonEmpty(getenv(EnvCOSAccessKey), profile.COS.AccessKey),
			SecretKey: firstNonEmpty(getenv(EnvCOSSecretKey), profile.COS.SecretKey),
		},
	}

	settings.APIKey = resolveWithLegacyEnv(settings, getenv, EnvAPIKey, LegacyEnvAPIKey, profile.APIKey)
	settings.ProjectCode = firstNonEmpty(ov.ProjectCode,
		resolveWithLegacyEnv(settings, getenv, EnvProjectCode, LegacyEnvProjectCode, profile.ProjectCode))

	return settings, nil
}

// EnvValueWithLegacy 回傳新版或舊版環境變數的值與「實際使用的變數名」；都未設定時回傳空字串。
// 供 `config init` 在 flag 未指定時 fallback 到環境變數（不經設定檔既有值）使用。
func EnvValueWithLegacy(getenv func(string) string, newEnv, legacyEnv string) (value, source string) {
	if v := getenv(newEnv); v != "" {
		return v, newEnv
	}
	if v := getenv(legacyEnv); v != "" {
		return v, legacyEnv
	}
	return "", ""
}

// resolveWithLegacyEnv 先看新環境變數，再看舊版 twccli 環境變數（並記錄警告），最後用設定檔值。
// 警告訊息只含變數名稱，絕不含值。
func resolveWithLegacyEnv(settings *Settings, getenv func(string) string, newEnv, legacyEnv, fileValue string) string {
	if v := getenv(newEnv); v != "" {
		return v
	}
	if v := getenv(legacyEnv); v != "" {
		settings.Warnings = append(settings.Warnings,
			fmt.Sprintf("偵測到舊版環境變數 %s，建議改用 %s", legacyEnv, newEnv))
		return v
	}
	return fileValue
}

// RequireAPIKey 在需要呼叫 API 的命令前確認 key 存在。
func (s *Settings) RequireAPIKey() error {
	if s.APIKey == "" {
		return &Error{Msg: fmt.Sprintf(
			"尚未設定 API key：請執行 `twaictl config init`，或設定環境變數 %s（profile: %s，設定檔: %s）",
			EnvAPIKey, s.ProfileName, s.ConfigPath)}
	}
	return nil
}

// Masked 回傳遮蔽 secret 後的副本，供 `config show` 與日誌使用。
func (s *Settings) Masked() *Settings {
	copied := *s
	if copied.APIKey != "" {
		copied.APIKey = MaskedValue
	}
	if copied.COS.SecretKey != "" {
		copied.COS.SecretKey = MaskedValue
	}
	return &copied
}
