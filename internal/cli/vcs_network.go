package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	genvcs "github.com/gilbertchiao/twaictl/internal/gen/vcs"
	"github.com/gilbertchiao/twaictl/internal/output"
	"github.com/gilbertchiao/twaictl/internal/twai/vcs"
)

// networkNameservers 把 nameservers 以 `, ` 串接，供 `network ls`／`network get` 共用。
func networkNameservers(ns []string) string {
	return strings.Join(ns, ", ")
}

// networkColumns 是 `vcs network ls` 的欄位定義。
var networkColumns = []output.Column[genvcs.NetworkSerializer]{
	{Name: "ID", Default: true, Value: func(n genvcs.NetworkSerializer) string { return fmt.Sprint(n.Id) }},
	{Name: "NAME", Default: true, Value: func(n genvcs.NetworkSerializer) string { return n.Name }},
	{Name: "CIDR", Default: true, Value: func(n genvcs.NetworkSerializer) string { return n.Cidr }},
	{Name: "GATEWAY", Default: true, Value: func(n genvcs.NetworkSerializer) string { return n.Gateway }},
	{Name: "STATUS", Default: true, Value: func(n genvcs.NetworkSerializer) string { return n.Status }},
	{Name: "ROUTER", Default: true, Value: func(n genvcs.NetworkSerializer) string { return output.Bool(n.WithRouter) }},
	{Name: "CREATED", Default: true, Value: func(n genvcs.NetworkSerializer) string { return output.FormatTime(n.CreateTime) }},
	{Name: "NAMESERVERS", Value: func(n genvcs.NetworkSerializer) string { return networkNameservers(n.Nameservers) }},
	{Name: "EXT NET", Value: func(n genvcs.NetworkSerializer) string { return n.ExtNet }},
	{Name: "DNS DOMAIN", Value: func(n genvcs.NetworkSerializer) string { return output.Str(n.DnsDomain) }},
}

// networkFirewallName 回傳網路關聯防火牆的名稱；沒有防火牆時回傳空字串。
func networkFirewallName(n genvcs.NetworkDetailSerializer) string {
	if n.Firewall == nil {
		return ""
	}
	return n.Firewall.Name
}

// networkDetailColumns 是 `vcs network get` 直式輸出的欄位定義。
var networkDetailColumns = []output.Column[genvcs.NetworkDetailSerializer]{
	{Name: "ID", Default: true, Value: func(n genvcs.NetworkDetailSerializer) string { return fmt.Sprint(n.Id) }},
	{Name: "Name", Default: true, Value: func(n genvcs.NetworkDetailSerializer) string { return n.Name }},
	{Name: "CIDR", Default: true, Value: func(n genvcs.NetworkDetailSerializer) string { return n.Cidr }},
	{Name: "Gateway", Default: true, Value: func(n genvcs.NetworkDetailSerializer) string { return n.Gateway }},
	{Name: "Status", Default: true, Value: func(n genvcs.NetworkDetailSerializer) string { return n.Status }},
	{Name: "Router", Default: true, Value: func(n genvcs.NetworkDetailSerializer) string { return output.Bool(n.WithRouter) }},
	{Name: "Nameservers", Default: true, Value: func(n genvcs.NetworkDetailSerializer) string { return networkNameservers(n.Nameservers) }},
	{Name: "Ext net", Default: true, Value: func(n genvcs.NetworkDetailSerializer) string { return n.ExtNet }},
	{Name: "Firewall", Default: true, Value: networkFirewallName},
	{Name: "Created", Default: true, Value: func(n genvcs.NetworkDetailSerializer) string { return output.FormatTime(n.CreateTime) }},
}

// newVCSNetworkCmd 建立 `vcs network` 子命令。
func newVCSNetworkCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{Use: "network", Short: "管理 VCS 網路"}
	cmd.AddCommand(newVCSNetworkLsCmd(a), newVCSNetworkGetCmd(a), newVCSNetworkCreateCmd(a), newVCSNetworkRmCmd(a))
	return cmd
}

// newVCSNetworkLsCmd 建立 `vcs network ls`。
func newVCSNetworkLsCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "列出網路",
		Long:  "列出網路。\n\n" + columnsHelp(networkColumns),
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if err := validateColumns(a, networkColumns); err != nil {
				return err
			}
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			res, err := svc.ListNetworks(ctx, projectID)
			if err != nil {
				return err
			}
			return render(a, res.Raw, res.Value, networkColumns)
		},
	}
}

// newVCSNetworkGetCmd 建立 `vcs network get`。
func newVCSNetworkGetCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id|name>",
		Short: "顯示單一網路的詳細資料",
		Long:  "顯示單一網路的詳細資料。\n\n" + columnsHelp(networkDetailColumns),
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if err := validateColumns(a, networkDetailColumns); err != nil {
				return err
			}
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			id, err := svc.ResolveNetworkID(ctx, projectID, args[0])
			if err != nil {
				return err
			}
			res, err := svc.GetNetwork(ctx, id)
			if err != nil {
				return err
			}
			return renderOne(a, res.Raw, res.Value, networkDetailColumns)
		},
	}
}

// newVCSNetworkCreateCmd 建立 `vcs network create`。
func newVCSNetworkCreateCmd(a *app) *cobra.Command {
	var name, cidr, gateway, dnsDomain string
	var withRouter bool
	cmd := &cobra.Command{
		Use:   "create --name <n> --cidr <cidr>",
		Short: "建立網路",
		Long: "建立網路。\n\n" +
			"注意：僅 tenant admin 可用，一般使用者呼叫可能得到 403。\n\n" + columnsHelp(networkColumns),
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if err := validateColumns(a, networkColumns); err != nil {
				return err
			}
			if err := requireFlag("--name", name); err != nil {
				return err
			}
			if err := requireFlag("--cidr", cidr); err != nil {
				return err
			}
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			res, err := svc.CreateNetwork(ctx, vcs.CreateNetworkInput{
				ProjectID: projectID, Name: name, CIDR: cidr, Gateway: gateway, DNSDomain: dnsDomain, WithRouter: withRouter,
			})
			if err != nil {
				return err
			}
			a.progress("已建立網路 %d", res.Value.Id)
			return renderOne(a, res.Raw, res.Value, networkColumns)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&name, "name", "", "網路名稱（必填）")
	flags.StringVar(&cidr, "cidr", "", "CIDR（必填，例如 10.0.0.0/24）")
	flags.StringVar(&gateway, "gateway", "", "gateway IP")
	flags.BoolVar(&withRouter, "router", false, "是否建立路由器")
	flags.StringVar(&dnsDomain, "dns-domain", "", "DNS domain")
	return cmd
}

// newVCSNetworkRmCmd 建立 `vcs network rm`：破壞性操作，需確認或 -y。
func newVCSNetworkRmCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "rm <id|name>...",
		Short: "刪除網路（破壞性操作，需確認或 -y）",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			ids := make([]int64, 0, len(args))
			for _, ref := range args {
				id, err := svc.ResolveNetworkID(ctx, projectID, ref)
				if err != nil {
					return err
				}
				ids = append(ids, id)
			}
			if err := a.confirm(fmt.Sprintf("刪除網路 %s", joinIDs(ids))); err != nil {
				return err
			}
			for _, id := range ids {
				if err := svc.DeleteNetwork(ctx, id); err != nil {
					return err
				}
				a.progress("已刪除網路 %d", id)
			}
			return nil
		},
	}
}
