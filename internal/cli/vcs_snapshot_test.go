package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilbertchiao/twaictl/internal/testutil"
)

func TestVCSSnapshotLsGoldenAndStatusFilter(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/snapshots/", 200, "vcs/snapshots")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "snapshot", "ls")
	require.Equal(t, ExitOK, code, stderr)
	testutil.AssertGolden(t, "vcs_snapshot_ls.txt", []byte(stdout))
	_, _, _ = runCLI(t, f.env(t), "", "vcs", "snapshot", "ls", "--status", "available")
	// 生成碼依 GetSnapshotsParams 欄位宣告順序（Status 先於 Project）組 query string，
	// 而非 url.Values.Encode() 的字母序，故順序為 status 在前（與 vcs.ResolveSolutionID
	// 測試中 project 在前的情況相反，同樣是照生成碼欄位順序）。
	assert.Equal(t, "status=available&project=101", f.Requests()[len(f.Requests())-1].Query)
}

func TestVCSSnapshotCreateResolvesVolumeName(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/volumes/", 200, "vcs/volumes")
	f.onFixture("POST", vcsBase+"/snapshots/", 200, "vcs/snapshot_detail")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "snapshot", "create", "--name", "s1", "--volume", "data-01", "--desc", "d")
	require.Equal(t, ExitOK, code, stderr)
	assert.JSONEq(t, `{"name":"s1","volume":1436398,"desc":"d"}`, string(f.find("POST", vcsBase+"/snapshots/").Body))
	assert.Contains(t, stdout, "snap-data-01")
}

func TestVCSSnapshotGetAndRm(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/snapshots/", 200, "vcs/snapshots")
	f.onFixture("GET", vcsBase+"/snapshots/501/", 200, "vcs/snapshot_detail")
	f.on("DELETE", vcsBase+"/snapshots/501/", 204, "")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "snapshot", "get", "snap-data-01")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stdout, "before upgrade")
	code, _, stderr = runCLI(t, f.env(t), "n\n", "vcs", "snapshot", "rm", "501")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Nil(t, f.find("DELETE", vcsBase+"/snapshots/501/"))
	code, _, stderr = runCLI(t, f.env(t), "", "vcs", "snapshot", "rm", "501", "-y")
	require.Equal(t, ExitOK, code, stderr)
}
