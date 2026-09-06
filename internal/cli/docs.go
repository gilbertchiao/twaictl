package cli

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// skippedDocCommandNames 是產生文件時要整節略過的命令名稱：
// help、completion 是 cobra 內建的輔助命令，不是 twaictl 自己實作的功能，
// 因此不列入 docs/commands.md。
var skippedDocCommandNames = map[string]bool{
	"help":       true,
	"completion": true,
}

// WriteCommandDocs 走訪以 root 為根的 cobra 命令樹，輸出 Markdown 格式的命令文件：
// 開頭是連到各命令小節的目錄，之後每個命令各自一節（標題為完整命令路徑，例如
// `## twaictl cos sync`），內容依序是 Short、Long、Usage、該命令專屬的 Flags 表
// （名稱／簡寫／預設／說明）。持續性（inherited）flag 只會在根節（`## twaictl`）
// 出現一次，不會在每個子命令節重複列出；`Hidden` 的命令與 flag、以及 cobra 內建
// 的 help／completion 命令一律略過。輸出依命令完整路徑排序，且不含任何時間戳記，
// 確保同一份命令樹每次產生的結果都逐位元組相同（`make docs-check` 依此比對）。
func WriteCommandDocs(w io.Writer, root *cobra.Command) error {
	commands := collectDocCommands(root)

	bw := bufio.NewWriter(w)
	if _, err := fmt.Fprintf(bw, "# %s 命令列表\n\n", root.Name()); err != nil {
		return fmt.Errorf("寫入文件標題失敗: %w", err)
	}
	if _, err := fmt.Fprintln(bw, "本文件由 `tools/gendocs` 從 cobra 命令樹自動產生（`make docs`），請勿手動修改。"); err != nil {
		return fmt.Errorf("寫入文件說明失敗: %w", err)
	}
	if _, err := fmt.Fprintln(bw); err != nil {
		return fmt.Errorf("寫入文件說明後空行失敗: %w", err)
	}

	if err := writeTOC(bw, commands); err != nil {
		return err
	}
	for _, cmd := range commands {
		if err := writeCommandSection(bw, cmd); err != nil {
			return err
		}
	}

	if err := bw.Flush(); err != nil {
		return fmt.Errorf("寫出 docs/commands.md 失敗: %w", err)
	}
	return nil
}

// collectDocCommands 收集命令樹中所有應該列入文件的命令（含巢狀子命令），
// 並依完整命令路徑（例如 "twaictl vcs volume action"）排序。
// Hidden 的命令與 skippedDocCommandNames 列出的內建命令會連同其子命令整個略過。
func collectDocCommands(root *cobra.Command) []*cobra.Command {
	var result []*cobra.Command

	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		if cmd.Hidden || skippedDocCommandNames[cmd.Name()] {
			return
		}
		result = append(result, cmd)
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(root)

	sort.Slice(result, func(i, j int) bool {
		return result[i].CommandPath() < result[j].CommandPath()
	})
	return result
}

// writeTOC 輸出目錄：每個命令一個連到該節錨點的連結。
func writeTOC(w io.Writer, commands []*cobra.Command) error {
	if _, err := fmt.Fprintln(w, "## 目錄"); err != nil {
		return fmt.Errorf("寫入目錄標題失敗: %w", err)
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return fmt.Errorf("寫入目錄標題後空行失敗: %w", err)
	}
	for _, cmd := range commands {
		path := cmd.CommandPath()
		if _, err := fmt.Fprintf(w, "- [%s](#%s)\n", path, commandAnchor(path)); err != nil {
			return fmt.Errorf("寫入目錄項目失敗（%s）: %w", path, err)
		}
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return fmt.Errorf("寫入目錄後空行失敗: %w", err)
	}
	return nil
}

// commandAnchor 把命令完整路徑轉成 GitHub 風格的標題錨點（小寫、空白換成 "-"）。
// twaictl 命令名稱只會出現小寫英文字母、數字與連字號（見各 newXxxCmd 的 Use 欄位），
// 不含需要額外處理的特殊字元，因此只需要處理大小寫與空白即可。
func commandAnchor(path string) string {
	return strings.ReplaceAll(strings.ToLower(path), " ", "-")
}

// writeCommandSection 輸出單一命令的完整小節：標題、Short、Long、Usage、Flags 表。
func writeCommandSection(w io.Writer, cmd *cobra.Command) error {
	path := cmd.CommandPath()

	if _, err := fmt.Fprintf(w, "## %s\n\n", path); err != nil {
		return fmt.Errorf("寫入命令標題失敗（%s）: %w", path, err)
	}
	if cmd.Short != "" {
		if _, err := fmt.Fprintf(w, "%s\n\n", escapeProse(cmd.Short)); err != nil {
			return fmt.Errorf("寫入 Short 說明失敗（%s）: %w", path, err)
		}
	}
	if cmd.Long != "" {
		if _, err := fmt.Fprintf(w, "%s\n\n", escapeProse(cmd.Long)); err != nil {
			return fmt.Errorf("寫入 Long 說明失敗（%s）: %w", path, err)
		}
	}
	if _, err := fmt.Fprintf(w, "**Usage**\n\n```\n%s\n```\n\n", cmd.UseLine()); err != nil {
		return fmt.Errorf("寫入 Usage 失敗（%s）: %w", path, err)
	}
	if err := writeFlagsTable(w, cmd); err != nil {
		return err
	}
	return nil
}

// writeFlagsTable 輸出命令「自己定義」的 flag（不含從父命令繼承來的持續性 flag，
// 那些只會在根節列一次，見 WriteCommandDocs 的說明）。沒有可列出的 flag 時輸出
// 「無」，不印空表格。
//
// `--help` 不會出現在任何命令的 Flags 表中：cobra 只在 Execute() 執行期間才會由
// InitDefaultHelpFlag 幫每個命令補上 `--help`，而 NewDocsRoot 建出的命令樹只走訪
// metadata、從不呼叫 Execute()，故 LocalFlags() 走訪不到它。這是刻意的行為
// （`--help` 是 cobra 內建、不是 twaictl 自己定義的 flag，列出來對讀者沒有額外資訊）。
func writeFlagsTable(w io.Writer, cmd *cobra.Command) error {
	path := cmd.CommandPath()

	var flags []*pflag.Flag
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		flags = append(flags, f)
	})

	if len(flags) == 0 {
		if _, err := fmt.Fprintln(w, "**Flags**：無"); err != nil {
			return fmt.Errorf("寫入 Flags 區塊失敗（%s）: %w", path, err)
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return fmt.Errorf("寫入 Flags 區塊後空行失敗（%s）: %w", path, err)
		}
		return nil
	}

	if _, err := fmt.Fprintln(w, "**Flags**"); err != nil {
		return fmt.Errorf("寫入 Flags 標題失敗（%s）: %w", path, err)
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return fmt.Errorf("寫入 Flags 標題後空行失敗（%s）: %w", path, err)
	}
	if _, err := fmt.Fprintln(w, "| 名稱 | 簡寫 | 預設 | 說明 |"); err != nil {
		return fmt.Errorf("寫入 Flags 表頭失敗（%s）: %w", path, err)
	}
	if _, err := fmt.Fprintln(w, "|---|---|---|---|"); err != nil {
		return fmt.Errorf("寫入 Flags 表頭分隔線失敗（%s）: %w", path, err)
	}
	for _, f := range flags {
		if err := writeFlagRow(w, path, f); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return fmt.Errorf("寫入 Flags 表格後空行失敗（%s）: %w", path, err)
	}
	return nil
}

// writeFlagRow 輸出 Flags 表的單一列。
func writeFlagRow(w io.Writer, cmdPath string, f *pflag.Flag) error {
	shorthand := ""
	if f.Shorthand != "" {
		shorthand = "`-" + f.Shorthand + "`"
	}
	def := ""
	if f.DefValue != "" {
		def = "`" + escapeTableCell(f.DefValue) + "`"
	}
	name := "`--" + escapeTableCell(f.Name) + "`"
	usage := escapeTableCell(f.Usage)

	if _, err := fmt.Fprintf(w, "| %s | %s | %s | %s |\n", name, shorthand, def, usage); err != nil {
		return fmt.Errorf("寫入 Flags 表格列失敗（%s, --%s）: %w", cmdPath, f.Name, err)
	}
	return nil
}

// escapeTableCell 讓字串可以安全放進 Markdown 表格的一個儲存格：
// 先跳脫角括號（見 escapeProse），再跳脫 "|"（否則會被誤判為新的欄位分隔），
// 並把換行壓成空白（表格儲存格不能跨行）。
func escapeTableCell(s string) string {
	s = escapeProse(s)
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

// escapeProse 跳脫一般說明文字（Short/Long/Flags 表儲存格）裡的角括號，避免
// Markdown/GitHub 把 CLI 常見的佔位符（例如 `<path>`、`<id|name>`）誤判成 HTML
// tag，導致該段文字整個從算繹結果消失。只能用在一般文字：Usage 的 fenced code
// block（`cmd.UseLine()`）本來就會照字面顯示角括號，不應該套用這個跳脫，
// 否則使用者複製貼上時會得到錯誤的 `&lt;`/`&gt;` 而不是原本的 `<`/`>`。
func escapeProse(s string) string {
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
