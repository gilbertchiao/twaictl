package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	genvcs "github.com/gilbertchiao/twaictl/internal/gen/vcs"
	"github.com/gilbertchiao/twaictl/internal/output"
	"github.com/gilbertchiao/twaictl/internal/twai/vcs"
)

// imageColumns 是 `vcs image ls` 的欄位定義。
var imageColumns = []output.Column[genvcs.ImageSerializer]{
	{Name: "ID", Default: true, Value: func(i genvcs.ImageSerializer) string { return fmt.Sprint(i.Id) }},
	{Name: "NAME", Default: true, Value: func(i genvcs.ImageSerializer) string { return i.Name }},
	{Name: "STATUS", Default: true, Value: func(i genvcs.ImageSerializer) string { return i.Status }},
	{Name: "SIZE", Default: true, Value: func(i genvcs.ImageSerializer) string { return output.Bytes(i.Size) }},
	{Name: "PUBLIC", Default: true, Value: func(i genvcs.ImageSerializer) string { return output.Bool(i.IsPublic) }},
	{Name: "CREATED", Default: true, Value: func(i genvcs.ImageSerializer) string { return output.FormatTime(i.CreateTime) }},
	{Name: "SERVER", Value: func(i genvcs.ImageSerializer) string { return i.Server.Hostname }},
	{Name: "DESCRIPTION", Value: func(i genvcs.ImageSerializer) string { return output.Str(i.Desc) }},
	{Name: "USER", Value: imageUser},
}

// imageDetailColumns 是 `vcs image get` 直式輸出的欄位定義。
var imageDetailColumns = []output.Column[genvcs.ImageDetailSerializer]{
	{Name: "ID", Default: true, Value: func(i genvcs.ImageDetailSerializer) string { return fmt.Sprint(i.Id) }},
	{Name: "Name", Default: true, Value: func(i genvcs.ImageDetailSerializer) string { return i.Name }},
	{Name: "Status", Default: true, Value: func(i genvcs.ImageDetailSerializer) string { return i.Status }},
	{Name: "OS", Default: true, Value: func(i genvcs.ImageDetailSerializer) string { return i.Os }},
	{Name: "OS version", Default: true, Value: func(i genvcs.ImageDetailSerializer) string { return i.OsVersion }},
	{Name: "Size", Default: true, Value: func(i genvcs.ImageDetailSerializer) string { return output.Bytes(i.Size) }},
	{Name: "Public", Default: true, Value: func(i genvcs.ImageDetailSerializer) string { return output.Bool(i.IsPublic) }},
	{Name: "Enabled", Default: true, Value: func(i genvcs.ImageDetailSerializer) string { return output.Bool(i.IsEnabled) }},
	{Name: "Server", Default: true, Value: func(i genvcs.ImageDetailSerializer) string { return i.Server.Hostname }},
	{Name: "Created", Default: true, Value: func(i genvcs.ImageDetailSerializer) string { return output.FormatTime(i.CreateTime) }},
	{Name: "Desc", Default: true, Value: func(i genvcs.ImageDetailSerializer) string { return i.Desc }},
}

// imageSavedColumns 是 `vcs image save` 成功送出後的直式輸出欄位定義。
var imageSavedColumns = []output.Column[genvcs.ImagePostSerializer]{
	{Name: "ID", Default: true, Value: func(i genvcs.ImagePostSerializer) string { return fmt.Sprint(i.Id) }},
	{Name: "Name", Default: true, Value: func(i genvcs.ImagePostSerializer) string { return i.Name }},
	{Name: "Platform", Default: true, Value: func(i genvcs.ImagePostSerializer) string { return i.Platform }},
	{Name: "Public", Default: true, Value: func(i genvcs.ImagePostSerializer) string { return output.Bool(i.IsPublic) }},
	{Name: "Created", Default: true, Value: func(i genvcs.ImagePostSerializer) string { return output.FormatTime(i.CreateTime) }},
}

// imageUser 回傳 image 擁有者的 username；User 為 nil 時回傳空字串。
func imageUser(i genvcs.ImageSerializer) string {
	if i.User == nil {
		return ""
	}
	return i.User.Username
}

// newVCSImageCmd 建立 `vcs image` 子命令。
func newVCSImageCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{Use: "image", Short: "管理 VCS image"}
	cmd.AddCommand(newVCSImageLsCmd(a), newVCSImageGetCmd(a), newVCSImageRmCmd(a), newVCSImageSaveCmd(a))
	return cmd
}

// newVCSImageLsCmd 建立 `vcs image ls`。
func newVCSImageLsCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "列出 image",
		Long:  "列出 image。\n\n" + columnsHelp(imageColumns),
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if err := validateColumns(a, imageColumns); err != nil {
				return err
			}
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			res, err := svc.ListImages(ctx, projectID)
			if err != nil {
				return err
			}
			return render(a, res.Raw, res.Value, imageColumns)
		},
	}
}

// newVCSImageGetCmd 建立 `vcs image get`。
func newVCSImageGetCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id|name>",
		Short: "顯示單一 image 的詳細資料",
		Long:  "顯示單一 image 的詳細資料。\n\n" + columnsHelp(imageDetailColumns),
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if err := validateColumns(a, imageDetailColumns); err != nil {
				return err
			}
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			id, err := svc.ResolveImageID(ctx, projectID, args[0])
			if err != nil {
				return err
			}
			res, err := svc.GetImage(ctx, id)
			if err != nil {
				return err
			}
			return renderOne(a, res.Raw, res.Value, imageDetailColumns)
		},
	}
}

// newVCSImageRmCmd 建立 `vcs image rm`：破壞性操作，需確認或 -y。
func newVCSImageRmCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "rm <id|name>...",
		Short: "刪除 image（破壞性操作，需確認或 -y）",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			ids := make([]int64, 0, len(args))
			for _, ref := range args {
				id, err := svc.ResolveImageID(ctx, projectID, ref)
				if err != nil {
					return err
				}
				ids = append(ids, id)
			}
			if err := a.confirm(fmt.Sprintf("刪除 image %s", joinIDs(ids))); err != nil {
				return err
			}
			for _, id := range ids {
				if err := svc.DeleteImage(ctx, id); err != nil {
					return err
				}
				a.progress("已刪除 image %d", id)
			}
			return nil
		},
	}
}

// newVCSImageSaveCmd 建立 `vcs image save`：把 VCS 實例（site）底下 server 目前的狀態
// 存成新 image。site 解析為 server id 的規則同 `vcs metrics`：預設從 site 底下唯一的
// server 自動解析，指定 --server 時直接使用（跳過 site → server 的自動解析）。
func newVCSImageSaveCmd(a *app) *cobra.Command {
	var name, osName, osVersion, desc, serverRef string
	cmd := &cobra.Command{
		Use:   "save <site id|name>",
		Short: "把 VCS 實例目前的狀態存成新 image",
		Long:  "把 VCS 實例目前的狀態存成新 image。\n\n" + columnsHelp(imageSavedColumns),
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if err := validateColumns(a, imageSavedColumns); err != nil {
				return err
			}
			if err := requireFlag("--name", name); err != nil {
				return err
			}
			// spec 把 os／os_version 標為選填，但真實 API 缺任一個都回 400
			// 「This field is required.」（見 docs/api-notes.md A40），因此在送出前就擋下。
			if err := requireFlag("--os", osName); err != nil {
				return err
			}
			if err := requireFlag("--os-version", osVersion); err != nil {
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
			res, err := svc.SaveImage(ctx, vcs.SaveImageInput{
				ServerID: serverID, Name: name, OS: osName, OSVersion: osVersion, Desc: desc,
			})
			if err != nil {
				return err
			}
			a.progress("已送出建立 image 請求")
			return renderOne(a, res.Raw, res.Value, imageSavedColumns)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&name, "name", "", "image 名稱（必填）")
	flags.StringVar(&osName, "os", "", "作業系統（必填，例如 Linux；spec 標選填但 API 實際要求）")
	flags.StringVar(&osVersion, "os-version", "", "作業系統版本（必填，例如 \"Ubuntu 24.04\"；spec 標選填但 API 實際要求）")
	flags.StringVar(&desc, "desc", "", "image 描述")
	flags.StringVar(&serverRef, "server", "", "指定 server id（未指定時由 site 自動解析，僅限剛好一台 server 的 site）")
	return cmd
}
