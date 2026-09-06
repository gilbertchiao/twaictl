package vcs

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListIPsAddressFilter(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/ips/", 200, fixture(t, "ips"))
	s := newService(t, f)

	res, err := s.ListIPs(context.Background(), 101, "")
	require.NoError(t, err)
	require.Len(t, res.Value, 3)
	assert.Equal(t, "project=101", f.Requests()[0].Query)

	_, err = s.ListIPs(context.Background(), 101, "203.0.113.219")
	require.NoError(t, err)
	assert.Equal(t, "project=101&address=203.0.113.219", f.Requests()[1].Query)
}

func TestGetIPDecodes(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/ips/1393847/", 200, fixture(t, "ip_detail"))
	s := newService(t, f)

	res, err := s.GetIP(context.Background(), 1393847)
	require.NoError(t, err)
	assert.Equal(t, "203.0.113.219", res.Value.Address)
}

func TestCreateIPSendsProject(t *testing.T) {
	f := newFakeAPI(t)
	f.on("POST", vcsBase+"/ips/", 200, fixture(t, "ip_detail"))
	s := newService(t, f)

	_, err := s.CreateIP(context.Background(), 101)
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, json.Unmarshal(f.Requests()[0].Body, &body))
	assert.Equal(t, map[string]any{"project": float64(101)}, body)
}

// TestCreateIPRejectsProjectIDOverInt32Range 驗證 projectID 超出 API 的 int32 範圍時，
// 在轉型前就回傳明確錯誤，而不是靜默截斷成錯誤的 id。
func TestCreateIPRejectsProjectIDOverInt32Range(t *testing.T) {
	f := newFakeAPI(t)
	s := newService(t, f)

	_, err := s.CreateIP(context.Background(), math.MaxInt32+1)
	assert.ErrorContains(t, err, "不是合法的 id")
	assert.Empty(t, f.Requests())
}

// TestCreateIPRejectsNonPositiveProjectID 驗證 projectID 為 0 或負數時同樣在轉型前
// 回傳明確錯誤：int32(projectID) 對 0/負數雖不會溢位，但這種 id 對 API 而言必然無效，
// 與「超出上限」是同一類「id 不在合法範圍」的錯誤，用同一個檢查與訊息涵蓋。
func TestCreateIPRejectsNonPositiveProjectID(t *testing.T) {
	f := newFakeAPI(t)
	s := newService(t, f)

	for _, id := range []int64{0, -1} {
		_, err := s.CreateIP(context.Background(), id)
		assert.ErrorContains(t, err, "不是合法的 id", "id=%d", id)
		assert.Empty(t, f.Requests(), "id=%d", id)
	}
}

func TestUpdateIPDescSendsPatch(t *testing.T) {
	f := newFakeAPI(t)
	f.on("PATCH", vcsBase+"/ips/1393847/", 200, fixture(t, "ip_detail"))
	s := newService(t, f)

	_, err := s.UpdateIPDesc(context.Background(), 1393847, "for web")
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, json.Unmarshal(f.Requests()[0].Body, &body))
	assert.Equal(t, map[string]any{"desc": "for web"}, body)
}

func TestDeleteIPChecksStatus(t *testing.T) {
	f := newFakeAPI(t)
	f.on("DELETE", vcsBase+"/ips/1393847/", 204, nil)
	f.on("DELETE", vcsBase+"/ips/9999/", 404, []byte(`{"message":"IP not found"}`))
	s := newService(t, f)

	require.NoError(t, s.DeleteIP(context.Background(), 1393847))
	err := s.DeleteIP(context.Background(), 9999)
	assert.ErrorContains(t, err, "IP not found")
}

func TestResolveIPIDByAddress(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/ips/", 200, fixture(t, "ips"))
	s := newService(t, f)

	id, err := s.ResolveIPID(context.Background(), 101, "203.0.113.219")
	require.NoError(t, err)
	assert.Equal(t, int64(1393847), id)

	id, err = s.ResolveIPID(context.Background(), 101, "1393846")
	require.NoError(t, err)
	assert.Equal(t, int64(1393846), id)
	assert.Len(t, f.Requests(), 1, "數字 id 不查 API")

	_, err = s.ResolveIPID(context.Background(), 101, "9.9.9.9")
	assert.ErrorContains(t, err, "找不到 ip")
}
