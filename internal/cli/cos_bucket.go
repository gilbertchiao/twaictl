package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/gilbertchiao/twaictl/internal/output"
	"github.com/gilbertchiao/twaictl/internal/twai"
	"github.com/gilbertchiao/twaictl/internal/twai/cos"
)

// cosBucketVersioningValues 是 `cos bucket create --versioning` 的合法值。
var cosBucketVersioningValues = map[string]bool{"on": true, "off": true}

// cosBucketDisplay 是 `cos bucket ls` 的顯示資料：cos.Bucket 是 internal/twai/cos 套件的
// 內部型別，欄位沒有 json tag（不適合原樣拿來當 -o json 輸出），這裡另外定義一份
// 帶 snake_case json tag 的顯示型別，供 render() 同時處理 table／json／yaml 三種格式。
type cosBucketDisplay struct {
	Name    string    `json:"name"`
	Created time.Time `json:"created"`
}

// newCosBucketDisplay 把 cos.Bucket 轉成顯示資料。
func newCosBucketDisplay(b cos.Bucket) cosBucketDisplay {
	return cosBucketDisplay{Name: b.Name, Created: b.Created}
}

// cosBucketColumns 是 `cos bucket ls` 的欄位定義。
var cosBucketColumns = []output.Column[cosBucketDisplay]{
	{Name: "Name", Default: true, Value: func(b cosBucketDisplay) string { return b.Name }},
	{Name: "Created", Default: true, Value: func(b cosBucketDisplay) string { return output.FormatTime(b.Created) }},
}

// newCosBucketCmd 建立 `cos bucket` 子命令。
func newCosBucketCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{Use: "bucket", Short: "管理 COS bucket"}
	cmd.AddCommand(newCosBucketLsCmd(a), newCosBucketCreateCmd(a), newCosBucketRmCmd(a))
	return cmd
}

// newCosBucketLsCmd 建立 `cos bucket ls`。
func newCosBucketLsCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "列出所有 bucket",
		Long:  "列出所有 bucket。\n\n" + columnsHelp(cosBucketColumns),
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if err := validateColumns(a, cosBucketColumns); err != nil {
				return err
			}
			client, err := a.s3Client()
			if err != nil {
				return err
			}
			buckets, err := client.ListBuckets(a.commandContext(c))
			if err != nil {
				return err
			}
			displays := make([]cosBucketDisplay, len(buckets))
			for i, b := range buckets {
				displays[i] = newCosBucketDisplay(b)
			}
			return render(a, nil, displays, cosBucketColumns)
		},
	}
	return cmd
}

// newCosBucketCreateCmd 建立 `cos bucket create`。
func newCosBucketCreateCmd(a *app) *cobra.Command {
	var versioning string
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "建立 bucket",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			name := args[0]
			if err := validateEnumFlag("--versioning", versioning, cosBucketVersioningValues); err != nil {
				return err
			}
			if a.opts.dryRun {
				_, _ = fmt.Fprintf(a.stdout, "dry-run: 建立 bucket %s\n", name)
				if versioning != "" {
					_, _ = fmt.Fprintf(a.stdout, "dry-run: 設定 bucket %s 版本控制為 %s\n", name, versioning)
				}
				return nil
			}
			ctx := a.commandContext(c)
			client, err := a.s3Client()
			if err != nil {
				return err
			}
			if err := client.CreateBucket(ctx, name); err != nil {
				return err
			}
			if versioning != "" {
				// bucket 這時已經建立成功；若接下來設定版本控制失敗，錯誤訊息必須說明
				// bucket 已建立，避免使用者誤以為整個 create 都沒生效而重複建立
				// （重複建立會撞到 409 BucketAlreadyOwnedByYou，反而更困惑）。
				if err := client.SetVersioning(ctx, name, versioning == "on"); err != nil {
					return fmt.Errorf("bucket %s 已建立，但設定版本控制失敗: %w", name, err)
				}
			}
			a.progress("已建立 bucket %s", name)
			return nil
		},
	}
	cmd.Flags().StringVar(&versioning, "versioning", "", "建立後立即設定版本控制：on|off")
	return cmd
}

// newCosBucketRmCmd 建立 `cos bucket rm`：破壞性操作，需確認或 -y。
func newCosBucketRmCmd(a *app) *cobra.Command {
	var purge bool
	cmd := &cobra.Command{
		Use:   "rm <name>...",
		Short: "刪除 bucket（破壞性操作，需確認或 -y；bucket 非空時會失敗）",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			ctx := a.commandContext(c)
			if purge {
				return a.cosBucketRmPurge(ctx, args)
			}
			if err := a.confirm(fmt.Sprintf("刪除 bucket %s", strings.Join(args, ", "))); err != nil {
				return err
			}
			if a.opts.dryRun {
				for _, name := range args {
					_, _ = fmt.Fprintf(a.stdout, "dry-run: 刪除 bucket %s\n", name)
				}
				return nil
			}
			client, err := a.s3Client()
			if err != nil {
				return err
			}
			for _, name := range args {
				if err := client.DeleteBucket(ctx, name); err != nil {
					return hintBucketNotEmpty(err)
				}
				a.progress("已刪除 bucket %s", name)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&purge, "purge", false, "先刪除 bucket 內所有物件版本與 delete marker 再刪除 bucket")
	return cmd
}

// cosBucketRmPurge 兩段式清空並刪除每個 bucket：第一段（cosCountVersionTargets）只計數
// 每個 bucket 底下的物件版本數，確認訊息用這個計數；確認後第二段對每個 bucket 各自重新
// list 並逐一刪除版本（cosStreamDeleteVersionTargets；--dry-run 則逐筆印出 dry-run 行），
// 再刪除 bucket 本身，全程不在記憶體中蒐集完整版本清單——bucket 版本數很多（千萬級）時
// 這是必要的，見 README「已知限制」。`cos bucket rm --purge` 沒有 -o json/yaml 摘要輸出，
// 因此第二段一律不累積 rmSummary（collect 傳 false）。
//
// pass 2 若發現的實際版本數與 pass 1 的計數不同（列出期間有版本新增或被其他人刪除），
// 照常處理完，只在 stderr 印出該 bucket 實際刪除數的提示，不視為錯誤。
func (a *app) cosBucketRmPurge(ctx context.Context, names []string) error {
	client, err := a.s3Client()
	if err != nil {
		return err
	}

	counts := make([]int, len(names))
	descs := make([]string, 0, len(names))
	for i, name := range names {
		count, err := a.cosCountVersionTargets(ctx, client, cos.Location{Bucket: name}, false)
		if err != nil {
			return err
		}
		counts[i] = count
		descs = append(descs, fmt.Sprintf("%s（%d 個物件版本）", name, count))
	}
	if err := a.confirm("清空並刪除 bucket " + strings.Join(descs, "、")); err != nil {
		return err
	}

	for i, name := range names {
		actual, _, err := a.cosStreamDeleteVersionTargets(ctx, client, cos.Location{Bucket: name}, false, false)
		if err != nil {
			return err
		}
		if actual != counts[i] {
			// dry-run 底下只是印出提示，沒有真的刪除（理由同 cos_object.go 的
			// cosRmAllVersions/cosRmRecursive）。
			verb := "刪除"
			if a.opts.dryRun {
				verb = "處理"
			}
			a.progress("bucket %s 實際%s %d 個物件版本（確認當下為 %d 個，期間有變動）", name, verb, actual, counts[i])
		}
		if a.opts.dryRun {
			_, _ = fmt.Fprintf(a.stdout, "dry-run: 刪除 bucket %s\n", name)
			continue
		}
		if err := client.DeleteBucket(ctx, name); err != nil {
			return hintBucketNotEmpty(err)
		}
		a.progress("已刪除 bucket %s", name)
	}
	return nil
}

// hintBucketNotEmpty 在 409 BucketNotEmpty 時補上 versioning 殘留的提示；用 %w 包裝，exit code 仍為 3。
func hintBucketNotEmpty(err error) error {
	var apiErr *twai.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusConflict && strings.Contains(apiErr.Message, "BucketNotEmpty") {
		return fmt.Errorf("%w（bucket 仍有物件；若 `cos ls` 已看不到任何物件，可能是版本控制留下的物件版本或 delete marker，"+
			"可用 `cos ls --versions` 查看，或改用 `cos bucket rm --purge` 一併清除）", err)
	}
	return err
}
