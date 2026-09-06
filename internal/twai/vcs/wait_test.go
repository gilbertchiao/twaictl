package vcs

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilbertchiao/twaictl/internal/twai"
)

func TestWaitSiteReadyPollsUntilReady(t *testing.T) {
	f := newFakeAPI(t)
	f.onSequence("GET", vcsBase+"/sites/7/",
		seq{Status: 200, Body: fixture(t, "site_initializing")},
		seq{Status: 200, Body: fixture(t, "site_initializing")},
		seq{Status: 200, Body: fixture(t, "site_ready")})
	s := newService(t, f)

	var seen []string
	err := s.WaitSiteReady(context.Background(), 7, time.Millisecond, func(st string) { seen = append(seen, st) })
	require.NoError(t, err)
	assert.Equal(t, []string{"Initializing", "Initializing", "Ready"}, seen)
}

func TestWaitSiteReadyFailsOnErrorStatus(t *testing.T) {
	f := newFakeAPI(t)
	errorBody := bytes.Replace(fixture(t, "site_initializing"), []byte("Initializing"), []byte("Error"), 1)
	f.on("GET", vcsBase+"/sites/7/", 200, errorBody)
	s := newService(t, f)

	err := s.WaitSiteReady(context.Background(), 7, time.Millisecond, func(string) {})
	assert.ErrorContains(t, err, "Error")
}

func TestWaitSiteDeletedStopsOn404(t *testing.T) {
	f := newFakeAPI(t)
	f.onSequence("GET", vcsBase+"/sites/7/",
		seq{Status: 200, Body: fixture(t, "site_ready")},
		seq{Status: 404, Body: []byte(`{"message":"not found"}`)})
	s := newService(t, f)
	require.NoError(t, s.WaitSiteDeleted(context.Background(), 7, time.Millisecond, func(string) {}))
}

func TestWaitSiteReadyTimeout(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/sites/7/", 200, fixture(t, "site_initializing"))
	s := newService(t, f)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
	defer cancel()
	err := s.WaitSiteReady(ctx, 7, time.Millisecond, func(string) {})
	assert.ErrorIs(t, err, twai.ErrWaitTimeout)
}

// lbDetailBody 組出最小可用的 LBDetailSerializer JSON，只包含 WaitLoadBalancerActive／
// WaitLoadBalancerDeleted 需要判斷的欄位（status、status_reason），其餘必填欄位省略
// （生成型別對缺漏欄位僅留零值，不影響 json.Unmarshal）。
func lbDetailBody(status, statusReason string) []byte {
	body := `{"id": 5, "status": "` + status + `"`
	if statusReason != "" {
		body += `, "status_reason": "` + statusReason + `"`
	}
	body += `}`
	return []byte(body)
}

func TestWaitLoadBalancerActive(t *testing.T) {
	f := newFakeAPI(t)
	f.onSequence("GET", vcsBase+"/loadbalancers/5/",
		seq{Status: 200, Body: lbDetailBody(LoadBalancerStatusBuild, "")},
		seq{Status: 200, Body: lbDetailBody(LoadBalancerStatusBuild, "")},
		seq{Status: 200, Body: lbDetailBody("ACTIVE", "")})
	s := newService(t, f)

	var seen []string
	err := s.WaitLoadBalancerActive(context.Background(), 5, time.Millisecond, func(st string) { seen = append(seen, st) })
	require.NoError(t, err)
	assert.Equal(t, []string{LoadBalancerStatusBuild, LoadBalancerStatusBuild, "ACTIVE"}, seen)
}

func TestWaitLoadBalancerActiveFailsOnErrorStatus(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/loadbalancers/5/", 200, lbDetailBody("ERROR", "quota exceeded"))
	s := newService(t, f)

	err := s.WaitLoadBalancerActive(context.Background(), 5, time.Millisecond, func(string) {})
	assert.ErrorContains(t, err, "ERROR")
	assert.ErrorContains(t, err, "quota exceeded")
}

// TestWaitLoadBalancerUpdatedRequiresLeavingActive 驗證 --wait 對 `lb action` 的保護：
// action 送出後 load balancer 通常仍是 ACTIVE（更新尚未真的開始），必須先觀察到離開
// ACTIVE（例如 UPDATING）才能判定完成，不會在第一次輪詢就誤判為已完成
// （比照 WaitServerStatus 的 mustLeave 語意）。
func TestWaitLoadBalancerUpdatedRequiresLeavingActive(t *testing.T) {
	f := newFakeAPI(t)
	f.onSequence("GET", vcsBase+"/loadbalancers/5/",
		seq{Status: 200, Body: lbDetailBody("ACTIVE", "")},
		seq{Status: 200, Body: lbDetailBody(LoadBalancerStatusUpdating, "")},
		seq{Status: 200, Body: lbDetailBody("ACTIVE", "")})
	s := newService(t, f)

	var seen []string
	err := s.WaitLoadBalancerUpdated(context.Background(), 5, time.Millisecond, func(st string) { seen = append(seen, st) })
	require.NoError(t, err)
	assert.Equal(t, []string{"ACTIVE", LoadBalancerStatusUpdating, "ACTIVE"}, seen)
}

// TestWaitLoadBalancerUpdatedDoesNotSucceedWhenNeverLeavingActive 是這個保護的迴歸測試：
// 若整個更新過程從未被輪詢到「已離開 ACTIVE」的瞬間（每次輪詢都還是 ACTIVE），
// --wait 不能在第一輪就誤判為已完成，而是持續等到逾時。
func TestWaitLoadBalancerUpdatedDoesNotSucceedWhenNeverLeavingActive(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/loadbalancers/5/", 200, lbDetailBody("ACTIVE", ""))
	s := newService(t, f)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
	defer cancel()
	err := s.WaitLoadBalancerUpdated(ctx, 5, time.Millisecond, func(string) {})
	assert.ErrorIs(t, err, twai.ErrWaitTimeout)
}

// TestWaitLoadBalancerUpdatedFailsOnErrorStatus 驗證 status 含 "ERROR" 子字串時視為失敗，
// 且錯誤訊息附上 StatusReason（同 WaitLoadBalancerActive）。
func TestWaitLoadBalancerUpdatedFailsOnErrorStatus(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/loadbalancers/5/", 200, lbDetailBody("ERROR", "boom"))
	s := newService(t, f)

	err := s.WaitLoadBalancerUpdated(context.Background(), 5, time.Millisecond, func(string) {})
	assert.ErrorContains(t, err, "ERROR")
	assert.ErrorContains(t, err, "boom")
}

func TestWaitLoadBalancerDeleted(t *testing.T) {
	f := newFakeAPI(t)
	f.onSequence("GET", vcsBase+"/loadbalancers/5/",
		seq{Status: 200, Body: lbDetailBody(LoadBalancerStatusDeleting, "")},
		seq{Status: 404, Body: []byte(`{"message":"not found"}`)})
	s := newService(t, f)

	var seen []string
	err := s.WaitLoadBalancerDeleted(context.Background(), 5, time.Millisecond, func(st string) { seen = append(seen, st) })
	require.NoError(t, err)
	assert.Equal(t, "Deleted", seen[len(seen)-1])
}

// TestWaitLoadBalancerDeletedStopsOnDeletedStatus 涵蓋 load balancer 進入 status "DELETED"
// （而非回 404）就視為刪除完成的情境；2026-08-28 實測未觀察到此狀態（刪除完成直接 404，
// 見 LoadBalancerStatusDeleted 的說明），保留此分支無害，仍以此測試涵蓋。
func TestWaitLoadBalancerDeletedStopsOnDeletedStatus(t *testing.T) {
	f := newFakeAPI(t)
	f.onSequence("GET", vcsBase+"/loadbalancers/5/",
		seq{Status: 200, Body: lbDetailBody(LoadBalancerStatusDeleting, "")},
		seq{Status: 200, Body: lbDetailBody("DELETED", "")})
	s := newService(t, f)

	var seen []string
	err := s.WaitLoadBalancerDeleted(context.Background(), 5, time.Millisecond, func(st string) { seen = append(seen, st) })
	require.NoError(t, err)
	assert.Equal(t, []string{LoadBalancerStatusDeleting, "DELETED"}, seen)
}
