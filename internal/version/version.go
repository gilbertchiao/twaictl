// Package version 保存由 ldflags 注入的 build 資訊與本 build 依據的 OpenAPI 版本。
package version

import (
	"fmt"
	"runtime"
	"strings"
)

// 以下三個變數由 goreleaser / Makefile 的 -ldflags -X 注入。
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// Info 是 `twaictl version` 輸出的完整內容。
type Info struct {
	Version   string            `json:"version" yaml:"version"`
	Commit    string            `json:"commit" yaml:"commit"`
	Date      string            `json:"date" yaml:"date"`
	GoVersion string            `json:"go_version" yaml:"go_version"`
	OS        string            `json:"os" yaml:"os"`
	Arch      string            `json:"arch" yaml:"arch"`
	OpenAPI   map[string]string `json:"openapi" yaml:"openapi"`
}

// Get 組合目前 build 的版本資訊。
func Get() Info {
	openapi := make(map[string]string, len(OpenAPIVersions))
	for service, ver := range OpenAPIVersions {
		openapi[service] = ver
	}
	return Info{
		Version:   Version,
		Commit:    Commit,
		Date:      Date,
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		OpenAPI:   openapi,
	}
}

// String 以人類可讀的兩行格式輸出；OpenAPI 服務依 OpenAPIServices 的固定順序排列。
func (i Info) String() string {
	parts := make([]string, 0, len(OpenAPIServices))
	for _, service := range OpenAPIServices {
		parts = append(parts, fmt.Sprintf("%s %s", service, i.OpenAPI[service]))
	}
	return fmt.Sprintf("twaictl %s (commit %s, built %s, %s %s/%s)\nOpenAPI: %s\n",
		i.Version, i.Commit, i.Date, i.GoVersion, i.OS, i.Arch, strings.Join(parts, ", "))
}
