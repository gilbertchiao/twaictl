package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	genvcs "github.com/gilbertchiao/twaictl/internal/gen/vcs"
	"github.com/gilbertchiao/twaictl/internal/output"
	"github.com/gilbertchiao/twaictl/internal/twai/vcs"
)

// vcsActionTargets 是各 action 完成時，site 底下 server 應到達的狀態集合（`vcs action --wait` 用）；
// shelve 依 flavor/雲平台設定可能停在 SHELVED 或直接進到 SHELVED_OFFLOADED，兩者都視為完成。
var vcsActionTargets = map[string][]string{
	"start":    {"ACTIVE"},
	"resume":   {"ACTIVE"},
	"unshelve": {"ACTIVE"},
	"reboot":   {"ACTIVE"},
	"stop":     {"SHUTOFF"},
	"suspend":  {"SUSPENDED"},
	"shelve":   {"SHELVED_OFFLOADED", "SHELVED"},
}

// vcsDestructiveActions 是需要確認（或 -y）的破壞性動作；start/resume/unshelve 不會中斷服務，不需確認。
var vcsDestructiveActions = map[string]bool{
	"stop":    true,
	"reboot":  true,
	"suspend": true,
	"shelve":  true,
}

// actionSummary 是 `vcs action` 在 -o json/yaml 時輸出的單一物件摘要。
type actionSummary struct {
	SiteID int64  `json:"site_id"`
	Action string `json:"action"`
}

// newVCSActionCmd 建立 `vcs action`。
func newVCSActionCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "action <id|name> <" + strings.Join(vcs.SiteActions, "|") + ">",
		Short: "對 VCS 實例送出動作（啟動／停止／重開機等）",
		Long: "對 VCS 實例送出動作（啟動／停止／重開機等）。\n\n" +
			"--wait 對 reboot 的限制：送出 reboot 後 server 通常仍是 ACTIVE（重開尚未開始），" +
			"因此 --wait 會先等到觀察到 server 離開 ACTIVE、再等它回到 ACTIVE 才算完成；" +
			"若整個重開過程比 --wait-interval（預設 5s）還快、沒有輪詢到「已離開 ACTIVE」的瞬間，" +
			"會持續等到 --wait-timeout 逾時（exit 5），此時實際上 reboot 多半已經完成，只是 --wait 沒能觀察到。",
		Args: cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			action := args[1]
			// action 是位置參數、必填：不能沿用 validateEnumList 對空字串「視為未指定、合法」
			// 的語意（那是為了 --meter 這類選填 flag 設計的），否則 `vcs action 7 ""`
			// 會直接送出 {"status":""} 給 API。
			if strings.TrimSpace(action) == "" {
				return fmt.Errorf("請指定動作（%s）", strings.Join(vcs.SiteActions, ", "))
			}
			// 再驗證 action 合法性，然後才解析 project／site，避免不合法的輸入
			// 也送出不必要的解析請求（見 TestVCSActionStartNoConfirmAndRejectsUnknown）。
			if err := validateEnumList("action", action, vcs.SiteActions); err != nil {
				return err
			}
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			id, err := svc.ResolveSiteID(ctx, projectID, args[0])
			if err != nil {
				return err
			}
			if vcsDestructiveActions[action] {
				if err := a.confirm(fmt.Sprintf("對 VCS 實例 %d 執行 %s", id, action)); err != nil {
					return err
				}
			}
			if err := svc.SiteAction(ctx, id, action); err != nil {
				return err
			}
			a.progress("已送出 %s 請求：%d", action, id)

			if a.opts.wait {
				waitCtx, cancel := a.waitContext(ctx)
				defer cancel()
				// reboot 送出後 server 通常仍是 ACTIVE（尚未真的開始重開），必須先觀察到
				// 離開 ACTIVE 才能判定完成，否則第一次輪詢就會誤判為已完成（見
				// WaitServerStatus 的 mustLeave 說明）；其他動作不需要這個保護。
				var mustLeave string
				if action == "reboot" {
					mustLeave = "ACTIVE"
				}
				err := svc.WaitServerStatus(waitCtx, id, vcsActionTargets[action], mustLeave, a.opts.waitInterval, func(st string) {
					a.progress("VCS 實例 %d server 狀態：%s", id, st)
				})
				if err != nil {
					// -o json/yaml 會讓上面的 progress 靜音，此時這行錯誤訊息是唯一能
					// 得知是哪個實例、哪個動作尚未完成的線索。
					return fmt.Errorf("VCS 實例 %d 尚未完成 %s: %w", id, action, err)
				}
				// server 到達目標狀態後 site 狀態仍會停留在 Stopping／Starting 等過渡狀態
				// 一段時間，期間 API 拒絕下一個動作（「Only Ready, NotReady site can be …」），
				// 因此再等 site 進入 Ready／NotReady 才算完成（見 docs/api-notes.md A39）。
				err = svc.WaitSiteSettled(waitCtx, id, a.opts.waitInterval, func(st string) {
					a.progress("VCS 實例 %d 狀態：%s", id, st)
				})
				if err != nil {
					return fmt.Errorf("VCS 實例 %d 尚未完成 %s: %w", id, action, err)
				}
			}

			return renderSummaryOne(a, actionSummary{SiteID: id, Action: action})
		},
	}
}

// eventColumns 是 `vcs events` 的欄位定義。
var eventColumns = []output.Column[genvcs.SiteEventLogSerializer]{
	{Name: "TIME", Default: true, Value: func(e genvcs.SiteEventLogSerializer) string { return output.FormatTime(e.EventTime) }},
	{Name: "STATUS", Default: true, Value: func(e genvcs.SiteEventLogSerializer) string { return e.Status }},
	{Name: "RESOURCE", Default: true, Value: func(e genvcs.SiteEventLogSerializer) string { return e.ResourceName }},
	{Name: "REASON", Default: true, Value: func(e genvcs.SiteEventLogSerializer) string { return e.StatusReason }},
}

// newVCSEventsCmd 建立 `vcs events`。
func newVCSEventsCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "events <id|name>",
		Short: "顯示 VCS 實例的事件紀錄",
		Long:  "顯示 VCS 實例的事件紀錄。\n\n" + columnsHelp(eventColumns),
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if err := validateColumns(a, eventColumns); err != nil {
				return err
			}
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			id, err := svc.ResolveSiteID(ctx, projectID, args[0])
			if err != nil {
				return err
			}
			res, err := svc.SiteEventLogs(ctx, id)
			if err != nil {
				return err
			}
			return render(a, res.Raw, res.Value, eventColumns)
		},
	}
}

// metricColumns 是 `vcs metrics` 非 disk meter（cpu/memory/net-in/net-out）的欄位定義。
var metricColumns = []output.Column[vcs.MetricPoint]{
	{Name: "TIME", Default: true, Value: func(m vcs.MetricPoint) string { return output.FormatTime(m.Timestamp) }},
	{Name: "VALUE", Default: true, Value: func(m vcs.MetricPoint) string { return m.Value }},
	{Name: "UNIT", Default: true, Value: func(m vcs.MetricPoint) string { return m.Unit }},
}

// diskMetricColumns 是 `vcs metrics --meter disk-read|disk-write` 的欄位定義；真實回應是
// 每個裝置各自一組資料（見 docs/api-notes.md A22），比其他 meter 多一個 DEVICE 欄位。
var diskMetricColumns = []output.Column[vcs.MetricPoint]{
	{Name: "DEVICE", Default: true, Value: func(m vcs.MetricPoint) string { return m.Device }},
	{Name: "TIME", Default: true, Value: func(m vcs.MetricPoint) string { return output.FormatTime(m.Timestamp) }},
	{Name: "VALUE", Default: true, Value: func(m vcs.MetricPoint) string { return m.Value }},
	{Name: "UNIT", Default: true, Value: func(m vcs.MetricPoint) string { return m.Unit }},
}

// metricColumnsFor 依 meter 選擇欄位定義：disk-read/disk-write 多一個 DEVICE 欄位。
func metricColumnsFor(meter string) []output.Column[vcs.MetricPoint] {
	if meter == "disk-read" || meter == "disk-write" {
		return diskMetricColumns
	}
	return metricColumns
}

// newVCSMetricsCmd 建立 `vcs metrics`。
func newVCSMetricsCmd(a *app) *cobra.Command {
	var serverRef, meter, begin, end string
	cmd := &cobra.Command{
		Use:   "metrics <id|name>",
		Short: "查詢 VCS 實例底下 server 的用量指標",
		Long: "查詢 VCS 實例底下 server 的用量指標。\n\n" + columnsHelp(metricColumns) +
			"\n\n--meter disk-read/disk-write 時（多一個 DEVICE 欄位）：\n" + columnsHelp(diskMetricColumns) +
			"\n\n--begin/--end 請用 RFC3339 格式（例如 2026-08-27T12:00:00Z）；" +
			"未指定時部分 meter 會回傳全部歷史資料（筆數可能很多）。\n\n" +
			"注意：net-in 目前伺服器端會回 500（API 自身的已知問題，非本工具造成，" +
			"見 docs/api-notes.md A22），twaictl 會照實回報錯誤。",
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			columns := metricColumnsFor(meter)
			if err := validateColumns(a, columns); err != nil {
				return err
			}
			// --meter 有非空預設值（"cpu"），空字串只有使用者明確傳入 --meter "" 才會出現；
			// validateEnumList 對空字串視為「未指定、合法」是給選填 flag 用的語意，這裡必須
			// 明確拒絕，否則會直接送出一個四種 meter 都不符合的請求。
			if strings.TrimSpace(meter) == "" {
				return fmt.Errorf("--meter 的值必須是下列其中之一：%s（目前為 \"\"）", strings.Join(vcs.Meters, ", "))
			}
			if err := validateEnumList("--meter", meter, vcs.Meters); err != nil {
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
			res, err := svc.Metrics(ctx, serverID, meter, begin, end)
			if err != nil {
				return err
			}
			if len(res.Value) == 0 {
				a.progress("沒有資料（server 可能未執行）")
			}
			return render(a, res.Raw, res.Value, columns)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&serverRef, "server", "", "指定 server id（未指定時由 site 自動解析，僅限剛好一台 server 的 site）")
	flags.StringVar(&meter, "meter", "cpu", "指標類型："+strings.Join(vcs.Meters, "|"))
	flags.StringVar(&begin, "begin", "", "查詢開始時間（RFC3339，例如 2026-08-27T12:00:00Z）")
	flags.StringVar(&end, "end", "", "查詢結束時間（RFC3339，例如 2026-08-27T12:00:00Z）")
	return cmd
}
