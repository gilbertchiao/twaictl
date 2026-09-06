package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	genvcs "github.com/gilbertchiao/twaictl/internal/gen/vcs"
	"github.com/gilbertchiao/twaictl/internal/output"
)

// flavorColumns 是 `vcs flavor ls` 的欄位定義。
//
// MEMORY (MB) / DISK (GB)：已於 2026-08-27 以真實 API 實測確認單位
// （16384 對應 16 GB 記憶體、磁碟數值以 GB 為單位），詳見 docs/api-notes.md。
//
// GPU：spec 原標示 `resource.gpu` 為 `integer`，但真實 API 回傳浮點值
// （例如 0.25、0.5），已透過 patch 修正為 `number`（見
// api/openapi/patches/VCS-flavor-gpu-number.patch），故此處改用 output.Float。
var flavorColumns = []output.Column[genvcs.FlavorSerializer]{
	{Name: "ID", Default: true, Value: func(f genvcs.FlavorSerializer) string { return fmt.Sprint(f.Id) }},
	{Name: "NAME", Default: true, Value: func(f genvcs.FlavorSerializer) string { return f.Name }},
	{Name: "CPU", Default: true, Value: func(f genvcs.FlavorSerializer) string { return output.Int64(f.Resource.Cpu) }},
	{Name: "MEMORY (MB)", Default: true, Value: func(f genvcs.FlavorSerializer) string { return output.Int64(f.Resource.Memory) }},
	{Name: "DISK (GB)", Default: true, Value: func(f genvcs.FlavorSerializer) string { return output.Int64(f.Resource.Disk) }},
	{Name: "GPU", Default: true, Value: func(f genvcs.FlavorSerializer) string { return output.Float(f.Resource.Gpu) }},
	{Name: "GPU TYPE", Default: true, Value: func(f genvcs.FlavorSerializer) string { return output.Str(f.Metadata.GpuType) }},
	{Name: "PUBLIC", Default: true, Value: func(f genvcs.FlavorSerializer) string { return boolPtrValue(f.IsPublic) }},
	{Name: "ENABLED", Value: func(f genvcs.FlavorSerializer) string { return boolPtrValue(f.IsEnabled) }},
	{Name: "PLATFORM", Value: func(f genvcs.FlavorSerializer) string { return f.Platform }},
}

// boolPtrValue 把 *bool 轉成顯示文字，nil 為空字串（與 output.Str / output.Int64 的慣例一致）；
// 目前只有 flavorColumns 的 PUBLIC／ENABLED 欄位在用（*bool 是唯一有這種需求的欄位型別）。
func boolPtrValue(b *bool) string {
	if b == nil {
		return ""
	}
	return output.Bool(*b)
}

// newVCSFlavorCmd 建立 `vcs flavor` 子命令。
func newVCSFlavorCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{Use: "flavor", Short: "查詢 VCS flavor（虛擬機規格）"}
	var allProjects bool
	lsCmd := &cobra.Command{
		Use:   "ls",
		Short: "列出 flavor",
		Long:  "列出 flavor。\n\n" + columnsHelp(flavorColumns),
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if err := validateColumns(a, flavorColumns); err != nil {
				return err
			}
			ctx := a.commandContext(c)
			svc, err := a.vcsService()
			if err != nil {
				return err
			}
			var projectID *int64
			if !allProjects {
				id, err := a.projectID(ctx)
				if err != nil {
					return err
				}
				projectID = &id
			}
			res, err := svc.ListFlavors(ctx, projectID)
			if err != nil {
				return err
			}
			return render(a, res.Raw, res.Value, flavorColumns)
		},
	}
	lsCmd.Flags().BoolVar(&allProjects, "all-projects", false, "列出所有 project 可用的 flavor（不以 project 過濾）")
	cmd.AddCommand(lsCmd)
	return cmd
}
