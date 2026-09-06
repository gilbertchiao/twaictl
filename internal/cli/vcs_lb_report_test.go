package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilbertchiao/twaictl/internal/testutil"
)

// TestVCSLbReportProjectGolden 驗證不帶 <id|name> 時查詢 project 報表：
// GET /loadbalancers/reports/?project=101，期間與 total_lb_num 走 stderr（progress），
// stdout 是攤平後的表格（ID/NAME/USER/CREATED/DELETED）。
func TestVCSLbReportProjectGolden(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/loadbalancers/reports/", 200, "vcs/lb_reports")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "lb", "report")
	require.Equal(t, ExitOK, code, stderr)
	assert.Equal(t, "project=101", f.find("GET", vcsBase+"/loadbalancers/reports/").Query)
	assert.Contains(t, stderr, "Period: 2026-07-28 10:23:34 ~ 2026-08-28 10:23:34（total_lb_num=2）")
	testutil.AssertGolden(t, "vcs_lb_report.txt", []byte(stdout))
}

// TestVCSLbReportProjectBeginEnd 驗證 --begin/--end 以 RFC3339（UTC）字串進 query，
// 且非 RFC3339 時在送出任何請求前就報錯（exit 1）。
func TestVCSLbReportProjectBeginEnd(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/loadbalancers/reports/", 200, "vcs/lb_reports")
	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "lb", "report",
		"--begin", "2026-08-01T00:00:00Z", "--end", "2026-08-28T00:00:00Z")
	require.Equal(t, ExitOK, code, stderr)
	assert.Equal(t, "project=101&begin_time=2026-08-01T00%3A00%3A00Z&end_time=2026-08-28T00%3A00%3A00Z",
		f.find("GET", vcsBase+"/loadbalancers/reports/").Query)

	f2 := newFakeAPI(t)
	code2, _, stderr2 := runCLI(t, f2.env(t), "", "vcs", "lb", "report", "--begin", "not-a-time")
	assert.Equal(t, ExitGeneral, code2, stderr2)
	assert.Contains(t, stderr2, "--begin")
	assert.Nil(t, f2.find("GET", vcsBase+"/loadbalancers/reports/"), "--begin 格式錯誤時不應送出請求")
}

// TestVCSLbReportMemberGolden 驗證帶 <id|name> + --member 時查詢 member 流量報表，
// query 含 member/method/interval/unit，輸出 TIME / BYTES READ 兩欄。
func TestVCSLbReportMemberGolden(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/loadbalancers/", 200, "vcs/loadbalancers")
	f.onFixture("GET", vcsBase+"/loadbalancers/5/reports/", 200, "vcs/lb_member_report")
	code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "lb", "report", "web-lb",
		"--member", "10.0.0.11", "--method", "avg", "--interval", "10", "--unit", "h")
	require.Equal(t, ExitOK, code, stderr)
	assert.Equal(t, "member=10.0.0.11&method=avg&interval=10&unit=h",
		f.find("GET", vcsBase+"/loadbalancers/5/reports/").Query)
	testutil.AssertGolden(t, "vcs_lb_member_report.txt", []byte(stdout))
}

// TestVCSLbReportMemberRequiresFlag 驗證「有 id 無 --member」與「無 id 有 --member」
// 皆在送出任何請求前報錯（exit 1）。
func TestVCSLbReportMemberRequiresFlag(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/loadbalancers/", 200, "vcs/loadbalancers")
	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "lb", "report", "web-lb")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Nil(t, f.find("GET", vcsBase+"/loadbalancers/5/reports/"))

	code2, _, stderr2 := runCLI(t, f.env(t), "", "vcs", "lb", "report", "--member", "10.0.0.11")
	assert.Equal(t, ExitGeneral, code2, stderr2)
	assert.Nil(t, f.find("GET", vcsBase+"/loadbalancers/reports/"))
}

// TestVCSLbReportMemberRejectsBlankValue 驗證 --member 帶空字串或純空白時視為未指定，
// 在送出任何請求前報錯（exit 1）；不能只檢查 --member 有沒有被指定
// （c.Flags().Changed 對 `--member ""` 一樣回傳 true）。
func TestVCSLbReportMemberRejectsBlankValue(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/loadbalancers/", 200, "vcs/loadbalancers")

	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "lb", "report", "web-lb", "--member", "")
	assert.Equal(t, ExitGeneral, code, stderr)
	assert.Nil(t, f.find("GET", vcsBase+"/loadbalancers/5/reports/"))

	code2, _, stderr2 := runCLI(t, f.env(t), "", "vcs", "lb", "report", "web-lb", "--member", "   ")
	assert.Equal(t, ExitGeneral, code2, stderr2)
	assert.Nil(t, f.find("GET", vcsBase+"/loadbalancers/5/reports/"))
}

// TestVCSLbReportMemberIntervalUnitPairing 驗證 member 報表模式下 --interval 與 --unit
// 必須同時指定或同時省略；--interval 指定時必須 > 0。以上皆在送出任何請求前報錯（exit 1）。
func TestVCSLbReportMemberIntervalUnitPairing(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"只有 interval", []string{"vcs", "lb", "report", "web-lb", "--member", "10.0.0.11", "--interval", "5"}},
		{"只有 unit", []string{"vcs", "lb", "report", "web-lb", "--member", "10.0.0.11", "--unit", "h"}},
		{"interval 為 0", []string{"vcs", "lb", "report", "web-lb", "--member", "10.0.0.11", "--interval", "0", "--unit", "h"}},
		{"interval 為負數", []string{"vcs", "lb", "report", "web-lb", "--member", "10.0.0.11", "--interval", "-1", "--unit", "h"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeAPI(t)
			f.onFixture("GET", vcsBase+"/loadbalancers/", 200, "vcs/loadbalancers")
			code, _, stderr := runCLI(t, f.env(t), "", tc.args...)
			assert.Equal(t, ExitGeneral, code, stderr)
			assert.Nil(t, f.find("GET", vcsBase+"/loadbalancers/5/reports/"), "%s 時不應送出請求", tc.name)
		})
	}
}

// TestVCSLbReportMemberIntervalUnitPairingOK 驗證 --interval／--unit 同時指定且 --interval
// > 0 時正常查詢（不應被上面的成對檢查誤擋）。
func TestVCSLbReportMemberIntervalUnitPairingOK(t *testing.T) {
	f := newFakeAPI(t)
	f.onFixture("GET", vcsBase+"/loadbalancers/", 200, "vcs/loadbalancers")
	f.onFixture("GET", vcsBase+"/loadbalancers/5/reports/", 200, "vcs/lb_member_report")
	code, _, stderr := runCLI(t, f.env(t), "", "vcs", "lb", "report", "web-lb",
		"--member", "10.0.0.11", "--interval", "5", "--unit", "h")
	require.Equal(t, ExitOK, code, stderr)
	assert.Equal(t, "member=10.0.0.11&interval=5&unit=h",
		f.find("GET", vcsBase+"/loadbalancers/5/reports/").Query)
}

// TestVCSLbReportProjectRejectsMemberOnlyFlags 驗證不帶 <id|name>（project 報表）時，
// --member／--method／--interval／--unit 任一個被指定都要報錯（exit 1），而不是被靜默
// 忽略——這些 flag 只對「帶 <id|name>」的 member 流量報表有意義。
func TestVCSLbReportProjectRejectsMemberOnlyFlags(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"member", []string{"vcs", "lb", "report", "--member", "10.0.0.11"}},
		{"method", []string{"vcs", "lb", "report", "--method", "avg"}},
		{"interval", []string{"vcs", "lb", "report", "--interval", "10"}},
		{"unit", []string{"vcs", "lb", "report", "--unit", "h"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeAPI(t)
			f.onFixture("GET", vcsBase+"/loadbalancers/reports/", 200, "vcs/lb_reports")
			code, _, stderr := runCLI(t, f.env(t), "", tc.args...)
			assert.Equal(t, ExitGeneral, code, stderr)
			assert.Nil(t, f.find("GET", vcsBase+"/loadbalancers/reports/"), "%s 被指定時不應送出 project 報表請求", tc.name)
		})
	}
}

// TestVCSLbReportProjectEmptyDetail 驗證 project 報表在 Detail.LB 為空陣列，或 API 回應
// 完全省略 LB 這個鍵（對應 genvcs 的 *[]ReportDetailObject 為 nil）兩種情況下都能正常
// 結束：exit 0、stdout 只有表頭（--no-header 時完全空白），stderr 仍印出 total_lb_num=0。
func TestVCSLbReportProjectEmptyDetail(t *testing.T) {
	cases := []struct {
		name    string
		fixture string
	}{
		{"空陣列", "vcs/lb_reports_empty"},
		{"缺省 LB 鍵", "vcs/lb_reports_no_lb_key"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeAPI(t)
			f.onFixture("GET", vcsBase+"/loadbalancers/reports/", 200, tc.fixture)

			code, stdout, stderr := runCLI(t, f.env(t), "", "vcs", "lb", "report")
			require.Equal(t, ExitOK, code, stderr)
			assert.Contains(t, stderr, "total_lb_num=0")
			assert.Equal(t, "ID  NAME  USER  CREATED  DELETED\n", stdout)

			f2 := newFakeAPI(t)
			f2.onFixture("GET", vcsBase+"/loadbalancers/reports/", 200, tc.fixture)
			code2, stdout2, stderr2 := runCLI(t, f2.env(t), "", "vcs", "lb", "report", "--no-header")
			require.Equal(t, ExitOK, code2, stderr2)
			assert.Contains(t, stderr2, "total_lb_num=0")
			assert.Empty(t, stdout2)
		})
	}
}
