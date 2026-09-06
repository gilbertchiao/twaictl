package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilbertchiao/twaictl/internal/testutil"
	"github.com/gilbertchiao/twaictl/internal/version"
)

// runCLI 以 buffer 執行 Execute，回傳 exit code、stdout、stderr。
func runCLI(t *testing.T, env map[string]string, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	isolated := withIsolatedConfigHome(t, env)
	getenv := func(key string) string { return isolated[key] }
	code := Execute(args, strings.NewReader(stdin), &stdout, &stderr, getenv)
	return code, stdout.String(), stderr.String()
}

// withIsolatedConfigHome 確保測試不會讀到開發機真實的 ~/.config/twaictl/config.yaml：
// 呼叫端沒有指定 XDG_CONFIG_HOME 時（env 為 nil 或該 key 不存在），
// 回傳一份複製的 map、注入 t.TempDir() 作為 XDG_CONFIG_HOME；
// 不修改呼叫端傳入的原始 map，避免影響呼叫端後續對同一個 map 的使用。
func withIsolatedConfigHome(t *testing.T, env map[string]string) map[string]string {
	t.Helper()
	if _, ok := env["XDG_CONFIG_HOME"]; ok {
		return env
	}
	isolated := make(map[string]string, len(env)+1)
	for k, v := range env {
		isolated[k] = v
	}
	isolated["XDG_CONFIG_HOME"] = t.TempDir()
	return isolated
}

func TestVersionTable(t *testing.T) {
	// 固定 build 變數，讓 golden 檔穩定
	restore := stubVersionInfo()
	defer restore()

	code, stdout, stderr := runCLI(t, nil, "", "version")

	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	testutil.AssertGolden(t, "version.txt", []byte(stdout))
}

func TestVersionJSON(t *testing.T) {
	restore := stubVersionInfo()
	defer restore()

	code, stdout, _ := runCLI(t, nil, "", "version", "-o", "json")

	require.Equal(t, 0, code)
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	assert.Equal(t, "v0.0.1-test", got["version"])
	assert.Equal(t, "1.5.4", got["openapi"].(map[string]any)["VCS"])
}

// stubVersionInfo 把 versionInfoFunc 換成固定值，回傳還原函式。
func stubVersionInfo() func() {
	original := versionInfoFunc
	versionInfoFunc = func() version.Info {
		return version.Info{
			Version: "v0.0.1-test", Commit: "abc1234", Date: "2026-08-26T00:00:00Z",
			GoVersion: "go1.26.0", OS: "linux", Arch: "amd64",
			OpenAPI: map[string]string{"VCS": "1.5.4", "Ceph": "1.1.0", "Common": "1.2.1"},
		}
	}
	return func() { versionInfoFunc = original }
}
