package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	genvcs "github.com/gilbertchiao/twaictl/internal/gen/vcs"
	"github.com/gilbertchiao/twaictl/internal/output"
)

// firstNetIP 回傳第一個網路介面的 IP；沒有介面時回傳空字串。
func firstNetIP(nets []genvcs.ServerNetObject) string {
	if len(nets) == 0 {
		return ""
	}
	return nets[0].Ip
}

// formatServerNets 把所有網路介面格式化為 `name(ip)`，以 `, ` 串接，供 `server get` 直式輸出使用。
func formatServerNets(nets []genvcs.ServerNetObject) string {
	parts := make([]string, 0, len(nets))
	for _, n := range nets {
		parts = append(parts, fmt.Sprintf("%s(%s)", n.Name, n.Ip))
	}
	return strings.Join(parts, ", ")
}

// serverColumns 是 `vcs server ls` 的欄位定義。
var serverColumns = []output.Column[genvcs.ServerSerializer]{
	{Name: "ID", Default: true, Value: func(s genvcs.ServerSerializer) string { return fmt.Sprint(s.Id) }},
	{Name: "HOSTNAME", Default: true, Value: func(s genvcs.ServerSerializer) string { return s.Hostname }},
	{Name: "SITE", Default: true, Value: func(s genvcs.ServerSerializer) string { return fmt.Sprint(s.Site) }},
	{Name: "STATUS", Default: true, Value: func(s genvcs.ServerSerializer) string { return string(s.Status) }},
	{Name: "FLAVOR", Default: true, Value: func(s genvcs.ServerSerializer) string { return s.Flavor }},
	{Name: "IMAGE", Default: true, Value: func(s genvcs.ServerSerializer) string { return s.Image }},
	{Name: "PRIVATE IP", Default: true, Value: func(s genvcs.ServerSerializer) string { return firstNetIP(s.PrivateNets) }},
	{Name: "PUBLIC IP", Default: true, Value: func(s genvcs.ServerSerializer) string { return firstNetIP(s.PublicNets) }},
	// ServerSerializer 沒有 create_time 欄位（與 SiteSerializer 不同），所以 ls 沒有 CREATED 欄。
	{Name: "KEYPAIR", Value: func(s genvcs.ServerSerializer) string { return s.Keypair }},
	{Name: "OS", Value: func(s genvcs.ServerSerializer) string { return s.Os }},
	{Name: "AZ", Value: func(s genvcs.ServerSerializer) string { return s.AvailabilityZone }},
	{Name: "FQDN", Value: func(s genvcs.ServerSerializer) string { return s.Fqdn }},
}

// serverDetailColumns 是 `vcs server get` 直式輸出的欄位定義。
//
// 沒有 Security groups 欄位：真實 API 的 server 詳細資料含 `security_groups[]{id,name}`
// （2026-08-27 實測），但 VCS.yaml 的 ServerSerializer 沒有這個欄位，故不臆造、不顯示
// （見 CLAUDE.md 規則 5；-o json 仍會透過 API 原始回應完整保留該欄位）。
var serverDetailColumns = []output.Column[genvcs.ServerSerializer]{
	{Name: "ID", Default: true, Value: func(s genvcs.ServerSerializer) string { return fmt.Sprint(s.Id) }},
	{Name: "Hostname", Default: true, Value: func(s genvcs.ServerSerializer) string { return s.Hostname }},
	{Name: "Site", Default: true, Value: func(s genvcs.ServerSerializer) string { return fmt.Sprint(s.Site) }},
	{Name: "Status", Default: true, Value: func(s genvcs.ServerSerializer) string { return string(s.Status) }},
	{Name: "Flavor", Default: true, Value: func(s genvcs.ServerSerializer) string { return s.Flavor }},
	{Name: "Image", Default: true, Value: func(s genvcs.ServerSerializer) string { return s.Image }},
	{Name: "OS", Default: true, Value: func(s genvcs.ServerSerializer) string { return s.Os }},
	{Name: "OS version", Default: true, Value: func(s genvcs.ServerSerializer) string { return s.OsVersion }},
	{Name: "Keypair", Default: true, Value: func(s genvcs.ServerSerializer) string { return s.Keypair }},
	{Name: "Availability zone", Default: true, Value: func(s genvcs.ServerSerializer) string { return s.AvailabilityZone }},
	{Name: "FQDN", Default: true, Value: func(s genvcs.ServerSerializer) string { return s.Fqdn }},
	{Name: "Private nets", Default: true, Value: func(s genvcs.ServerSerializer) string { return formatServerNets(s.PrivateNets) }},
	{Name: "Public nets", Default: true, Value: func(s genvcs.ServerSerializer) string { return formatServerNets(s.PublicNets) }},
}

// newVCSServerCmd 是 `vcs server` 子命令的父命令。
func newVCSServerCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{Use: "server", Short: "查詢 VCS 實例底下的 server"}
	cmd.AddCommand(newVCSServerLsCmd(a), newVCSServerGetCmd(a))
	return cmd
}

// newVCSServerLsCmd 建立 `vcs server ls`。
func newVCSServerLsCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "列出 project 下的所有 server",
		Long:  "列出 project 下的所有 server。\n\n" + columnsHelp(serverColumns),
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if err := validateColumns(a, serverColumns); err != nil {
				return err
			}
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			res, err := svc.ListServers(ctx, projectID)
			if err != nil {
				return err
			}
			return render(a, res.Raw, res.Value, serverColumns)
		},
	}
}

// newVCSServerGetCmd 建立 `vcs server get`。
//
// 只接受數字 id（不像 `vcs get` 支援以名稱解析）：ServerSerializer 沒有可唯一識別的
// 名稱欄位（hostname 在同一 project 下不保證唯一），brief 也只要求 `<id>`。
func newVCSServerGetCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "顯示單一 server 的詳細資料",
		Long:  "顯示單一 server 的詳細資料。\n\n" + columnsHelp(serverDetailColumns),
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if err := validateColumns(a, serverDetailColumns); err != nil {
				return err
			}
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("server id 必須為數字（目前 %q）: %w", args[0], err)
			}
			ctx := a.commandContext(c)
			svc, err := a.vcsService()
			if err != nil {
				return err
			}
			res, err := svc.GetServer(ctx, id)
			if err != nil {
				return err
			}
			return renderOne(a, res.Raw, res.Value, serverDetailColumns)
		},
	}
}
