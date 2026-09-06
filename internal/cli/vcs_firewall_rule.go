package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	genvcs "github.com/gilbertchiao/twaictl/internal/gen/vcs"
	"github.com/gilbertchiao/twaictl/internal/output"
	"github.com/gilbertchiao/twaictl/internal/twai/vcs"
)

// firewallRuleTraffic 把 rule 的來源／目的以 `ip [port]` 摘要：ip 為空時以 `*` 代替；
// port 非空時在後面接空白與方括號，範圍的 `a:b` 換成 `a-b`（例如 `0.0.0.0/0 [22]`、
// `0.0.0.0/0 [1-65535]`、`::/0 [22]`）。改用空白＋方括號而非直接接冒號，是為了跟
// IPv6 位址本身的冒號區隔——`ip:port` 對 IPv4 已有歧義（`0.0.0.0/0:1:65535`），對
// IPv6 會更難閱讀（`::/0:22`）。
func firewallRuleTraffic(ip, port string) string {
	if ip == "" {
		ip = "*"
	}
	if port == "" {
		return ip
	}
	return ip + " [" + strings.ReplaceAll(port, ":", "-") + "]"
}

// firewallRuleColumns 是 `vcs firewall rule ls` 的欄位定義。
var firewallRuleColumns = []output.Column[genvcs.FirewallRuleSerializer]{
	{Name: "ID", Default: true, Value: func(r genvcs.FirewallRuleSerializer) string { return fmt.Sprint(r.Id) }},
	{Name: "NAME", Default: true, Value: func(r genvcs.FirewallRuleSerializer) string { return r.Name }},
	{Name: "PROTOCOL", Default: true, Value: func(r genvcs.FirewallRuleSerializer) string { return r.Protocol }},
	{Name: "ACTION", Default: true, Value: func(r genvcs.FirewallRuleSerializer) string { return r.Action }},
	{Name: "SOURCE", Default: true, Value: func(r genvcs.FirewallRuleSerializer) string {
		return firewallRuleTraffic(r.SourceIpAddress, r.SourcePort)
	}},
	{Name: "DESTINATION", Default: true, Value: func(r genvcs.FirewallRuleSerializer) string {
		return firewallRuleTraffic(r.DestinationIpAddress, r.DestinationPort)
	}},
	{Name: "IP VERSION", Value: func(r genvcs.FirewallRuleSerializer) string { return fmt.Sprint(r.IpVersion) }},
	{Name: "PLATFORM", Value: func(r genvcs.FirewallRuleSerializer) string { return r.Platform }},
}

// firewallRuleDetailColumns 是 `vcs firewall rule get`／`update` 直式輸出的欄位定義。
var firewallRuleDetailColumns = []output.Column[genvcs.FirewallRuleDetailSerializer]{
	{Name: "ID", Default: true, Value: func(r genvcs.FirewallRuleDetailSerializer) string { return fmt.Sprint(r.Id) }},
	{Name: "Name", Default: true, Value: func(r genvcs.FirewallRuleDetailSerializer) string { return r.Name }},
	{Name: "Protocol", Default: true, Value: func(r genvcs.FirewallRuleDetailSerializer) string { return r.Protocol }},
	{Name: "Action", Default: true, Value: func(r genvcs.FirewallRuleDetailSerializer) string { return r.Action }},
	{Name: "Source IP", Default: true, Value: func(r genvcs.FirewallRuleDetailSerializer) string { return r.SourceIpAddress }},
	{Name: "Source port", Default: true, Value: func(r genvcs.FirewallRuleDetailSerializer) string { return r.SourcePort }},
	{Name: "Destination IP", Default: true, Value: func(r genvcs.FirewallRuleDetailSerializer) string { return r.DestinationIpAddress }},
	{Name: "Destination port", Default: true, Value: func(r genvcs.FirewallRuleDetailSerializer) string { return r.DestinationPort }},
	{Name: "IP version", Default: true, Value: func(r genvcs.FirewallRuleDetailSerializer) string { return fmt.Sprint(r.IpVersion) }},
	{Name: "Created", Default: true, Value: func(r genvcs.FirewallRuleDetailSerializer) string { return output.FormatTime(r.CreateTime) }},
	{Name: "Project", Default: true, Value: func(r genvcs.FirewallRuleDetailSerializer) string { return fmt.Sprint(r.Project) }},
	{Name: "Platform", Value: func(r genvcs.FirewallRuleDetailSerializer) string { return r.Platform }},
}

// firewallRuleCreateColumns 是 `vcs firewall rule create` 直式輸出的欄位定義（回應為 FirewallRuleSerializer，無 create_time）。
var firewallRuleCreateColumns = []output.Column[genvcs.FirewallRuleSerializer]{
	{Name: "ID", Default: true, Value: func(r genvcs.FirewallRuleSerializer) string { return fmt.Sprint(r.Id) }},
	{Name: "Name", Default: true, Value: func(r genvcs.FirewallRuleSerializer) string { return r.Name }},
	{Name: "Protocol", Default: true, Value: func(r genvcs.FirewallRuleSerializer) string { return r.Protocol }},
	{Name: "Action", Default: true, Value: func(r genvcs.FirewallRuleSerializer) string { return r.Action }},
	{Name: "Source", Default: true, Value: func(r genvcs.FirewallRuleSerializer) string {
		return firewallRuleTraffic(r.SourceIpAddress, r.SourcePort)
	}},
	{Name: "Destination", Default: true, Value: func(r genvcs.FirewallRuleSerializer) string {
		return firewallRuleTraffic(r.DestinationIpAddress, r.DestinationPort)
	}},
}

// firewallRuleFlags 是 create／update 共用的規則欄位 flag；update 只送使用者明確指定
// （`Changed`）的 flag，因此用 cobra 的 Changed 判斷而非比較空字串。
type firewallRuleFlags struct {
	name, protocol, action, src, srcPort, dst, dstPort string
}

func (f *firewallRuleFlags) register(cmd *cobra.Command, nameRequired bool) {
	flags := cmd.Flags()
	nameHelp := "規則名稱"
	if nameRequired {
		nameHelp += "（必填）"
	}
	flags.StringVar(&f.name, "name", "", nameHelp)
	flags.StringVar(&f.protocol, "protocol", "", "協定："+strings.Join(vcs.FirewallRuleProtocols, "|"))
	flags.StringVar(&f.action, "action", "", "動作："+strings.Join(vcs.FirewallRuleActions, "|"))
	flags.StringVar(&f.src, "src", "", "來源 IPv4／IPv6 位址或 CIDR")
	flags.StringVar(&f.srcPort, "src-port", "", "來源 port 或範圍（例如 22 或 80:90）")
	flags.StringVar(&f.dst, "dst", "", "目的 IPv4／IPv6 位址或 CIDR")
	flags.StringVar(&f.dstPort, "dst-port", "", "目的 port 或範圍（例如 22 或 80:90）")
}

// validate 檢查 enum 類 flag 的值。
func (f *firewallRuleFlags) validate() error {
	if err := validateEnumList("--protocol", f.protocol, vcs.FirewallRuleProtocols); err != nil {
		return err
	}
	return validateEnumList("--action", f.action, vcs.FirewallRuleActions)
}

// validateNotEmptyIfChanged 只給 `update` 用：validateEnumList 對空字串一律視為
// 「使用者未指定」而放行（create 靠這個語意判斷該欄位要不要送出），但 update 改用
// c.Flags().Changed 判斷要不要送出該欄位——若使用者明確打了 --protocol ""，
// Changed 為 true，validateEnumList 卻不會擋下這個空字串，最終會送出
// {"protocol": ""} 這種在 API 端沒有意義的值。這裡在 update 專屬地補上這個檢查；
// create 不受影響（create 呼叫的是 f.validate()，沒有呼叫這個方法）。
// --name 也一併擋下：與 --src／--src-port／--dst／--dst-port 不同，name 不是「可清除」
// 欄位（沒有「空字串代表清除限制」的語意），明確指定空字串必是使用者的失誤。
func (f *firewallRuleFlags) validateNotEmptyIfChanged(c *cobra.Command) error {
	if c.Flags().Changed("name") && f.name == "" {
		return fmt.Errorf("--name 不可為空字串")
	}
	if c.Flags().Changed("protocol") && f.protocol == "" {
		return fmt.Errorf("--protocol 不可為空字串（合法值：%s）", strings.Join(vcs.FirewallRuleProtocols, "|"))
	}
	if c.Flags().Changed("action") && f.action == "" {
		return fmt.Errorf("--action 不可為空字串（合法值：%s）", strings.Join(vcs.FirewallRuleActions, "|"))
	}
	return nil
}

// newVCSFirewallRuleCmd 建立 `vcs firewall rule` 子命令。
func newVCSFirewallRuleCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{Use: "rule", Short: "管理 firewall rule（tenant admin 限定）"}
	cmd.AddCommand(newVCSFirewallRuleLsCmd(a), newVCSFirewallRuleGetCmd(a), newVCSFirewallRuleCreateCmd(a),
		newVCSFirewallRuleUpdateCmd(a), newVCSFirewallRuleRmCmd(a))
	return cmd
}

// newVCSFirewallRuleLsCmd 建立 `vcs firewall rule ls`。
func newVCSFirewallRuleLsCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "列出 firewall rule",
		Long:  "列出 firewall rule。\n\n" + columnsHelp(firewallRuleColumns),
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if err := validateColumns(a, firewallRuleColumns); err != nil {
				return err
			}
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			res, err := svc.ListFirewallRules(ctx, projectID)
			if err != nil {
				return err
			}
			return render(a, res.Raw, res.Value, firewallRuleColumns)
		},
	}
}

// newVCSFirewallRuleGetCmd 建立 `vcs firewall rule get`。
func newVCSFirewallRuleGetCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id|name>",
		Short: "顯示單一 firewall rule 的詳細資料",
		Long:  "顯示單一 firewall rule 的詳細資料。\n\n" + columnsHelp(firewallRuleDetailColumns),
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if err := validateColumns(a, firewallRuleDetailColumns); err != nil {
				return err
			}
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			id, err := svc.ResolveFirewallRuleID(ctx, projectID, args[0])
			if err != nil {
				return err
			}
			res, err := svc.GetFirewallRule(ctx, id)
			if err != nil {
				return err
			}
			return renderOne(a, res.Raw, res.Value, firewallRuleDetailColumns)
		},
	}
}

// newVCSFirewallRuleCreateCmd 建立 `vcs firewall rule create`。
func newVCSFirewallRuleCreateCmd(a *app) *cobra.Command {
	var f firewallRuleFlags
	cmd := &cobra.Command{
		Use:   "create --name <n> [--protocol icmp|tcp|udp] [--action allow|deny|reject] [--src <ip|cidr>] [--src-port <p>] [--dst <ip|cidr>] [--dst-port <p>]",
		Short: "建立 firewall rule",
		Long:  "建立 firewall rule；未指定的欄位不送出，由 API 採預設值。\n\n" + columnsHelp(firewallRuleCreateColumns),
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if err := validateColumns(a, firewallRuleCreateColumns); err != nil {
				return err
			}
			if err := requireFlag("--name", f.name); err != nil {
				return err
			}
			if err := f.validate(); err != nil {
				return err
			}
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			res, err := svc.CreateFirewallRule(ctx, vcs.FirewallRuleInput{
				ProjectID: projectID, Name: f.name, Protocol: f.protocol, Action: f.action,
				SourceIP: f.src, SourcePort: f.srcPort, DestinationIP: f.dst, DestinationPort: f.dstPort,
			})
			if err != nil {
				return err
			}
			a.progress("已建立 firewall rule %d", res.Value.Id)
			return renderOne(a, res.Raw, res.Value, firewallRuleCreateColumns)
		},
	}
	f.register(cmd, true)
	return cmd
}

// newVCSFirewallRuleUpdateCmd 建立 `vcs firewall rule update`：只送使用者明確指定的 flag（PATCH）。
func newVCSFirewallRuleUpdateCmd(a *app) *cobra.Command {
	var f firewallRuleFlags
	cmd := &cobra.Command{
		Use:   "update <id|name> [--name <n>] [--protocol ...] [--action ...] [--src ...] [--src-port ...] [--dst ...] [--dst-port ...]",
		Short: "更新 firewall rule（只送出有指定的欄位）",
		Long: "更新 firewall rule；只送出有指定的 flag，其餘欄位維持原值。" +
			"實測（2026-08-29）source/destination 的 IP 與 port 四個欄位都不接受空字串" +
			"（API 回 503），無法用空字串清除，需刪除重建；twaictl 仍照原樣送出由 API 決定。\n\n" +
			columnsHelp(firewallRuleDetailColumns),
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if err := validateColumns(a, firewallRuleDetailColumns); err != nil {
				return err
			}
			if err := f.validate(); err != nil {
				return err
			}
			if err := f.validateNotEmptyIfChanged(c); err != nil {
				return err
			}
			in := vcs.UpdateFirewallRuleInput{
				Name: changedString(c, "name", &f.name), Protocol: changedString(c, "protocol", &f.protocol),
				Action: changedString(c, "action", &f.action), SourceIP: changedString(c, "src", &f.src),
				SourcePort: changedString(c, "src-port", &f.srcPort), DestinationIP: changedString(c, "dst", &f.dst),
				DestinationPort: changedString(c, "dst-port", &f.dstPort),
			}
			if in == (vcs.UpdateFirewallRuleInput{}) {
				return fmt.Errorf("請至少指定一個要更新的欄位（--name/--protocol/--action/--src/--src-port/--dst/--dst-port）")
			}
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			id, err := svc.ResolveFirewallRuleID(ctx, projectID, args[0])
			if err != nil {
				return err
			}
			res, err := svc.UpdateFirewallRule(ctx, id, in)
			if err != nil {
				return err
			}
			a.progress("已更新 firewall rule %d", id)
			return renderOne(a, res.Raw, res.Value, firewallRuleDetailColumns)
		},
	}
	f.register(cmd, false)
	return cmd
}

// newVCSFirewallRuleRmCmd 建立 `vcs firewall rule rm`：破壞性操作，需確認或 -y。
func newVCSFirewallRuleRmCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "rm <id|name>...",
		Short: "刪除 firewall rule（破壞性操作，需確認或 -y）",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			ids := make([]int64, 0, len(args))
			for _, ref := range args {
				id, err := svc.ResolveFirewallRuleID(ctx, projectID, ref)
				if err != nil {
					return err
				}
				ids = append(ids, id)
			}
			if err := a.confirm(fmt.Sprintf("刪除 firewall rule %s", joinIDs(ids))); err != nil {
				return err
			}
			for _, id := range ids {
				if err := svc.DeleteFirewallRule(ctx, id); err != nil {
					return err
				}
				a.progress("已刪除 firewall rule %d", id)
			}
			return nil
		},
	}
}
