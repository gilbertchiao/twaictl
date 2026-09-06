package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWriteCommandDocsListsAllVisibleCommands 確認產生的文件涵蓋所有可見命令
// （含巢狀子命令，例如 `vcs volume action`），並略過隱藏的 flag／cobra 內建的
// help、completion 命令。
func TestWriteCommandDocsListsAllVisibleCommands(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, WriteCommandDocs(&buf, NewDocsRoot()))
	out := buf.String()

	assert.Contains(t, out, "## twaictl cos sync")
	assert.Contains(t, out, "## twaictl vcs volume action")
	assert.NotContains(t, out, "## twaictl completion")
	assert.NotContains(t, out, "## twaictl help")
	// --wait-interval 是 root.go 裡 MarkHidden 的內部旋鈕；「隱藏 flag 略過」指的是
	// 不會出現在 Flags 表的某一列（反引號包住的 `--wait-interval`），不是禁止這串字元
	// 出現在任何地方——`vcs action` 的 Long 說明文字就會提到它（描述 --wait 的極限狀況），
	// 那是正常文件內容，不應該被這個測試擋下來。
	assert.NotContains(t, out, "`--wait-interval`")
}

// TestWriteCommandDocsFlagsTable 確認 `cos sync` 這節有列出其專屬 flag `--delete`。
func TestWriteCommandDocsFlagsTable(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, WriteCommandDocs(&buf, NewDocsRoot()))
	out := buf.String()

	section := docSection(t, out, "## twaictl cos sync")
	assert.Contains(t, section, "--delete")
}

// TestDocsFileIsUpToDate 確認已 commit 的 docs/commands.md 與目前命令樹重新產生的
// 結果一致；不一致時提示開發者執行 `make docs` 重新生成。
func TestDocsFileIsUpToDate(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, WriteCommandDocs(&buf, NewDocsRoot()))

	committed, err := os.ReadFile("../../docs/commands.md")
	require.NoError(t, err)

	assert.Equal(t, string(committed), buf.String(),
		"docs/commands.md 與命令樹不一致，請執行 `make docs` 重新生成後再 commit")
}

// TestWriteCommandDocsEscapesAngleBrackets 確認 Short/Long/flag 說明裡的角括號
// 佔位符（例如 `<path>`）會被跳脫成 `&lt;path&gt;`，避免 Markdown/GitHub 把它當成
// HTML tag、整段從算繹結果消失；同時確認 Usage 的 fenced code block 裡的角括號
// 維持原樣（那裡本來就會照字面顯示，跳脫反而失真）。
func TestWriteCommandDocsEscapesAngleBrackets(t *testing.T) {
	child := &cobra.Command{
		Use:   "child <four>",
		Short: "desc <one>",
		Long:  "long desc <two> line",
		Run:   func(*cobra.Command, []string) {},
	}
	child.Flags().String("thing", "", "flag desc <three>")

	root := &cobra.Command{Use: "root", Short: "root command"}
	root.AddCommand(child)

	var buf bytes.Buffer
	require.NoError(t, WriteCommandDocs(&buf, root))
	out := buf.String()

	assert.Contains(t, out, "&lt;one&gt;")
	assert.Contains(t, out, "&lt;two&gt;")
	assert.Contains(t, out, "&lt;three&gt;")
	assert.NotContains(t, out, "<one>")
	assert.NotContains(t, out, "<two>")
	assert.NotContains(t, out, "<three>")

	// Usage 的 fenced code block 裡的 <four> 不應該被跳脫。
	assert.Contains(t, out, "<four>")
	assert.NotContains(t, out, "&lt;four&gt;")
}

// docSection 從完整文件內容中切出從 heading 開始、到下一個 "## " 節標題（或文件
// 結尾）為止的區段，方便測試只斷言某一節裡的內容，不會誤比對到其他節。
func docSection(t *testing.T, doc, heading string) string {
	t.Helper()
	start := strings.Index(doc, heading)
	require.GreaterOrEqualf(t, start, 0, "找不到節標題 %q", heading)

	rest := doc[start+len(heading):]
	if next := strings.Index(rest, "\n## "); next >= 0 {
		return doc[start : start+len(heading)+next]
	}
	return doc[start:]
}
