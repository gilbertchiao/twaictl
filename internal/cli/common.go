package cli

import (
	"bufio"
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gilbertchiao/twaictl/internal/config"
	"github.com/gilbertchiao/twaictl/internal/output"
	"github.com/gilbertchiao/twaictl/internal/twai/cos"
	"github.com/gilbertchiao/twaictl/internal/twai/vcs"
)

// outputOptions 把全域 flags 轉成 output.Options。
func (o *globalOptions) outputOptions() output.Options {
	var columns []string
	for _, c := range strings.Split(o.columns, ",") {
		if c = strings.TrimSpace(c); c != "" {
			columns = append(columns, c)
		}
	}
	return output.Options{Format: output.Format(o.output), Columns: columns, NoHeader: o.noHeader}
}

// vcsService 延遲建立 VCS wrapper（需要 API key）。
func (a *app) vcsService() (*vcs.Service, error) {
	if a.vcs != nil {
		return a.vcs, nil
	}
	client, err := a.apiClient()
	if err != nil {
		return nil, err
	}
	a.vcs = vcs.New(client)
	return a.vcs, nil
}

// projectID 把生效的 project code 解析成數字 id，並在同一次執行內快取。
func (a *app) projectID(ctx context.Context) (int64, error) {
	if a.resolvedProjectID != 0 {
		return a.resolvedProjectID, nil
	}
	code := a.settings.ProjectCode
	if code == "" {
		return 0, &config.Error{Msg: "尚未設定 project code：請以 --project 指定，或執行 `twaictl config init`"}
	}
	svc, err := a.vcsService()
	if err != nil {
		return 0, err
	}
	id, err := svc.ResolveProjectID(ctx, code)
	if err != nil {
		return 0, err
	}
	a.resolvedProjectID = id
	return id, nil
}

// vcsProject 一次取得 VCS Service 與已解析的 project id，供只需要這兩者、
// 不必個別處理錯誤的命令（`vcs server ls`、`vcs action`、`vcs metrics` 等）使用。
func (a *app) vcsProject(ctx context.Context) (*vcs.Service, int64, error) {
	svc, err := a.vcsService()
	if err != nil {
		return nil, 0, err
	}
	projectID, err := a.projectID(ctx)
	if err != nil {
		return nil, 0, err
	}
	return svc, projectID, nil
}

// cosService 延遲建立 COS（Ceph）wrapper（需要 API key）。
func (a *app) cosService() (*cos.Service, error) {
	if a.cos != nil {
		return a.cos, nil
	}
	client, err := a.apiClient()
	if err != nil {
		return nil, err
	}
	a.cos = cos.New(client)
	return a.cos, nil
}

// cosProjectID 把生效的 project code 解析成 Ceph 的數字 project id，並在同一次執行內快取。
//
// 注意：Ceph 的 project id 與 VCS 的 project id 是各自獨立的編號空間（即使 project code
// 相同，兩邊解析出來的數字 id 也不一定相同），因此不能沿用 (*app).projectID 已快取的
// VCS id，必須各自向 Ceph 的 `GET /projects/?name=` 查詢、各自快取
// （見 internal/twai/cos.Service.ResolveProjectID 的說明）。
func (a *app) cosProjectID(ctx context.Context) (int64, error) {
	if a.resolvedCOSProjectID != 0 {
		return a.resolvedCOSProjectID, nil
	}
	code := a.settings.ProjectCode
	if code == "" {
		return 0, &config.Error{Msg: "尚未設定 project code：請以 --project 指定，或執行 `twaictl config init`"}
	}
	svc, err := a.cosService()
	if err != nil {
		return 0, err
	}
	id, err := svc.ResolveProjectID(ctx, code)
	if err != nil {
		return 0, err
	}
	a.resolvedCOSProjectID = id
	return id, nil
}

// requireS3Credentials 確認 COS S3 的 access key／secret key 皆已設定；
// 任一為空即回傳 *config.Error（cli 層會對應 exit code 2），並提示如何取得
// （`cos key get --save`，或直接以環境變數指定）。
func (a *app) requireS3Credentials() error {
	if a.settings.COS.AccessKey == "" || a.settings.COS.SecretKey == "" {
		return &config.Error{Msg: "尚未設定 COS S3 金鑰：請執行 `twaictl cos key get --save`，" +
			"或設定環境變數 TWAI_COS_ACCESS_KEY / TWAI_COS_SECRET_KEY"}
	}
	return nil
}

// s3Client 延遲建立 S3 資料面 client（需要 COS S3 access/secret key），同一次執行只建立一次。
func (a *app) s3Client() (*cos.S3, error) {
	if a.s3 != nil {
		return a.s3, nil
	}
	if err := a.requireS3Credentials(); err != nil {
		return nil, err
	}
	client, err := cos.NewS3(cos.S3Options{
		Endpoint:  a.settings.COS.Endpoint,
		AccessKey: a.settings.COS.AccessKey,
		SecretKey: a.settings.COS.SecretKey,
		Timeout:   a.opts.timeout,
	})
	if err != nil {
		return nil, err
	}
	a.s3 = client
	return a.s3, nil
}

// columnsHelp 產生 --columns 可用欄位與預設欄位的說明文字，供各命令的 Long 使用。
// README 只泛稱「欄位名稱見各命令 --help」，實際的欄位清單需要由各命令自己的欄位定義
// （output.Column 的 Name）產生，避免文件與程式碼的欄位名稱不同步。
func columnsHelp[T any](columns []output.Column[T]) string {
	names := make([]string, len(columns))
	var defaults []string
	for i, c := range columns {
		names[i] = c.Name
		if c.Default {
			defaults = append(defaults, c.Name)
		}
	}
	return fmt.Sprintf("可用欄位（--columns，不分大小寫）：%s\n預設欄位：%s", strings.Join(names, ", "), strings.Join(defaults, ", "))
}

// validateColumns 在送出任何請求（包含 --dry-run 印出的 curl 命令）之前，先驗證
// --columns 指定的欄位是否存在，避免這種純使用者輸入錯誤要等到 render 階段——
// 甚至在 --dry-run 下永遠等不到，因為 ErrDryRun 會讓 RunE 提早結束——才被發現。
// 只在 table 模式檢查，因為 json/yaml 輸出 API 原始回應，本就不受 --columns 影響
// （與 output.Render／renderOne 的既有行為一致，見 README「全域 flags」一節）。
func validateColumns[T any](a *app, columns []output.Column[T]) error {
	opts := a.opts.outputOptions()
	if opts.Format != output.FormatTable {
		return nil
	}
	_, err := output.SelectColumns(opts.Columns, columns)
	return err
}

// render 輸出清單。
func render[T any](a *app, raw []byte, items []T, columns []output.Column[T]) error {
	return output.Render(a.stdout, a.opts.outputOptions(), raw, items, columns)
}

// renderOne 輸出單一物件：table 直式「欄位: 值」，json/yaml 用 API 原始回應。
// table 模式一樣遵循全域 --columns（DESIGN 3.2），未知欄位回傳 output.ErrUnknownColumn。
func renderOne[T any](a *app, raw []byte, item T, columns []output.Column[T]) error {
	opts := a.opts.outputOptions()
	if opts.Format != output.FormatTable {
		return output.Render(a.stdout, opts, raw, []T{item}, columns)
	}
	selected, err := output.SelectColumns(opts.Columns, columns)
	if err != nil {
		return err
	}
	pairs := make([][2]string, 0, len(selected))
	for _, c := range selected {
		pairs = append(pairs, [2]string{c.Name, c.Value(item)})
	}
	return output.RenderKeyValues(a.stdout, pairs)
}

// confirm 對破壞性操作要求確認；-y 或 --dry-run 略過（--dry-run 不會送出請求，
// 要求確認毫無意義，反而會讓 CI／腳本在空 stdin 下卡住或誤判取消）。
// 提示在 stderr，stdout 維持乾淨。
func (a *app) confirm(action string) error {
	if a.opts.yes || a.opts.dryRun {
		return nil
	}
	_, _ = fmt.Fprintf(a.stderr, "即將%s，確定嗎？[y/N] ", action)
	line, _ := bufio.NewReader(a.stdin).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return nil
	}
	return fmt.Errorf("已取消")
}

// waitContext 依 --wait-timeout 建立輪詢用的 context。
func (a *app) waitContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, a.opts.waitTimeout)
}

// progress 在 stderr 印進度；-o json/yaml 時靜音以免干擾 pipe。
func (a *app) progress(format string, args ...any) {
	if a.opts.output != string(output.FormatTable) {
		return
	}
	_, _ = fmt.Fprintf(a.stderr, "twaictl: "+format+"\n", args...)
}

// changedString 回傳 flag 有被使用者指定時的值指標（含明確指定空字串），否則 nil。
func changedString(c *cobra.Command, name string, value *string) *string {
	if !c.Flags().Changed(name) {
		return nil
	}
	return value
}

// commandContext 取得 cobra 提供的 context。
func (a *app) commandContext(cmd *cobra.Command) context.Context {
	if ctx := cmd.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}
