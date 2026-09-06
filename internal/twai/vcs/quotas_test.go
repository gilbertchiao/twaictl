package vcs

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	genvcs "github.com/gilbertchiao/twaictl/internal/gen/vcs"
)

func TestProjectQuotaFirstAndEmpty(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/project_quotas/", 200, fixture(t, "project_quotas"))
	s := newService(t, f)

	res, err := s.ProjectQuota(context.Background(), 101)
	require.NoError(t, err)
	assert.Equal(t, 24.0, res.Value.Cpu.Usage)
	assert.Equal(t, -1.0, res.Value.Cpu.Quota)
	assert.Equal(t, "project=101", f.Requests()[0].Query)
	// Raw 是整個陣列（-o json 用它），不是取出的第一筆。
	assert.JSONEq(t, string(fixture(t, "project_quotas")), string(res.Raw))

	f2 := newFakeAPI(t)
	f2.on("GET", vcsBase+"/project_quotas/", 200, []byte("[]"))
	s2 := newService(t, f2)
	_, err = s2.ProjectQuota(context.Background(), 101)
	assert.ErrorContains(t, err, "quota")
}

func TestUserQuotasDecodes(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/projects/101/user_quotas/", 200, fixture(t, "user_quotas"))
	s := newService(t, f)

	res, err := s.UserQuotas(context.Background(), 101)
	require.NoError(t, err)
	require.Len(t, res.Value, 2)
	assert.Equal(t, "alice", res.Value[0].User.Username)
	assert.Equal(t, "bob", res.Value[1].User.Username)
	assert.Nil(t, res.Value[1].FloatingIp, "bob 未回傳 floating_ip")
}

// quotaUsage 是測試用的簡短建構子，組出 genvcs.QuotaUsageSerializer。
func quotaUsage(usage, quota float64) genvcs.QuotaUsageSerializer {
	return genvcs.QuotaUsageSerializer{Usage: usage, Quota: quota}
}

func TestQuotaRowsUnlimited(t *testing.T) {
	floating := quotaUsage(5, 10)
	static := quotaUsage(4, 10)
	rows := QuotaRows(
		quotaUsage(24, -1), quotaUsage(0, 80), quotaUsage(83968, -1),
		&floating, &static,
	)
	require.Len(t, rows, 5)
	assert.Equal(t, QuotaRow{Resource: "cpu", Usage: 24, Quota: -1}, rows[0])
	assert.Equal(t, QuotaRow{Resource: "gpu", Usage: 0, Quota: 80}, rows[1])
	assert.Equal(t, QuotaRow{Resource: "memory", Usage: 83968, Quota: -1}, rows[2])
	assert.Equal(t, QuotaRow{Resource: "floating_ip", Usage: 5, Quota: 10}, rows[3])
	assert.Equal(t, QuotaRow{Resource: "static_ip", Usage: 4, Quota: 10}, rows[4])

	// floating/static 為 nil 時省略對應列。
	rows = QuotaRows(quotaUsage(1, 2), quotaUsage(1, 2), quotaUsage(1, 2), nil, nil)
	assert.Len(t, rows, 3)
}

func TestProjectSolutionIDsDecodes(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/projects/101/solutions/", 200, fixture(t, "project_solutions"))
	s := newService(t, f)

	res, err := s.ProjectSolutionIDs(context.Background(), 101)
	require.NoError(t, err)
	assert.Equal(t, []int{11, 99}, res.Value)
	assert.JSONEq(t, string(fixture(t, "project_solutions")), string(res.Raw))
}

func TestListSolutionsQueriesCommon(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", "/api/v3/solutions/", 200, fixture(t, "solutions"))
	s := newService(t, f)

	items, err := s.ListSolutions(context.Background(), 101)
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, "Ubuntu 22.04", items[0].Name)
}
