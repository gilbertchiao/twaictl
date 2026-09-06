package cli

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	gencommon "github.com/gilbertchiao/twaictl/internal/gen/common"
	genvcs "github.com/gilbertchiao/twaictl/internal/gen/vcs"
	"github.com/gilbertchiao/twaictl/internal/output"
	"github.com/gilbertchiao/twaictl/internal/twai"
)

// whoamiResult 是 -o json/yaml 的輸出結構。
type whoamiResult struct {
	APIVersion  string                   `json:"api_version" yaml:"api_version"`
	Profile     string                   `json:"profile" yaml:"profile"`
	ProjectCode string                   `json:"project_code" yaml:"project_code"`
	ProjectID   int64                    `json:"project_id" yaml:"project_id"`
	Projects    []genvcs.GroupSerializer `json:"projects" yaml:"projects"`
}

var projectColumns = []output.Column[genvcs.GroupSerializer]{
	{Name: "ID", Default: true, Value: func(p genvcs.GroupSerializer) string { return fmt.Sprint(p.Id) }},
	{Name: "NAME", Default: true, Value: func(p genvcs.GroupSerializer) string { return p.Name }},
	{Name: "PLATFORM", Default: true, Value: func(p genvcs.GroupSerializer) string { return p.Platform }},
	{Name: "TRANSPARENT", Default: true, Value: func(p genvcs.GroupSerializer) string { return output.Bool(p.TransparentMode) }},
}

func newConfigWhoamiCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "驗證 API key 並顯示可用的 project",
		Long: "驗證 API key，並顯示 API 版本、目前 profile/project、可用的 project 清單。\n\n" +
			"下方 project 清單的" + columnsHelp(projectColumns),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateColumns(a, projectColumns); err != nil {
				return err
			}
			ctx := a.commandContext(cmd)
			client, err := a.apiClient()
			if err != nil {
				return err
			}
			verResp, err := client.Common.GetVersionWithResponse(ctx, &gencommon.GetVersionParams{XApiHost: a.settings.APIHosts.Common})
			if err != nil {
				return fmt.Errorf("查詢 API 版本失敗: %w", err)
			}
			if err := twai.CheckResponse(verResp.HTTPResponse, verResp.Body); err != nil {
				return err
			}
			svc, err := a.vcsService()
			if err != nil {
				return err
			}
			projects, err := svc.ListProjects(ctx)
			if err != nil {
				return err
			}
			result := whoamiResult{Profile: a.settings.ProfileName, ProjectCode: a.settings.ProjectCode, Projects: projects.Value}
			if verResp.JSON200 != nil {
				result.APIVersion = verResp.JSON200.Version
			}
			for _, p := range projects.Value {
				// --project（進而 ProjectCode）可以是 project code 或數字 id
				// （見 README 對 --project 的說明），兩種都要能對到這裡列出的 project。
				if p.Name == result.ProjectCode || strconv.FormatInt(p.Id, 10) == result.ProjectCode {
					result.ProjectID = p.Id
				}
			}
			return a.renderWhoami(result)
		},
	}
}

func (a *app) renderWhoami(r whoamiResult) error {
	if a.opts.output != string(output.FormatTable) {
		raw, err := json.Marshal(r)
		if err != nil {
			return fmt.Errorf("序列化失敗: %w", err)
		}
		return output.Render(a.stdout, a.opts.outputOptions(), raw, []whoamiResult{r}, nil)
	}
	project := r.ProjectCode
	switch {
	case project == "":
		project = "（未設定）"
	case r.ProjectID == 0:
		project += "（找不到對應的 project）"
	default:
		project = fmt.Sprintf("%s (id %d)", project, r.ProjectID)
	}
	if err := output.RenderKeyValues(a.stdout, [][2]string{
		{"API version", r.APIVersion}, {"Profile", r.Profile}, {"Project", project},
	}); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(a.stdout)
	return render(a, nil, r.Projects, projectColumns)
}
