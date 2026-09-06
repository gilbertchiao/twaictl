package vcs

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListVolumesRequiresProject(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/volumes/", 200, fixture(t, "volumes"))
	s := newService(t, f)

	res, err := s.ListVolumes(context.Background(), 101)
	require.NoError(t, err)
	require.Len(t, res.Value, 2)
	assert.Equal(t, "project=101", f.Requests()[0].Query)
}

func TestGetVolumeDecodes(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/volumes/1436398/", 200, fixture(t, "volume_detail"))
	s := newService(t, f)

	res, err := s.GetVolume(context.Background(), 1436398)
	require.NoError(t, err)
	assert.Equal(t, "data-01", res.Value.Name)
}

// TestCreateVolumeBody 驗證 Desc 空字串時不送 desc 欄位；VolumeType 有指定時原樣送出。
func TestCreateVolumeBody(t *testing.T) {
	f := newFakeAPI(t)
	f.on("POST", vcsBase+"/volumes/", 200, fixture(t, "volume_created"))
	s := newService(t, f)

	res, err := s.CreateVolume(context.Background(), CreateVolumeInput{
		ProjectID: 101, Name: "v1", SizeGB: 50, VolumeType: "ssd",
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1436400), res.Value.ID)

	var body map[string]any
	require.NoError(t, json.Unmarshal(f.Requests()[0].Body, &body))
	assert.Equal(t, map[string]any{"name": "v1", "project": float64(101), "size": float64(50), "volume_type": "ssd"}, body, "desc 空字串時不送")
}

func TestDeleteVolumeChecksStatus(t *testing.T) {
	f := newFakeAPI(t)
	f.on("DELETE", vcsBase+"/volumes/1436398/", 204, nil)
	f.on("DELETE", vcsBase+"/volumes/9999/", 404, []byte(`{"message":"Volume not found"}`))
	s := newService(t, f)

	require.NoError(t, s.DeleteVolume(context.Background(), 1436398))
	err := s.DeleteVolume(context.Background(), 9999)
	assert.ErrorContains(t, err, "Volume not found")
}

// TestVolumeActionOmitsZeroFields 驗證 serverID／sizeGB 為 0 時不送對應欄位：
// attach 只送 server，extend 只送 size。
func TestVolumeActionOmitsZeroFields(t *testing.T) {
	f := newFakeAPI(t)
	f.on("PUT", vcsBase+"/volumes/1436399/action/", 200, fixture(t, "volume_action"))
	s := newService(t, f)

	_, err := s.VolumeAction(context.Background(), 1436399, "attach", 3658462, 0)
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, json.Unmarshal(f.Requests()[0].Body, &body))
	assert.Equal(t, map[string]any{"status": "attach", "server": float64(3658462)}, body, "sizeGB 為 0 時不送 size")

	_, err = s.VolumeAction(context.Background(), 1436399, "extend", 0, 600)
	require.NoError(t, err)
	// 用新的 map 接收：json.Unmarshal 對既有 map 只會覆寫/新增鍵，不會清掉上一輪殘留的
	// "server" 鍵，重用同一個 body 會誤判成沒有省略欄位。
	var extendBody map[string]any
	require.NoError(t, json.Unmarshal(f.Requests()[1].Body, &extendBody))
	assert.Equal(t, map[string]any{"status": "extend", "size": float64(600)}, extendBody, "serverID 為 0 時不送 server")
}

// TestVolumeActionDecodesResult 驗證回應解析成 VolumeActionResult。
func TestVolumeActionDecodesResult(t *testing.T) {
	f := newFakeAPI(t)
	f.on("PUT", vcsBase+"/volumes/1436399/action/", 200, fixture(t, "volume_action"))
	s := newService(t, f)

	res, err := s.VolumeAction(context.Background(), 1436399, "attach", 3658462, 0)
	require.NoError(t, err)
	assert.Equal(t, FlexibleID("1436399"), res.Value.VolumeID, "真實回應 volume_id／server_id 皆為字串（A38），FlexibleID 應原樣保留")
	assert.Equal(t, FlexibleID("3658462"), res.Value.ServerID)
	assert.Equal(t, "/dev/vdc", res.Value.Mountpoint)
}

// TestVolumeActionAcceptsNumericServerIDAndEmptyBody 驗證 server_id 為數字（依 spec）時
// 也能解析，以及 detach 成功時回 200 空 body（A38）不視為錯誤、Raw 為空。
func TestVolumeActionAcceptsNumericServerIDAndEmptyBody(t *testing.T) {
	f := newFakeAPI(t)
	f.on("PUT", vcsBase+"/volumes/1436399/action/", 200, []byte(`{"mountpoint": "/dev/vdc", "server_id": 3658462, "volume_id": 1436399}`))
	s := newService(t, f)
	res, err := s.VolumeAction(context.Background(), 1436399, "attach", 3658462, 0)
	require.NoError(t, err)
	assert.Equal(t, FlexibleID("3658462"), res.Value.ServerID)

	f2 := newFakeAPI(t)
	f2.on("PUT", vcsBase+"/volumes/1436399/action/", 200, nil)
	s2 := newService(t, f2)
	res, err = s2.VolumeAction(context.Background(), 1436399, "detach", 3658462, 0)
	require.NoError(t, err, "detach 成功時 API 不回 body，不應視為解析錯誤")
	assert.Empty(t, res.Raw)
	assert.Equal(t, VolumeActionResult{}, res.Value)

	f3 := newFakeAPI(t)
	f3.on("PUT", vcsBase+"/volumes/1436399/action/", 400, []byte(`{"detail":"40001: Volume status is in-use."}`))
	s3 := newService(t, f3)
	_, err = s3.VolumeAction(context.Background(), 1436399, "detach", 3658462, 0)
	require.Error(t, err, "非 2xx 仍須回 APIError，不能因 body 檢查而被吞掉")
}

// TestResolveVolumeIDByName 驗證以名稱解析 volume id；純數字 ref 不查 API。
func TestResolveVolumeIDByName(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/volumes/", 200, fixture(t, "volumes"))
	s := newService(t, f)

	id, err := s.ResolveVolumeID(context.Background(), 101, "scratch")
	require.NoError(t, err)
	assert.Equal(t, int64(1436399), id)

	id, err = s.ResolveVolumeID(context.Background(), 101, "1436398")
	require.NoError(t, err)
	assert.Equal(t, int64(1436398), id)
	assert.Len(t, f.Requests(), 1, "數字 id 不查 API")

	_, err = s.ResolveVolumeID(context.Background(), 101, "nope")
	assert.ErrorContains(t, err, "找不到 volume")
}
