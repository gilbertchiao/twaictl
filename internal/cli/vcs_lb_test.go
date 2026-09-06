package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	genvcs "github.com/gilbertchiao/twaictl/internal/gen/vcs"
	"github.com/gilbertchiao/twaictl/internal/testutil"
)

// strPtr 是本檔測試用的小工具，回傳指向傳入字串複本的指標。
func strPtr(s string) *string { return &s }

// TestLbMonitorSummary 是 lbMonitorSummary 的表格驅動測試，涵蓋 ruling #2(b) 新增的
// 「只有 HttpMethod」與「只有 UrlPath」兩個分支（既有 fixture 的 monitor 兩個欄位皆有值，
// 沒有涵蓋到這兩種只有其中一個的情況）。
func TestLbMonitorSummary(t *testing.T) {
	cases := []struct {
		name string
		m    *genvcs.LBHealthMonitorSerializer
		want string
	}{
		{"nil monitor", nil, ""},
		{
			"http_method 與 url_path 都有",
			&genvcs.LBHealthMonitorSerializer{MonitorType: strPtr("HTTP"), HttpMethod: strPtr("GET"), UrlPath: strPtr("/")},
			"HTTP GET /",
		},
		{
			"只有 http_method",
			&genvcs.LBHealthMonitorSerializer{MonitorType: strPtr("HTTP"), HttpMethod: strPtr("GET")},
			"HTTP GET",
		},
		{
			"只有 url_path",
			&genvcs.LBHealthMonitorSerializer{MonitorType: strPtr("HTTP"), UrlPath: strPtr("/health")},
			"HTTP /health",
		},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, lbMonitorSummary(c.m), c.name)
	}
}

func TestVCSLbLsGolden(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/loadbalancers/", 200, "vcs/loadbalancers")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "lb", "ls")
	require.Equal(t, ExitOK, code, stderr)
	assert.Equal(t, "project=101", f.find("GET", vcsBase+"/loadbalancers/").Query)
	testutil.AssertGolden(t, "vcs_lb_ls.txt", []byte(stdout))
}

func TestVCSLbGetByNameGolden(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/loadbalancers/", 200, "vcs/loadbalancers")
	f.onFixture("GET", vcsBase+"/loadbalancers/5/", 200, "vcs/loadbalancer_detail")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "lb", "get", "web-lb")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stdout, "10.0.0.11:80(ACTIVE), 10.0.0.12:80(ERROR)")
	testutil.AssertGolden(t, "vcs_lb_get.txt", []byte(stdout))
}

func TestVCSLbRmConfirmAndWait(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/loadbalancers/", 200, "vcs/loadbalancers")
	f.on("DELETE", vcsBase+"/loadbalancers/5/", 204, "")
	code, _, stderr := runCLI(t, f.env(t), "n\n", "vcs", "lb", "rm", "web-lb")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Nil(t, f.find("DELETE", vcsBase+"/loadbalancers/5/"), "取消後不可送出 DELETE")

	code, _, stderr = runCLI(t, f.env(t), "", "vcs", "lb", "rm", "web-lb", "-y")
	require.Equal(t, ExitOK, code, stderr)
	assert.NotNil(t, f.find("DELETE", vcsBase+"/loadbalancers/5/"))

	f2 := newFakeAPI(t)
	f2.onFixture("GET", vcsBase+"/loadbalancers/", 200, "vcs/loadbalancers")
	f2.on("DELETE", vcsBase+"/loadbalancers/5/", 204, "")
	f2.onSequence("GET", vcsBase+"/loadbalancers/5/",
		seq{status: 200, body: `{"id": 5, "status": "DELETING"}`},
		seq{status: 404, body: `{"message":"not found"}`})
	code2, _, stderr2 := runCLI(t, f2.env(t), "", "vcs", "lb", "rm", "web-lb", "-y", "--wait", "--wait-interval", "1ms")
	require.Equal(t, ExitOK, code2, stderr2)
	assert.Contains(t, stderr2, "Deleted")
}
