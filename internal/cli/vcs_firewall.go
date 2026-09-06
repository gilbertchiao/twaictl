package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	genvcs "github.com/gilbertchiao/twaictl/internal/gen/vcs"
	"github.com/gilbertchiao/twaictl/internal/output"
	"github.com/gilbertchiao/twaictl/internal/twai/vcs"
)

// firewallColumns 是 `vcs firewall ls` 的欄位定義。
var firewallColumns = []output.Column[genvcs.FirewallSerializer]{
	{Name: "ID", Default: true, Value: func(f genvcs.FirewallSerializer) string { return fmt.Sprint(f.Id) }},
	{Name: "NAME", Default: true, Value: func(f genvcs.FirewallSerializer) string { return f.Name }},
	{Name: "STATUS", Default: true, Value: func(f genvcs.FirewallSerializer) string { return f.Status }},
	{Name: "DESC", Default: true, Value: func(f genvcs.FirewallSerializer) string { return f.Desc }},
	{Name: "USER", Value: func(f genvcs.FirewallSerializer) string { return f.User.Username }},
	{Name: "PLATFORM", Value: func(f genvcs.FirewallSerializer) string { return f.Platform }},
}

// firewallCreateColumns 是 `vcs firewall create` 直式輸出（回應為 FirewallSerializer）。
var firewallCreateColumns = []output.Column[genvcs.FirewallSerializer]{
	{Name: "ID", Default: true, Value: func(f genvcs.FirewallSerializer) string { return fmt.Sprint(f.Id) }},
	{Name: "Name", Default: true, Value: func(f genvcs.FirewallSerializer) string { return f.Name }},
	{Name: "Status", Default: true, Value: func(f genvcs.FirewallSerializer) string { return f.Status }},
	{Name: "Desc", Default: true, Value: func(f genvcs.FirewallSerializer) string { return f.Desc }},
	{Name: "User", Default: true, Value: func(f genvcs.FirewallSerializer) string { return f.User.Username }},
}

// firewallRulesSummary 把 firewall 詳細資料的 rules 以 `name(id)` 串接。
func firewallRulesSummary(rules []genvcs.FirewallDetailRuleObject) string {
	parts := make([]string, 0, len(rules))
	for _, r := range rules {
		parts = append(parts, fmt.Sprintf("%s(%d)", r.Name, r.Id))
	}
	return strings.Join(parts, ", ")
}

// firewallNetworksSummary 把 firewall 詳細資料的 associate_networks 以 `name(id)` 串接。
func firewallNetworksSummary(nets []genvcs.FirewallDetailNetworkObject) string {
	parts := make([]string, 0, len(nets))
	for _, n := range nets {
		parts = append(parts, fmt.Sprintf("%s(%d)", n.Name, n.Id))
	}
	return strings.Join(parts, ", ")
}

// firewallDetailColumns 是 `vcs firewall get`／`update` 直式輸出的欄位定義。
var firewallDetailColumns = []output.Column[genvcs.FirewallDetailSerializer]{
	{Name: "ID", Default: true, Value: func(f genvcs.FirewallDetailSerializer) string { return fmt.Sprint(f.Id) }},
	{Name: "Name", Default: true, Value: func(f genvcs.FirewallDetailSerializer) string { return f.Name }},
	{Name: "Status", Default: true, Value: func(f genvcs.FirewallDetailSerializer) string { return f.Status }},
	{Name: "Status reason", Default: true, Value: func(f genvcs.FirewallDetailSerializer) string { return f.StatusReason }},
	{Name: "Desc", Default: true, Value: func(f genvcs.FirewallDetailSerializer) string { return f.Desc }},
	{Name: "Rules", Default: true, Value: func(f genvcs.FirewallDetailSerializer) string { return firewallRulesSummary(f.Rules) }},
	{Name: "Networks", Default: true, Value: func(f genvcs.FirewallDetailSerializer) string { return firewallNetworksSummary(f.AssociateNetworks) }},
	{Name: "Created", Default: true, Value: func(f genvcs.FirewallDetailSerializer) string { return output.TimePtr(f.CreateTime) }},
	{Name: "Project", Default: true, Value: func(f genvcs.FirewallDetailSerializer) string { return fmt.Sprint(f.Project) }},
	{Name: "User", Default: true, Value: func(f genvcs.FirewallDetailSerializer) string { return f.User.Username }},
	{Name: "Platform", Value: func(f genvcs.FirewallDetailSerializer) string { return f.Platform }},
}

// resolveFirewallRuleRefs 把多個 <id|name> 解析成 rule id 清單（保留輸入順序）。
func resolveFirewallRuleRefs(ctx context.Context, svc *vcs.Service, projectID int64, refs []string) ([]int64, error) {
	ids := make([]int64, 0, len(refs))
	for _, ref := range refs {
		id, err := svc.ResolveFirewallRuleID(ctx, projectID, ref)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// resolveNetworkRefs 把多個 <id|name> 解析成 network id 清單（保留輸入順序）。
func resolveNetworkRefs(ctx context.Context, svc *vcs.Service, projectID int64, refs []string) ([]int64, error) {
	ids := make([]int64, 0, len(refs))
	for _, ref := range refs {
		id, err := svc.ResolveNetworkID(ctx, projectID, ref)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// newVCSFirewallCmd 建立 `vcs firewall` 子命令（spec 描述僅 tenant admin 可用）。
func newVCSFirewallCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{Use: "firewall", Short: "管理 firewall 與 firewall rule（tenant admin 限定）"}
	cmd.AddCommand(newVCSFirewallLsCmd(a), newVCSFirewallGetCmd(a), newVCSFirewallCreateCmd(a),
		newVCSFirewallUpdateCmd(a), newVCSFirewallRmCmd(a), newVCSFirewallRuleCmd(a))
	return cmd
}

// newVCSFirewallLsCmd 建立 `vcs firewall ls`。
func newVCSFirewallLsCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "列出 firewall",
		Long:  "列出 firewall。\n\n" + columnsHelp(firewallColumns),
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if err := validateColumns(a, firewallColumns); err != nil {
				return err
			}
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			res, err := svc.ListFirewalls(ctx, projectID)
			if err != nil {
				return err
			}
			return render(a, res.Raw, res.Value, firewallColumns)
		},
	}
}

// newVCSFirewallGetCmd 建立 `vcs firewall get`。
func newVCSFirewallGetCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id|name>",
		Short: "顯示單一 firewall 的詳細資料",
		Long:  "顯示單一 firewall 的詳細資料。\n\n" + columnsHelp(firewallDetailColumns),
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if err := validateColumns(a, firewallDetailColumns); err != nil {
				return err
			}
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			id, err := svc.ResolveFirewallID(ctx, projectID, args[0])
			if err != nil {
				return err
			}
			res, err := svc.GetFirewall(ctx, id)
			if err != nil {
				return err
			}
			return renderOne(a, res.Raw, res.Value, firewallDetailColumns)
		},
	}
}

// newVCSFirewallCreateCmd 建立 `vcs firewall create`。
func newVCSFirewallCreateCmd(a *app) *cobra.Command {
	var name, desc string
	var ruleRefs, networkRefs []string
	cmd := &cobra.Command{
		Use:   "create --name <n> [--desc <d>] [--rule <id|name>]... [--network <id|name>]...",
		Short: "建立 firewall（可同時指定規則與要套用的私有網路）",
		Long:  "建立 firewall。--rule／--network 可重複指定，接受 id 或名稱。\n\n" + columnsHelp(firewallCreateColumns),
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if err := validateColumns(a, firewallCreateColumns); err != nil {
				return err
			}
			if err := requireFlag("--name", name); err != nil {
				return err
			}
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			ruleIDs, err := resolveFirewallRuleRefs(ctx, svc, projectID, ruleRefs)
			if err != nil {
				return err
			}
			networkIDs, err := resolveNetworkRefs(ctx, svc, projectID, networkRefs)
			if err != nil {
				return err
			}
			res, err := svc.CreateFirewall(ctx, vcs.CreateFirewallInput{ProjectID: projectID, Name: name, Desc: desc, RuleIDs: ruleIDs, NetworkIDs: networkIDs})
			if err != nil {
				return err
			}
			a.progress("已建立 firewall %d", res.Value.Id)
			return renderOne(a, res.Raw, res.Value, firewallCreateColumns)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&name, "name", "", "firewall 名稱（必填）")
	flags.StringVar(&desc, "desc", "", "描述")
	flags.StringArrayVar(&ruleRefs, "rule", nil, "firewall rule 的 id 或名稱（可重複）")
	flags.StringArrayVar(&networkRefs, "network", nil, "要套用此 firewall 的私有網路 id 或名稱（可重複）")
	return cmd
}

// newVCSFirewallUpdateCmd 建立 `vcs firewall update`：--rule／--network 為整份取代（PATCH 語意），
// --clear-rules／--clear-networks 送空陣列清空。
func newVCSFirewallUpdateCmd(a *app) *cobra.Command {
	var desc string
	var ruleRefs, networkRefs []string
	var clearRules, clearNetworks bool
	cmd := &cobra.Command{
		Use:   "update <id|name> [--desc <d>] [--rule <id|name>]... [--network <id|name>]... [--clear-rules] [--clear-networks]",
		Short: "更新 firewall（規則／網路清單為整份取代）",
		Long: "更新 firewall。--rule／--network 會以指定的清單整份取代既有設定（API 為 PATCH 整份取代語意，" +
			"沒有增量新增／移除）；--clear-rules／--clear-networks 清空對應清單；未指定的欄位維持原值。\n\n" +
			columnsHelp(firewallDetailColumns),
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if err := validateColumns(a, firewallDetailColumns); err != nil {
				return err
			}
			if len(ruleRefs) > 0 && clearRules {
				return fmt.Errorf("--rule 與 --clear-rules 不可同時使用")
			}
			if len(networkRefs) > 0 && clearNetworks {
				return fmt.Errorf("--network 與 --clear-networks 不可同時使用")
			}
			in := vcs.UpdateFirewallInput{Desc: changedString(c, "desc", &desc)}
			if in.Desc == nil && len(ruleRefs) == 0 && len(networkRefs) == 0 && !clearRules && !clearNetworks {
				return fmt.Errorf("請至少指定一個要更新的欄位（--desc/--rule/--network/--clear-rules/--clear-networks）")
			}
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			id, err := svc.ResolveFirewallID(ctx, projectID, args[0])
			if err != nil {
				return err
			}
			if clearRules {
				empty := []int64{}
				in.RuleIDs = &empty
			} else if len(ruleRefs) > 0 {
				ids, err := resolveFirewallRuleRefs(ctx, svc, projectID, ruleRefs)
				if err != nil {
					return err
				}
				in.RuleIDs = &ids
			}
			if clearNetworks {
				empty := []int64{}
				in.NetworkIDs = &empty
			} else if len(networkRefs) > 0 {
				ids, err := resolveNetworkRefs(ctx, svc, projectID, networkRefs)
				if err != nil {
					return err
				}
				in.NetworkIDs = &ids
			}
			res, err := svc.UpdateFirewall(ctx, id, in)
			if err != nil {
				return err
			}
			a.progress("已更新 firewall %d", id)
			return renderOne(a, res.Raw, res.Value, firewallDetailColumns)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&desc, "desc", "", "描述（可指定空字串清除）")
	flags.StringArrayVar(&ruleRefs, "rule", nil, "以此清單整份取代 firewall 的規則（id 或名稱，可重複）")
	flags.StringArrayVar(&networkRefs, "network", nil, "以此清單整份取代套用的私有網路（id 或名稱，可重複）")
	flags.BoolVar(&clearRules, "clear-rules", false, "清空規則清單（與 --rule 互斥）")
	flags.BoolVar(&clearNetworks, "clear-networks", false, "清空套用的私有網路（與 --network 互斥）")
	return cmd
}

// newVCSFirewallRmCmd 建立 `vcs firewall rm`：破壞性操作，需確認或 -y。
func newVCSFirewallRmCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "rm <id|name>...",
		Short: "刪除 firewall（破壞性操作，需確認或 -y）",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			ids := make([]int64, 0, len(args))
			for _, ref := range args {
				id, err := svc.ResolveFirewallID(ctx, projectID, ref)
				if err != nil {
					return err
				}
				ids = append(ids, id)
			}
			if err := a.confirm(fmt.Sprintf("刪除 firewall %s", joinIDs(ids))); err != nil {
				return err
			}
			for _, id := range ids {
				if err := svc.DeleteFirewall(ctx, id); err != nil {
					return err
				}
				a.progress("已刪除 firewall %d", id)
			}
			return nil
		},
	}
}
