// Command gendocs 從 twaictl 的 cobra 命令樹產生 docs/commands.md 的內容，寫到 stdout。
//
// 用法（見 Makefile 的 `docs` / `docs-check` target）：
//
//	go run ./tools/gendocs > docs/commands.md
package main

import (
	"fmt"
	"os"

	"github.com/gilbertchiao/twaictl/internal/cli"
)

func main() {
	if err := cli.WriteCommandDocs(os.Stdout, cli.NewDocsRoot()); err != nil {
		fmt.Fprintf(os.Stderr, "gendocs: %v\n", err)
		os.Exit(1)
	}
}
