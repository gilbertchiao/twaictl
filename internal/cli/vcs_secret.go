package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/gilbertchiao/twaictl/internal/output"
	"github.com/gilbertchiao/twaictl/internal/twai/vcs"
)

// secretExpires 顯示 secret 的到期時間；ExpireTime 為 nil 時顯示空字串。
func secretExpires(s vcs.Secret) string {
	if s.ExpireTime == nil {
		return ""
	}
	return output.FormatTimeString(*s.ExpireTime)
}

// secretCreated 顯示 secret 的建立時間；CreateTime 為 nil 時顯示空字串。
func secretCreated(s vcs.Secret) string {
	if s.CreateTime == nil {
		return ""
	}
	return output.FormatTimeString(*s.CreateTime)
}

// secretUser 顯示 secret 所屬使用者名稱；User 為 nil 時顯示空字串。
func secretUser(s vcs.Secret) string {
	if s.User == nil {
		return ""
	}
	return s.User.Username
}

// secretProjectName 顯示 secret 所屬 project 名稱；Project 為 nil 時顯示空字串。
func secretProjectName(s vcs.Secret) string {
	if s.Project == nil {
		return ""
	}
	return s.Project.Name
}

// secretColumns 是 `vcs secret ls` 的欄位定義。
var secretColumns = []output.Column[vcs.Secret]{
	{Name: "ID", Default: true, Value: func(s vcs.Secret) string { return string(s.ID) }},
	{Name: "NAME", Default: true, Value: func(s vcs.Secret) string { return s.Name }},
	{Name: "STATUS", Default: true, Value: func(s vcs.Secret) string { return s.Status }},
	{Name: "EXPIRES", Default: true, Value: secretExpires},
	{Name: "CREATED", Default: true, Value: secretCreated},
	{Name: "DESC", Value: func(s vcs.Secret) string { return s.Desc }},
	{Name: "USER", Value: secretUser},
}

// secretDetailColumns 是 `vcs secret get`／`create` 直式輸出的欄位定義。
var secretDetailColumns = []output.Column[vcs.Secret]{
	{Name: "ID", Default: true, Value: func(s vcs.Secret) string { return string(s.ID) }},
	{Name: "Name", Default: true, Value: func(s vcs.Secret) string { return s.Name }},
	{Name: "Status", Default: true, Value: func(s vcs.Secret) string { return s.Status }},
	{Name: "Desc", Default: true, Value: func(s vcs.Secret) string { return s.Desc }},
	{Name: "Expires", Default: true, Value: secretExpires},
	{Name: "Created", Default: true, Value: secretCreated},
	{Name: "Project", Default: true, Value: secretProjectName},
	{Name: "User", Default: true, Value: secretUser},
}

// newVCSSecretCmd 建立 `vcs secret` 子命令。
func newVCSSecretCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{Use: "secret", Short: "管理 secret（TLS 憑證等，供 load balancer 使用）"}
	cmd.AddCommand(newVCSSecretLsCmd(a), newVCSSecretGetCmd(a), newVCSSecretCreateCmd(a), newVCSSecretRmCmd(a))
	return cmd
}

// newVCSSecretLsCmd 建立 `vcs secret ls`。
func newVCSSecretLsCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "列出 secret",
		Long:  "列出 secret。\n\n" + columnsHelp(secretColumns),
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if err := validateColumns(a, secretColumns); err != nil {
				return err
			}
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			res, err := svc.ListSecrets(ctx, projectID)
			if err != nil {
				return err
			}
			return render(a, res.Raw, res.Value, secretColumns)
		},
	}
}

// newVCSSecretGetCmd 建立 `vcs secret get`。
func newVCSSecretGetCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id|name>",
		Short: "顯示單一 secret 的詳細資料",
		Long:  "顯示單一 secret 的詳細資料。\n\n" + columnsHelp(secretDetailColumns),
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if err := validateColumns(a, secretDetailColumns); err != nil {
				return err
			}
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			id, err := svc.ResolveSecretID(ctx, projectID, args[0])
			if err != nil {
				return err
			}
			res, err := svc.GetSecret(ctx, id)
			if err != nil {
				return err
			}
			return renderOne(a, res.Raw, res.Value, secretDetailColumns)
		},
	}
}

// newVCSSecretCreateCmd 建立 `vcs secret create`。
//
// payload 由 twaictl 讀入後以 base64 編碼送出，絕不寫進 progress / log / error 訊息；
// --dry-run 印出的 curl 命令會把 payload 遮蔽為 ***（見 internal/twai/curl.go 的
// maskedBodyFields），避免 TLS 私鑰等機密內容明文出現在常被貼進 issue / log 的
// dry-run 輸出中。
func newVCSSecretCreateCmd(a *app) *cobra.Command {
	var name, payloadFile, expire, desc string
	var payloadStdin bool
	cmd := &cobra.Command{
		Use:   "create --name <n> (--payload-file <path> | --payload-stdin)",
		Short: "建立 secret",
		Long: "建立 secret。\n\n" +
			"payload 由 twaictl 讀入後以 base64 編碼送出；--dry-run 的 curl 會把 payload 遮蔽為 ***。\n\n" +
			"實測：payload 必須是 PKCS#12 bundle，PEM／DER 憑證會被 API 以 " +
			"`Invalid certification` 拒絕；--expire 可省略（回應 expire_time 為 null）。\n\n" +
			columnsHelp(secretDetailColumns),
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if err := validateColumns(a, secretDetailColumns); err != nil {
				return err
			}
			if err := requireFlag("--name", name); err != nil {
				return err
			}
			fileGiven := c.Flags().Changed("payload-file")
			if fileGiven == payloadStdin {
				return fmt.Errorf("請指定 --payload-file 或 --payload-stdin 其中之一")
			}
			var expireTime *time.Time
			if expire != "" {
				parsed, err := time.Parse(time.RFC3339, expire)
				if err != nil {
					return fmt.Errorf("--expire 必須是 RFC3339（例如 2027-01-01T00:00:00Z）: %w", err)
				}
				utc := parsed.UTC()
				expireTime = &utc
			}

			var payload []byte
			var payloadSource string
			if fileGiven {
				data, err := os.ReadFile(payloadFile)
				if err != nil {
					return fmt.Errorf("讀取 --payload-file %s 失敗: %w", payloadFile, err)
				}
				payload = data
				payloadSource = fmt.Sprintf("--payload-file %s", payloadFile)
			} else {
				data, err := io.ReadAll(a.stdin)
				if err != nil {
					return fmt.Errorf("讀取 stdin payload 失敗: %w", err)
				}
				payload = data
				payloadSource = "--payload-stdin"
			}
			// 空 payload（不論來自空檔案或空 stdin）一律視為錯誤：base64 編碼空字串
			// 仍是合法值，若不擋下來會靜默建立一個內容為空的 secret，之後掛到
			// load balancer 才會神祕失敗。
			if len(payload) == 0 {
				return fmt.Errorf("payload 為空（%s）", payloadSource)
			}

			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			res, err := svc.CreateSecret(ctx, vcs.CreateSecretInput{
				ProjectID: projectID, Name: name, Desc: desc, Payload: payload, ExpireTime: expireTime,
			})
			if err != nil {
				return err
			}
			a.progress("已建立 secret %s", res.Value.ID)
			return renderOne(a, res.Raw, res.Value, secretDetailColumns)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&name, "name", "", "secret 名稱（必填）")
	flags.StringVar(&payloadFile, "payload-file", "",
		"PKCS#12（.p12／.pfx，含憑證與私鑰，可無密碼）檔案內容；twaictl 讀入後以 base64 編碼送出（與 --payload-stdin 擇一）")
	flags.BoolVar(&payloadStdin, "payload-stdin", false,
		"PKCS#12（.p12／.pfx，含憑證與私鑰，可無密碼）檔案內容；twaictl 讀入後以 base64 編碼送出（從 stdin 原樣讀入，不去除結尾換行；與 --payload-file 擇一）")
	flags.StringVar(&expire, "expire", "", "到期時間（RFC3339，例如 2027-01-01T00:00:00Z）")
	flags.StringVar(&desc, "desc", "", "描述")
	return cmd
}

// newVCSSecretRmCmd 建立 `vcs secret rm`：破壞性操作，需確認或 -y。
func newVCSSecretRmCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "rm <id|name>...",
		Short: "刪除 secret（破壞性操作，需確認或 -y）",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			ids := make([]string, 0, len(args))
			for _, ref := range args {
				id, err := svc.ResolveSecretID(ctx, projectID, ref)
				if err != nil {
					return err
				}
				ids = append(ids, id)
			}
			if err := a.confirm(fmt.Sprintf("刪除 secret %s", strings.Join(ids, ", "))); err != nil {
				return err
			}
			for _, id := range ids {
				if err := svc.DeleteSecret(ctx, id); err != nil {
					return err
				}
				a.progress("已刪除 secret %s", id)
			}
			return nil
		},
	}
}
