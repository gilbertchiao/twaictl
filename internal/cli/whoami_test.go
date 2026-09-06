package cli

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilbertchiao/twaictl/internal/config"
	"github.com/gilbertchiao/twaictl/internal/testutil"
)

func TestWhoamiTableGolden(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", "/api/v3/version/", 200, `{"version":"v5"}`)
	code, stdout, stderr := runCLI(t, f.env(t), "", "config", "whoami")
	require.Equal(t, ExitOK, code, stderr)
	testutil.AssertGolden(t, "whoami.txt", []byte(stdout))
	assert.Equal(t, "goc", f.find("GET", "/api/v3/version/").Header.Get("x-api-host"))
}

func TestWhoamiJSON(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", "/api/v3/version/", 200, `{"version":"v5"}`)
	code, stdout, _ := runCLI(t, f.env(t), "", "config", "whoami", "-o", "json")
	require.Equal(t, ExitOK, code)
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	assert.Equal(t, "v5", got["api_version"])
	assert.Equal(t, "ENT1", got["project_code"])
	assert.Equal(t, float64(101), got["project_id"])
	assert.Len(t, got["projects"], 2)
}

func TestWhoamiUnauthorizedIsExitAPI(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", "/api/v3/version/", 401, `{"message":"Invalid API key"}`)
	code, _, stderr := runCLI(t, f.env(t), "", "config", "whoami")
	assert.Equal(t, ExitAPI, code)
	assert.Contains(t, stderr, "401")
	assert.Contains(t, stderr, "Invalid API key")
	assert.NotContains(t, stderr, "test-key")
}

func TestWhoamiWithoutAPIKeyIsExitConfig(t *testing.T) {
	f := newFakeAPI(t)
	env := f.env(t)
	delete(env, config.EnvAPIKey)
	code, _, stderr := runCLI(t, env, "", "config", "whoami")
	assert.Equal(t, ExitConfig, code)
	assert.Contains(t, stderr, "config init")
}

func TestWhoamiNumericProjectCode(t *testing.T) {
	// TWAI_PROJECT_CODE 也可以是數字 project id（--project 支援 code 或數字 id，見 README）；
	// 但 whoami 原本只比對 p.Name == ProjectCode，數字 id 永遠對不上任何 project 的 Name，
	// 會被誤報成「找不到對應的 project」。
	f := newFakeAPI(t)
	f.on("GET", "/api/v3/version/", 200, `{"version":"v5"}`)
	env := f.env(t)
	env[config.EnvProjectCode] = "101"
	code, stdout, _ := runCLI(t, env, "", "config", "whoami")
	assert.Equal(t, ExitOK, code)
	assert.Contains(t, stdout, "101 (id 101)")
	assert.NotContains(t, stdout, "找不到")
}

func TestWhoamiProjectCodeNotFoundStillSucceeds(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", "/api/v3/version/", 200, `{"version":"v5"}`)
	env := f.env(t)
	env[config.EnvProjectCode] = "NOPE"
	code, stdout, _ := runCLI(t, env, "", "config", "whoami")
	assert.Equal(t, ExitOK, code)
	assert.Contains(t, stdout, "NOPE（找不到對應的 project）")
}
