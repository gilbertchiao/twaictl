package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	genvcs "github.com/gilbertchiao/twaictl/internal/gen/vcs"
	"github.com/gilbertchiao/twaictl/internal/output"
	"github.com/gilbertchiao/twaictl/internal/twai/vcs"
)

// secgRuleRow 是 `vcs secg ls` 攤平後的顯示列：每條規則一列，帶上所屬 security group 的 id。
// json tag 只在 items 需要被直接序列化時使用（`-o json` 一律用 API 原始回應 res.Raw，
// 目前不會走到這個路徑，保留 tag 是為了與其他命令的慣例一致、避免日後改動漏掉）。
type secgRuleRow struct {
	SecurityGroupID string `json:"security_group_id"`
	RuleID          string `json:"rule_id"`
	Direction       string `json:"direction"`
	Protocol        string `json:"protocol"`
	Ports           string `json:"ports"`
	Remote          string `json:"remote_ip_prefix"`
	Ethertype       string `json:"ethertype"`
}

// formatPortRange 把 min/max port 格式化為顯示文字：兩者皆 0（spec 未限制 port）顯示
// `any`；min == max（單一 port）顯示該值；否則顯示 `min-max`。
func formatPortRange(portMin, portMax int) string {
	if portMin == 0 && portMax == 0 {
		return "any"
	}
	if portMin == portMax {
		return strconv.Itoa(portMin)
	}
	return fmt.Sprintf("%d-%d", portMin, portMax)
}

// flattenSecurityGroups 把 security group 清單攤平成每條規則一列。
func flattenSecurityGroups(groups []genvcs.SecurityGroupSerializer) []secgRuleRow {
	var rows []secgRuleRow
	for _, g := range groups {
		for _, r := range g.SecurityGroupRules {
			rows = append(rows, secgRuleRow{
				SecurityGroupID: g.Id,
				RuleID:          r.Id,
				Direction:       string(r.Direction),
				Protocol:        string(r.Protocol),
				Ports:           formatPortRange(r.PortRangeMin, r.PortRangeMax),
				Remote:          r.RemoteIpPrefix,
				Ethertype:       r.Ethertype,
			})
		}
	}
	return rows
}

// secgRuleColumns 是 `vcs secg ls` 的欄位定義。
var secgRuleColumns = []output.Column[secgRuleRow]{
	{Name: "SECURITY GROUP", Default: true, Value: func(r secgRuleRow) string { return r.SecurityGroupID }},
	{Name: "RULE ID", Default: true, Value: func(r secgRuleRow) string { return r.RuleID }},
	{Name: "DIRECTION", Default: true, Value: func(r secgRuleRow) string { return r.Direction }},
	{Name: "PROTOCOL", Default: true, Value: func(r secgRuleRow) string { return r.Protocol }},
	{Name: "PORTS", Default: true, Value: func(r secgRuleRow) string { return r.Ports }},
	{Name: "REMOTE", Default: true, Value: func(r secgRuleRow) string { return r.Remote }},
	{Name: "ETHERTYPE", Default: true, Value: func(r secgRuleRow) string { return r.Ethertype }},
}

// newVCSSecgCmd 建立 `vcs secg` 子命令。
func newVCSSecgCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{Use: "secg", Short: "管理 VCS security group 規則"}
	cmd.AddCommand(newVCSSecgLsCmd(a), newVCSSecgAddRuleCmd(a), newVCSSecgRmRuleCmd(a))
	return cmd
}

// newVCSSecgLsCmd 建立 `vcs secg ls`：--server 覆寫 server 解析，做法同 `vcs metrics`
// （未指定時透過 ResolveServerIDForSite 由 site 自動解析，僅限剛好一台 server 的 site）。
func newVCSSecgLsCmd(a *app) *cobra.Command {
	var serverRef string
	cmd := &cobra.Command{
		Use:   "ls <id|name>",
		Short: "列出 VCS 實例的 security group 規則",
		Long:  "列出 VCS 實例的 security group 規則。\n\n" + columnsHelp(secgRuleColumns),
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if err := validateColumns(a, secgRuleColumns); err != nil {
				return err
			}
			serverID, err := parseServerFlag(serverRef)
			if err != nil {
				return err
			}
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			siteID, err := svc.ResolveSiteID(ctx, projectID, args[0])
			if err != nil {
				return err
			}
			if serverID == 0 {
				serverID, err = svc.ResolveServerIDForSite(ctx, projectID, siteID)
				if err != nil {
					return err
				}
			}
			res, err := svc.ListSecurityGroups(ctx, projectID, serverID)
			if err != nil {
				return err
			}
			return render(a, res.Raw, flattenSecurityGroups(res.Value), secgRuleColumns)
		},
	}
	cmd.Flags().StringVar(&serverRef, "server", "", "指定 server id（未指定時由 site 自動解析，僅限剛好一台 server 的 site）")
	return cmd
}

// newVCSSecgAddRuleCmd 建立 `vcs secg add-rule`：direction/protocol 必填且驗證 enum，
// port 1-65535（0 代表未指定，不送對應欄位）；所有驗證都在送出任何 API 請求之前完成。
func newVCSSecgAddRuleCmd(a *app) *cobra.Command {
	var direction, protocol, remote string
	var portMin, portMax int
	cmd := &cobra.Command{
		Use:   "add-rule <security-group-id> --direction <ingress|egress> --protocol <tcp|udp|icmp|udplite|sctp|dccp>",
		Short: "對 security group 新增一條規則",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if err := requireFlag("--direction", direction); err != nil {
				return err
			}
			if err := validateEnumList("--direction", direction, vcs.SecurityRuleDirections); err != nil {
				return err
			}
			if err := requireFlag("--protocol", protocol); err != nil {
				return err
			}
			if err := validateEnumList("--protocol", protocol, vcs.SecurityRuleProtocols); err != nil {
				return err
			}
			if err := validatePort("--port-min", portMin); err != nil {
				return err
			}
			if err := validatePort("--port-max", portMax); err != nil {
				return err
			}
			if portMin != 0 && portMax != 0 && portMin > portMax {
				return fmt.Errorf("--port-min 不可大於 --port-max（%d > %d）", portMin, portMax)
			}
			// 只給其中一個時，另一個補成相同值（單一 port）；兩個都省略則兩者皆不送
			// （AddSecurityGroupRule 對 0 值不送對應欄位）。
			switch {
			case portMax == 0 && portMin != 0:
				portMax = portMin
			case portMin == 0 && portMax != 0:
				portMin = portMax
			}
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			sgID := args[0]
			err = svc.AddSecurityGroupRule(ctx, sgID, vcs.SecurityRuleInput{
				ProjectID: projectID, Direction: direction, Protocol: protocol,
				RemoteIPPrefix: remote, PortMin: portMin, PortMax: portMax,
			})
			if err != nil {
				return err
			}
			a.progress("已新增規則到 security group %s", sgID)
			return nil
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&direction, "direction", "", "規則方向（必填）："+strings.Join(vcs.SecurityRuleDirections, "|"))
	flags.StringVar(&protocol, "protocol", "", "協定（必填）："+strings.Join(vcs.SecurityRuleProtocols, "|"))
	flags.IntVar(&portMin, "port-min", 0, "port range 下限（1-65535，省略代表不限）")
	flags.IntVar(&portMax, "port-max", 0, "port range 上限（1-65535，省略時等於 --port-min）")
	flags.StringVar(&remote, "remote", "", "來源／目的 CIDR（例如 0.0.0.0/0）")
	return cmd
}

// validatePort 確認 port 為 0（未指定）或介於 1-65535。
func validatePort(flagName string, port int) error {
	if port == 0 {
		return nil
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("%s 必須介於 1-65535（目前 %d）", flagName, port)
	}
	return nil
}

// newVCSSecgRmRuleCmd 建立 `vcs secg rm-rule`：破壞性操作，需確認或 -y。
func newVCSSecgRmRuleCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "rm-rule <rule-id>...",
		Short: "刪除 security group 規則（破壞性操作，需確認或 -y）",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			if err := a.confirm(fmt.Sprintf("刪除規則 %s", strings.Join(args, ", "))); err != nil {
				return err
			}
			for _, ruleID := range args {
				if err := svc.DeleteSecurityGroupRule(ctx, projectID, ruleID); err != nil {
					return err
				}
				a.progress("已刪除規則 %s", ruleID)
			}
			return nil
		},
	}
}
