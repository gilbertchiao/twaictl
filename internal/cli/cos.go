package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gilbertchiao/twaictl/internal/config"
	"github.com/gilbertchiao/twaictl/internal/output"
	"github.com/gilbertchiao/twaictl/internal/twai/cos"
)

// cosKeysDisplay 是 `cos key get/create/renew/rm` table 直式輸出用的資料，
// secret 是否遮蔽已依 showSecret 在 newCOSKeysDisplay 決定，欄位本身只負責取值。
type cosKeysDisplay struct {
	PublicAccessKey string
	PublicSecretKey string
	PrivateKeys     string
}

// cosKeysColumns 是 cosKeysDisplay 直式輸出的欄位定義。
var cosKeysColumns = []output.Column[cosKeysDisplay]{
	{Name: "Public access key", Default: true, Value: func(k cosKeysDisplay) string { return k.PublicAccessKey }},
	{Name: "Public secret key", Default: true, Value: func(k cosKeysDisplay) string { return k.PublicSecretKey }},
	{Name: "Private keys", Default: true, Value: func(k cosKeysDisplay) string { return k.PrivateKeys }},
}

// newCOSKeysDisplay 把 cos.Keys 轉成直式輸出用的字串：showSecret 為 false 時，
// public secret 顯示為 config.MaskedValue，private 只顯示 name(access_key)；
// showSecret 為 true 時兩者都附上 secret。
func newCOSKeysDisplay(keys cos.Keys, showSecret bool) cosKeysDisplay {
	var d cosKeysDisplay
	if keys.Public != nil {
		d.PublicAccessKey = keys.Public.AccessKey
		d.PublicSecretKey = config.MaskedValue
		if showSecret {
			d.PublicSecretKey = keys.Public.SecretKey
		}
	}
	parts := make([]string, 0, len(keys.Private))
	for _, k := range keys.Private {
		if showSecret {
			parts = append(parts, fmt.Sprintf("%s(%s, %s)", k.Name, k.AccessKey, k.SecretKey))
		} else {
			parts = append(parts, fmt.Sprintf("%s(%s)", k.Name, k.AccessKey))
		}
	}
	d.PrivateKeys = strings.Join(parts, ", ")
	return d
}

// renderCOSKeys 是 `cos key get/create/renew/rm` 共用的輸出收尾：table 模式為直式，
// secret 依 showSecret 遮蔽；-o json/yaml 一律輸出 API 原始回應（本就含 secret，
// 無法只遮蔽其中一部分而不破壞 JSON 結構），因此額外在 stderr 印一次警告，
// 避免使用者在不知情的情況下把含 secret 的輸出導向 log 或其他系統。
func (a *app) renderCOSKeys(raw []byte, keys cos.Keys, showSecret bool) error {
	if output.Format(a.opts.output) != output.FormatTable {
		_, _ = fmt.Fprintln(a.stderr, "twaictl: 警告: 輸出含 secret key")
	}
	return renderOne(a, raw, newCOSKeysDisplay(keys, showSecret), cosKeysColumns)
}

// saveCOSKey 把 public 金鑰寫入目前 profile 的 cos.access_key/secret_key。
// profile 不存在時建立最小 profile；不改變 current_profile——`--save` 的目的只是
// 補齊 S3 資料面憑證，不代表使用者要切換目前生效的 profile。
func (a *app) saveCOSKey(keys cos.Keys) error {
	if keys.Public == nil {
		return fmt.Errorf("API 未回傳 public key，無法儲存")
	}
	profileName := a.settings.ProfileName
	profile := a.config.Profiles[profileName] // 不存在時為零值，保留既有欄位再覆寫
	profile.COS.AccessKey = keys.Public.AccessKey
	profile.COS.SecretKey = keys.Public.SecretKey

	if a.config.Profiles == nil {
		a.config.Profiles = make(map[string]config.Profile)
	}
	a.config.Profiles[profileName] = profile

	if err := config.Save(a.configPath, a.config); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(a.stderr, "twaictl: 已將 COS S3 金鑰寫入 profile %s（%s）\n", profileName, a.configPath)
	return nil
}

// saveCOSKeyAfterRender 在「回應已經輸出完成」之後才嘗試寫入 profile：
// `renew` 會讓舊 key 立即失效、`get` 也可能是使用者取得目前 secret 的唯一機會，
// 若把「輸出」放在「儲存」之後，一旦 config.Save 失敗（唯讀檔案系統、磁碟問題等），
// 使用者會在遠端狀態已經改變（renew）或至少已經呼叫過 API 的情況下，什麼都看不到、
// 也就永久失去了這次的 secret。因此一律先呼叫 renderCOSKeys 完成輸出，
// 儲存失敗只讓 exit code 非 0、並在 stderr 額外提示。
//
// 提示只在使用者這次的輸出「本來就沒看到 secret」時才需要——showSecret 為 true，
// 或輸出格式是 json/yaml（一律含 secret）時，使用者已經拿到 secret，
// 不需要、也不應該再多印一次可能引導誤解的提示。
func (a *app) saveCOSKeyAfterRender(keys cos.Keys, showSecret bool) error {
	if err := a.saveCOSKey(keys); err != nil {
		if !showSecret && output.Format(a.opts.output) == output.FormatTable {
			_, _ = fmt.Fprintln(a.stderr, "twaictl: 警告: 儲存失敗，請以 --show-secret 重新執行 `cos key get` 取得金鑰")
		}
		return fmt.Errorf("儲存失敗: %w", err)
	}
	return nil
}

// renderAndSaveCOSKeys 是 `cos key get/renew` 共用的收尾：不論 renderCOSKeys
// 成不成功，只要 save 為真就一定會嘗試呼叫 saveCOSKeyAfterRender——
// renew 的舊 key 在請求送出當下就已經在遠端立即失效，若 render 失敗（例如
// stdout 是已關閉的 pipe、磁碟已滿）就跳過儲存，會讓這把新 key「既沒顯示、
// 也沒存檔」，使用者將永久失去它。兩個步驟各自的錯誤都保留在回傳的錯誤鏈中
// （errors.Join；任一為 nil 時等同只回傳另一個，兩者皆 nil 時回傳 nil），
// 讓 exit code 仍反映失敗，但不會互相蓋掉對方的錯誤訊息。
func (a *app) renderAndSaveCOSKeys(raw []byte, keys cos.Keys, showSecret, save bool) error {
	renderErr := a.renderCOSKeys(raw, keys, showSecret)
	var saveErr error
	if save {
		saveErr = a.saveCOSKeyAfterRender(keys, showSecret)
	}
	return errors.Join(renderErr, saveErr)
}

// newCosCmd 建立 `cos` 子命令。
func newCosCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{Use: "cos", Short: "管理 COS（S3 相容）：金鑰、bucket、物件、用量"}
	cmd.AddCommand(newCosKeyCmd(a))
	cmd.AddCommand(newCosBucketCmd(a))
	cmd.AddCommand(newCosLsCmd(a))
	cmd.AddCommand(newCosCpCmd(a))
	cmd.AddCommand(newCosSyncCmd(a))
	cmd.AddCommand(newCosRmCmd(a))
	cmd.AddCommand(newCosAclCmd(a))
	cmd.AddCommand(newCosContentTypeCmd(a))
	cmd.AddCommand(newCosUsageCmd(a))
	return cmd
}

// newCosKeyCmd 建立 `cos key` 子命令。
func newCosKeyCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{Use: "key", Short: "管理 COS S3 金鑰（public / private）"}
	cmd.AddCommand(newCosKeyGetCmd(a), newCosKeyCreateCmd(a), newCosKeyRenewCmd(a), newCosKeyRmCmd(a))
	return cmd
}

// newCosKeyGetCmd 建立 `cos key get`。
func newCosKeyGetCmd(a *app) *cobra.Command {
	var showSecret, save bool
	cmd := &cobra.Command{
		Use:   "get",
		Short: "顯示目前 project 的 COS S3 金鑰",
		Long:  "顯示目前 project 的 COS S3 金鑰。\n\n" + columnsHelp(cosKeysColumns),
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if err := validateColumns(a, cosKeysColumns); err != nil {
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
			res, err := svc.GetKeys(ctx, id)
			if err != nil {
				return err
			}
			return a.renderAndSaveCOSKeys(res.Raw, res.Value, showSecret, save)
		},
	}
	flags := cmd.Flags()
	flags.BoolVar(&showSecret, "show-secret", false, "顯示 secret key（table 模式預設遮蔽；-o json/yaml 一律含 secret）")
	flags.BoolVar(&save, "save", false, "把 public 金鑰寫入目前 profile 的 cos.access_key/secret_key")
	return cmd
}

// newCosKeyCreateCmd 建立 `cos key create`。
func newCosKeyCreateCmd(a *app) *cobra.Command {
	var name string
	var showSecret bool
	cmd := &cobra.Command{
		Use:   "create",
		Short: "建立新的 private S3 金鑰",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if err := validateColumns(a, cosKeysColumns); err != nil {
				return err
			}
			if name == "" {
				return fmt.Errorf("--name 為必填")
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
			res, err := svc.CreateKey(ctx, id, name)
			if err != nil {
				return err
			}
			if err := a.renderCOSKeys(res.Raw, res.Value, showSecret); err != nil {
				return err
			}
			if !showSecret && output.Format(a.opts.output) == output.FormatTable {
				a.progress("以 --show-secret 或 -o json 取得新金鑰的 secret")
			}
			return nil
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&name, "name", "", "新金鑰的名稱（必填）")
	flags.BoolVar(&showSecret, "show-secret", false, "顯示新建立金鑰的 secret")
	return cmd
}

// newCosKeyRenewCmd 建立 `cos key renew`：破壞性操作（舊 key 立即失效），需確認或 -y。
//
// 舊 key 一旦輪替就無法復原，使用者必須靠這次呼叫的回應才能拿到新 secret，
// 因此輸出邏輯與 `cos key get` 共用：--show-secret 顯示新 secret，
// --save 把（可能已更新的）public 金鑰寫回目前 profile。
func newCosKeyRenewCmd(a *app) *cobra.Command {
	var name string
	var all, showSecret, save bool
	cmd := &cobra.Command{
		Use:   "renew",
		Short: "輪替 S3 金鑰（破壞性操作：舊 key 立即失效，需確認或 -y）",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if err := validateColumns(a, cosKeysColumns); err != nil {
				return err
			}
			if name == "" && !all {
				return fmt.Errorf("--name 或 --all 至少需指定一個")
			}
			if err := a.confirm("輪替 COS S3 金鑰（舊 key 將立即失效）"); err != nil {
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
			res, err := svc.RenewKey(ctx, id, name, all)
			if err != nil {
				return err
			}
			return a.renderAndSaveCOSKeys(res.Raw, res.Value, showSecret, save)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&name, "name", "", "只輪替指定名稱的 private 金鑰")
	flags.BoolVar(&all, "all", false, "輪替所有 private 金鑰")
	flags.BoolVar(&showSecret, "show-secret", false, "顯示輪替後的新 secret（table 模式預設遮蔽；-o json/yaml 一律含 secret）")
	flags.BoolVar(&save, "save", false, "把輪替後的 public 金鑰寫入目前 profile 的 cos.access_key/secret_key")
	return cmd
}

// newCosKeyRmCmd 建立 `cos key rm`：破壞性操作，需確認或 -y。
func newCosKeyRmCmd(a *app) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "rm",
		Short: "刪除指定名稱的 private S3 金鑰（破壞性操作，需確認或 -y）",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if err := validateColumns(a, cosKeysColumns); err != nil {
				return err
			}
			if name == "" {
				return fmt.Errorf("--name 為必填")
			}
			if err := a.confirm(fmt.Sprintf("刪除 COS S3 金鑰 %s", name)); err != nil {
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
			res, err := svc.DeleteKey(ctx, id, name)
			if err != nil {
				return err
			}
			return a.renderCOSKeys(res.Raw, res.Value, false)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "要刪除的金鑰名稱（必填）")
	return cmd
}
