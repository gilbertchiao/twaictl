package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	genvcs "github.com/gilbertchiao/twaictl/internal/gen/vcs"
	"github.com/gilbertchiao/twaictl/internal/output"
	"github.com/gilbertchiao/twaictl/internal/twai/vcs"
)

// volumeAttachedHostname 回傳 volume 目前掛載的 server hostname；未掛載（attached_host
// 為零值）時回傳空字串。
func volumeAttachedHostname(v genvcs.VolumeSerializer) string {
	return v.AttachedHost.Hostname
}

// volumeColumns 是 `vcs volume ls` 的欄位定義。
var volumeColumns = []output.Column[genvcs.VolumeSerializer]{
	{Name: "ID", Default: true, Value: func(v genvcs.VolumeSerializer) string { return fmt.Sprint(v.Id) }},
	{Name: "NAME", Default: true, Value: func(v genvcs.VolumeSerializer) string { return v.Name }},
	{Name: "SIZE (GB)", Default: true, Value: func(v genvcs.VolumeSerializer) string { return fmt.Sprint(v.Size) }},
	{Name: "TYPE", Default: true, Value: func(v genvcs.VolumeSerializer) string { return output.Str(v.VolumeType) }},
	{Name: "STATUS", Default: true, Value: func(v genvcs.VolumeSerializer) string { return v.Status }},
	{Name: "ATTACHED", Default: true, Value: volumeAttachedHostname},
	{Name: "BOOTABLE", Default: true, Value: func(v genvcs.VolumeSerializer) string { return output.Bool(v.IsBootable) }},
	{Name: "CREATED", Default: true, Value: func(v genvcs.VolumeSerializer) string { return output.FormatTime(v.CreateTime) }},
}

// volumeDetailColumns 是 `vcs volume get` 直式輸出的欄位定義。
var volumeDetailColumns = []output.Column[genvcs.VolumeSerializer]{
	{Name: "ID", Default: true, Value: func(v genvcs.VolumeSerializer) string { return fmt.Sprint(v.Id) }},
	{Name: "Name", Default: true, Value: func(v genvcs.VolumeSerializer) string { return v.Name }},
	{Name: "Size (GB)", Default: true, Value: func(v genvcs.VolumeSerializer) string { return fmt.Sprint(v.Size) }},
	{Name: "Type", Default: true, Value: func(v genvcs.VolumeSerializer) string { return output.Str(v.VolumeType) }},
	{Name: "Status", Default: true, Value: func(v genvcs.VolumeSerializer) string { return v.Status }},
	{Name: "Attached", Default: true, Value: volumeAttachedHostname},
	{Name: "Mountpoint", Default: true, Value: func(v genvcs.VolumeSerializer) string { return strings.Join(v.Mountpoint, ", ") }},
	{Name: "Bootable", Default: true, Value: func(v genvcs.VolumeSerializer) string { return output.Bool(v.IsBootable) }},
	{Name: "UUID", Default: true, Value: func(v genvcs.VolumeSerializer) string { return output.Str(v.VolumeUuid) }},
	{Name: "Project", Default: true, Value: func(v genvcs.VolumeSerializer) string { return v.Project.Name }},
	{Name: "Created", Default: true, Value: func(v genvcs.VolumeSerializer) string { return output.FormatTime(v.CreateTime) }},
}

// volumeCreatedColumns 是 `vcs volume create` 成功送出後的直式輸出欄位定義。
var volumeCreatedColumns = []output.Column[vcs.VolumeCreated]{
	{Name: "ID", Default: true, Value: func(v vcs.VolumeCreated) string { return fmt.Sprint(v.ID) }},
	{Name: "Name", Default: true, Value: func(v vcs.VolumeCreated) string { return v.Name }},
	{Name: "Platform", Default: true, Value: func(v vcs.VolumeCreated) string { return v.Platform }},
	{Name: "Size (GB)", Default: true, Value: func(v vcs.VolumeCreated) string { return fmt.Sprint(v.SizeGB) }},
	{Name: "Type", Default: true, Value: func(v vcs.VolumeCreated) string { return v.VolumeType }},
}

// volumeActionColumns 是 `vcs volume action` 成功送出後的直式輸出欄位定義。
var volumeActionColumns = []output.Column[vcs.VolumeActionResult]{
	{Name: "Volume ID", Default: true, Value: func(r vcs.VolumeActionResult) string { return string(r.VolumeID) }},
	{Name: "Server ID", Default: true, Value: func(r vcs.VolumeActionResult) string { return string(r.ServerID) }},
	{Name: "Mountpoint", Default: true, Value: func(r vcs.VolumeActionResult) string { return r.Mountpoint }},
}

// volumeActionSummary 是 `vcs volume action` 在 API 未回 body 時（detach）-o json/yaml 的輸出。
type volumeActionSummary struct {
	VolumeID int64  `json:"volume_id"`
	Action   string `json:"action"`
}

// newVCSVolumeCmd 建立 `vcs volume` 子命令。
func newVCSVolumeCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{Use: "volume", Short: "管理 VCS volume（磁碟）"}
	cmd.AddCommand(newVCSVolumeLsCmd(a), newVCSVolumeGetCmd(a), newVCSVolumeCreateCmd(a), newVCSVolumeRmCmd(a), newVCSVolumeActionCmd(a))
	return cmd
}

// newVCSVolumeLsCmd 建立 `vcs volume ls`。
func newVCSVolumeLsCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "列出 volume",
		Long:  "列出 volume。\n\n" + columnsHelp(volumeColumns),
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if err := validateColumns(a, volumeColumns); err != nil {
				return err
			}
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			res, err := svc.ListVolumes(ctx, projectID)
			if err != nil {
				return err
			}
			return render(a, res.Raw, res.Value, volumeColumns)
		},
	}
}

// newVCSVolumeGetCmd 建立 `vcs volume get`。
func newVCSVolumeGetCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id|name>",
		Short: "顯示單一 volume 的詳細資料",
		Long:  "顯示單一 volume 的詳細資料。\n\n" + columnsHelp(volumeDetailColumns),
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if err := validateColumns(a, volumeDetailColumns); err != nil {
				return err
			}
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			id, err := svc.ResolveVolumeID(ctx, projectID, args[0])
			if err != nil {
				return err
			}
			res, err := svc.GetVolume(ctx, id)
			if err != nil {
				return err
			}
			return renderOne(a, res.Raw, res.Value, volumeDetailColumns)
		},
	}
}

// newVCSVolumeCreateCmd 建立 `vcs volume create`。
func newVCSVolumeCreateCmd(a *app) *cobra.Command {
	var name, volumeType, desc string
	var sizeGB int
	cmd := &cobra.Command{
		Use:   "create --name <n> --size <GB>",
		Short: "建立 volume",
		Long:  "建立 volume。\n\n" + columnsHelp(volumeCreatedColumns),
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if err := validateColumns(a, volumeCreatedColumns); err != nil {
				return err
			}
			if err := requireFlag("--name", name); err != nil {
				return err
			}
			if sizeGB <= 0 {
				return fmt.Errorf("--size 必須大於 0（目前 %d）", sizeGB)
			}
			if err := validateEnumFlag("--type", volumeType, vcsVolumeTypes); err != nil {
				return err
			}
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			res, err := svc.CreateVolume(ctx, vcs.CreateVolumeInput{
				ProjectID: projectID, Name: name, SizeGB: int64(sizeGB), VolumeType: volumeType, Desc: desc,
			})
			if err != nil {
				return err
			}
			a.progress("已建立 volume %d", res.Value.ID)
			return renderOne(a, res.Raw, res.Value, volumeCreatedColumns)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&name, "name", "", "volume 名稱（必填）")
	flags.IntVar(&sizeGB, "size", 0, "容量（GB，必填，必須大於 0）")
	flags.StringVar(&volumeType, "type", "", "volume 類型：hdd|ssd|LUKS-hdd|LUKS-ssd")
	flags.StringVar(&desc, "desc", "", "描述")
	return cmd
}

// newVCSVolumeRmCmd 建立 `vcs volume rm`：破壞性操作，需確認或 -y。
func newVCSVolumeRmCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "rm <id|name>...",
		Short: "刪除 volume（破壞性操作，需確認或 -y）",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			ids := make([]int64, 0, len(args))
			for _, ref := range args {
				id, err := svc.ResolveVolumeID(ctx, projectID, ref)
				if err != nil {
					return err
				}
				ids = append(ids, id)
			}
			if err := a.confirm(fmt.Sprintf("刪除 volume %s", joinIDs(ids))); err != nil {
				return err
			}
			for _, id := range ids {
				if err := svc.DeleteVolume(ctx, id); err != nil {
					return err
				}
				a.progress("已刪除 volume %d", id)
			}
			return nil
		},
	}
}

// newVCSVolumeActionCmd 建立 `vcs volume action`：attach/detach 需以 --site（解析成
// server id）或 --server 指定目標；extend 需 --size 指定新容量；detach 可能影響執行中
// 的 VM，需確認或 -y。
func newVCSVolumeActionCmd(a *app) *cobra.Command {
	var siteRef, serverRef string
	var sizeGB int
	cmd := &cobra.Command{
		Use:   "action <id|name> <" + strings.Join(vcs.VolumeActions, "|") + ">",
		Short: "對 volume 執行 attach／detach／extend",
		Long:  "對 volume 執行 attach／detach／extend。\n\n" + columnsHelp(volumeActionColumns),
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			if err := validateColumns(a, volumeActionColumns); err != nil {
				return err
			}
			action := args[1]
			// action 是位置參數、必填：不能沿用 validateEnumList 對空字串「視為未指定、合法」
			// 的語意，否則 `vcs volume action 7 ""` 會直接送出 {"status":""} 給 API
			// （做法同 vcs_action.go 的 newVCSActionCmd）。
			if strings.TrimSpace(action) == "" {
				return fmt.Errorf("請指定動作（%s）", strings.Join(vcs.VolumeActions, ", "))
			}
			if err := validateEnumList("action", action, vcs.VolumeActions); err != nil {
				return err
			}
			// preflightServerID 是 --server 在 pre-flight 階段（尚未呼叫任何 API 之前）解析
			// 出來的值：attach/detach 且指定 --server 時才會用到，siteRef 分支則留給稍後
			// 解析 project 之後才查 API。
			var preflightServerID int64
			switch action {
			case "attach", "detach":
				if siteRef == "" && serverRef == "" {
					return fmt.Errorf("attach/detach 需指定 --site 或 --server")
				}
				if siteRef != "" && serverRef != "" {
					return fmt.Errorf("--site 與 --server 只能擇一")
				}
				// parseServerFlag 對空字串回傳 0（VolumeAction 把 serverID == 0 當「未指定」
				// 而省略 server 欄位），非空時要求正整數：--server 0 若放行，會讓 attach/detach
				// 靜默送出沒有 server 的請求，負數同樣不是合法 server id，兩者都要在這裡
				// （送出任何 API 請求之前）擋下。
				var parseErr error
				preflightServerID, parseErr = parseServerFlag(serverRef)
				if parseErr != nil {
					return parseErr
				}
			case "extend":
				if sizeGB <= 0 {
					return fmt.Errorf("extend 需指定 --size（必須大於 0）")
				}
			}
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			id, err := svc.ResolveVolumeID(ctx, projectID, args[0])
			if err != nil {
				return err
			}
			var serverID int64
			if action == "attach" || action == "detach" {
				if serverRef != "" {
					serverID = preflightServerID
				} else {
					siteID, siteErr := svc.ResolveSiteID(ctx, projectID, siteRef)
					if siteErr != nil {
						return siteErr
					}
					serverID, err = svc.ResolveServerIDForSite(ctx, projectID, siteID)
					if err != nil {
						return err
					}
				}
			}
			if action == "detach" {
				if err := a.confirm(fmt.Sprintf("對 volume %d 執行 detach（可能影響執行中的 VM）", id)); err != nil {
					return err
				}
			}
			var size int64
			if action == "extend" {
				size = int64(sizeGB)
			}
			res, err := svc.VolumeAction(ctx, id, action, serverID, size)
			if err != nil {
				return err
			}
			a.progress("已對 volume %d 執行 %s", id, action)
			if len(res.Raw) == 0 {
				// detach 成功時 API 不回 body（見 docs/api-notes.md A38）：table 模式只印上面的
				// progress，-o json/yaml 則輸出摘要，讓腳本仍有可解析的結果。
				return renderSummaryOne(a, volumeActionSummary{VolumeID: id, Action: action})
			}
			return renderOne(a, res.Raw, res.Value, volumeActionColumns)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&siteRef, "site", "", "VCS 實例 site 的 id 或名稱（attach/detach 用，與 --server 擇一）")
	flags.StringVar(&serverRef, "server", "", "server id（attach/detach 用，與 --site 擇一）")
	flags.IntVar(&sizeGB, "size", 0, "擴充後容量（GB，extend 用，必須大於 0）")
	return cmd
}
