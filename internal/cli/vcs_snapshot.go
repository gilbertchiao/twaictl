package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	genvcs "github.com/gilbertchiao/twaictl/internal/gen/vcs"
	"github.com/gilbertchiao/twaictl/internal/output"
)

// snapshotVolumeName 回傳 snapshot 來源 volume 的名稱；缺少時回傳空字串。
func snapshotVolumeName(s genvcs.SnapshotSerializer) string {
	return output.Str(s.Volume.Name)
}

// snapshotVolumeID 回傳 snapshot 來源 volume 的 id；缺少時回傳空字串。
func snapshotVolumeID(s genvcs.SnapshotSerializer) string {
	return output.Int64(s.Volume.Id)
}

// snapshotColumns 是 `vcs snapshot ls` 的欄位定義。
var snapshotColumns = []output.Column[genvcs.SnapshotSerializer]{
	{Name: "ID", Default: true, Value: func(s genvcs.SnapshotSerializer) string { return fmt.Sprint(s.Id) }},
	{Name: "NAME", Default: true, Value: func(s genvcs.SnapshotSerializer) string { return s.Name }},
	{Name: "STATUS", Default: true, Value: func(s genvcs.SnapshotSerializer) string { return s.Status }},
	{Name: "VOLUME", Default: true, Value: snapshotVolumeName},
	{Name: "CREATED", Default: true, Value: func(s genvcs.SnapshotSerializer) string { return output.FormatTime(s.CreateTime) }},
	{Name: "DESC", Default: true, Value: func(s genvcs.SnapshotSerializer) string { return s.Desc }},
}

// snapshotDetailColumns 是 `vcs snapshot get` 直式輸出的欄位定義。
var snapshotDetailColumns = []output.Column[genvcs.SnapshotSerializer]{
	{Name: "ID", Default: true, Value: func(s genvcs.SnapshotSerializer) string { return fmt.Sprint(s.Id) }},
	{Name: "Name", Default: true, Value: func(s genvcs.SnapshotSerializer) string { return s.Name }},
	{Name: "Status", Default: true, Value: func(s genvcs.SnapshotSerializer) string { return s.Status }},
	{Name: "Volume", Default: true, Value: snapshotVolumeName},
	{Name: "Volume ID", Default: true, Value: snapshotVolumeID},
	{Name: "Created", Default: true, Value: func(s genvcs.SnapshotSerializer) string { return output.FormatTime(s.CreateTime) }},
	{Name: "Desc", Default: true, Value: func(s genvcs.SnapshotSerializer) string { return s.Desc }},
}

// newVCSSnapshotCmd 建立 `vcs snapshot` 子命令。
func newVCSSnapshotCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{Use: "snapshot", Short: "管理 VCS volume snapshot"}
	cmd.AddCommand(newVCSSnapshotLsCmd(a), newVCSSnapshotGetCmd(a), newVCSSnapshotCreateCmd(a), newVCSSnapshotRmCmd(a))
	return cmd
}

// newVCSSnapshotLsCmd 建立 `vcs snapshot ls`。
func newVCSSnapshotLsCmd(a *app) *cobra.Command {
	var status string
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "列出 snapshot",
		Long:  "列出 snapshot。\n\n" + columnsHelp(snapshotColumns),
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if err := validateColumns(a, snapshotColumns); err != nil {
				return err
			}
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			res, err := svc.ListSnapshots(ctx, projectID, status)
			if err != nil {
				return err
			}
			return render(a, res.Raw, res.Value, snapshotColumns)
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "以狀態篩選")
	return cmd
}

// newVCSSnapshotGetCmd 建立 `vcs snapshot get`。
func newVCSSnapshotGetCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id|name>",
		Short: "顯示單一 snapshot 的詳細資料",
		Long:  "顯示單一 snapshot 的詳細資料。\n\n" + columnsHelp(snapshotDetailColumns),
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if err := validateColumns(a, snapshotDetailColumns); err != nil {
				return err
			}
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			id, err := svc.ResolveSnapshotID(ctx, projectID, args[0])
			if err != nil {
				return err
			}
			res, err := svc.GetSnapshot(ctx, id)
			if err != nil {
				return err
			}
			return renderOne(a, res.Raw, res.Value, snapshotDetailColumns)
		},
	}
}

// newVCSSnapshotCreateCmd 建立 `vcs snapshot create`。
func newVCSSnapshotCreateCmd(a *app) *cobra.Command {
	var name, volumeRef, desc string
	cmd := &cobra.Command{
		Use:   "create --name <n> --volume <id|name>",
		Short: "建立 snapshot",
		Long:  "建立 snapshot。\n\n" + columnsHelp(snapshotDetailColumns),
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if err := validateColumns(a, snapshotDetailColumns); err != nil {
				return err
			}
			if err := requireFlag("--name", name); err != nil {
				return err
			}
			if err := requireFlag("--volume", volumeRef); err != nil {
				return err
			}
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			volumeID, err := svc.ResolveVolumeID(ctx, projectID, volumeRef)
			if err != nil {
				return err
			}
			res, err := svc.CreateSnapshot(ctx, name, volumeID, desc)
			if err != nil {
				return err
			}
			a.progress("已建立 snapshot %d", res.Value.Id)
			return renderOne(a, res.Raw, res.Value, snapshotDetailColumns)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&name, "name", "", "snapshot 名稱（必填）")
	flags.StringVar(&volumeRef, "volume", "", "來源 volume 的 id 或名稱（必填）")
	flags.StringVar(&desc, "desc", "", "描述")
	return cmd
}

// newVCSSnapshotRmCmd 建立 `vcs snapshot rm`：破壞性操作，需確認或 -y。
func newVCSSnapshotRmCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "rm <id|name>...",
		Short: "刪除 snapshot（破壞性操作，需確認或 -y）",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			ids := make([]int64, 0, len(args))
			for _, ref := range args {
				id, err := svc.ResolveSnapshotID(ctx, projectID, ref)
				if err != nil {
					return err
				}
				ids = append(ids, id)
			}
			if err := a.confirm(fmt.Sprintf("刪除 snapshot %s", joinIDs(ids))); err != nil {
				return err
			}
			for _, id := range ids {
				if err := svc.DeleteSnapshot(ctx, id); err != nil {
					return err
				}
				a.progress("已刪除 snapshot %d", id)
			}
			return nil
		},
	}
}
