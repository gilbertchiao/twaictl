package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/gilbertchiao/twaictl/internal/version"
)

// versionInfoFunc 可在測試中替換，讓輸出不受實際 build 變數影響。
var versionInfoFunc = version.Get

func newVersionCmd(opts *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "顯示 twaictl 版本、commit、build 時間與依據的 OpenAPI 版本",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			info := versionInfoFunc()
			switch opts.output {
			case "json":
				data, err := json.MarshalIndent(info, "", "  ")
				if err != nil {
					return fmt.Errorf("序列化版本資訊為 JSON 失敗: %w", err)
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			case "yaml":
				data, err := yaml.Marshal(info)
				if err != nil {
					return fmt.Errorf("序列化版本資訊為 YAML 失敗: %w", err)
				}
				_, _ = fmt.Fprint(cmd.OutOrStdout(), string(data))
			default:
				_, _ = fmt.Fprint(cmd.OutOrStdout(), info.String())
			}
			return nil
		},
	}
}
