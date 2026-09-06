package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	genvcs "github.com/gilbertchiao/twaictl/internal/gen/vcs"
	"github.com/gilbertchiao/twaictl/internal/output"
	"github.com/gilbertchiao/twaictl/internal/twai/vcs"
)

// aspDescription 顯示 ASP 描述（spec 回應欄位叫 description，與 request 的 desc 不同）；nil 時空字串。
func aspDescription(p genvcs.AutoScalingPolicySerializer) string {
	if p.Description == nil {
		return ""
	}
	return *p.Description
}

// aspColumns 是 `vcs asp ls` 的欄位定義。
var aspColumns = []output.Column[genvcs.AutoScalingPolicySerializer]{
	{Name: "ID", Default: true, Value: func(p genvcs.AutoScalingPolicySerializer) string { return fmt.Sprint(p.Id) }},
	{Name: "NAME", Default: true, Value: func(p genvcs.AutoScalingPolicySerializer) string { return p.Name }},
	{Name: "METER", Default: true, Value: func(p genvcs.AutoScalingPolicySerializer) string { return string(p.MeterName) }},
	{Name: "SCALE UP", Default: true, Value: func(p genvcs.AutoScalingPolicySerializer) string { return fmt.Sprint(p.ScaleupThreshold) }},
	{Name: "SCALE DOWN", Default: true, Value: func(p genvcs.AutoScalingPolicySerializer) string { return fmt.Sprint(p.ScaledownThreshold) }},
	{Name: "MAX SIZE", Default: true, Value: func(p genvcs.AutoScalingPolicySerializer) string { return fmt.Sprint(p.ScaleMaxSize) }},
	{Name: "DESC", Value: aspDescription},
	{Name: "USER", Value: func(p genvcs.AutoScalingPolicySerializer) string { return p.User.Username }},
	{Name: "PLATFORM", Value: func(p genvcs.AutoScalingPolicySerializer) string { return p.Platform }},
}

// aspDetailColumns 是 `vcs asp get`／`create` 直式輸出的欄位定義。
var aspDetailColumns = []output.Column[genvcs.AutoScalingPolicySerializer]{
	{Name: "ID", Default: true, Value: func(p genvcs.AutoScalingPolicySerializer) string { return fmt.Sprint(p.Id) }},
	{Name: "Name", Default: true, Value: func(p genvcs.AutoScalingPolicySerializer) string { return p.Name }},
	{Name: "Meter", Default: true, Value: func(p genvcs.AutoScalingPolicySerializer) string { return string(p.MeterName) }},
	{Name: "Scale up threshold", Default: true, Value: func(p genvcs.AutoScalingPolicySerializer) string { return fmt.Sprint(p.ScaleupThreshold) }},
	{Name: "Scale down threshold", Default: true, Value: func(p genvcs.AutoScalingPolicySerializer) string { return fmt.Sprint(p.ScaledownThreshold) }},
	{Name: "Max size", Default: true, Value: func(p genvcs.AutoScalingPolicySerializer) string { return fmt.Sprint(p.ScaleMaxSize) }},
	{Name: "Desc", Default: true, Value: aspDescription},
	{Name: "Project", Default: true, Value: func(p genvcs.AutoScalingPolicySerializer) string { return fmt.Sprint(p.Project) }},
	{Name: "User", Default: true, Value: func(p genvcs.AutoScalingPolicySerializer) string { return p.User.Username }},
	{Name: "Platform", Value: func(p genvcs.AutoScalingPolicySerializer) string { return p.Platform }},
}

// newVCSAspCmd 建立 `vcs asp`（auto scaling policy）子命令。
func newVCSAspCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{Use: "asp", Short: "管理 auto scaling policy（自動擴縮政策）"}
	cmd.AddCommand(newVCSAspLsCmd(a), newVCSAspGetCmd(a), newVCSAspCreateCmd(a), newVCSAspRmCmd(a),
		newVCSAspAttachCmd(a), newVCSAspDetachCmd(a))
	return cmd
}

// newVCSAspLsCmd 建立 `vcs asp ls`。
func newVCSAspLsCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "列出 auto scaling policy",
		Long:  "列出 auto scaling policy。\n\n" + columnsHelp(aspColumns),
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if err := validateColumns(a, aspColumns); err != nil {
				return err
			}
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			res, err := svc.ListAutoScalingPolicies(ctx, projectID)
			if err != nil {
				return err
			}
			return render(a, res.Raw, res.Value, aspColumns)
		},
	}
}

// newVCSAspGetCmd 建立 `vcs asp get`。
func newVCSAspGetCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id|name>",
		Short: "顯示單一 auto scaling policy 的詳細資料",
		Long:  "顯示單一 auto scaling policy 的詳細資料。\n\n" + columnsHelp(aspDetailColumns),
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if err := validateColumns(a, aspDetailColumns); err != nil {
				return err
			}
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			id, err := svc.ResolveAutoScalingPolicyID(ctx, projectID, args[0])
			if err != nil {
				return err
			}
			res, err := svc.GetAutoScalingPolicy(ctx, id)
			if err != nil {
				return err
			}
			return renderOne(a, res.Raw, res.Value, aspDetailColumns)
		},
	}
}

// newVCSAspCreateCmd 建立 `vcs asp create`。
func newVCSAspCreateCmd(a *app) *cobra.Command {
	var name, meter, desc string
	var scaleUp, scaleDown, maxSize int
	cmd := &cobra.Command{
		Use:   "create --name <n> --meter <m> --scale-up <pct> --max-size <n> [--scale-down <pct>] [--desc <d>]",
		Short: "建立 auto scaling policy",
		Long: "建立 auto scaling policy。--meter 接受與 `vcs metrics` 相同的短名（cpu|memory|disk-read|disk-write|net-in|net-out）" +
			"或 API 名稱（cpu_util、memory.usage、…）；--scale-up／--scale-down 為觸發擴／縮的門檻值，" +
			"--max-size 為擴到最多幾台。\n\n" + columnsHelp(aspDetailColumns),
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if err := validateColumns(a, aspDetailColumns); err != nil {
				return err
			}
			if err := requireFlag("--name", name); err != nil {
				return err
			}
			if err := requireFlag("--meter", meter); err != nil {
				return err
			}
			meterName, err := vcs.ASPMeterName(meter)
			if err != nil {
				return err
			}
			if !c.Flags().Changed("scale-up") {
				return fmt.Errorf("--scale-up 為必填")
			}
			if !c.Flags().Changed("max-size") {
				return fmt.Errorf("--max-size 為必填")
			}
			if maxSize <= 0 {
				return fmt.Errorf("--max-size 必須是正整數（目前 %d）", maxSize)
			}
			if scaleUp <= 0 {
				return fmt.Errorf("--scale-up 必須是正整數（目前 %d）", scaleUp)
			}
			var scaleDownPtr *int
			if c.Flags().Changed("scale-down") {
				if scaleDown < 0 {
					return fmt.Errorf("--scale-down 不可為負數（目前 %d）", scaleDown)
				}
				if scaleDown >= scaleUp {
					return fmt.Errorf("--scale-down 必須小於 --scale-up（目前 %d >= %d）", scaleDown, scaleUp)
				}
				scaleDownPtr = &scaleDown
			}
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			res, err := svc.CreateAutoScalingPolicy(ctx, vcs.CreateAutoScalingPolicyInput{
				ProjectID: projectID, Name: name, Desc: desc, MeterName: meterName,
				ScaleUpThreshold: scaleUp, ScaleDownThreshold: scaleDownPtr, ScaleMaxSize: maxSize,
			})
			if err != nil {
				return err
			}
			a.progress("已建立 auto scaling policy %d", res.Value.Id)
			return renderOne(a, res.Raw, res.Value, aspDetailColumns)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&name, "name", "", "政策名稱（必填）")
	flags.StringVar(&meter, "meter", "", "監控指標（必填）：cpu|memory|disk-read|disk-write|net-in|net-out，或 API 名稱")
	flags.IntVar(&scaleUp, "scale-up", 0, "擴容門檻（必填；例如 cpu 使用率 80）")
	flags.IntVar(&scaleDown, "scale-down", 0, "縮容門檻（選填，須小於 --scale-up）")
	flags.IntVar(&maxSize, "max-size", 0, "最多擴到幾台 server（必填，正整數）")
	flags.StringVar(&desc, "desc", "", "描述")
	return cmd
}

// newVCSAspRmCmd 建立 `vcs asp rm`：破壞性操作，需確認或 -y。
func newVCSAspRmCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "rm <id|name>...",
		Short: "刪除 auto scaling policy（破壞性操作，需確認或 -y）",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			ids := make([]int64, 0, len(args))
			for _, ref := range args {
				id, err := svc.ResolveAutoScalingPolicyID(ctx, projectID, ref)
				if err != nil {
					return err
				}
				ids = append(ids, id)
			}
			if err := a.confirm(fmt.Sprintf("刪除 auto scaling policy %s", joinIDs(ids))); err != nil {
				return err
			}
			for _, id := range ids {
				if err := svc.DeleteAutoScalingPolicy(ctx, id); err != nil {
					return err
				}
				a.progress("已刪除 auto scaling policy %d", id)
			}
			return nil
		},
	}
}

// checkAspTarget 檢查 --site／--server 恰好指定一個；在呼叫任何 API（含 project 解析）之前執行。
// 「都沒給」與「兩個都給」是兩種不同的使用者錯誤，分開給訊息比合併成同一句更精確，
// 使用者不用自己猜是漏了還是重複了。
func checkAspTarget(siteRef, serverRef string) error {
	if siteRef == "" && serverRef == "" {
		return fmt.Errorf("請指定 --site 或 --server 其中之一")
	}
	if siteRef != "" && serverRef != "" {
		return fmt.Errorf("--site 與 --server 不可同時使用")
	}
	return nil
}

// resolveAspTarget 把 --site <id|name> 或 --server <id>（已由 checkAspTarget 確認二擇一）解析成 server id：
// --site 走 ResolveSiteID → ResolveServerIDForSite（多台 server 的 site 需改用 --server）。
func resolveAspTarget(ctx context.Context, svc *vcs.Service, projectID int64, siteRef, serverRef string) (int64, error) {
	if serverRef != "" {
		return parseServerFlag(serverRef)
	}
	siteID, err := svc.ResolveSiteID(ctx, projectID, siteRef)
	if err != nil {
		return 0, err
	}
	return svc.ResolveServerIDForSite(ctx, projectID, siteID)
}

// aspAttachmentColumns 是 `vcs asp attach` 的直式輸出。
var aspAttachmentColumns = []output.Column[vcs.AutoScalingPolicyAttachment]{
	{Name: "Auto scaling policy", Default: true, Value: func(r vcs.AutoScalingPolicyAttachment) string { return fmt.Sprint(r.AutoScalingPolicy) }},
	{Name: "Load balancer", Default: true, Value: func(r vcs.AutoScalingPolicyAttachment) string {
		if r.LoadBalancer == nil {
			return ""
		}
		return fmt.Sprint(*r.LoadBalancer)
	}},
}

// newVCSAspAttachCmd 建立 `vcs asp attach`。
func newVCSAspAttachCmd(a *app) *cobra.Command {
	var siteRef, serverRef, lbRef, scaleUpAction, scaleDownAction string
	var port int
	cmd := &cobra.Command{
		Use:   "attach <id|name> (--site <id|name> | --server <id>) [--lb <id|name>] [--port <n>] [--scale-up-action <url>] [--scale-down-action <url>]",
		Short: "把 auto scaling policy 掛到 server（觸發時會自動建立新 server）",
		Long: "把 auto scaling policy 掛到 server。--site 為 VCS 實例（自動解析成其唯一的 server；多台 server 的實例請改用 --server）。\n" +
			"指定 --lb 時，擴出的 server 會自動加入該 load balancer（原 server 除外），--port 為 server 監聽的 port（預設同 load balancer）；" +
			"--port 與 --lb 是彼此獨立的選填欄位，未指定 --lb 時 --port 的用途由 API 決定。\n" +
			"--scale-up-action／--scale-down-action 為觸發擴／縮時以 HTTP POST 通知的 URL。\n\n" +
			"注意：政策觸發擴容時會建立新的 server，可能產生費用。\n\n" + columnsHelp(aspAttachmentColumns),
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if err := validateColumns(a, aspAttachmentColumns); err != nil {
				return err
			}
			if err := checkAspTarget(siteRef, serverRef); err != nil {
				return err
			}
			if c.Flags().Changed("port") && (port < 1 || port > 65535) {
				return fmt.Errorf("--port 必須介於 1–65535（目前 %d）", port)
			}
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			serverID, err := resolveAspTarget(ctx, svc, projectID, siteRef, serverRef)
			if err != nil {
				return err
			}
			policyID, err := svc.ResolveAutoScalingPolicyID(ctx, projectID, args[0])
			if err != nil {
				return err
			}
			var lbID int64
			if lbRef != "" {
				if lbID, err = svc.ResolveLoadBalancerID(ctx, projectID, lbRef); err != nil {
					return err
				}
			}
			res, err := svc.AttachAutoScalingPolicy(ctx, vcs.AttachAutoScalingPolicyInput{
				PolicyID: policyID, ServerID: serverID, LoadBalancerID: lbID, ProtocolPort: port,
				ScaleUpAction: scaleUpAction, ScaleDownAction: scaleDownAction,
			})
			if err != nil {
				return err
			}
			a.progress("已將 auto scaling policy %d 掛到 server %d", policyID, serverID)
			return renderOne(a, res.Raw, res.Value, aspAttachmentColumns)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&siteRef, "site", "", "VCS 實例 site 的 id 或名稱（與 --server 擇一）")
	flags.StringVar(&serverRef, "server", "", "server id（與 --site 擇一）")
	flags.StringVar(&lbRef, "lb", "", "擴出的 server 要加入的 load balancer id 或名稱")
	flags.IntVar(&port, "port", 0, "server 監聽的 port（預設同 load balancer；未指定 --lb 時 port 的用途由 API 決定）")
	flags.StringVar(&scaleUpAction, "scale-up-action", "", "擴容時通知的 URL（HTTP POST）")
	flags.StringVar(&scaleDownAction, "scale-down-action", "", "縮容時通知的 URL（HTTP POST）")
	return cmd
}

// newVCSAspDetachCmd 建立 `vcs asp detach`：移除 server 的 auto scaling policy，需確認或 -y。
func newVCSAspDetachCmd(a *app) *cobra.Command {
	var siteRef, serverRef string
	cmd := &cobra.Command{
		Use:   "detach (--site <id|name> | --server <id>)",
		Short: "移除 server 上的 auto scaling policy（需確認或 -y）",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if err := checkAspTarget(siteRef, serverRef); err != nil {
				return err
			}
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			serverID, err := resolveAspTarget(ctx, svc, projectID, siteRef, serverRef)
			if err != nil {
				return err
			}
			if err := a.confirm(fmt.Sprintf("移除 server %d 的 auto scaling policy", serverID)); err != nil {
				return err
			}
			if err := svc.DetachAutoScalingPolicy(ctx, serverID); err != nil {
				return err
			}
			a.progress("已移除 server %d 的 auto scaling policy", serverID)
			return nil
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&siteRef, "site", "", "VCS 實例 site 的 id 或名稱（與 --server 擇一）")
	flags.StringVar(&serverRef, "server", "", "server id（與 --site 擇一）")
	return cmd
}
