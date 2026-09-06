package vcs

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveProjectIDByCodeAndNumber(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/projects/", 200, fixture(t, "projects"))
	s := newService(t, f)

	id, err := s.ResolveProjectID(context.Background(), "GOV2")
	require.NoError(t, err)
	assert.Equal(t, int64(202), id)
	assert.Equal(t, "name=GOV2", f.Requests()[0].Query, "以 name 查詢縮小清單")

	id, err = s.ResolveProjectID(context.Background(), "999")
	require.NoError(t, err)
	assert.Equal(t, int64(999), id)
	assert.Len(t, f.Requests(), 1, "數字 id 不查 API")

	_, err = s.ResolveProjectID(context.Background(), "NOPE")
	assert.ErrorContains(t, err, "找不到 project")
}

func TestResolveSiteIDByName(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/sites/", 200, fixture(t, "sites"))
	s := newService(t, f)

	id, err := s.ResolveSiteID(context.Background(), 101, "web-01")
	require.NoError(t, err)
	assert.Equal(t, int64(7), id)

	_, err = s.ResolveSiteID(context.Background(), 101, "nope")
	assert.ErrorContains(t, err, "找不到 site")
}

func TestResolveNetworkIDByName(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/networks/", 200, fixture(t, "networks"))
	s := newService(t, f)

	id, err := s.ResolveNetworkID(context.Background(), 101, "backend")
	require.NoError(t, err)
	assert.Equal(t, int64(328292), id)

	id, err = s.ResolveNetworkID(context.Background(), 101, "328291")
	require.NoError(t, err)
	assert.Equal(t, int64(328291), id)
	assert.Len(t, f.Requests(), 1, "數字 id 不查 API")

	_, err = s.ResolveNetworkID(context.Background(), 101, "nope")
	assert.ErrorContains(t, err, "找不到 network")
}

func TestResolveLoadBalancerID(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/loadbalancers/", 200, []byte(
		`[{"id": 5, "name": "web-lb"}, {"id": 6, "name": "api-lb"}, {"id": 9, "name": "dup"}, {"id": 10, "name": "dup"}]`))
	s := newService(t, f)

	id, err := s.ResolveLoadBalancerID(context.Background(), 101, "web-lb")
	require.NoError(t, err)
	assert.Equal(t, int64(5), id)

	id, err = s.ResolveLoadBalancerID(context.Background(), 101, "5")
	require.NoError(t, err)
	assert.Equal(t, int64(5), id)
	assert.Len(t, f.Requests(), 1, "數字 id 不查 API")

	_, err = s.ResolveLoadBalancerID(context.Background(), 101, "dup")
	assert.ErrorContains(t, err, "對應多筆資源")

	_, err = s.ResolveLoadBalancerID(context.Background(), 101, "nope")
	assert.ErrorContains(t, err, "找不到 load balancer")
}

func TestResolveSolutionIDQueriesCommonWithCategoryOS(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", "/api/v3/solutions/", 200, fixture(t, "solutions"))
	s := newService(t, f)

	id, err := s.ResolveSolutionID(context.Background(), 101, "Ubuntu 22.04")
	require.NoError(t, err)
	assert.Equal(t, int64(11), id)
	// 生成碼依 GetSolutionsParams 欄位宣告順序（Project 先於 Category）組 query string，
	// 而非 url.Values.Encode() 的字母序，故順序為 project 在前。
	assert.Equal(t, "project=101&category=os", f.Requests()[0].Query)
	assert.Equal(t, "goc", f.Requests()[0].Header.Get("x-api-host"))
}
