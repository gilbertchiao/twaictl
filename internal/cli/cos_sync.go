package cli

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gilbertchiao/twaictl/internal/output"
	"github.com/gilbertchiao/twaictl/internal/twai/cos"
)

// syncSummary 是 `cos sync` 在 -o json/yaml 時輸出的單筆動作（略過的檔案不列出）。
type syncSummary struct {
	Action string `json:"action"` // upload | delete
	Src    string `json:"src,omitempty"`
	Dst    string `json:"dst"`
	Size   int64  `json:"size,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// syncUpload 是決定要上傳的單一檔案與原因。
type syncUpload struct {
	file   cosUploadFile
	reason string
}

// newCosSyncCmd 建立 `cos sync`：只支援本地目錄 → COS，以大小 + ETag（MD5）判斷差異，只上傳有變更的檔案。
func newCosSyncCmd(a *app) *cobra.Command {
	var deleteExtra bool
	cmd := &cobra.Command{
		Use:   "sync <local_dir> cos://bucket[/prefix]",
		Short: "把本地目錄同步到 COS 前綴（只上傳有變更的檔案；--delete 一併刪除遠端多出的物件）",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			srcDir, dstURL := args[0], args[1]
			info, err := os.Stat(srcDir)
			if err != nil {
				return fmt.Errorf("%s 不是目錄: %w", srcDir, err)
			}
			if !info.IsDir() {
				return fmt.Errorf("%s 不是目錄", srcDir)
			}
			loc, ok := cos.ParseURL(dstURL)
			if !ok {
				return fmt.Errorf("%s 不是合法的 cos:// URL（目前只支援本地目錄 → COS 方向）", dstURL)
			}
			return a.cosSync(a.commandContext(c), srcDir, loc, deleteExtra)
		},
	}
	cmd.Flags().BoolVar(&deleteExtra, "delete", false, "刪除遠端前綴底下本地不存在的物件（破壞性操作，需確認或 -y）")
	return cmd
}

// cosSync 是 sync 的主流程：走訪本地 → list 遠端 → 決定上傳／刪除 → 確認 → 執行（或 dry-run 印出）。
func (a *app) cosSync(ctx context.Context, srcDir string, loc cos.Location, deleteExtra bool) error {
	prefix := remotePrefix(loc.Key)
	files, err := a.walkLocalFiles(srcDir, prefix)
	if err != nil {
		return err
	}
	client, err := a.s3Client()
	if err != nil {
		return err
	}
	remote := map[string]cos.Object{}
	if err := client.ListObjects(ctx, loc.Bucket, prefix, func(o cos.Object) error {
		remote[o.Key] = o
		return nil
	}); err != nil {
		return err
	}

	var deletions []string
	if deleteExtra {
		deletions = planSyncDeletions(files, remote)
	}
	if len(files) == 0 && len(deletions) == 0 {
		// 本地目錄底下完全沒有檔案（且沒有東西可刪）：與「有檔案但都略過」不同，
		// 後者仍要照常印出 sync 完成的總結（上傳 0、略過 N），只有這裡才是真的
		// 沒有任何本地內容可以同步。-o json（非 dry-run）仍要輸出 `[]`
		// （與 cos cp 同情境一致，也與下方正常路徑「dry-run 時不輸出 JSON」的
		// 規則一致），而不是留空白 stdout，讓呼叫端不必特判「沒有動作」與
		// 「輸出被吃掉」。
		a.progress("沒有需要同步的檔案")
		if a.opts.dryRun {
			return nil
		}
		return renderSummary(a, []syncSummary{})
	}

	uploads, skipped, err := planSyncUploads(files, remote)
	if err != nil {
		return err
	}
	if len(deletions) > 0 {
		if err := a.confirm(fmt.Sprintf("刪除 %d 個本地不存在的遠端物件（%s）", len(deletions), rmRecursiveTargetDesc(loc.Bucket, prefix))); err != nil {
			return err
		}
	}

	summaries := make([]syncSummary, 0, len(uploads)+len(deletions))
	for _, u := range uploads {
		dst := cos.Location{Bucket: loc.Bucket, Key: u.file.key}.String()
		if a.opts.dryRun {
			_, _ = fmt.Fprintf(a.stdout, "dry-run: 上傳 %s → %s（%s）\n", u.file.path, dst, u.reason)
			continue
		}
		if err := client.Upload(ctx, loc.Bucket, u.file.key, u.file.path); err != nil {
			return err
		}
		a.progress("上傳 %s → %s（%s，%s）", u.file.path, dst, output.Bytes(u.file.size), u.reason)
		summaries = append(summaries, syncSummary{Action: "upload", Src: u.file.path, Dst: dst, Size: u.file.size, Reason: u.reason})
	}
	for _, key := range deletions {
		dst := cos.Location{Bucket: loc.Bucket, Key: key}.String()
		if a.opts.dryRun {
			_, _ = fmt.Fprintf(a.stdout, "dry-run: 刪除 %s\n", dst)
			continue
		}
		if err := client.DeleteObject(ctx, loc.Bucket, key); err != nil {
			return err
		}
		a.progress("已刪除 %s", dst)
		summaries = append(summaries, syncSummary{Action: "delete", Dst: dst})
	}

	if a.opts.dryRun {
		_, _ = fmt.Fprintf(a.stdout, "dry-run: 上傳 %d、略過 %d、刪除 %d\n", len(uploads), skipped, len(deletions))
		return nil
	}
	a.progress("sync 完成：上傳 %d、略過 %d、刪除 %d", len(uploads), skipped, len(deletions))
	return renderSummary(a, summaries)
}

// planSyncUploads 依「遠端不存在 → 大小 → MD5（ETag 為純 MD5 時）→ mtime（multipart ETag 時）」決定要上傳的檔案，
// 回傳上傳清單與略過數。
func planSyncUploads(files []cosUploadFile, remote map[string]cos.Object) ([]syncUpload, int, error) {
	var uploads []syncUpload
	skipped := 0
	for _, f := range files {
		obj, exists := remote[f.key]
		switch {
		case !exists:
			uploads = append(uploads, syncUpload{file: f, reason: "新檔案"})
		case obj.Size != f.size:
			uploads = append(uploads, syncUpload{file: f, reason: "大小不同"})
		case cos.IsPlainMD5ETag(obj.ETag):
			sum, err := cos.FileMD5(f.path)
			if err != nil {
				return nil, 0, fmt.Errorf("比對 %s 失敗: %w", f.path, err)
			}
			// 部分 S3 相容服務（例如某些 Ceph 版本）回傳大寫 hex ETag；FileMD5 固定回傳
			// 小寫，用 EqualFold 不分大小寫比對，避免內容相同的檔案被誤判「內容不同」而重傳。
			if !strings.EqualFold(sum, strings.Trim(obj.ETag, `"`)) {
				uploads = append(uploads, syncUpload{file: f, reason: "內容不同"})
			} else {
				skipped++
			}
		case f.modTime.After(obj.LastModified):
			// multipart 上傳的 ETag 不是內容 MD5，退而以修改時間判斷
			uploads = append(uploads, syncUpload{file: f, reason: "本地較新"})
		default:
			skipped++
		}
	}
	return uploads, skipped, nil
}

// planSyncDeletions 回傳遠端有、本地沒有的 key（略過以 `/` 結尾的目錄標記），依 key 排序以便輸出穩定。
func planSyncDeletions(files []cosUploadFile, remote map[string]cos.Object) []string {
	local := make(map[string]bool, len(files))
	for _, f := range files {
		local[f.key] = true
	}
	var keys []string
	for key := range remote {
		if !local[key] && !strings.HasSuffix(key, "/") {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}
