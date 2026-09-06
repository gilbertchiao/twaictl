// Package cli 定義 twaictl 的所有 cobra 命令。
package cli

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/gilbertchiao/twaictl/internal/config"
	"github.com/gilbertchiao/twaictl/internal/output"
	"github.com/gilbertchiao/twaictl/internal/twai"
	"github.com/gilbertchiao/twaictl/internal/twai/cos"
	"github.com/gilbertchiao/twaictl/internal/twai/vcs"
	"github.com/gilbertchiao/twaictl/internal/version"
)

// globalOptions 保存所有全域 flag 的值（設計文件 3.2 節）。
type globalOptions struct {
	profile      string
	project      string
	output       string
	columns      string
	noHeader     bool
	verbose      bool
	dryRun       bool
	yes          bool
	wait         bool
	timeout      time.Duration
	waitTimeout  time.Duration
	waitInterval time.Duration
}

// app 是一次命令執行的共享狀態，由 root command 建立並傳給各子命令。
type app struct {
	opts   *globalOptions
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	getenv func(string) string
	logger *slog.Logger

	configPath string
	config     *config.Config
	settings   *config.Settings

	vcs *vcs.Service
	// resolvedProjectID 快取 (*app).projectID 解析出的結果。
	resolvedProjectID int64
	// client 快取 (*app).apiClient 建立出的結果，避免同一次執行重複建立 transport
	// （例如 whoami 同時用到 Common 與 VCS 服務，之前會各自建立一組 client）。
	client *twai.Client

	cos *cos.Service
	// resolvedCOSProjectID 快取 (*app).cosProjectID 解析出的結果；與上面 resolvedProjectID
	// 是各自獨立的編號空間（Ceph 的 project id 與 VCS 的不同），不可共用同一個欄位。
	resolvedCOSProjectID int64
	// s3 快取 (*app).s3Client 建立出的結果。
	s3 *cos.S3
}

// Execute 建立 root command 並執行，回傳 process exit code。
// 所有 I/O 與環境變數都由參數注入，讓測試可以完全控制。
func Execute(args []string, stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string) int {
	rootCmd := newRootCmd(stdin, stdout, stderr, getenv)
	rootCmd.SetArgs(args)
	if err := rootCmd.Execute(); err != nil {
		if errors.Is(err, twai.ErrDryRun) {
			return ExitOK
		}
		// stderr 的寫入失敗無法再回報給使用者，故忽略其 error（errcheck 已知例外情境）。
		_, _ = fmt.Fprintf(stderr, "twaictl: %v\n", err)
		return exitCodeFor(err)
	}
	return ExitOK
}

func newRootCmd(stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string) *cobra.Command {
	a := &app{
		opts:   &globalOptions{},
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
		getenv: getenv,
	}

	rootCmd := &cobra.Command{
		Use:   "twaictl",
		Short: "台智雲 TWCC 平台的非官方命令列工具",
		Long: "twaictl 是台智雲（Taiwan AI Cloud, TWAI）TWCC 平台的非官方（community）命令列工具，\n" +
			"以 Go 重新實作、取代已停止維護的 twcc/TWCC-CLI。",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			a.initLogger()
			if err := validateOutputFlag(a.opts.output); err != nil {
				return err
			}
			if err := validateWaitFlags(a.opts); err != nil {
				return err
			}
			if !needsSettings(cmd) {
				return nil
			}
			return a.loadSettings(isConfigInitCmd(cmd) || isCOSKeySaveCmd(cmd))
		},
	}
	rootCmd.SetIn(stdin)
	rootCmd.SetOut(stdout)
	rootCmd.SetErr(stderr)

	flags := rootCmd.PersistentFlags()
	flags.StringVar(&a.opts.profile, "profile", "", "使用設定檔中的哪個 profile（預設 default，或 TWAI_PROFILE）")
	flags.StringVar(&a.opts.project, "project", "", "覆寫 profile 中的 project code")
	flags.StringVarP(&a.opts.output, "output", "o", "table", "輸出格式：table|json|yaml")
	flags.StringVar(&a.opts.columns, "columns", "", "table 模式只顯示指定欄位（逗號分隔）")
	flags.BoolVar(&a.opts.noHeader, "no-header", false, "table 模式不印表頭")
	flags.BoolVarP(&a.opts.verbose, "verbose", "v", false, "輸出 debug 等級 log 到 stderr")
	flags.BoolVar(&a.opts.dryRun, "dry-run", false, "不送出請求，改印出等價的 curl 命令")
	flags.DurationVar(&a.opts.timeout, "timeout", 30*time.Second, "單次 HTTP 請求逾時")
	flags.BoolVar(&a.opts.wait, "wait", false, "對非同步操作輪詢直到完成")
	flags.DurationVar(&a.opts.waitTimeout, "wait-timeout", 10*time.Minute, "--wait 的逾時")
	flags.BoolVarP(&a.opts.yes, "yes", "y", false, "略過破壞性操作的確認提示")
	flags.DurationVar(&a.opts.waitInterval, "wait-interval", vcs.DefaultWaitInterval, "--wait 的輪詢間隔")
	_ = flags.MarkHidden("wait-interval") // 內部測試用旋鈕，不對使用者公開

	rootCmd.AddCommand(newVersionCmd(a.opts))
	rootCmd.AddCommand(newConfigCmd(a))
	rootCmd.AddCommand(newProjectCmd(a))
	rootCmd.AddCommand(newVCSCmd(a))
	rootCmd.AddCommand(newCosCmd(a))
	rootCmd.AddCommand(newAPICmd(a))
	return rootCmd
}

// NewDocsRoot 建立一份完整的命令樹供 `tools/gendocs` 走訪產生 docs/commands.md。
// I/O 全部導向 io.Discard（stdin 則給一個讀不到任何內容的 Reader）、getenv 一律
// 回傳空字串：呼叫端只會用 cobra.Command 的 metadata（Use/Short/Long/Flags）
// 走訪整棵樹，不會呼叫 Execute()，因此不會觸發 PersistentPreRunE、
// 不會讀取任何真實設定檔或環境變數、也不會打任何 API。
func NewDocsRoot() *cobra.Command {
	return newRootCmd(strings.NewReader(""), io.Discard, io.Discard, func(string) string { return "" })
}

// validateOutputFlag 確認 -o 的值合法。
func validateOutputFlag(format string) error {
	_, err := output.ParseFormat(format)
	return err
}

// validateWaitFlags 確認 --wait-interval / --wait-timeout 為正值。
// 兩者都會傳給 time.NewTicker / context.WithTimeout：非正值會在輪詢時才 panic
// 或立即逾時，且此時可能已經送出破壞性請求（例如 vcs rm --wait），
// 因此在命令實際執行前（PersistentPreRunE）就先擋下。
func validateWaitFlags(opts *globalOptions) error {
	if opts.waitInterval <= 0 {
		return fmt.Errorf("--wait-interval 必須大於 0（目前 %s）", opts.waitInterval)
	}
	if opts.waitTimeout <= 0 {
		return fmt.Errorf("--wait-timeout 必須大於 0（目前 %s）", opts.waitTimeout)
	}
	return nil
}

// initLogger 建立輸出到 stderr 的 slog logger；--verbose 時為 Debug 等級。
func (a *app) initLogger() {
	level := slog.LevelWarn
	if a.opts.verbose {
		level = slog.LevelDebug
	}
	a.logger = slog.New(slog.NewTextHandler(a.stderr, &slog.HandlerOptions{Level: level}))
}

// commandsWithoutSettings 是不需要設定檔的診斷 / shell 整合命令；設定檔損毀時仍須可用
// （例如壞掉的設定檔會讓每次按 Tab 觸發 shell 補全都失敗）。
var commandsWithoutSettings = map[string]bool{
	"version":                       true,
	"completion":                    true,
	"help":                          true,
	cobra.ShellCompRequestCmd:       true,
	cobra.ShellCompNoDescRequestCmd: true,
}

// needsSettings 沿 parent 鏈往上找，讓 `completion bash` 這類子命令也被涵蓋。
func needsSettings(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if commandsWithoutSettings[c.Name()] {
			return false
		}
	}
	return true
}

// isConfigInitCmd 判斷目前執行的是否為 `config init`。
// `config init` 是唯一允許 --profile 指向「尚未存在」profile 的命令——
// 因為建立該 profile正是這個命令的目的；其餘命令（如 config show）
// 對不存在的 profile 一律視為設定錯誤。
func isConfigInitCmd(cmd *cobra.Command) bool {
	return cmd.Name() == "init" && cmd.Parent() != nil && cmd.Parent().Name() == "config"
}

// isCOSKeySaveCmd 判斷目前執行的是否為 --save 生效的 `cos key get` 或 `cos key renew`。
// 這兩個命令在目前 profile 不存在時會建立一個最小 profile（見 saveCOSKey），
// 因此比照 config init，允許 --profile / TWAI_PROFILE 明確指向「尚未存在」的 profile。
// cobra 會在呼叫 PersistentPreRunE 之前就先解析完 flag，所以這裡能直接讀解析後的
// 布林值（GetBool）——刻意不用 cmd.Flags().Changed("save")：Changed 只代表「使用者
// 有沒有打這個 flag」，`--save=false` 也會讓 Changed 為 true，若誤用 Changed 會讓
// 「明確關閉 --save」的呼叫也放寬了 profile 必須存在的檢查，讀 GetBool 才能正確
// 反映「這次執行到底會不會呼叫 saveCOSKey」。沒有生效 --save 的一般 `get`/`renew`
// 仍維持原本規則：指向不存在的 profile 是設定錯誤。
func isCOSKeySaveCmd(cmd *cobra.Command) bool {
	if cmd.Name() != "get" && cmd.Name() != "renew" {
		return false
	}
	parent := cmd.Parent()
	if parent == nil || parent.Name() != "key" {
		return false
	}
	grandparent := parent.Parent()
	if grandparent == nil || grandparent.Name() != "cos" {
		return false
	}
	save, err := cmd.Flags().GetBool("save")
	return err == nil && save
}

// loadSettings 讀取設定檔並解析優先序；警告（例如舊版環境變數）印到 stderr 一次。
// allowMissingProfile 為 true 時（僅 `config init`），交給 config.Resolve 的
// Overrides.AllowMissingProfile 處理：--profile 指向不存在的 profile 不視為錯誤，
// 改用零值 Profile 繼續解析，讓 config init 可以建立新 profile。
func (a *app) loadSettings(allowMissingProfile bool) error {
	path, err := config.DefaultPath(a.getenv)
	if err != nil {
		return err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	settings, err := config.Resolve(cfg, path, a.getenv, config.Overrides{
		Profile:             a.opts.profile,
		ProjectCode:         a.opts.project,
		AllowMissingProfile: allowMissingProfile,
	})
	if err != nil {
		return err
	}
	for _, warning := range settings.Warnings {
		_, _ = fmt.Fprintf(a.stderr, "twaictl: 警告: %s\n", warning)
	}
	a.configPath = path
	a.config = cfg
	a.settings = settings
	return nil
}

// apiClient 建立需要 API key 的 client；Phase 1 起所有呼叫 API 的命令都經由此處。
// 同一次執行只會建立一次（memoize），例如 whoami 同時透過 Common 與 vcsService()
// 用到 client，不需要也不該各自建立一組 transport。
func (a *app) apiClient() (*twai.Client, error) {
	if a.client != nil {
		return a.client, nil
	}
	if err := a.settings.RequireAPIKey(); err != nil {
		return nil, err
	}
	client, err := twai.NewClient(a.settings, twai.ClientOptions{
		Version:      version.Version,
		Timeout:      a.opts.timeout,
		DryRun:       a.opts.dryRun,
		DryRunWriter: a.stdout,
		Verbose:      a.opts.verbose,
		Logger:       a.logger,
	})
	if err != nil {
		return nil, err
	}
	a.client = client
	return client, nil
}
