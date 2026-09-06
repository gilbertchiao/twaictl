package vcs

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestListSnapshotsStatusFilter 驗證 status 空字串時不送查詢參數，非空時以 status 篩選。
func TestListSnapshotsStatusFilter(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/snapshots/", 200, fixture(t, "snapshots"))
	s := newService(t, f)

	res, err := s.ListSnapshots(context.Background(), 101, "")
	require.NoError(t, err)
	require.Len(t, res.Value, 2)
	assert.Equal(t, "project=101", f.Requests()[0].Query, "status 空字串時不送")

	_, err = s.ListSnapshots(context.Background(), 101, "available")
	require.NoError(t, err)
	// 生成碼依 GetSnapshotsParams 欄位宣告順序（Status 先於 Project）組 query string，
	// 而非 url.Values.Encode() 的字母序，故順序為 status 在前。
	assert.Equal(t, "status=available&project=101", f.Requests()[1].Query)
}

func TestGetSnapshotDecodes(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/snapshots/501/", 200, fixture(t, "snapshot_detail"))
	s := newService(t, f)

	res, err := s.GetSnapshot(context.Background(), 501)
	require.NoError(t, err)
	assert.Equal(t, "snap-data-01", res.Value.Name)
}

// TestCreateSnapshotBody 驗證 desc 空字串時不送 desc 欄位；volume 以 int32 送出。
func TestCreateSnapshotBody(t *testing.T) {
	f := newFakeAPI(t)
	f.on("POST", vcsBase+"/snapshots/", 200, fixture(t, "snapshot_detail"))
	s := newService(t, f)

	_, err := s.CreateSnapshot(context.Background(), "s1", 1436398, "")
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, json.Unmarshal(f.Requests()[0].Body, &body))
	assert.Equal(t, map[string]any{"name": "s1", "volume": float64(1436398)}, body, "desc 空字串時不送")

	_, err = s.CreateSnapshot(context.Background(), "s1", 1436398, "d")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(f.Requests()[1].Body, &body))
	assert.Equal(t, map[string]any{"name": "s1", "volume": float64(1436398), "desc": "d"}, body)
}

// TestCreateSnapshotRejectsVolumeIDOverInt32Range 驗證 volumeID 超出 API 的 int32 範圍時，
// 在轉型前就回傳明確錯誤，而不是靜默截斷成錯誤的 id。
func TestCreateSnapshotRejectsVolumeIDOverInt32Range(t *testing.T) {
	f := newFakeAPI(t)
	s := newService(t, f)

	_, err := s.CreateSnapshot(context.Background(), "s1", math.MaxInt32+1, "")
	assert.ErrorContains(t, err, "不是合法的 id")
	assert.Empty(t, f.Requests())
}

// TestCreateSnapshotRejectsNonPositiveVolumeID 驗證 volumeID 為 0 或負數時同樣在轉型前
// 回傳明確錯誤：int32(volumeID) 對 0/負數雖不會溢位，但這種 id 對 API 而言必然無效，
// 與「超出上限」是同一類「id 不在合法範圍」的錯誤，用同一個檢查與訊息涵蓋。
func TestCreateSnapshotRejectsNonPositiveVolumeID(t *testing.T) {
	f := newFakeAPI(t)
	s := newService(t, f)

	for _, id := range []int64{0, -1} {
		_, err := s.CreateSnapshot(context.Background(), "s1", id, "")
		assert.ErrorContains(t, err, "不是合法的 id", "id=%d", id)
		assert.Empty(t, f.Requests(), "id=%d", id)
	}
}

func TestDeleteSnapshotChecksStatus(t *testing.T) {
	f := newFakeAPI(t)
	f.on("DELETE", vcsBase+"/snapshots/501/", 204, nil)
	f.on("DELETE", vcsBase+"/snapshots/9999/", 404, []byte(`{"message":"Snapshot not found"}`))
	s := newService(t, f)

	require.NoError(t, s.DeleteSnapshot(context.Background(), 501))
	err := s.DeleteSnapshot(context.Background(), 9999)
	assert.ErrorContains(t, err, "Snapshot not found")
}

// TestResolveSnapshotIDByName 驗證以名稱解析 snapshot id；純數字 ref 不查 API。
func TestResolveSnapshotIDByName(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/snapshots/", 200, fixture(t, "snapshots"))
	s := newService(t, f)

	id, err := s.ResolveSnapshotID(context.Background(), 101, "snap-data-01")
	require.NoError(t, err)
	assert.Equal(t, int64(501), id)

	id, err = s.ResolveSnapshotID(context.Background(), 101, "502")
	require.NoError(t, err)
	assert.Equal(t, int64(502), id)
	assert.Len(t, f.Requests(), 1, "數字 id 不查 API")

	_, err = s.ResolveSnapshotID(context.Background(), 101, "nope")
	assert.ErrorContains(t, err, "找不到 snapshot")
}
