package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/gilbertchiao/twaictl/internal/twai/cos"
)

// aclSummary 是 `cos acl` 在 -o json/yaml 時輸出的單筆結果。
type aclSummary struct {
	Bucket string `json:"bucket"`
	Key    string `json:"key"`
	ACL    string `json:"acl"`
}

// contentTypeSummary 是 `cos content-type` 在 -o json/yaml 時輸出的單筆結果。
type contentTypeSummary struct {
	Bucket      string `json:"bucket"`
	Key         string `json:"key"`
	ContentType string `json:"content_type"`
}

// newCosAclCmd 建立 `cos acl`：以 canned ACL 把物件設為公開讀取或私有。
func newCosAclCmd(a *app) *cobra.Command {
	var public, private, recursive bool
	cmd := &cobra.Command{
		Use:   "acl cos://bucket/key... (--public | --private) [--recursive]",
		Short: "設定物件 ACL：--public 為 public-read（匿名可讀，需確認或 -y）、--private 為 private",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if public == private {
				return fmt.Errorf("--public 與 --private 必須擇一")
			}
			locs, err := parseObjectArgs(args, recursive)
			if err != nil {
				return err
			}
			return a.cosAcl(a.commandContext(c), locs, public, recursive)
		},
	}
	flags := cmd.Flags()
	flags.BoolVar(&public, "public", false, "設為 public-read（任何人可讀）")
	flags.BoolVar(&private, "private", false, "設為 private")
	flags.BoolVar(&recursive, "recursive", false, "對前綴下的所有物件套用")
	return cmd
}

// expandObjectTargets 把 locs 展開成實際物件：recursive 時以前綴 ListObjects，否則原樣回傳。
func (a *app) expandObjectTargets(ctx context.Context, client *cos.S3, locs []cos.Location, recursive bool) ([]cos.Location, error) {
	if !recursive {
		return locs, nil
	}
	var targets []cos.Location
	for _, loc := range locs {
		if err := client.ListObjects(ctx, loc.Bucket, remotePrefix(loc.Key), func(o cos.Object) error {
			targets = append(targets, cos.Location{Bucket: loc.Bucket, Key: o.Key})
			return nil
		}); err != nil {
			return nil, err
		}
	}
	return targets, nil
}

// cosAcl 展開目標、（--public 時）確認、逐一設定 ACL 或 dry-run 印出。
// 非遞迴的 --dry-run：locs 本身就是完整目標，不需要展開（遞迴才要 ListObjects），
// 因此不建立 s3Client，讓「只想看看會動到哪些物件」的 dry-run 不必先準備好 COS 憑證。
func (a *app) cosAcl(ctx context.Context, locs []cos.Location, public, recursive bool) error {
	aclName := "private"
	if public {
		aclName = "public-read"
	}
	var client *cos.S3
	if recursive || !a.opts.dryRun {
		c, err := a.s3Client()
		if err != nil {
			return err
		}
		client = c
	}
	targets, err := a.expandObjectTargets(ctx, client, locs, recursive)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		a.progress("沒有符合的物件")
		return nil
	}
	if public {
		if err := a.confirm(fmt.Sprintf("將 %d 個物件設為公開讀取（public-read）", len(targets))); err != nil {
			return err
		}
	}
	summaries := make([]aclSummary, 0, len(targets))
	for _, t := range targets {
		if a.opts.dryRun {
			_, _ = fmt.Fprintf(a.stdout, "dry-run: 設定 %s 為 %s\n", t.String(), aclName)
			continue
		}
		if err := client.SetObjectACL(ctx, t.Bucket, t.Key, public); err != nil {
			return err
		}
		a.progress("已設定 %s 為 %s", t.String(), aclName)
		summaries = append(summaries, aclSummary{Bucket: t.Bucket, Key: t.Key, ACL: aclName})
	}
	if a.opts.dryRun {
		return nil
	}
	return renderSummary(a, summaries)
}

// newCosContentTypeCmd 建立 `cos content-type`：就地修改物件的 Content-Type（以 CopyObject 實作，保留內容、metadata 與公開狀態）。
func newCosContentTypeCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "content-type cos://bucket/key <type>",
		Short: "修改物件的 Content-Type（例如 text/html）",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			locs, err := parseObjectArgs(args[:1], false)
			if err != nil {
				return err
			}
			loc, contentType := locs[0], args[1]
			if a.opts.dryRun {
				_, _ = fmt.Fprintf(a.stdout, "dry-run: 設定 %s 的 Content-Type 為 %s\n", loc.String(), contentType)
				return nil
			}
			client, err := a.s3Client()
			if err != nil {
				return err
			}
			if err := client.SetContentType(a.commandContext(c), loc.Bucket, loc.Key, contentType); err != nil {
				return err
			}
			a.progress("已設定 %s 的 Content-Type 為 %s", loc.String(), contentType)
			return renderSummary(a, []contentTypeSummary{{Bucket: loc.Bucket, Key: loc.Key, ContentType: contentType}})
		},
	}
	return cmd
}
