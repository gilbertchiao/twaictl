package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	genvcs "github.com/gilbertchiao/twaictl/internal/gen/vcs"
	"github.com/gilbertchiao/twaictl/internal/output"
)

// lbListenerSummary 把 listener 清單格式化為 `name:PROTOCOL/port`，以 `, ` 串接。
func lbListenerSummary(ls []genvcs.ListenerSerializer) string {
	parts := make([]string, 0, len(ls))
	for _, l := range ls {
		parts = append(parts, fmt.Sprintf("%s:%s/%d", l.Name, l.Protocol, l.ProtocolPort))
	}
	return strings.Join(parts, ", ")
}

// lbPoolSummary 把 pool 清單格式化為 `name(PROTOCOL,METHOD)`，以 `, ` 串接。
func lbPoolSummary(ps []genvcs.PoolSerializer) string {
	parts := make([]string, 0, len(ps))
	for _, p := range ps {
		parts = append(parts, fmt.Sprintf("%s(%s,%s)", p.Name, p.Protocol, p.Method))
	}
	return strings.Join(parts, ", ")
}

// lbMembersSummary 把 pool member 清單格式化為 `ip:port(status)`，以 `, ` 串接；
// nil（API 未回傳 members）顯示為空字串。
func lbMembersSummary(ms *[]genvcs.PoolMemberSerializer) string {
	if ms == nil {
		return ""
	}
	parts := make([]string, 0, len(*ms))
	for _, m := range *ms {
		parts = append(parts, fmt.Sprintf("%s:%s(%s)", output.Str(m.Ip), output.Int64(m.Port), output.Str(m.Status)))
	}
	return strings.Join(parts, ", ")
}

// lbMonitorSummary 把 health monitor 格式化為 `HTTP GET / expect 200 delay=5 timeout=3 retries=3`；
// nil（pool 未設定 monitor）顯示為空字串。
func lbMonitorSummary(m *genvcs.LBHealthMonitorSerializer) string {
	if m == nil {
		return ""
	}
	parts := make([]string, 0, 2)
	if m.MonitorType != nil {
		parts = append(parts, *m.MonitorType)
	}
	switch {
	case m.HttpMethod != nil && m.UrlPath != nil:
		parts = append(parts, fmt.Sprintf("%s %s", *m.HttpMethod, *m.UrlPath))
	case m.HttpMethod != nil:
		parts = append(parts, *m.HttpMethod)
	case m.UrlPath != nil:
		parts = append(parts, *m.UrlPath)
	}
	if m.ExpectedCodes != nil {
		parts = append(parts, fmt.Sprintf("expect %s", *m.ExpectedCodes))
	}
	if m.Delay != nil {
		parts = append(parts, fmt.Sprintf("delay=%d", *m.Delay))
	}
	if m.Timeout != nil {
		parts = append(parts, fmt.Sprintf("timeout=%d", *m.Timeout))
	}
	if m.MaxRetries != nil {
		parts = append(parts, fmt.Sprintf("retries=%d", *m.MaxRetries))
	}
	return strings.Join(parts, " ")
}

// lbPoolDetailSummary 把單一 pool 的詳細資料格式化為
// `name(PROTOCOL,METHOD) members=[...] monitor=[...]`。
func lbPoolDetailSummary(p genvcs.PoolDetailSerializer) string {
	return fmt.Sprintf("%s(%s,%s) members=[%s] monitor=[%s]",
		p.Name, p.Protocol, p.Method, lbMembersSummary(p.Members), lbMonitorSummary(p.Monitor))
}

// lbPoolDetailsSummary 把 pool 詳細清單以 `; ` 串接，供 `vcs lb get` 顯示。
func lbPoolDetailsSummary(ps []genvcs.PoolDetailSerializer) string {
	parts := make([]string, 0, len(ps))
	for _, p := range ps {
		parts = append(parts, lbPoolDetailSummary(p))
	}
	return strings.Join(parts, "; ")
}

// lbColumns 是 `vcs lb ls` 的欄位定義。
var lbColumns = []output.Column[genvcs.LBSerializer]{
	{Name: "ID", Default: true, Value: func(l genvcs.LBSerializer) string { return fmt.Sprint(l.Id) }},
	{Name: "NAME", Default: true, Value: func(l genvcs.LBSerializer) string { return l.Name }},
	{Name: "STATUS", Default: true, Value: func(l genvcs.LBSerializer) string { return l.Status }},
	{Name: "NETWORK", Default: true, Value: func(l genvcs.LBSerializer) string { return l.PrivateNet.Name }},
	{Name: "LISTENERS", Default: true, Value: func(l genvcs.LBSerializer) string { return lbListenerSummary(l.Listeners) }},
	{Name: "POOLS", Default: true, Value: func(l genvcs.LBSerializer) string { return lbPoolSummary(l.Pools) }},
	{Name: "CREATED", Default: true, Value: func(l genvcs.LBSerializer) string { return output.FormatTime(l.CreateTime) }},
	{Name: "DESC", Value: func(l genvcs.LBSerializer) string { return l.Desc }},
	{Name: "USER", Value: func(l genvcs.LBSerializer) string { return l.User.Username }},
}

// lbDetailColumns 是 `vcs lb get` 直式輸出的欄位定義。
var lbDetailColumns = []output.Column[genvcs.LBDetailSerializer]{
	{Name: "ID", Default: true, Value: func(l genvcs.LBDetailSerializer) string { return fmt.Sprint(l.Id) }},
	{Name: "Name", Default: true, Value: func(l genvcs.LBDetailSerializer) string { return l.Name }},
	{Name: "Status", Default: true, Value: func(l genvcs.LBDetailSerializer) string { return l.Status }},
	{Name: "Status reason", Default: true, Value: func(l genvcs.LBDetailSerializer) string { return output.Str(l.StatusReason) }},
	{Name: "VIP", Default: true, Value: func(l genvcs.LBDetailSerializer) string { return output.Str(l.Vip) }},
	{Name: "Network", Default: true, Value: func(l genvcs.LBDetailSerializer) string { return l.PrivateNet.Name }},
	{Name: "Listeners", Default: true, Value: func(l genvcs.LBDetailSerializer) string { return lbListenerSummary(l.Listeners) }},
	{Name: "Pools", Default: true, Value: func(l genvcs.LBDetailSerializer) string { return lbPoolDetailsSummary(l.Pools) }},
	{Name: "Total connections", Default: true, Value: func(l genvcs.LBDetailSerializer) string { return output.Int64(l.TotalConnections) }},
	{Name: "Active connections", Default: true, Value: func(l genvcs.LBDetailSerializer) string { return output.Int64(l.ActiveConnections) }},
	{Name: "Created", Default: true, Value: func(l genvcs.LBDetailSerializer) string { return output.FormatTime(l.CreateTime) }},
	{Name: "Desc", Default: true, Value: func(l genvcs.LBDetailSerializer) string { return l.Desc }},
	{Name: "User", Default: true, Value: func(l genvcs.LBDetailSerializer) string { return l.User.Username }},
}

// newVCSLbCmd 建立 `vcs lb` 子命令。
func newVCSLbCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{Use: "lb", Short: "管理 load balancer"}
	cmd.AddCommand(newVCSLbLsCmd(a), newVCSLbGetCmd(a), newVCSLbCreateCmd(a), newVCSLbUpdateCmd(a), newVCSLbActionCmd(a), newVCSLbRmCmd(a), newVCSLbReportCmd(a))
	return cmd
}

// newVCSLbLsCmd 建立 `vcs lb ls`。
func newVCSLbLsCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "列出 load balancer",
		Long:  "列出 load balancer。\n\n" + columnsHelp(lbColumns),
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if err := validateColumns(a, lbColumns); err != nil {
				return err
			}
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			res, err := svc.ListLoadBalancers(ctx, projectID)
			if err != nil {
				return err
			}
			return render(a, res.Raw, res.Value, lbColumns)
		},
	}
}

// newVCSLbGetCmd 建立 `vcs lb get`。
func newVCSLbGetCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id|name>",
		Short: "顯示單一 load balancer 的詳細資料",
		Long:  "顯示單一 load balancer 的詳細資料。\n\n" + columnsHelp(lbDetailColumns),
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if err := validateColumns(a, lbDetailColumns); err != nil {
				return err
			}
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			id, err := svc.ResolveLoadBalancerID(ctx, projectID, args[0])
			if err != nil {
				return err
			}
			res, err := svc.GetLoadBalancer(ctx, id)
			if err != nil {
				return err
			}
			return renderOne(a, res.Raw, res.Value, lbDetailColumns)
		},
	}
}

// newVCSLbRmCmd 建立 `vcs lb rm`：破壞性操作，需確認或 -y；支援 --wait 等待刪除完成。
func newVCSLbRmCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "rm <id|name>...",
		Short: "刪除 load balancer（破壞性操作，需確認或 -y）",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			ids := make([]int64, 0, len(args))
			for _, ref := range args {
				id, err := svc.ResolveLoadBalancerID(ctx, projectID, ref)
				if err != nil {
					return err
				}
				ids = append(ids, id)
			}
			if err := a.confirm(fmt.Sprintf("刪除 load balancer %s", joinIDs(ids))); err != nil {
				return err
			}
			for _, id := range ids {
				if err := svc.DeleteLoadBalancer(ctx, id); err != nil {
					return err
				}
				a.progress("已刪除 load balancer %d", id)
			}
			if !a.opts.wait {
				return nil
			}
			waitCtx, cancel := a.waitContext(ctx)
			defer cancel()
			for _, id := range ids {
				err := svc.WaitLoadBalancerDeleted(waitCtx, id, a.opts.waitInterval, func(st string) {
					a.progress("load balancer %d 狀態：%s", id, st)
				})
				if err != nil {
					return fmt.Errorf("load balancer %d 尚未刪除完成: %w", id, err)
				}
			}
			return nil
		},
	}
}
