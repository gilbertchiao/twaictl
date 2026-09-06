package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func envMap(m map[string]string) func(string) string {
	return func(key string) string { return m[key] }
}

func TestDefaultPathUsesXDGConfigHome(t *testing.T) {
	path, err := DefaultPath(envMap(map[string]string{"XDG_CONFIG_HOME": "/xdg"}))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/xdg", "twaictl", "config.yaml"), path)
}

func TestDefaultPathFallsBackToHomeDotConfig(t *testing.T) {
	path, err := DefaultPath(envMap(map[string]string{"HOME": "/home/me"}))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/home/me", ".config", "twaictl", "config.yaml"), path)
}

func TestLoadMissingFileReturnsEmptyConfig(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "", cfg.CurrentProfile)
	assert.Empty(t, cfg.Profiles)
}

func TestLoadParsesYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := `current_profile: work
profiles:
  work:
    api_key: secret-key
    project_code: GOV123
    apigateway: https://gw.example
    api_hosts:
      vcs: my-vcs
    cos:
      endpoint: https://cos.example
      access_key: ak
      secret_key: sk
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "work", cfg.CurrentProfile)
	p := cfg.Profiles["work"]
	assert.Equal(t, "secret-key", p.APIKey)
	assert.Equal(t, "GOV123", p.ProjectCode)
	assert.Equal(t, "https://gw.example", p.APIGateway)
	assert.Equal(t, "my-vcs", p.APIHosts.VCS)
	assert.Equal(t, "", p.APIHosts.COS)
	assert.Equal(t, "sk", p.COS.SecretKey)
}

func TestLoadInvalidYAMLReturnsConfigError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("profiles: [not a map"), 0o600))

	_, err := Load(path)
	require.Error(t, err)
	var cfgErr *Error
	assert.ErrorAs(t, err, &cfgErr)
}

func TestLoadYAMLErrorDoesNotLeakValues(t *testing.T) {
	// cos 應為 map，卻給了字串；yaml.v3 的錯誤訊息會把值的前幾個字元原文放進反引號，
	// 若使用者誤貼 secret 到錯誤層級，不能讓它出現在錯誤訊息裡。
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := "profiles:\n  default:\n    cos: my-super-secret-token-value\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	_, err := Load(path)
	require.Error(t, err)
	var cfgErr *Error
	require.ErrorAs(t, err, &cfgErr)
	assert.NotContains(t, err.Error(), "my-supe")
	assert.Contains(t, err.Error(), "***")
}

func TestSaveWritesWithRestrictivePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.yaml")
	cfg := &Config{
		CurrentProfile: "default",
		Profiles:       map[string]Profile{"default": {APIKey: "k", ProjectCode: "GOV1"}},
	}

	require.NoError(t, Save(path, cfg))

	loaded, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, cfg, loaded)

	if runtime.GOOS != "windows" {
		st, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), st.Mode().Perm())
		dirSt, err := os.Stat(filepath.Dir(path))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o700), dirSt.Mode().Perm())
	}
}

func TestSaveDoesNotLeaveTempFileOnSuccess(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Save(filepath.Join(dir, "config.yaml"), &Config{}))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "config.yaml", entries[0].Name())
}
