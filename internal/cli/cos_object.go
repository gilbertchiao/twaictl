package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/gilbertchiao/twaictl/internal/output"
	"github.com/gilbertchiao/twaictl/internal/twai/cos"
)

// cosObjectDisplay 是 `cos ls` 的顯示資料：cos.Object 是 internal/twai/cos 套件的內部型別，
// 欄位沒有 json tag，這裡另外定義一份帶 snake_case json tag 的顯示型別，
// 供 render() 同時處理 table／json／yaml 三種格式。
type cosObjectDisplay struct {
	Key          string    `json:"key"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"last_modified"`
	ETag         string    `json:"etag"`
}

// newCosObjectDisplay 把 cos.Object 轉成顯示資料。
func newCosObjectDisplay(o cos.Object) cosObjectDisplay {
	return cosObjectDisplay{Key: o.Key, Size: o.Size, LastModified: o.LastModified, ETag: o.ETag}
}

// cosObjectColumns 是 `cos ls` 的欄位定義；ETag 預設不顯示（需要時可用 --columns etag 指定）。
var cosObjectColumns = []output.Column[cosObjectDisplay]{
	{Name: "Key", Default: true, Value: func(o cosObjectDisplay) string { return o.Key }},
	{Name: "Size", Default: true, Value: func(o cosObjectDisplay) string { return output.Bytes(o.Size) }},
	{Name: "Modified", Default: true, Value: func(o cosObjectDisplay) string { return output.FormatTime(o.LastModified) }},
	{Name: "ETag", Default: false, Value: func(o cosObjectDisplay) string { return o.ETag }},
}

// rmSummary 是 `cos rm` 在 -o json/yaml 時輸出的單筆操作摘要。
type rmSummary struct {
	Bucket    string `json:"bucket"`
	Key       string `json:"key"`
	VersionID string `json:"version_id,omitempty"`
}

// renderSummary 在 -o json 或 -o yaml 時把 items 序列化印到 stdout；cp/rm/sync/acl/
// content-type 這類操作型命令（不是清單型的 render）在 table 模式下 stdout 本就沒有輸出
// （進度另外經由 progress 印到 stderr），因此不透過 output.Render／--columns 那一套邏輯。
// json 直接 MarshalIndent 印出陣列；yaml 先 marshal 成 JSON（套用 json tag），
// 再交給 output.RenderRaw 轉成 YAML，讓兩種格式的欄位名稱與型別保持一致。
func renderSummary[T any](a *app, items []T) error {
	if items == nil {
		items = []T{}
	}
	return renderSummaryValue(a, items)
}

// renderSummaryOne 是 renderSummary 的單一物件版本，供摘要本身就是單一物件（不是陣列）
// 的命令使用（例如 `vcs action` 一次只對一個 VCS 實例送出動作）。
func renderSummaryOne[T any](a *app, item T) error {
	return renderSummaryValue(a, item)
}

// summaryWanted 回傳 true 時，renderSummary／renderSummaryOne 之後才真的會輸出
// （-o json 或 -o yaml）；-o table（預設）什麼都不印。兩段式刪除（`cos rm --recursive`／
// `--all-versions`、`cos bucket rm --purge`）的第二段用這個判斷決定要不要把每筆結果
// 累積進 rmSummary slice——table 模式下累積純屬浪費記憶體，在千萬物件量級的 bucket
// 上這個差異有意義。
func summaryWanted(a *app) bool {
	format := output.Format(a.opts.output)
	return format == output.FormatJSON || format == output.FormatYAML
}

// renderSummaryValue 是 renderSummary／renderSummaryOne 共用的實作：table 格式不印任何
// 東西（維持 nil）；json／yaml 之外的格式也不印（沒有第三種摘要格式）。
func renderSummaryValue(a *app, v any) error {
	if !summaryWanted(a) {
		return nil
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化為 JSON 失敗: %w", err)
	}
	if output.Format(a.opts.output) == output.FormatJSON {
		_, err = fmt.Fprintln(a.stdout, string(data))
		return err
	}
	return output.RenderRaw(a.stdout, output.FormatYAML, data)
}

// remotePrefix 正規化 S3 prefix：非空且不以 `/` 結尾時補上 `/`，避免用字串前綴比對時
// 誤命中望文生義的手足前綴（例如 prefix "dir" 會誤命中 "directory/other.txt"）；
// 空字串維持空字串（代表整個 bucket）。`cos cp`（上傳目錄、遞迴下載）與
// `cos rm --recursive` 都是把使用者提供的 key 當成前綴去 ListObjects，
// 因此共用同一個正規化規則。
func remotePrefix(key string) string {
	if key != "" && !strings.HasSuffix(key, "/") {
		return key + "/"
	}
	return key
}

// parseObjectArgs 把 cos:// 參數解析成 Location；非 recursive 時每個參數都必須是完整 key。
// `cos rm`、`cos acl`、`cos content-type` 共用同一份檢查。
func parseObjectArgs(args []string, recursive bool) ([]cos.Location, error) {
	locs := make([]cos.Location, 0, len(args))
	for _, arg := range args {
		loc, ok := cos.ParseURL(arg)
		if !ok {
			return nil, fmt.Errorf("%s 不是合法的 cos:// URL", arg)
		}
		if !recursive && (loc.Key == "" || strings.HasSuffix(loc.Key, "/")) {
			return nil, fmt.Errorf("%s 不是完整的 key，請加 --recursive", arg)
		}
		locs = append(locs, loc)
	}
	return locs, nil
}

// cosObjectVersionDisplay 是 `cos ls --versions` 的顯示資料（帶 snake_case json tag）。
type cosObjectVersionDisplay struct {
	Key          string    `json:"key"`
	VersionID    string    `json:"version_id"`
	IsLatest     bool      `json:"is_latest"`
	DeleteMarker bool      `json:"delete_marker"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"last_modified"`
	ETag         string    `json:"etag,omitempty"`
}

// cosObjectVersionColumns 是 `cos ls --versions` 的欄位定義。
var cosObjectVersionColumns = []output.Column[cosObjectVersionDisplay]{
	{Name: "Key", Default: true, Value: func(v cosObjectVersionDisplay) string { return v.Key }},
	{Name: "Version", Default: true, Value: func(v cosObjectVersionDisplay) string { return v.VersionID }},
	{Name: "Latest", Default: true, Value: func(v cosObjectVersionDisplay) string { return output.Bool(v.IsLatest) }},
	{Name: "Type", Default: true, Value: func(v cosObjectVersionDisplay) string {
		if v.DeleteMarker {
			return "delete marker"
		}
		return "object"
	}},
	{Name: "Size", Default: true, Value: func(v cosObjectVersionDisplay) string {
		if v.DeleteMarker {
			return ""
		}
		return output.Bytes(v.Size)
	}},
	{Name: "Modified", Default: true, Value: func(v cosObjectVersionDisplay) string { return output.FormatTime(v.LastModified) }},
	{Name: "ETag", Default: false, Value: func(v cosObjectVersionDisplay) string { return v.ETag }},
}

// versionTarget 是 rm --all-versions / bucket rm --purge 蒐集到的單一待刪除版本。
type versionTarget struct {
	bucket, key, versionID string
	deleteMarker           bool
}

// describeVersion 產生進度／dry-run 訊息中的版本描述：「版本 v1」或「delete marker v3」。
func describeVersion(t versionTarget) string {
	if t.deleteMarker {
		return "delete marker " + t.versionID
	}
	return "版本 " + t.versionID
}

// versionTargetPrefix 依 exactKey 算出要餵給 ListObjectVersions 的 prefix：exactKey 為 true
// （非 --recursive）時用 loc.Key 本身（下面 fn 還會再過濾成完全等於 loc.Key，避免
// ListObjectVersions 的 prefix 比對誤命中 "a.txt.bak" 這類手足 key）；否則用
// remotePrefix(loc.Key) 正規化後的前綴，取得該前綴下的所有版本。
func versionTargetPrefix(loc cos.Location, exactKey bool) string {
	if exactKey {
		return loc.Key
	}
	return remotePrefix(loc.Key)
}

// cosCountVersionTargets 是兩段式刪除的第一段：只計數 loc 底下符合條件（exactKey 規則同
// versionTargetPrefix）的物件版本數，不蒐集任何 key／versionID，讓確認訊息可以顯示
// 真實數量，同時千萬版本量級的 bucket 也不會在這一步佔用隨版本數線性成長的記憶體。
func (a *app) cosCountVersionTargets(ctx context.Context, client *cos.S3, loc cos.Location, exactKey bool) (int, error) {
	prefix := versionTargetPrefix(loc, exactKey)
	count := 0
	err := client.ListObjectVersions(ctx, loc.Bucket, prefix, func(v cos.ObjectVersion) error {
		if exactKey && v.Key != loc.Key {
			return nil
		}
		count++
		return nil
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}

// cosStreamDeleteVersionTargets 是兩段式刪除的第二段：重新呼叫 ListObjectVersions，
// --dry-run 時每一筆版本一找到就立即印出 dry-run 行；真正刪除時則延後一筆
// （lookbehind，見下方 pending 的說明），與下一頁的 ListObjectVersions 交錯進行，
// 不在記憶體中蒐集完整版本清單。collect 為 true 時（-o json/yaml，見 summaryWanted）
// 才把每筆結果累積進回傳的 summaries；回傳的 count 是這一段實際處理到的筆數，供呼叫端
// 與第一段的計數比較，偵測期間是否有變動。
func (a *app) cosStreamDeleteVersionTargets(ctx context.Context, client *cos.S3, loc cos.Location, exactKey, collect bool) (count int, summaries []rmSummary, err error) {
	prefix := versionTargetPrefix(loc, exactKey)

	if a.opts.dryRun {
		// --dry-run 不會真的呼叫 DeleteObjectVersion，不影響下一頁分頁用的
		// version-id-marker 是否還存在（見下面真正刪除分支的說明），不需要 lookbehind，
		// 逐筆看到就立即印出即可。
		err = client.ListObjectVersions(ctx, loc.Bucket, prefix, func(v cos.ObjectVersion) error {
			if exactKey && v.Key != loc.Key {
				return nil
			}
			count++
			t := versionTarget{bucket: loc.Bucket, key: v.Key, versionID: v.VersionID, deleteMarker: v.DeleteMarker}
			_, _ = fmt.Fprintf(a.stdout, "dry-run: 刪除 cos://%s/%s（%s）\n", t.bucket, t.key, describeVersion(t))
			return nil
		})
		return count, nil, err
	}

	// 真正刪除時：ListObjectVersions（internal/twai/cos.S3.ListObjectVersions）在回應沒有
	// NextVersionIdMarker 時，會退回用「這一頁最後一筆」的 versionID 當下一頁請求的
	// version-id-marker；那個 versionID 必須在下一頁請求送出之前仍然存在，否則 S3／Ceph
	// 會回 400 InvalidArgument——version id 是不透明字串，不像 V1 ListObjects 的 Marker
	// 只是純字串比對、不需要對應的 key 還存在（見 cosRmRecursive 的說明，V1 物件路徑因此
	// 不需要這裡的處理）。
	//
	// 因此這裡用「延後一筆」的 lookbehind：每一筆版本先記成 pending，等下一筆版本的
	// callback 被呼叫時（若剛好跨頁，代表下一頁的 list 請求已經送出且成功）才真正刪除
	// 上一筆 pending；迴圈結束後再補刪最後一筆 pending。記憶體成本維持 O(1)（同一時間
	// 最多一筆待刪），且能保證任何被拿去當 marker 的版本，在被拿去 list 下一頁之前
	// 不會被刪除。
	var pending *versionTarget
	deletePending := func() error {
		if pending == nil {
			return nil
		}
		t := *pending
		pending = nil
		if delErr := client.DeleteObjectVersion(ctx, t.bucket, t.key, t.versionID); delErr != nil {
			return delErr
		}
		a.progress("已刪除 cos://%s/%s（%s）", t.bucket, t.key, describeVersion(t))
		if collect {
			summaries = append(summaries, rmSummary{Bucket: t.bucket, Key: t.key, VersionID: t.versionID})
		}
		return nil
	}

	err = client.ListObjectVersions(ctx, loc.Bucket, prefix, func(v cos.ObjectVersion) error {
		if exactKey && v.Key != loc.Key {
			return nil
		}
		if delErr := deletePending(); delErr != nil {
			return delErr
		}
		count++
		t := versionTarget{bucket: loc.Bucket, key: v.Key, versionID: v.VersionID, deleteMarker: v.DeleteMarker}
		pending = &t
		return nil
	})
	if err != nil {
		return count, summaries, err
	}
	if delErr := deletePending(); delErr != nil {
		return count, summaries, delErr
	}
	return count, summaries, nil
}

// newCosLsCmd 建立 `cos ls`。
func newCosLsCmd(a *app) *cobra.Command {
	var versions bool
	cmd := &cobra.Command{
		Use:   "ls <bucket|cos://bucket[/prefix]> [prefix]",
		Short: "列出 bucket 內的物件",
		Long: "列出 bucket 內的物件。\n\n" + columnsHelp(cosObjectColumns) +
			"\n\n--versions 時：\n" + columnsHelp(cosObjectVersionColumns),
		Args: cobra.RangeArgs(1, 2),
		RunE: func(c *cobra.Command, args []string) error {
			if versions {
				if err := validateColumns(a, cosObjectVersionColumns); err != nil {
					return err
				}
			} else if err := validateColumns(a, cosObjectColumns); err != nil {
				return err
			}
			bucket, prefix, err := parseCosLsArgs(args)
			if err != nil {
				return err
			}
			client, err := a.s3Client()
			if err != nil {
				return err
			}
			ctx := a.commandContext(c)
			if versions {
				items := []cosObjectVersionDisplay{}
				err = client.ListObjectVersions(ctx, bucket, prefix, func(v cos.ObjectVersion) error {
					items = append(items, cosObjectVersionDisplay{Key: v.Key, VersionID: v.VersionID, IsLatest: v.IsLatest,
						DeleteMarker: v.DeleteMarker, Size: v.Size, LastModified: v.LastModified, ETag: v.ETag})
					return nil
				})
				if err != nil {
					return err
				}
				return render(a, nil, items, cosObjectVersionColumns)
			}
			// 初始化為空切片（而非 nil）：bucket 沒有符合的物件時，-o json 才會輸出
			// "[]" 而不是 json.Marshal(nil slice) 產生的 "null"，對消費 JSON 輸出的
			// 腳本較友善（不必額外處理 null 才能當陣列疊代）。
			items := []cosObjectDisplay{}
			err = client.ListObjects(ctx, bucket, prefix, func(o cos.Object) error {
				items = append(items, newCosObjectDisplay(o))
				return nil
			})
			if err != nil {
				return err
			}
			return render(a, nil, items, cosObjectColumns)
		},
	}
	cmd.Flags().BoolVar(&versions, "versions", false, "列出所有物件版本與 delete marker（versioning bucket 用）")
	return cmd
}

// parseCosLsArgs 解析 `cos ls` 的參數：第一個參數可以是純 bucket 名稱、`cos://bucket[/prefix]`，
// 或是不含 `cos://` 但含 `/` 的 "bucket/prefix" 形式（以第一個 `/` 為界切開，方便不想打
// `cos://` 前綴的使用者）；第二個參數（若有）為 prefix。多於一種來源都指定 prefix 時視為錯誤。
func parseCosLsArgs(args []string) (bucket, prefix string, err error) {
	first := args[0]
	switch {
	case cos.IsRemote(first):
		loc, ok := cos.ParseURL(first)
		if !ok {
			return "", "", fmt.Errorf("%s 不是合法的 cos:// URL", first)
		}
		bucket, prefix = loc.Bucket, loc.Key
	case strings.Contains(first, "/"):
		bucket, prefix, _ = strings.Cut(first, "/")
	default:
		bucket = first
	}
	if len(args) == 2 {
		if prefix != "" {
			return "", "", fmt.Errorf("prefix 不可同時由 %s 與第二個參數指定", first)
		}
		prefix = args[1]
	}
	return bucket, prefix, nil
}

// newCosRmCmd 建立 `cos rm`：破壞性操作，需確認或 -y。
func newCosRmCmd(a *app) *cobra.Command {
	var recursive, allVersions bool
	cmd := &cobra.Command{
		Use:   "rm cos://bucket/key...",
		Short: "刪除 COS 物件（破壞性操作，需確認或 -y）",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			locs, err := parseObjectArgs(args, recursive)
			if err != nil {
				return err
			}
			ctx := a.commandContext(c)
			if allVersions {
				return a.cosRmAllVersions(ctx, locs, recursive)
			}
			if !recursive {
				if err := a.confirm(fmt.Sprintf("刪除 %d 個物件", len(locs))); err != nil {
					return err
				}
				return a.cosRmObjects(ctx, locs)
			}
			return a.cosRmRecursive(ctx, locs)
		},
	}
	cmd.Flags().BoolVar(&recursive, "recursive", false, "遞迴刪除前綴下的所有物件")
	cmd.Flags().BoolVar(&allVersions, "all-versions", false, "刪除該 key（或 --recursive 前綴下）的所有物件版本與 delete marker（versioning bucket 用）")
	return cmd
}

// cosRmAllVersions 處理 --all-versions：兩段式刪除。第一段（cosCountVersionTargets）只計數，
// 確認訊息用這個計數；確認後第二段（cosStreamDeleteVersionTargets）重新 list 並逐一刪除
// （--dry-run 則逐筆印出），全程不在記憶體中蒐集完整版本清單——bucket 版本數很多（千萬級）
// 時這是必要的，見 README「已知限制」。
//
// pass 2 若發現的實際版本數與 pass 1 的計數不同（列出期間有版本新增或被其他人刪除），
// 照常處理完，只在 stderr 印出實際刪除數提示，不視為錯誤。
func (a *app) cosRmAllVersions(ctx context.Context, locs []cos.Location, recursive bool) error {
	client, err := a.s3Client()
	if err != nil {
		return err
	}

	exactKey := !recursive
	total := 0
	descs := make([]string, 0, len(locs))
	for _, loc := range locs {
		count, err := a.cosCountVersionTargets(ctx, client, loc, exactKey)
		if err != nil {
			return err
		}
		total += count
		if recursive {
			descs = append(descs, rmRecursiveTargetDesc(loc.Bucket, remotePrefix(loc.Key)))
		} else {
			descs = append(descs, loc.String())
		}
	}
	if total == 0 {
		a.progress("沒有符合的物件版本")
		return nil
	}
	if err := a.confirm(fmt.Sprintf("刪除 %d 個物件版本（含 delete marker；%s）", total, strings.Join(descs, "、"))); err != nil {
		return err
	}

	collect := summaryWanted(a)
	var summaries []rmSummary
	actual := 0
	for _, loc := range locs {
		n, found, err := a.cosStreamDeleteVersionTargets(ctx, client, loc, exactKey, collect)
		if err != nil {
			return err
		}
		actual += n
		summaries = append(summaries, found...)
	}
	if actual != total {
		// dry-run 只是印出「會刪除」的提示，並沒有真的呼叫 DeleteObjectVersion，
		// 故訊息改用「處理」而非「刪除」，避免使用者誤以為 dry-run 底下也真的刪了東西。
		verb := "刪除"
		if a.opts.dryRun {
			verb = "處理"
		}
		a.progress("實際%s %d 個物件版本（確認當下為 %d 個，期間有變動）", verb, actual, total)
	}
	if a.opts.dryRun {
		return nil
	}
	return renderSummary(a, summaries)
}

// cosRmObjects 逐一刪除已知完整 key 的物件（非 --recursive）。
func (a *app) cosRmObjects(ctx context.Context, locs []cos.Location) error {
	if a.opts.dryRun {
		for _, loc := range locs {
			_, _ = fmt.Fprintf(a.stdout, "dry-run: 刪除 %s\n", loc.String())
		}
		return nil
	}
	client, err := a.s3Client()
	if err != nil {
		return err
	}
	summaries := make([]rmSummary, 0, len(locs))
	for _, loc := range locs {
		if err := client.DeleteObject(ctx, loc.Bucket, loc.Key); err != nil {
			return err
		}
		a.progress("已刪除 %s", loc.String())
		summaries = append(summaries, rmSummary{Bucket: loc.Bucket, Key: loc.Key})
	}
	return renderSummary(a, summaries)
}

// cosRmRecursive 以每個 loc.Key 正規化後的前綴（見 remotePrefix）兩段式刪除物件：
// 第一段（pass 1）只呼叫 ListObjects 計數，不蒐集任何 key；確認訊息用這個計數。
// 確認後第二段（pass 2）重新呼叫 ListObjects，每一筆物件一找到就立即刪除
// （或 --dry-run 印出 dry-run 行），與下一頁的 ListObjects 交錯進行，全程不在記憶體中
// 蒐集完整物件清單——bucket 物件數很多（千萬級）時這是必要的，見 README「已知限制」。
//
// pass 2 與 pass 1 用的是同一套 Marker 分頁：Marker 只是「從這個 key 之後開始列」的
// 字串參數，不依賴該 key 是否還存在，因此刪除已走訪過的 key 不影響分頁的正確性，
// pass 2 邊列邊刪是安全的。
//
// 正規化前綴（非空時強制補 `/`）避免 "dir" 誤刪 "directory/other.txt" 這類手足前綴的物件。
// 一個物件都沒找到時，直接印出提示並成功結束，不會印出確認提示（避免在空 stdin 下
// 卡住，或誤讓使用者以為要刪除東西）。
//
// 若 pass 2 實際看到的物件數與 pass 1 不同（列出期間有物件新增或被其他人刪除），
// 照常處理完，只在 stderr 印出實際刪除數提示，不視為錯誤。
func (a *app) cosRmRecursive(ctx context.Context, locs []cos.Location) error {
	client, err := a.s3Client()
	if err != nil {
		return err
	}

	total := 0
	descs := make([]string, 0, len(locs))
	for _, loc := range locs {
		prefix := remotePrefix(loc.Key)
		count := 0
		if err := client.ListObjects(ctx, loc.Bucket, prefix, func(cos.Object) error {
			count++
			return nil
		}); err != nil {
			return err
		}
		total += count
		descs = append(descs, rmRecursiveTargetDesc(loc.Bucket, prefix))
	}

	if total == 0 {
		a.progress("沒有符合的物件")
		return nil
	}

	if err := a.confirm(fmt.Sprintf("刪除 %d 個物件（%s）", total, strings.Join(descs, "、"))); err != nil {
		return err
	}

	collect := summaryWanted(a)
	var summaries []rmSummary
	actual := 0
	for _, loc := range locs {
		prefix := remotePrefix(loc.Key)
		if err := client.ListObjects(ctx, loc.Bucket, prefix, func(o cos.Object) error {
			actual++
			if a.opts.dryRun {
				_, _ = fmt.Fprintf(a.stdout, "dry-run: 刪除 cos://%s/%s\n", loc.Bucket, o.Key)
				return nil
			}
			if err := client.DeleteObject(ctx, loc.Bucket, o.Key); err != nil {
				return err
			}
			a.progress("已刪除 cos://%s/%s", loc.Bucket, o.Key)
			if collect {
				summaries = append(summaries, rmSummary{Bucket: loc.Bucket, Key: o.Key})
			}
			return nil
		}); err != nil {
			return err
		}
	}

	if actual != total {
		// 理由同 cosRmAllVersions：dry-run 底下只是印出提示，沒有真的刪除。
		verb := "刪除"
		if a.opts.dryRun {
			verb = "處理"
		}
		a.progress("實際%s %d 個物件（確認當下為 %d 個，期間有變動）", verb, actual, total)
	}

	if a.opts.dryRun {
		return nil
	}
	return renderSummary(a, summaries)
}

// rmRecursiveTargetDesc 把 bucket／正規化後的前綴組成確認訊息裡人類可讀的描述；
// 前綴為空代表整個 bucket，用「整個 bucket cos://b」明確表達，避免使用者誤以為
// 「（前綴 cos://b/）」是指某個叫空字串的前綴。
func rmRecursiveTargetDesc(bucket, prefix string) string {
	if prefix == "" {
		return fmt.Sprintf("整個 bucket cos://%s", bucket)
	}
	return fmt.Sprintf("前綴 cos://%s/%s", bucket, prefix)
}
