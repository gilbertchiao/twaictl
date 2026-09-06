package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	genvcs "github.com/gilbertchiao/twaictl/internal/gen/vcs"
	"github.com/gilbertchiao/twaictl/internal/output"
)

var projectDetailColumns = []output.Column[genvcs.GroupDetailSerializer]{
	{Name: "ID", Default: true, Value: func(p genvcs.GroupDetailSerializer) string { return fmt.Sprint(p.Id) }},
	{Name: "Name", Default: true, Value: func(p genvcs.GroupDetailSerializer) string { return p.Name }},
	{Name: "Status", Default: true, Value: func(p genvcs.GroupDetailSerializer) string { return p.Status }},
	{Name: "Description", Default: true, Value: func(p genvcs.GroupDetailSerializer) string { return p.Desc }},
	{Name: "UUID", Default: true, Value: func(p genvcs.GroupDetailSerializer) string { return p.Uuid.String() }},
	{Name: "Transparent", Default: true, Value: func(p genvcs.GroupDetailSerializer) string { return output.Bool(p.TransparentMode) }},
}

func newProjectCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{Use: "project", Short: "查詢 project"}
	cmd.AddCommand(&cobra.Command{
		Use:   "ls",
		Short: "列出可用的 project",
		Long:  "列出可用的 project。\n\n" + columnsHelp(projectColumns),
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if err := validateColumns(a, projectColumns); err != nil {
				return err
			}
			svc, err := a.vcsService()
			if err != nil {
				return err
			}
			res, err := svc.ListProjects(a.commandContext(c))
			if err != nil {
				return err
			}
			return render(a, res.Raw, res.Value, projectColumns)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "get <id|code>",
		Short: "顯示單一 project 的詳細資料",
		Long:  "顯示單一 project 的詳細資料。\n\n" + columnsHelp(projectDetailColumns),
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if err := validateColumns(a, projectDetailColumns); err != nil {
				return err
			}
			ctx := a.commandContext(c)
			svc, err := a.vcsService()
			if err != nil {
				return err
			}
			id, err := svc.ResolveProjectID(ctx, args[0])
			if err != nil {
				return err
			}
			res, err := svc.GetProject(ctx, id)
			if err != nil {
				return err
			}
			return renderOne(a, res.Raw, res.Value, projectDetailColumns)
		},
	})
	cmd.AddCommand(newProjectQuotaCmd(a), newProjectSolutionsCmd(a))
	return cmd
}
