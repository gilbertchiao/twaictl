// Package config 負責 twaictl 設定檔的讀寫，以及
// 「命令列 flag → 環境變數 → 設定檔 profile → 內建預設值」的優先序解析。
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"gopkg.in/yaml.v3"
)

// backtickQuoted 對應 yaml.v3 錯誤訊息中以反引號包住的片段（例如
// cannot unmarshal !!str `my-supe...` into config.COSSettings）。
// yaml.v3 會把型別不符的值原文（截斷後）放進反引號，若使用者把 secret
// 貼到錯誤的欄位層級，這段文字就可能含有 secret 的一部分，因此一律遮蔽。
var backtickQuoted = regexp.MustCompile("`[^`]*`")

// sanitizeYAMLError 保留 yaml.Unmarshal 錯誤中的行號等結構資訊，
// 但把反引號包住的原文值片段換成 MaskedValue，避免洩漏設定內容。
func sanitizeYAMLError(err error) string {
	return backtickQuoted.ReplaceAllString(err.Error(), MaskedValue)
}

// Error 代表設定層級的錯誤（無 profile、缺 API key、設定檔格式錯誤），
// cli 層以 errors.As 判斷後對應 exit code 2。
type Error struct {
	Msg string
}

func (e *Error) Error() string { return e.Msg }

// Config 對應設定檔的整體結構。
type Config struct {
	CurrentProfile string             `yaml:"current_profile,omitempty"`
	Profiles       map[string]Profile `yaml:"profiles,omitempty"`
}

// Profile 是單一 profile 的內容；所有欄位皆選填，空值代表「使用下一層來源」。
type Profile struct {
	APIKey      string      `yaml:"api_key,omitempty"`
	ProjectCode string      `yaml:"project_code,omitempty"`
	APIGateway  string      `yaml:"apigateway,omitempty"`
	APIHosts    APIHosts    `yaml:"api_hosts,omitempty"`
	COS         COSSettings `yaml:"cos,omitempty"`
}

// APIHosts 是各服務的 x-api-host header 值（平台識別字串）。
type APIHosts struct {
	VCS    string `yaml:"vcs,omitempty" json:"vcs"`
	COS    string `yaml:"cos,omitempty" json:"cos"`
	Common string `yaml:"common,omitempty" json:"common"`
}

// COSSettings 是 COS（S3 相容）資料面的連線設定。
type COSSettings struct {
	Endpoint  string `yaml:"endpoint,omitempty" json:"endpoint"`
	AccessKey string `yaml:"access_key,omitempty" json:"access_key"`
	SecretKey string `yaml:"secret_key,omitempty" json:"secret_key"`
}

// DefaultPath 回傳設定檔預設路徑：$XDG_CONFIG_HOME/twaictl/config.yaml，
// 未設定時為 ~/.config/twaictl/config.yaml（macOS / Windows 也沿用此規則以求一致）。
func DefaultPath(getenv func(string) string) (string, error) {
	configHome := getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home := getenv("HOME")
		if home == "" {
			home = getenv("USERPROFILE") // Windows
		}
		if home == "" {
			var err error
			home, err = os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("無法判斷家目錄: %w", err)
			}
		}
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, "twaictl", "config.yaml"), nil
}

// Load 讀取設定檔；檔案不存在時回傳空的 Config（尚未 init 是正常狀態）。
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("讀取設定檔 %s 失敗: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, &Error{Msg: fmt.Sprintf("設定檔 %s 格式錯誤: %s", path, sanitizeYAMLError(err))}
	}
	return &cfg, nil
}

// Save 以 0600 權限寫入設定檔（目錄 0700）。先寫到同目錄的暫存檔再 rename，避免寫到一半損毀。
func Save(path string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("序列化設定失敗: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("建立設定目錄 %s 失敗: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".config-*.yaml.tmp")
	if err != nil {
		return fmt.Errorf("建立暫存檔失敗: %w", err)
	}
	tmpPath := tmp.Name()
	// 任何失敗都要清掉暫存檔
	cleanup := func() { _ = os.Remove(tmpPath) }

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("設定暫存檔權限失敗: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("寫入暫存檔失敗: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("關閉暫存檔失敗: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return fmt.Errorf("寫入設定檔 %s 失敗: %w", path, err)
	}
	return nil
}
