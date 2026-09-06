package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilbertchiao/twaictl/internal/config"
	"github.com/gilbertchiao/twaictl/internal/testutil"
)

func TestConfigInitNonInteractiveWritesProfile(t *testing.T) {
	xdg := t.TempDir()
	env := map[string]string{"XDG_CONFIG_HOME": xdg}

	code, stdout, stderr := runCLI(t, env, "",
		"config", "init", "--api-key", "abc123", "--project-code", "GOV999", "--api-host-vcs", "custom-vcs")

	require.Equal(t, ExitOK, code, stderr)
	assert.Empty(t, stdout, "stdout 只放資料；提示訊息走 stderr")
	assert.Contains(t, stderr, "已寫入")
	assert.NotContains(t, stderr, "abc123")

	path := filepath.Join(xdg, "twaictl", "config.yaml")
	cfg, err := config.Load(path)
	require.NoError(t, err)
	assert.Equal(t, "default", cfg.CurrentProfile)
	assert.Equal(t, "abc123", cfg.Profiles["default"].APIKey)
	assert.Equal(t, "GOV999", cfg.Profiles["default"].ProjectCode)
	assert.Equal(t, "custom-vcs", cfg.Profiles["default"].APIHosts.VCS)
	assert.Equal(t, "", cfg.Profiles["default"].APIHosts.COS, "沒指定的不要寫入預設值，保留覆寫空間")

	if runtime.GOOS != "windows" {
		st, _ := os.Stat(path)
		assert.Equal(t, os.FileMode(0o600), st.Mode().Perm())
	}
}

func TestConfigInitInteractivePromptsForMissingValues(t *testing.T) {
	env := map[string]string{"XDG_CONFIG_HOME": t.TempDir()}
	stdin := "typed-key\nGOV111\n"

	code, _, stderr := runCLI(t, env, stdin, "config", "init")

	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stderr, "API key")
	assert.Contains(t, stderr, "Project code")
	assert.NotContains(t, stderr, "typed-key")

	cfg, err := config.Load(filepath.Join(env["XDG_CONFIG_HOME"], "twaictl", "config.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "typed-key", cfg.Profiles["default"].APIKey)
	assert.Equal(t, "GOV111", cfg.Profiles["default"].ProjectCode)
}

func TestConfigInitEmptyAPIKeyIsConfigError(t *testing.T) {
	env := map[string]string{"XDG_CONFIG_HOME": t.TempDir()}
	code, _, stderr := runCLI(t, env, "\n\n", "config", "init")
	assert.Equal(t, ExitConfig, code)
	assert.Contains(t, stderr, "API key 不可為空")
}

func TestConfigInitUsesEnvAPIKeyWhenFlagMissing(t *testing.T) {
	env := map[string]string{
		"XDG_CONFIG_HOME":     t.TempDir(),
		config.EnvAPIKey:      "envkey",
		config.EnvProjectCode: "ENVPROJ",
	}
	code, _, stderr := runCLI(t, env, "", "config", "init")

	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stderr, config.EnvAPIKey)
	assert.NotContains(t, stderr, "envkey")
	assert.NotContains(t, stderr, "API key:", "flag/env 皆有值時不應再互動提示")

	cfg, err := config.Load(filepath.Join(env["XDG_CONFIG_HOME"], "twaictl", "config.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "envkey", cfg.Profiles["default"].APIKey)
	assert.Equal(t, "ENVPROJ", cfg.Profiles["default"].ProjectCode)
}

func TestConfigInitUsesLegacyEnvAPIKey(t *testing.T) {
	env := map[string]string{
		"XDG_CONFIG_HOME":           t.TempDir(),
		config.LegacyEnvAPIKey:      "legacykey",
		config.LegacyEnvProjectCode: "LEGACYPROJ",
	}
	code, _, stderr := runCLI(t, env, "", "config", "init")

	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stderr, config.LegacyEnvAPIKey)
	assert.NotContains(t, stderr, "legacykey")

	cfg, err := config.Load(filepath.Join(env["XDG_CONFIG_HOME"], "twaictl", "config.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "legacykey", cfg.Profiles["default"].APIKey)
	assert.Equal(t, "LEGACYPROJ", cfg.Profiles["default"].ProjectCode)
}

func TestConfigInitFlagBeatsEnv(t *testing.T) {
	env := map[string]string{
		"XDG_CONFIG_HOME": t.TempDir(),
		config.EnvAPIKey:  "envkey",
	}
	code, _, stderr := runCLI(t, env, "", "config", "init", "--api-key", "flagkey", "--project-code", "P1")
	require.Equal(t, ExitOK, code, stderr)

	cfg, err := config.Load(filepath.Join(env["XDG_CONFIG_HOME"], "twaictl", "config.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "flagkey", cfg.Profiles["default"].APIKey)
}

// erroringReader 的 Read 一律回傳非 EOF 的 error，用來驗證 prompt 不會把它誤判為空輸入。
type erroringReader struct{}

func (erroringReader) Read([]byte) (int, error) {
	return 0, errors.New("boom")
}

// TestConfigInitPromptReadErrorIsNotSwallowed 驗證 stdin 讀取失敗（非 EOF）時，
// prompt 會回傳 error 而非把它當成空輸入；exit code 1（一般錯誤），stderr 含「讀取輸入失敗」。
func TestConfigInitPromptReadErrorIsNotSwallowed(t *testing.T) {
	env := map[string]string{"XDG_CONFIG_HOME": t.TempDir()}
	isolated := withIsolatedConfigHome(t, env)
	getenv := func(key string) string { return isolated[key] }
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"config", "init"}, erroringReader{}, &stdout, &stderr, getenv)

	assert.Equal(t, ExitGeneral, code)
	assert.Contains(t, stderr.String(), "讀取輸入失敗")
}

// partialReadErroringReader 的 Read 在同一次呼叫中同時回傳非空資料與非 EOF 的 error——
// 這是 io.Reader 契約明確允許、bufio.Reader 內部確實會發生的情境（fill() 先寫入緩衝區
// 再記錄 error）。用來驗證 prompt 不會把「讀到一半就出錯」的殘缺資料當成有效輸入。
type partialReadErroringReader struct{}

func (partialReadErroringReader) Read(p []byte) (int, error) {
	n := copy(p, "partial-key")
	return n, errors.New("boom")
}

// TestConfigInitPromptPartialReadErrorIsNotSaved 驗證 ReadString 同時回傳非空 line 與
// 非 EOF error 時（Codex review 找到的 P2），prompt 一律視為失敗，不會把截斷的資料
// 當成使用者輸入寫入設定檔；exit code 1、stderr 含「讀取輸入失敗」且不洩漏殘缺內容、
// 設定檔完全不存在。
func TestConfigInitPromptPartialReadErrorIsNotSaved(t *testing.T) {
	xdg := t.TempDir()
	env := map[string]string{"XDG_CONFIG_HOME": xdg}
	isolated := withIsolatedConfigHome(t, env)
	getenv := func(key string) string { return isolated[key] }
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"config", "init"}, partialReadErroringReader{}, &stdout, &stderr, getenv)

	assert.Equal(t, ExitGeneral, code)
	assert.Contains(t, stderr.String(), "讀取輸入失敗")
	assert.NotContains(t, stderr.String(), "partial-key")

	_, err := os.Stat(filepath.Join(xdg, "twaictl", "config.yaml"))
	assert.True(t, os.IsNotExist(err), "讀取失敗時不應寫入設定檔")
}

func TestConfigInitToNamedProfileKeepsOthers(t *testing.T) {
	env := map[string]string{"XDG_CONFIG_HOME": t.TempDir()}
	code, _, _ := runCLI(t, env, "", "config", "init", "--api-key", "k1", "--project-code", "P1")
	require.Equal(t, ExitOK, code)
	code, _, _ = runCLI(t, env, "", "--profile", "work", "config", "init", "--api-key", "k2", "--project-code", "P2")
	require.Equal(t, ExitOK, code)

	cfg, err := config.Load(filepath.Join(env["XDG_CONFIG_HOME"], "twaictl", "config.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "k1", cfg.Profiles["default"].APIKey)
	assert.Equal(t, "k2", cfg.Profiles["work"].APIKey)
	assert.Equal(t, "work", cfg.CurrentProfile, "最後 init 的 profile 成為 current_profile")
}

func TestConfigShowMasksSecretsGolden(t *testing.T) {
	env := map[string]string{"XDG_CONFIG_HOME": t.TempDir()}
	code, _, _ := runCLI(t, env, "", "config", "init",
		"--api-key", "abc123", "--project-code", "GOV999", "--cos-access-key", "AKIA", "--cos-secret-key", "s3cret")
	require.Equal(t, ExitOK, code)

	code, stdout, stderr := runCLI(t, env, "", "config", "show")

	require.Equal(t, ExitOK, code, stderr)
	assert.NotContains(t, stdout, "abc123")
	assert.NotContains(t, stdout, "s3cret")
	assert.Contains(t, stdout, "AKIA")
	// 路徑因 TempDir 而異，golden 前先把它換成固定字串
	normalized := replaceLine(stdout, "config_path:", "config_path: <path>")
	testutil.AssertGolden(t, "config_show.txt", []byte(normalized))
}

func TestConfigShowJSON(t *testing.T) {
	env := map[string]string{"XDG_CONFIG_HOME": t.TempDir(), config.EnvAPIKey: "env-key", config.EnvProjectCode: "ENVP"}
	code, stdout, _ := runCLI(t, env, "", "config", "show", "-o", "json")
	require.Equal(t, ExitOK, code)

	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	assert.Equal(t, config.MaskedValue, got["api_key"])
	assert.Equal(t, "ENVP", got["project_code"])
	assert.Equal(t, "goc", got["api_hosts"].(map[string]any)["common"])
}

func TestConfigShowWithoutAnyConfigStillWorks(t *testing.T) {
	env := map[string]string{"XDG_CONFIG_HOME": t.TempDir()}
	code, stdout, _ := runCLI(t, env, "", "config", "show")
	assert.Equal(t, ExitOK, code)
	assert.Contains(t, stdout, `api_key: ""`)
}

func TestUnknownProfileIsConfigError(t *testing.T) {
	env := map[string]string{"XDG_CONFIG_HOME": t.TempDir()}
	code, _, stderr := runCLI(t, env, "", "--profile", "ghost", "config", "show")
	assert.Equal(t, ExitConfig, code)
	assert.Contains(t, stderr, "ghost")
}

// TestPromptSecretFallsBackToLineReadWhenStdinIsNotTerminal 驗證 stdin 不是
// *os.File（例如測試常用的 strings.Reader）時，promptSecret 會退回一般的逐行
// 讀取，行為與 prompt 完全一致。
func TestPromptSecretFallsBackToLineReadWhenStdinIsNotTerminal(t *testing.T) {
	var stderr bytes.Buffer
	a := &app{stdin: strings.NewReader("typed-secret\n"), stderr: &stderr}

	got, err := a.promptSecret(bufio.NewReader(a.stdin), "API key: ")

	require.NoError(t, err)
	assert.Equal(t, "typed-secret", got)
	assert.Contains(t, stderr.String(), "API key: ")
}

// TestPromptSecretFallsBackWhenStdinIsNonTerminalFile 驗證 stdin 即使是
// *os.File，只要不是連到終端機（例如這裡用的暫存檔）就同樣退回逐行讀取，
// 不會誤判走進 term.ReadPassword 分支。
func TestPromptSecretFallsBackWhenStdinIsNonTerminalFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "stdin-*")
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	_, err = f.WriteString("file-secret\n")
	require.NoError(t, err)
	_, err = f.Seek(0, 0)
	require.NoError(t, err)

	var stderr bytes.Buffer
	a := &app{stdin: f, stderr: &stderr}

	got, err := a.promptSecret(bufio.NewReader(a.stdin), "API key: ")

	require.NoError(t, err)
	assert.Equal(t, "file-secret", got)
}

// replaceLine 把以 prefix 開頭的那一行整行換成 replacement。
func replaceLine(text, prefix, replacement string) string {
	lines := splitLines(text)
	for i, line := range lines {
		if len(line) >= len(prefix) && line[:len(prefix)] == prefix {
			lines[i] = replacement
		}
	}
	return joinLines(lines)
}

func splitLines(s string) []string { return strings.Split(s, "\n") }
func joinLines(l []string) string  { return strings.Join(l, "\n") }
