package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveDefaultsWhenNothingConfigured(t *testing.T) {
	s, err := Resolve(&Config{}, "/x/config.yaml", envMap(nil), Overrides{})
	require.NoError(t, err)

	assert.Equal(t, "default", s.ProfileName)
	assert.Equal(t, "/x/config.yaml", s.ConfigPath)
	assert.Equal(t, "", s.APIKey)
	assert.Equal(t, DefaultAPIGateway, s.APIGateway)
	assert.Equal(t, DefaultAPIHosts, s.APIHosts)
	assert.Equal(t, DefaultCOSEndpoint, s.COS.Endpoint)
	assert.Empty(t, s.Warnings)
}

func TestResolvePrecedenceFlagOverEnvOverFile(t *testing.T) {
	cfg := &Config{
		CurrentProfile: "work",
		Profiles: map[string]Profile{
			"work": {APIKey: "file-key", ProjectCode: "FILE", APIGateway: "https://file.gw",
				APIHosts: APIHosts{VCS: "file-vcs"}},
		},
	}
	env := envMap(map[string]string{
		EnvProjectCode: "ENV",
		EnvAPIHostVCS:  "env-vcs",
	})

	s, err := Resolve(cfg, "/x", env, Overrides{ProjectCode: "FLAG"})
	require.NoError(t, err)

	assert.Equal(t, "work", s.ProfileName) // 來自 current_profile
	assert.Equal(t, "file-key", s.APIKey)  // 只有檔案有
	assert.Equal(t, "FLAG", s.ProjectCode) // flag > env > file
	assert.Equal(t, "https://file.gw", s.APIGateway)
	assert.Equal(t, "env-vcs", s.APIHosts.VCS)           // env > file
	assert.Equal(t, DefaultAPIHosts.COS, s.APIHosts.COS) // 沒設定就用預設
}

func TestResolveProfileSelectionOrder(t *testing.T) {
	cfg := &Config{
		CurrentProfile: "fromfile",
		Profiles: map[string]Profile{
			"fromfile": {APIKey: "a"}, "fromenv": {APIKey: "b"}, "fromflag": {APIKey: "c"},
		},
	}
	env := envMap(map[string]string{EnvProfile: "fromenv"})

	s, err := Resolve(cfg, "/x", env, Overrides{})
	require.NoError(t, err)
	assert.Equal(t, "fromenv", s.ProfileName)

	s, err = Resolve(cfg, "/x", env, Overrides{Profile: "fromflag"})
	require.NoError(t, err)
	assert.Equal(t, "fromflag", s.ProfileName)
	assert.Equal(t, "c", s.APIKey)
}

func TestResolveUnknownExplicitProfileIsConfigError(t *testing.T) {
	cfg := &Config{Profiles: map[string]Profile{"default": {}}}
	_, err := Resolve(cfg, "/x", envMap(nil), Overrides{Profile: "nope"})
	var cfgErr *Error
	require.ErrorAs(t, err, &cfgErr)
	assert.Contains(t, err.Error(), "nope")
}

func TestResolveMissingDefaultProfileIsNotAnError(t *testing.T) {
	// 尚未 config init 時，default profile 不存在是正常的（例如要跑 config init 本身）
	s, err := Resolve(&Config{}, "/x", envMap(map[string]string{EnvAPIKey: "env-key"}), Overrides{})
	require.NoError(t, err)
	assert.Equal(t, "env-key", s.APIKey)
}

func TestResolveMissingCurrentProfileFromFileIsConfigError(t *testing.T) {
	// 設定檔的 current_profile 指向不存在的 profile，也算「明確指定」，要報錯而非靜默用零值
	cfg := &Config{CurrentProfile: "work", Profiles: map[string]Profile{"default": {}}}
	_, err := Resolve(cfg, "/x", envMap(nil), Overrides{})
	var cfgErr *Error
	require.ErrorAs(t, err, &cfgErr)
	assert.Contains(t, err.Error(), "work")
}

func TestResolveLegacyTWCCEnvVarsWithWarning(t *testing.T) {
	env := envMap(map[string]string{
		LegacyEnvAPIKey:      "legacy-key",
		LegacyEnvProjectCode: "LEGACY",
	})
	s, err := Resolve(&Config{}, "/x", env, Overrides{})
	require.NoError(t, err)

	assert.Equal(t, "legacy-key", s.APIKey)
	assert.Equal(t, "LEGACY", s.ProjectCode)
	require.Len(t, s.Warnings, 2)
	assert.Contains(t, s.Warnings[0], "_TWCC_API_KEY_")
	assert.Contains(t, s.Warnings[0], "TWAI_API_KEY")
	assert.NotContains(t, s.Warnings[0], "legacy-key", "警告訊息不可洩漏 key")
}

func TestResolveNewEnvWinsOverLegacyEnvWithoutWarning(t *testing.T) {
	env := envMap(map[string]string{EnvAPIKey: "new", LegacyEnvAPIKey: "old"})
	s, err := Resolve(&Config{}, "/x", env, Overrides{})
	require.NoError(t, err)
	assert.Equal(t, "new", s.APIKey)
	assert.Empty(t, s.Warnings)
}

func TestRequireAPIKey(t *testing.T) {
	s := &Settings{}
	err := s.RequireAPIKey()
	var cfgErr *Error
	require.ErrorAs(t, err, &cfgErr)
	assert.Contains(t, err.Error(), "twaictl config init")

	s.APIKey = "k"
	assert.NoError(t, s.RequireAPIKey())
}

func TestMaskedHidesSecretsAndDoesNotMutate(t *testing.T) {
	s := &Settings{APIKey: "secret", COS: COSSettings{AccessKey: "ak", SecretKey: "sk"}}
	m := s.Masked()

	assert.Equal(t, MaskedValue, m.APIKey)
	assert.Equal(t, MaskedValue, m.COS.SecretKey)
	assert.Equal(t, "ak", m.COS.AccessKey, "access key 不是 secret，保留")
	assert.Equal(t, "secret", s.APIKey, "原物件不可被改動")

	empty := (&Settings{}).Masked()
	assert.Equal(t, "", empty.APIKey, "空值維持空，讓使用者看得出尚未設定")
}

// TestResolveAllowMissingProfileUsesZeroValueProfile 驗證 Overrides.AllowMissingProfile：
// 明確指定但尚不存在的 profile 不應報錯，而是用零值 Profile 繼續解析
// （供 `config init` 建立新 profile 使用）；未開啟時行為維持原樣。
func TestResolveAllowMissingProfileUsesZeroValueProfile(t *testing.T) {
	cfg := &Config{CurrentProfile: "work", Profiles: map[string]Profile{"default": {APIKey: "d"}}}
	s, err := Resolve(cfg, "/x", envMap(nil), Overrides{Profile: "newone", AllowMissingProfile: true})
	require.NoError(t, err)
	assert.Equal(t, "newone", s.ProfileName)
	assert.Equal(t, "", s.APIKey)
	assert.Equal(t, DefaultAPIGateway, s.APIGateway)

	// 未開啟時行為不變
	_, err = Resolve(cfg, "/x", envMap(nil), Overrides{Profile: "newone"})
	var cfgErr *Error
	require.ErrorAs(t, err, &cfgErr)
}

func TestEnvValueWithLegacyPrefersNewEnv(t *testing.T) {
	env := envMap(map[string]string{EnvAPIKey: "new", LegacyEnvAPIKey: "old"})
	value, source := EnvValueWithLegacy(env, EnvAPIKey, LegacyEnvAPIKey)
	assert.Equal(t, "new", value)
	assert.Equal(t, EnvAPIKey, source)
}

func TestEnvValueWithLegacyFallsBackToLegacyEnv(t *testing.T) {
	env := envMap(map[string]string{LegacyEnvAPIKey: "old"})
	value, source := EnvValueWithLegacy(env, EnvAPIKey, LegacyEnvAPIKey)
	assert.Equal(t, "old", value)
	assert.Equal(t, LegacyEnvAPIKey, source)
}

func TestEnvValueWithLegacyReturnsEmptyWhenNeitherSet(t *testing.T) {
	value, source := EnvValueWithLegacy(envMap(nil), EnvAPIKey, LegacyEnvAPIKey)
	assert.Equal(t, "", value)
	assert.Equal(t, "", source)
}
