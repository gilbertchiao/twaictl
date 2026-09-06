package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	genvcs "github.com/gilbertchiao/twaictl/internal/gen/vcs"
	"github.com/gilbertchiao/twaictl/internal/output"
	"github.com/gilbertchiao/twaictl/internal/twai/vcs"
)

// keypairColumns 是 `vcs keypair ls` 的欄位定義。
var keypairColumns = []output.Column[genvcs.KeypairSerializer]{
	{Name: "NAME", Default: true, Value: func(k genvcs.KeypairSerializer) string { return k.Name }},
	{Name: "FINGERPRINT", Default: true, Value: func(k genvcs.KeypairSerializer) string { return k.Fingerprint }},
	{Name: "USER", Default: true, Value: func(k genvcs.KeypairSerializer) string { return k.User.Username }},
	{Name: "PLATFORM", Value: func(k genvcs.KeypairSerializer) string { return k.Platform }},
}

// keypairDetailColumns 是 `vcs keypair get` 直式輸出的欄位定義。
var keypairDetailColumns = []output.Column[vcs.KeypairDetail]{
	{Name: "Name", Default: true, Value: func(k vcs.KeypairDetail) string { return k.Name }},
	{Name: "Fingerprint", Default: true, Value: func(k vcs.KeypairDetail) string { return k.Fingerprint }},
	{Name: "Created", Default: true, Value: func(k vcs.KeypairDetail) string { return output.FormatTimeString(output.Str(k.CreateTime)) }},
	{Name: "User", Default: true, Value: func(k vcs.KeypairDetail) string { return k.User.Username }},
	{Name: "Public key", Default: true, Value: func(k vcs.KeypairDetail) string { return output.Str(k.PublicKey) }},
}

// keypairCreateResult 是 `vcs keypair create` 在 -o json/yaml 時的結構化輸出；
// table 模式改由 stdout 印出原始文字或寫入 --out 指定的檔案（見 newVCSKeypairCreateCmd）。
type keypairCreateResult struct {
	Name       string `json:"name" yaml:"name"`
	PrivateKey string `json:"private_key" yaml:"private_key"`
}

// newVCSKeypairCmd 建立 `vcs keypair` 子命令。
func newVCSKeypairCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{Use: "keypair", Short: "管理 VCS SSH 金鑰對"}
	cmd.AddCommand(newVCSKeypairLsCmd(a), newVCSKeypairGetCmd(a), newVCSKeypairCreateCmd(a), newVCSKeypairRmCmd(a))
	return cmd
}

// newVCSKeypairLsCmd 建立 `vcs keypair ls`。
func newVCSKeypairLsCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "列出金鑰對",
		Long:  "列出金鑰對。\n\n" + columnsHelp(keypairColumns),
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if err := validateColumns(a, keypairColumns); err != nil {
				return err
			}
			ctx := a.commandContext(c)
			svc, err := a.vcsService()
			if err != nil {
				return err
			}
			res, err := svc.ListKeypairs(ctx)
			if err != nil {
				return err
			}
			return render(a, res.Raw, res.Value, keypairColumns)
		},
	}
}

// newVCSKeypairGetCmd 建立 `vcs keypair get`。
func newVCSKeypairGetCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "get <name>",
		Short: "顯示單一金鑰對的詳細資料",
		Long:  "顯示單一金鑰對的詳細資料。\n\n" + columnsHelp(keypairDetailColumns),
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if err := validateColumns(a, keypairDetailColumns); err != nil {
				return err
			}
			ctx := a.commandContext(c)
			svc, err := a.vcsService()
			if err != nil {
				return err
			}
			res, err := svc.GetKeypair(ctx, args[0])
			if err != nil {
				return err
			}
			return renderOne(a, res.Raw, res.Value, keypairDetailColumns)
		},
	}
}

// newVCSKeypairCreateCmd 建立 `vcs keypair create`。
//
// private key 只可能出現在三個地方：table 模式的 stdout（未指定 --out 時）、
// --out 指定的檔案（權限 0600）、或 -o json/yaml 的結構化輸出；絕不寫進 stderr / log / progress 訊息。
//
// 指定 --out 且未同時指定 --public-key-file（即「新建金鑰對」而非「匯入既有公鑰」）時，
// 會在呼叫 CreateKeypair 之前就以 O_EXCL 先保留（建立）該檔案：API 只會在建立當下回傳
// 一次 private key，若等到 API 呼叫成功後才發現檔案已存在或無法寫入，使用者就會永久遺失
// 這把 private key，而且遠端已經建立的金鑰對通常無法用同名重新嘗試取得。
// 匯入既有公鑰時 API 本來就不會回傳 private key，因此不需要、也不預先保留檔案。
func newVCSKeypairCreateCmd(a *app) *cobra.Command {
	var publicKeyFile, out string
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "建立金鑰對（或以 --public-key-file 匯入既有公鑰）",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			name := args[0]
			if out != "" && output.Format(a.opts.output) != output.FormatTable {
				return fmt.Errorf("--out 不能與 -o json/yaml 同時使用")
			}
			publicKey := ""
			if publicKeyFile != "" {
				data, err := os.ReadFile(publicKeyFile)
				if err != nil {
					return fmt.Errorf("讀取 --public-key-file %s 失敗: %w", publicKeyFile, err)
				}
				publicKey = strings.TrimSpace(string(data))
			}

			ctx := a.commandContext(c)
			svc, err := a.vcsService()
			if err != nil {
				return err
			}

			var reserved *os.File
			if out != "" && publicKeyFile == "" {
				reserved, err = reserveKeyFile(out)
				if err != nil {
					return err
				}
			}

			privateKey, err := svc.CreateKeypair(ctx, name, publicKey)
			if err != nil {
				if reserved != nil {
					a.abandonReservedFile(reserved, out)
				}
				return err
			}
			if reserved != nil {
				return a.finishReservedKeyFile(reserved, out, name, privateKey)
			}
			return a.renderKeypairCreated(name, privateKey, out)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&publicKeyFile, "public-key-file", "", "匯入既有公鑰的檔案路徑；不指定則由伺服器產生新的金鑰對")
	flags.StringVar(&out, "out", "", "把 private key 寫入指定檔案（權限 0600），而非印到 stdout；檔案已存在時不覆寫，且不可與 -o json/yaml 併用")
	return cmd
}

// reserveKeyFile 在呼叫 API 前以 O_EXCL 建立並保留檔案（尚未寫入內容），
// 讓路徑問題（檔案已存在、目錄不存在、無寫入權限等）在建立遠端金鑰對之前就先被擋下。
func reserveKeyFile(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("寫入 %s 失敗：檔案已存在: %w", path, err)
		}
		return nil, fmt.Errorf("寫入 %s 失敗: %w", path, err)
	}
	return f, nil
}

// abandonReservedFile 在 CreateKeypair 呼叫失敗後清理事先保留的檔案：關閉並刪除，
// 避免留下一個永遠不會有內容的空檔案。清理本身失敗只記 debug log，
// 不能蓋掉原本要回傳給使用者的 API 錯誤（使用者真正需要知道的是「為什麼建立金鑰對失敗」）。
func (a *app) abandonReservedFile(f *os.File, path string) {
	if err := f.Close(); err != nil {
		a.logger.Debug("關閉保留檔案失敗", "path", path, "error", err)
	}
	if err := os.Remove(path); err != nil {
		a.logger.Debug("刪除保留檔案失敗", "path", path, "error", err)
	}
}

// finishReservedKeyFile 在 CreateKeypair 成功後，把 private key 寫進事先保留、已開啟的檔案。
func (a *app) finishReservedKeyFile(f *os.File, path, name, privateKey string) error {
	if privateKey == "" {
		// 理論上不會發生：只有 --public-key-file 匯入才會回應空字串，
		// 而匯入模式不會走到這裡（不會預先保留檔案）。防禦性處理：
		// 清掉保留下來的空檔，照一般「已建立」訊息回報，避免留下無用的空檔。
		a.abandonReservedFile(f, path)
		a.progress("金鑰對 %s 已建立", name)
		return nil
	}
	if _, err := f.WriteString(privateKey); err != nil {
		_ = f.Close()
		return a.rescueKeyToStdout(path, privateKey, err)
	}
	if err := f.Close(); err != nil {
		return a.rescueKeyToStdout(path, privateKey, err)
	}
	a.progress("已將 private key 寫入 %s（權限 0600）", path)
	return nil
}

// rescueKeyToStdout 是寫入 / 關閉保留檔案失敗時的最後手段：API 不會再回傳第二次
// private key，因此寧可把內容印到 stdout 也不能讓使用者整把遺失金鑰；
// stderr 只說明發生了什麼（不含金鑰內容），並回傳非 nil error 讓 exit code 反映
// 「操作沒有完全成功」。
func (a *app) rescueKeyToStdout(path, privateKey string, cause error) error {
	_, _ = fmt.Fprint(a.stdout, privateKey)
	a.progress("寫入 %s 失敗，已改印到 stdout", path)
	return fmt.Errorf("寫入 %s 失敗: %w", path, cause)
}

// renderKeypairCreated 依全域輸出格式與 out 決定 private key 的輸出位置；
// 用於「未事先保留 --out 檔案」的情況（--out 為空，或 --public-key-file 匯入模式）。
func (a *app) renderKeypairCreated(name, privateKey, out string) error {
	if output.Format(a.opts.output) != output.FormatTable {
		return renderKeypairCreateStructured(a, name, privateKey)
	}
	if privateKey == "" {
		// 匯入既有公鑰時 API 不回傳 private key，沒有內容可印，只回報成功。
		a.progress("金鑰對 %s 已建立", name)
		return nil
	}
	if out == "" {
		// 依 brief 要求原樣印出，不加換行、不加任何前綴。
		_, err := fmt.Fprint(a.stdout, privateKey)
		return err
	}
	// 走到這裡代表 --public-key-file 與 --out 併用，但 API 意外回傳了非空的
	// private key（匯入模式通常不會發生）；沿用同一套 O_EXCL、不覆寫既有內容的規則。
	f, err := reserveKeyFile(out)
	if err != nil {
		return err
	}
	return a.finishReservedKeyFile(f, out, name, privateKey)
}

// renderKeypairCreateStructured 輸出 -o json/yaml 時的結構化物件。
func renderKeypairCreateStructured(a *app, name, privateKey string) error {
	result := keypairCreateResult{Name: name, PrivateKey: privateKey}
	if output.Format(a.opts.output) == output.FormatYAML {
		data, err := yaml.Marshal(result)
		if err != nil {
			return fmt.Errorf("轉換為 YAML 失敗: %w", err)
		}
		_, err = a.stdout.Write(data)
		return err
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化為 JSON 失敗: %w", err)
	}
	_, err = fmt.Fprintln(a.stdout, string(data))
	return err
}

// newVCSKeypairRmCmd 建立 `vcs keypair rm`：破壞性操作，需確認或 -y。
func newVCSKeypairRmCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "rm <name>...",
		Short: "刪除金鑰對（破壞性操作，需確認或 -y）",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			ctx := a.commandContext(c)
			svc, err := a.vcsService()
			if err != nil {
				return err
			}
			if err := a.confirm(fmt.Sprintf("刪除金鑰對 %s", strings.Join(args, ", "))); err != nil {
				return err
			}
			for _, name := range args {
				if err := svc.DeleteKeypair(ctx, name); err != nil {
					return err
				}
				a.progress("已刪除金鑰對：%s", name)
			}
			return nil
		},
	}
}
