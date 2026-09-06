package version

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetContainsBuildInfoAndOpenAPIVersions(t *testing.T) {
	info := Get()

	assert.Equal(t, Version, info.Version)
	assert.Equal(t, Commit, info.Commit)
	assert.Equal(t, Date, info.Date)
	assert.True(t, strings.HasPrefix(info.GoVersion, "go"), "GoVersion 應以 go 開頭")
	assert.NotEmpty(t, info.OS)
	assert.NotEmpty(t, info.Arch)
	require.Len(t, info.OpenAPI, 3)
	assert.Equal(t, "1.5.4", info.OpenAPI["VCS"])
}

func TestStringFormat(t *testing.T) {
	info := Info{
		Version: "v0.0.1", Commit: "abc1234", Date: "2026-08-26T00:00:00Z",
		GoVersion: "go1.26.5", OS: "linux", Arch: "amd64",
		OpenAPI: map[string]string{"VCS": "1.5.4", "Ceph": "1.1.0", "Common": "1.2.1"},
	}
	want := "twaictl v0.0.1 (commit abc1234, built 2026-08-26T00:00:00Z, go1.26.5 linux/amd64)\n" +
		"OpenAPI: VCS 1.5.4, Ceph 1.1.0, Common 1.2.1\n"
	assert.Equal(t, want, info.String())
}
