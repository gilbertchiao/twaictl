package cli

import (
	"errors"

	"github.com/gilbertchiao/twaictl/internal/config"
	"github.com/gilbertchiao/twaictl/internal/twai"
)

// Exit code 定義（設計文件 3.5 節）。
const (
	ExitOK          = 0 // 成功
	ExitGeneral     = 1 // 一般錯誤（使用者輸入、找不到資源）
	ExitConfig      = 2 // 設定錯誤（無 profile、缺 API key）
	ExitAPI         = 3 // API 錯誤（HTTP 4xx / 5xx；twaictl api 不跟隨重導向，3xx 亦屬此類）
	ExitNetwork     = 4 // 網路 / 逾時
	ExitWaitTimeout = 5 // --wait 逾時
)

// exitCodeFor 依 error 型別決定 process exit code。判斷順序很重要：
// 先看最特定的型別（wait 逾時、設定、API），最後才是網路。
func exitCodeFor(err error) int {
	if err == nil {
		return ExitOK
	}
	if errors.Is(err, twai.ErrDryRun) {
		return ExitOK
	}
	if errors.Is(err, twai.ErrWaitTimeout) {
		return ExitWaitTimeout
	}
	var cfgErr *config.Error
	if errors.As(err, &cfgErr) {
		return ExitConfig
	}
	var apiErr *twai.APIError
	if errors.As(err, &apiErr) {
		return ExitAPI
	}
	if twai.IsNetworkError(err) {
		return ExitNetwork
	}
	return ExitGeneral
}
