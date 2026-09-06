package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	genvcs "github.com/gilbertchiao/twaictl/internal/gen/vcs"
	"github.com/gilbertchiao/twaictl/internal/output"
	"github.com/gilbertchiao/twaictl/internal/twai/vcs"
)

// lbReportMethods 是 `vcs lb report --method` 的合法值
// （GetLoadbalancersLoadbalancerIdReportsParamsMethod 的 enum）。
var lbReportMethods = []string{"sum", "avg"}

// lbReportUnits 是 `vcs lb report --unit` 的合法值
// （GetLoadbalancersLoadbalancerIdReportsParamsUnit 的 enum；大小寫皆有意義，
// m＝分鐘、M＝月）。
var lbReportUnits = []string{"s", "m", "h", "d", "w", "M", "y"}

// lbReportRow 是 `vcs lb report`（不帶 <id|name>，project 報表）攤平後的單一列：
// 由 LBReportSerializer.Projects[].Detail.LB 攤平而來，不分屬於哪個 project
// （目前一個 project code 只會解析出一個 project id，Projects 實務上只有一筆）。
type lbReportRow struct {
	ID, Name, User, Created, Deleted string
}

// lbReportColumns 是 `vcs lb report`（project 報表）的欄位定義。
var lbReportColumns = []output.Column[lbReportRow]{
	{Name: "ID", Default: true, Value: func(r lbReportRow) string { return r.ID }},
	{Name: "NAME", Default: true, Value: func(r lbReportRow) string { return r.Name }},
	{Name: "USER", Default: true, Value: func(r lbReportRow) string { return r.User }},
	{Name: "CREATED", Default: true, Value: func(r lbReportRow) string { return r.Created }},
	{Name: "DELETED", Default: true, Value: func(r lbReportRow) string { return r.Deleted }},
}

// flattenLbReport 把 LBReportSerializer.Projects[].Detail.LB 攤平成 lbReportRow 清單。
// DeleteTime 若為零值（API 回應 delete_time 為 null，通常代表 LB 仍在使用中，
// 見 loadbalancers.go 的 LoadBalancerReport 說明），output.FormatTime 會顯示為空字串。
func flattenLbReport(v genvcs.LBReportSerializer) []lbReportRow {
	var rows []lbReportRow
	for _, p := range v.Projects {
		if p.Detail.LB == nil {
			continue
		}
		for _, lb := range *p.Detail.LB {
			var username string
			if lb.User.User != nil {
				username = lb.User.User.Username
			}
			rows = append(rows, lbReportRow{
				ID:      fmt.Sprint(lb.Id),
				Name:    lb.Name,
				User:    username,
				Created: output.FormatTime(lb.CreateTime),
				Deleted: output.FormatTime(lb.DeleteTime),
			})
		}
	}
	return rows
}

// lbTotalLbNum 加總 Projects[].Summery.TotalLbNum，供 report 的 Period 進度訊息使用。
func lbTotalLbNum(v genvcs.LBReportSerializer) int64 {
	var total int64
	for _, p := range v.Projects {
		if p.Summery.TotalLbNum != nil {
			total += *p.Summery.TotalLbNum
		}
	}
	return total
}

// lbMemberReportColumns 是 `vcs lb report <id|name> --member <ip>`（member 流量報表）的
// 欄位定義：BYTES READ 是預設欄位（以二進位單位顯示），RAW BYTES 是非預設欄位
// （顯示原始位元組數，供需要精確數字或給其他工具解析的情境使用）。
var lbMemberReportColumns = []output.Column[vcs.LoadBalancerMemberReportPoint]{
	{Name: "TIME", Default: true, Value: func(p vcs.LoadBalancerMemberReportPoint) string { return output.FormatTimeString(p.Time) }},
	{Name: "BYTES READ", Default: true, Value: func(p vcs.LoadBalancerMemberReportPoint) string { return output.Bytes(p.BytesRead) }},
	{Name: "RAW BYTES", Value: func(p vcs.LoadBalancerMemberReportPoint) string { return fmt.Sprint(p.BytesRead) }},
}

// parseReportTimeFlag 解析 `--begin`／`--end` 的 RFC3339 字串；空字串代表未指定，
// 回傳 nil（讓 API 套用預設值：end=now、begin=一個月前）。
func parseReportTimeFlag(flagName, value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, fmt.Errorf("%s 必須是 RFC3339（例如 2026-08-27T12:00:00Z）: %w", flagName, err)
	}
	utc := parsed.UTC()
	return &utc, nil
}

// newVCSLbReportCmd 建立 `vcs lb report`：不帶 <id|name> 時查詢 project 下所有 load
// balancer 的建立／刪除報表；帶 <id|name> 時必須搭配 --member，查詢該 member 的流量報表。
func newVCSLbReportCmd(a *app) *cobra.Command {
	var member, begin, end, method, unit string
	var interval int32
	cmd := &cobra.Command{
		Use:   "report [<id|name>] [--member <ip>] [--begin <time>] [--end <time>]",
		Short: "查詢 load balancer 的建立／刪除報表（project）或 member 流量報表",
		Long: "查詢 load balancer 報表，依是否指定 <id|name> 分成兩種：\n\n" +
			"不帶 <id|name>：查詢目前 project 下所有 load balancer 的建立／刪除報表，\n" +
			"期間與 total_lb_num 會印在 stderr（進度訊息）：\n" + columnsHelp(lbReportColumns) + "\n\n" +
			"帶 <id|name>：查詢該 load balancer 底下指定 pool member（以 IP 表示）的流量報表，\n" +
			"必須搭配 --member：\n" + columnsHelp(lbMemberReportColumns) + "\n\n" +
			"--interval／--unit 必須同時指定或同時省略，--interval 指定時須大於 0。\n\n" +
			"--begin/--end 請用 RFC3339 格式（例如 2026-08-27T12:00:00Z）；" +
			"未指定時 API 預設 end=now、begin=一個月前。\n\n" +
			"注意：API 對不存在的 load balancer 查詢 member 報表會回 503" +
			"（實測，非本工具造成），twaictl 照實回報錯誤（exit 3）。",
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			// --member／--method／--interval／--unit 只對「帶 <id|name>」的 member 流量報表
			// 有意義；project 報表（不帶 <id|name>）任一個都不可指定，避免使用者誤以為這些
			// flag 對 project 報表也有效，卻被靜默忽略（見 memberOnlyFlags 的檢查）。
			memberOnlyFlags := []string{"member", "method", "interval", "unit"}
			memberFlagChanged := false
			for _, name := range memberOnlyFlags {
				if c.Flags().Changed(name) {
					memberFlagChanged = true
					break
				}
			}

			if len(args) == 0 {
				if memberFlagChanged {
					return fmt.Errorf("--member、--method、--interval、--unit 僅供查詢單一 load balancer 的 member 流量報表使用" +
						"（需搭配 <id|name>），查詢 project 報表時不可指定")
				}
				if err := validateColumns(a, lbReportColumns); err != nil {
					return err
				}
			} else {
				// 用 requireFlag（非空白檢查）而不是 c.Flags().Changed("member")：
				// 後者只確認 --member 有沒有被指定，`--member ""` 或純空白一樣視為
				// 「已指定」而通過檢查，導致以空 member 送出請求。
				if err := requireFlag("--member", member); err != nil {
					return err
				}
				if err := validateColumns(a, lbMemberReportColumns); err != nil {
					return err
				}
				if err := validateEnumList("--method", method, lbReportMethods); err != nil {
					return err
				}
				if err := validateEnumList("--unit", unit, lbReportUnits); err != nil {
					return err
				}
				// --interval 與 --unit 必須成對出現：API 的區間長度需要單位才有意義，
				// 只指定其中一個會讓另一個被靜默忽略（interval 沒有 unit 不知道單位，
				// unit 沒有 interval 則沒有長度可套用）。
				intervalChanged := c.Flags().Changed("interval")
				unitChanged := c.Flags().Changed("unit")
				if intervalChanged != unitChanged {
					return fmt.Errorf("--interval 與 --unit 必須一起指定")
				}
				if intervalChanged && interval <= 0 {
					return fmt.Errorf("--interval 必須大於 0（目前 %d）", interval)
				}
			}

			beginTime, err := parseReportTimeFlag("--begin", begin)
			if err != nil {
				return err
			}
			endTime, err := parseReportTimeFlag("--end", end)
			if err != nil {
				return err
			}

			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}

			if len(args) == 0 {
				res, err := svc.LoadBalancerReport(ctx, projectID, vcs.LoadBalancerReportParams{Begin: beginTime, End: endTime})
				if err != nil {
					return err
				}
				a.progress("Period: %s ~ %s（total_lb_num=%d）",
					output.FormatTime(res.Value.BeginTime), output.FormatTime(res.Value.EndTime), lbTotalLbNum(res.Value))
				return render(a, res.Raw, flattenLbReport(res.Value), lbReportColumns)
			}

			id, err := svc.ResolveLoadBalancerID(ctx, projectID, args[0])
			if err != nil {
				return err
			}
			res, err := svc.LoadBalancerMemberReport(ctx, id, vcs.LoadBalancerMemberReportParams{
				Member: member, Begin: beginTime, End: endTime, Method: method, Interval: interval, Unit: unit,
			})
			if err != nil {
				return err
			}
			return render(a, res.Raw, res.Value, lbMemberReportColumns)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&member, "member", "", "pool member 的 ip（帶 <id|name> 時必填）")
	flags.StringVar(&begin, "begin", "", "查詢開始時間（RFC3339，例如 2026-08-27T12:00:00Z）")
	flags.StringVar(&end, "end", "", "查詢結束時間（RFC3339，例如 2026-08-27T12:00:00Z）")
	flags.StringVar(&method, "method", "", "計算方式（僅 member 報表）：sum|avg")
	flags.Int32Var(&interval, "interval", 0, "計算區間長度（僅 member 報表，須大於 0，需搭配 --unit）")
	flags.StringVar(&unit, "unit", "", "計算區間單位（僅 member 報表，需搭配 --interval）：s|m|h|d|w|M|y")
	return cmd
}
