package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	genvcs "github.com/gilbertchiao/twaictl/internal/gen/vcs"
	"github.com/gilbertchiao/twaictl/internal/output"
)

// ipType 把 *IPSerializerType 轉成顯示文字，nil 為空字串。
func ipType(t *genvcs.IPSerializerType) string {
	if t == nil {
		return ""
	}
	return string(*t)
}

// ipResource 把 occupied_resource 格式化為 `type/id`；type、id 皆為指標，
// 任一為 nil（含整個 occupied_resource 為 nil）都顯示為空字串。
func ipResource(r *genvcs.OccupiedResourceSerializer) string {
	if r == nil || r.Type == nil || r.Id == nil {
		return ""
	}
	return fmt.Sprintf("%s/%s", *r.Type, *r.Id)
}

// eipColumns 是 `vcs eip ls` 的欄位定義。
var eipColumns = []output.Column[genvcs.IPSerializer]{
	{Name: "ID", Default: true, Value: func(i genvcs.IPSerializer) string { return output.Int64(i.Id) }},
	{Name: "ADDRESS", Default: true, Value: func(i genvcs.IPSerializer) string { return i.Address }},
	{Name: "TYPE", Default: true, Value: func(i genvcs.IPSerializer) string { return ipType(i.Type) }},
	{Name: "STATUS", Default: true, Value: func(i genvcs.IPSerializer) string { return output.Str(i.Status) }},
	{Name: "RESOURCE", Default: true, Value: func(i genvcs.IPSerializer) string { return ipResource(i.OccupiedResource) }},
	{Name: "DESC", Default: true, Value: func(i genvcs.IPSerializer) string { return output.Str(i.Desc) }},
	{Name: "CREATED", Default: true, Value: func(i genvcs.IPSerializer) string { return output.TimePtr(i.CreateTime) }},
}

// eipDetailColumns 是 `vcs eip get`／`create`／`update` 直式輸出的欄位定義。
var eipDetailColumns = []output.Column[genvcs.IPSerializer]{
	{Name: "ID", Default: true, Value: func(i genvcs.IPSerializer) string { return output.Int64(i.Id) }},
	{Name: "Address", Default: true, Value: func(i genvcs.IPSerializer) string { return i.Address }},
	{Name: "Type", Default: true, Value: func(i genvcs.IPSerializer) string { return ipType(i.Type) }},
	{Name: "Status", Default: true, Value: func(i genvcs.IPSerializer) string { return output.Str(i.Status) }},
	{Name: "Status reason", Default: true, Value: func(i genvcs.IPSerializer) string { return output.Str(i.StatusReason) }},
	{Name: "Resource", Default: true, Value: func(i genvcs.IPSerializer) string { return ipResource(i.OccupiedResource) }},
	{Name: "Desc", Default: true, Value: func(i genvcs.IPSerializer) string { return output.Str(i.Desc) }},
	{Name: "Created", Default: true, Value: func(i genvcs.IPSerializer) string { return output.TimePtr(i.CreateTime) }},
}

// newVCSEipCmd 建立 `vcs eip` 子命令。
func newVCSEipCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{Use: "eip", Short: "管理 VCS 浮動 IP（Elastic IP）"}
	cmd.AddCommand(newVCSEipLsCmd(a), newVCSEipGetCmd(a), newVCSEipCreateCmd(a), newVCSEipUpdateCmd(a), newVCSEipRmCmd(a))
	return cmd
}

// newVCSEipLsCmd 建立 `vcs eip ls`。
func newVCSEipLsCmd(a *app) *cobra.Command {
	var address string
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "列出浮動 IP",
		Long:  "列出浮動 IP。\n\n" + columnsHelp(eipColumns),
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if err := validateColumns(a, eipColumns); err != nil {
				return err
			}
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			res, err := svc.ListIPs(ctx, projectID, address)
			if err != nil {
				return err
			}
			return render(a, res.Raw, res.Value, eipColumns)
		},
	}
	cmd.Flags().StringVar(&address, "address", "", "以 IP 位址篩選")
	return cmd
}

// newVCSEipGetCmd 建立 `vcs eip get`。
func newVCSEipGetCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id|address>",
		Short: "顯示單一浮動 IP 的詳細資料",
		Long:  "顯示單一浮動 IP 的詳細資料。\n\n" + columnsHelp(eipDetailColumns),
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if err := validateColumns(a, eipDetailColumns); err != nil {
				return err
			}
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			id, err := svc.ResolveIPID(ctx, projectID, args[0])
			if err != nil {
				return err
			}
			res, err := svc.GetIP(ctx, id)
			if err != nil {
				return err
			}
			return renderOne(a, res.Raw, res.Value, eipDetailColumns)
		},
	}
}

// newVCSEipCreateCmd 建立 `vcs eip create`：申請新的 static 浮動 IP，無參數。
func newVCSEipCreateCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "create",
		Short: "申請新的浮動 IP",
		Long: "申請新的浮動 IP（STATIC EIP）。\n\n" +
			"注意：僅 tenant admin 可用，一般使用者呼叫可能得到 403。\n\n" + columnsHelp(eipDetailColumns),
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if err := validateColumns(a, eipDetailColumns); err != nil {
				return err
			}
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			res, err := svc.CreateIP(ctx, projectID)
			if err != nil {
				return err
			}
			a.progress("已建立 IP %s", output.Int64(res.Value.Id))
			return renderOne(a, res.Raw, res.Value, eipDetailColumns)
		},
	}
}

// newVCSEipUpdateCmd 建立 `vcs eip update`：目前只能更新 desc。
func newVCSEipUpdateCmd(a *app) *cobra.Command {
	var desc string
	cmd := &cobra.Command{
		Use:   "update <id|address> --desc <text>",
		Short: "更新浮動 IP 的描述",
		Long:  "更新浮動 IP 的描述。\n\n" + columnsHelp(eipDetailColumns),
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if err := validateColumns(a, eipDetailColumns); err != nil {
				return err
			}
			// 不用 requireFlag：desc 允許明確指定為空字串（清除描述），
			// 只有完全沒帶 --desc 才算缺參數。
			if !c.Flags().Changed("desc") {
				return fmt.Errorf("請指定 --desc")
			}
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			id, err := svc.ResolveIPID(ctx, projectID, args[0])
			if err != nil {
				return err
			}
			res, err := svc.UpdateIPDesc(ctx, id, desc)
			if err != nil {
				return err
			}
			a.progress("已更新 IP %d", id)
			return renderOne(a, res.Raw, res.Value, eipDetailColumns)
		},
	}
	cmd.Flags().StringVar(&desc, "desc", "", "描述（必填；可為空字串以清除描述）")
	return cmd
}

// newVCSEipRmCmd 建立 `vcs eip rm`：破壞性操作，需確認或 -y。
func newVCSEipRmCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "rm <id|address>...",
		Short: "刪除浮動 IP（破壞性操作，需確認或 -y）",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			ids := make([]int64, 0, len(args))
			for _, ref := range args {
				id, err := svc.ResolveIPID(ctx, projectID, ref)
				if err != nil {
					return err
				}
				ids = append(ids, id)
			}
			if err := a.confirm(fmt.Sprintf("刪除 IP %s", joinIDs(ids))); err != nil {
				return err
			}
			for _, id := range ids {
				if err := svc.DeleteIP(ctx, id); err != nil {
					return err
				}
				a.progress("已刪除 IP %d", id)
			}
			return nil
		},
	}
}
