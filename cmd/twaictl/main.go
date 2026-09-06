// twaictl 是台智雲（Taiwan AI Cloud, TWAI）TWCC 平台的非官方命令列工具。
// 本檔案只負責把 OS 層的 I/O 交給 internal/cli，方便測試時以 buffer 取代。
package main

import (
	"os"

	"github.com/gilbertchiao/twaictl/internal/cli"
)

func main() {
	exitCode := cli.Execute(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Getenv)
	os.Exit(exitCode)
}
