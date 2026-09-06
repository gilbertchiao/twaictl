package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	genvcs "github.com/gilbertchiao/twaictl/internal/gen/vcs"
	"github.com/gilbertchiao/twaictl/internal/output"
)

// siteColumns 是 `vcs ls` 的欄位定義。
var siteColumns = []output.Column[genvcs.SiteSerializer]{
	{Name: "ID", Default: true, Value: func(s genvcs.SiteSerializer) string { return fmt.Sprint(s.Id) }},
	{Name: "NAME", Default: true, Value: func(s genvcs.SiteSerializer) string { return s.Name }},
	{Name: "STATUS", Default: true, Value: func(s genvcs.SiteSerializer) string { return string(s.Status) }},
	{Name: "PUBLIC IP", Default: true, Value: func(s genvcs.SiteSerializer) string { return s.PublicIp }},
	{Name: "HOSTNAME", Default: true, Value: firstServerHostname},
	{Name: "CREATED", Default: true, Value: func(s genvcs.SiteSerializer) string { return output.FormatTime(s.CreateTime) }},
	{Name: "SOLUTION", Value: func(s genvcs.SiteSerializer) string { return fmt.Sprint(s.Solution) }},
	{Name: "PROJECT", Value: func(s genvcs.SiteSerializer) string { return fmt.Sprint(s.Project) }},
	{Name: "USER", Value: func(s genvcs.SiteSerializer) string { return s.User.Username }},
	{Name: "PROGRESS", Value: func(s genvcs.SiteSerializer) string { return fmt.Sprint(s.Progress) }},
}

// siteDetailColumns 是 `vcs get` 直式輸出的欄位定義。
var siteDetailColumns = []output.Column[genvcs.SiteSerializer]{
	{Name: "ID", Default: true, Value: func(s genvcs.SiteSerializer) string { return fmt.Sprint(s.Id) }},
	{Name: "Name", Default: true, Value: func(s genvcs.SiteSerializer) string { return s.Name }},
	{Name: "Status", Default: true, Value: func(s genvcs.SiteSerializer) string { return string(s.Status) }},
	{Name: "Public IP", Default: true, Value: func(s genvcs.SiteSerializer) string { return s.PublicIp }},
	{Name: "Solution", Default: true, Value: func(s genvcs.SiteSerializer) string { return fmt.Sprint(s.Solution) }},
	{Name: "Project", Default: true, Value: func(s genvcs.SiteSerializer) string { return fmt.Sprint(s.Project) }},
	{Name: "User", Default: true, Value: func(s genvcs.SiteSerializer) string { return s.User.Username }},
	{Name: "Created", Default: true, Value: func(s genvcs.SiteSerializer) string { return output.FormatTime(s.CreateTime) }},
	{Name: "Servers", Default: true, Value: formatSiteServers},
	{Name: "Progress", Default: true, Value: func(s genvcs.SiteSerializer) string { return fmt.Sprint(s.Progress) }},
	{Name: "Termination protection", Default: true, Value: func(s genvcs.SiteSerializer) string {
		return output.Bool(s.TerminationProtection)
	}},
}

// firstServerHostname 回傳第一台 server 的 hostname；無 server 時回傳空字串。
func firstServerHostname(s genvcs.SiteSerializer) string {
	if s.Servers == nil || len(*s.Servers) == 0 {
		return ""
	}
	return output.Str((*s.Servers)[0].Hostname)
}

// formatSiteServers 把所有 server 格式化為 `hostname(flavor_id,status)`，以 `, ` 串接。
func formatSiteServers(s genvcs.SiteSerializer) string {
	if s.Servers == nil {
		return ""
	}
	parts := make([]string, 0, len(*s.Servers))
	for _, srv := range *s.Servers {
		hostname := output.Str(srv.Hostname)
		flavorID := output.Int64(srv.FlavorId)
		status := ""
		if srv.Status != nil {
			status = string(*srv.Status)
		}
		parts = append(parts, fmt.Sprintf("%s(%s,%s)", hostname, flavorID, status))
	}
	return strings.Join(parts, ", ")
}

// joinIDs 把 id 列表以 `, ` 串接，供確認訊息使用。
func joinIDs(ids []int64) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatInt(id, 10)
	}
	return strings.Join(parts, ", ")
}

// newVCSCmd 是 `vcs` 父命令；子命令分散於 vcs_*.go 各檔（例如 vcs_flavor.go、vcs_image.go、
// vcs_network.go、vcs_volume.go、vcs_secg.go 等）。
func newVCSCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{Use: "vcs", Short: "管理 VCS 虛擬機實例"}
	cmd.AddCommand(newVCSLsCmd(a), newVCSGetCmd(a), newVCSCreateCmd(a), newVCSRmCmd(a), newVCSFlavorCmd(a), newVCSImageCmd(a), newVCSKeypairCmd(a),
		newVCSServerCmd(a), newVCSActionCmd(a), newVCSEventsCmd(a), newVCSMetricsCmd(a), newVCSNetworkCmd(a), newVCSEipCmd(a),
		newVCSVolumeCmd(a), newVCSSnapshotCmd(a), newVCSSecgCmd(a), newVCSSecretCmd(a), newVCSLbCmd(a), newVCSFirewallCmd(a), newVCSAspCmd(a))
	return cmd
}

// newVCSLsCmd 建立 `vcs ls`。
func newVCSLsCmd(a *app) *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "列出 VCS 實例",
		Long:  "列出 VCS 實例。\n\n" + columnsHelp(siteColumns),
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if err := validateColumns(a, siteColumns); err != nil {
				return err
			}
			ctx := a.commandContext(c)
			svc, err := a.vcsService()
			if err != nil {
				return err
			}
			projectID, err := a.projectID(ctx)
			if err != nil {
				return err
			}
			res, err := svc.ListSites(ctx, projectID, all)
			if err != nil {
				return err
			}
			return render(a, res.Raw, res.Value, siteColumns)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "列出所有使用者的 VCS 實例（all_users=1）")
	return cmd
}

// newVCSGetCmd 建立 `vcs get`。
func newVCSGetCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id|name>",
		Short: "顯示單一 VCS 實例的詳細資料",
		Long:  "顯示單一 VCS 實例的詳細資料。\n\n" + columnsHelp(siteDetailColumns),
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if err := validateColumns(a, siteDetailColumns); err != nil {
				return err
			}
			ctx := a.commandContext(c)
			svc, err := a.vcsService()
			if err != nil {
				return err
			}
			projectID, err := a.projectID(ctx)
			if err != nil {
				return err
			}
			id, err := svc.ResolveSiteID(ctx, projectID, args[0])
			if err != nil {
				return err
			}
			res, err := svc.GetSite(ctx, id)
			if err != nil {
				return err
			}
			return renderOne(a, res.Raw, res.Value, siteDetailColumns)
		},
	}
}

// newVCSRmCmd 建立 `vcs rm`：破壞性操作，需確認或 -y。
func newVCSRmCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "rm <id|name>...",
		Short: "刪除 VCS 實例（破壞性操作，需確認或 -y）",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			ctx := a.commandContext(c)
			svc, err := a.vcsService()
			if err != nil {
				return err
			}
			projectID, err := a.projectID(ctx)
			if err != nil {
				return err
			}
			ids := make([]int64, 0, len(args))
			for _, ref := range args {
				id, err := svc.ResolveSiteID(ctx, projectID, ref)
				if err != nil {
					return err
				}
				ids = append(ids, id)
			}
			if err := a.confirm(fmt.Sprintf("刪除 VCS 實例 %s", joinIDs(ids))); err != nil {
				return err
			}
			for _, id := range ids {
				if err := svc.DeleteSite(ctx, id); err != nil {
					return err
				}
				a.progress("已送出刪除請求：%d", id)
			}
			if !a.opts.wait {
				return nil
			}
			waitCtx, cancel := a.waitContext(ctx)
			defer cancel()
			for _, id := range ids {
				err := svc.WaitSiteDeleted(waitCtx, id, a.opts.waitInterval, func(st string) {
					a.progress("VCS 實例 %d 狀態：%s", id, st)
				})
				if err != nil {
					// -o json/yaml 會讓上面的 progress 靜音，此時這行錯誤訊息是唯一能
					// 得知是哪個實例尚未刪除完成的線索。
					return fmt.Errorf("VCS 實例 %d 尚未刪除完成: %w", id, err)
				}
			}
			return nil
		},
	}
}
