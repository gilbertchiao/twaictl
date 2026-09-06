package cli

import (
	"strconv"

	"github.com/spf13/cobra"

	"github.com/gilbertchiao/twaictl/internal/output"
	"github.com/gilbertchiao/twaictl/internal/twai/cos"
)

// cosUsageColumns 是 `cos usage` 直式輸出的欄位。
var cosUsageColumns = []output.Column[cos.Usage]{
	{Name: "Key name", Default: true, Value: func(u cos.Usage) string { return u.KeyName }},
	{Name: "Used space", Default: true, Value: func(u cos.Usage) string { return output.Bytes(u.TotalUsedSpace) }},
	{Name: "Objects", Default: true, Value: func(u cos.Usage) string { return strconv.FormatInt(u.TotalObjects, 10) }},
}

// cosBucketUsageColumns 是 `cos usage --per-bucket` 的欄位。
var cosBucketUsageColumns = []output.Column[cos.BucketUsage]{
	{Name: "Name", Default: true, Value: func(b cos.BucketUsage) string { return b.Name }},
	{Name: "Used space", Default: true, Value: func(b cos.BucketUsage) string { return output.Bytes(b.UsedSpace) }},
	{Name: "Objects", Default: true, Value: func(b cos.BucketUsage) string { return strconv.FormatInt(b.NumObjects, 10) }},
	{Name: "Modified", Default: true, Value: func(b cos.BucketUsage) string { return output.FormatTimeString(b.LastModified) }},
}

// newCosUsageCmd 建立 `cos usage`：呼叫 Ceph 管理 API（經 apigateway）查詢用量，不需要 S3 金鑰。
func newCosUsageCmd(a *app) *cobra.Command {
	var keyName string
	var perBucket bool
	cmd := &cobra.Command{
		Use:   "usage",
		Short: "顯示 COS 用量（總用量，或 --per-bucket 逐 bucket）",
		Long: "顯示 COS 用量。預設顯示 public 金鑰底下所有 bucket 的合計，--key-name 可改查指定的 private 金鑰。\n\n" +
			columnsHelp(cosUsageColumns) + "\n\n--per-bucket 時：\n" + columnsHelp(cosBucketUsageColumns),
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if perBucket {
				if err := validateColumns(a, cosBucketUsageColumns); err != nil {
					return err
				}
			} else if err := validateColumns(a, cosUsageColumns); err != nil {
				return err
			}
			ctx := a.commandContext(c)
			id, err := a.cosProjectID(ctx)
			if err != nil {
				return err
			}
			svc, err := a.cosService()
			if err != nil {
				return err
			}
			if perBucket {
				res, err := svc.ListBucketUsage(ctx, id, keyName)
				if err != nil {
					return err
				}
				return render(a, res.Raw, res.Value, cosBucketUsageColumns)
			}
			res, err := svc.GetUsage(ctx, id, keyName)
			if err != nil {
				return err
			}
			return renderOne(a, res.Raw, res.Value, cosUsageColumns)
		},
	}
	cmd.Flags().StringVar(&keyName, "key-name", "", "查詢指定 private 金鑰底下的 bucket（預設為 public 金鑰）")
	cmd.Flags().BoolVar(&perBucket, "per-bucket", false, "逐 bucket 列出用量與物件數")
	return cmd
}
