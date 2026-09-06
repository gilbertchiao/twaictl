package cli

import (
	"strconv"

	"github.com/spf13/cobra"

	"github.com/gilbertchiao/twaictl/internal/output"
	"github.com/gilbertchiao/twaictl/internal/twai/vcs"
)

// quotaResourceLabel 把 QuotaRow.Resource 轉成顯示文字；memory 額外標註單位（MB）。
func quotaResourceLabel(resource string) string {
	if resource == "memory" {
		return "memory (MB)"
	}
	return resource
}

// formatQuotaUsage 把用量數值格式化為顯示文字（不做 unlimited 特判，usage 不會是 -1）。
func formatQuotaUsage(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// formatQuotaLimit 把配額數值格式化為顯示文字；-1 依 API 慣例代表無上限，顯示 unlimited。
func formatQuotaLimit(v float64) string {
	if v < 0 {
		return "unlimited"
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// quotaColumns 是 `project quota`（未指定 --user）的欄位定義。
var quotaColumns = []output.Column[vcs.QuotaRow]{
	{Name: "RESOURCE", Default: true, Value: func(r vcs.QuotaRow) string { return quotaResourceLabel(r.Resource) }},
	{Name: "USAGE", Default: true, Value: func(r vcs.QuotaRow) string { return formatQuotaUsage(r.Usage) }},
	{Name: "QUOTA", Default: true, Value: func(r vcs.QuotaRow) string { return formatQuotaLimit(r.Quota) }},
}

// userQuotaRow 是 `project quota --user` 攤平後的顯示列：每位使用者、每個資源一列。
type userQuotaRow struct {
	User string
	vcs.QuotaRow
}

// userQuotaColumns 是 `project quota --user` 的欄位定義。
var userQuotaColumns = []output.Column[userQuotaRow]{
	{Name: "USER", Default: true, Value: func(r userQuotaRow) string { return r.User }},
	{Name: "RESOURCE", Default: true, Value: func(r userQuotaRow) string { return quotaResourceLabel(r.Resource) }},
	{Name: "USAGE", Default: true, Value: func(r userQuotaRow) string { return formatQuotaUsage(r.Usage) }},
	{Name: "QUOTA", Default: true, Value: func(r userQuotaRow) string { return formatQuotaLimit(r.Quota) }},
}

// solutionRow 是 `project solutions` 攤平後的顯示列：id 對照名稱，找不到時 Name 留空。
type solutionRow struct {
	ID   int
	Name string
}

// solutionColumns 是 `project solutions` 的欄位定義。
var solutionColumns = []output.Column[solutionRow]{
	{Name: "ID", Default: true, Value: func(r solutionRow) string { return strconv.Itoa(r.ID) }},
	{Name: "NAME", Default: true, Value: func(r solutionRow) string { return r.Name }},
}

// newProjectQuotaCmd 建立 `project quota`。
func newProjectQuotaCmd(a *app) *cobra.Command {
	var userMode bool
	cmd := &cobra.Command{
		Use:   "quota",
		Short: "查詢 project 配額",
		Long: "查詢 project 配額。\n\n" + columnsHelp(quotaColumns) +
			"\n\n--user 時改列出每位使用者的配額，欄位為 USER、RESOURCE、USAGE、QUOTA。",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if userMode {
				if err := validateColumns(a, userQuotaColumns); err != nil {
					return err
				}
			} else if err := validateColumns(a, quotaColumns); err != nil {
				return err
			}
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			if userMode {
				res, err := svc.UserQuotas(ctx, projectID)
				if err != nil {
					return err
				}
				var rows []userQuotaRow
				for _, u := range res.Value {
					for _, row := range vcs.QuotaRows(u.Cpu, u.Gpu, u.Memory, u.FloatingIp, u.StaticIp) {
						rows = append(rows, userQuotaRow{User: u.User.Username, QuotaRow: row})
					}
				}
				return render(a, res.Raw, rows, userQuotaColumns)
			}
			res, err := svc.ProjectQuota(ctx, projectID)
			if err != nil {
				return err
			}
			rows := vcs.QuotaRows(res.Value.Cpu, res.Value.Gpu, res.Value.Memory, res.Value.FloatingIp, res.Value.StaticIp)
			return render(a, res.Raw, rows, quotaColumns)
		},
	}
	cmd.Flags().BoolVar(&userMode, "user", false, "列出每位使用者的配額（而非整個 project）")
	return cmd
}

// newProjectSolutionsCmd 建立 `project solutions`：id 來自 ProjectSolutionIDs，
// 名稱來自 ListSolutions（Common /solutions/）對照，找不到對應名稱時留空。
func newProjectSolutionsCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "solutions",
		Short: "列出 project 已啟用的 solution",
		Long:  "列出 project 已啟用的 solution。\n\n" + columnsHelp(solutionColumns),
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if err := validateColumns(a, solutionColumns); err != nil {
				return err
			}
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			idsRes, err := svc.ProjectSolutionIDs(ctx, projectID)
			if err != nil {
				return err
			}
			// json/yaml 直接輸出 API 原始回應（id 清單），不需要用 ListSolutions 對照名稱；
			// 只有 table 模式才查 Common，避免 Common 暫時失敗連累已經取得的 id 清單。
			if output.Format(a.opts.output) != output.FormatTable {
				return render(a, idsRes.Raw, []solutionRow(nil), solutionColumns)
			}
			solutions, err := svc.ListSolutions(ctx, projectID)
			if err != nil {
				return err
			}
			names := make(map[int]string, len(solutions))
			for _, sol := range solutions {
				names[int(sol.Id)] = sol.Name
			}
			rows := make([]solutionRow, len(idsRes.Value))
			for i, id := range idsRes.Value {
				rows[i] = solutionRow{ID: id, Name: names[id]}
			}
			return render(a, idsRes.Raw, rows, solutionColumns)
		},
	}
}
