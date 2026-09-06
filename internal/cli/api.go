package cli

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gilbertchiao/twaictl/internal/output"
	"github.com/gilbertchiao/twaictl/internal/twai"
	"github.com/gilbertchiao/twaictl/internal/version"
)

// allowedAPIMethods 是 `twaictl api` 支援的 HTTP method（大寫比對）。
var allowedAPIMethods = map[string]bool{
	"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true,
}

// apiEscapeHatchLong 是 `twaictl api --help` 的說明文字：明確聲明這是逃生口，
// 不做任何驗證、不要求確認、也不做名稱→id 解析，寫入類請求的 body 會原封不動送出。
const apiEscapeHatchLong = "直接呼叫任意 API endpoint，做為其他子命令都無法涵蓋（或尚未支援）時的逃生口。\n\n" +
	"本命令不做任何輸入驗證、不會要求確認、也不做名稱→id 解析：<path> 必須是實際的 API 路徑\n" +
	"（query string 直接寫在 path 裡，例如 /sites/?project=60000）；寫入類請求\n" +
	"（POST/PUT/PATCH/DELETE）的 body 會原封不動送出，請自行確認內容正確再執行——\n" +
	"沒有二次確認機制，用錯 path/body 造成的後果需自行負責。\n\n" +
	"--dry-run 遮蔽 JSON body 內任何深度的 payload／password 欄位；非 JSON body\n" +
	"會原樣印出，請自行確認機密內容不會外洩。\n\n" +
	"範例：\n" +
	"  twaictl api GET /sites/\n" +
	"  twaictl api POST /sites/ --data '{\"name\":\"demo\"}'\n" +
	"  twaictl api POST /sites/ --data @body.json\n" +
	"  echo '{\"name\":\"demo\"}' | twaictl api POST /sites/ --data @-\n" +
	"  twaictl api GET /usage/ --service cos\n" +
	"  twaictl api GET /solutions/ --service common"

// newAPICmd 建立 `twaictl api` 逃生口命令。
func newAPICmd(a *app) *cobra.Command {
	var service string
	var data string
	var headerFlags []string

	cmd := &cobra.Command{
		Use:   "api <METHOD> <path>",
		Short: "直接呼叫任意 API endpoint（逃生口，不驗證、不確認、不做名稱解析）",
		Long:  apiEscapeHatchLong,
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			method := strings.ToUpper(args[0])
			if !allowedAPIMethods[method] {
				return fmt.Errorf("不支援的 method %q（可用：GET、POST、PUT、PATCH、DELETE）", args[0])
			}
			path := args[1]

			rawService, err := parseRawService(service)
			if err != nil {
				return err
			}

			// 用 Changed 而非 data != "" 判斷 --data 是否「有被指定」：使用者可能刻意
			// 打 --data ''（明確指定空 body），此時 data 仍是空字串，若只看值會誤判成
			// 「沒指定 --data」而放行 GET/DELETE，讓下面這個互斥檢查失效。
			dataGiven := c.Flags().Changed("data")
			if dataGiven && (method == "GET" || method == "DELETE") {
				return fmt.Errorf("%s 不可搭配 --data", method)
			}

			headers, err := parseAPIHeaderFlags(headerFlags)
			if err != nil {
				return err
			}

			var body []byte
			if dataGiven {
				body, err = a.readAPIData(data)
				if err != nil {
					return err
				}
				if len(body) == 0 {
					// 使用者明確指定了空 body（--data ''／空檔案／空 stdin）：
					// Transport.injectHeaders 只在 req.Body 非 nil 時才會補預設
					// Content-Type，空 body 不會建立 io.Reader（見 raw.go 的
					// `if len(body) > 0`），因此不會被自動補上，在這裡直接補，讓
					// 「明確指定空 body」與「有內容的 body」得到一致的預設行為。
					// http.CanonicalHeaderKey 已保證不會與使用者透過 --header 指定的
					// Content-Type 相撞（parseAPIHeaderFlags 也用同一個函式正規化）。
					contentTypeKey := http.CanonicalHeaderKey("Content-Type")
					if _, ok := headers[contentTypeKey]; !ok {
						headers[contentTypeKey] = "application/json"
					}
				}
				// 非空 body 的 Content-Type 預設值仍交給 Transport.injectHeaders 處理
				// （見 transport.go）：不在這裡另外塞一份預設值，避免使用者以
				// --header content-type=... 覆寫時，headers map 同時存在使用者版與
				// 這裡塞的預設版（兩者只有大小寫不同，但 http.Header.Set 會正規化成
				// 同一個 key），由誰覆蓋誰變成取決於 map 疊代順序而不確定。
			}

			client, err := a.rawClient()
			if err != nil {
				return err
			}
			res, err := client.Do(a.commandContext(c), rawService, method, path, body, headers)
			if err != nil {
				return err
			}
			return a.renderAPIResponse(res)
		},
	}

	cmd.Flags().StringVar(&service, "service", "vcs", "呼叫哪個服務：vcs、cos、common")
	cmd.Flags().StringVar(&data, "data", "", "request body：字面 JSON 字串、@file 讀檔、或 @- 讀 stdin（僅 POST/PUT/PATCH 可用）")
	cmd.Flags().StringArrayVar(&headerFlags, "header", nil, "附加自訂 header（k=v，可重複；重複指定同名 header 以最後一次為準，不會附加；不可指定 x-api-key/x-api-host/User-Agent）")
	return cmd
}

// parseRawService 驗證 --service 的值。
func parseRawService(s string) (twai.RawService, error) {
	switch twai.RawService(s) {
	case twai.RawServiceVCS, twai.RawServiceCOS, twai.RawServiceCommon:
		return twai.RawService(s), nil
	}
	return "", fmt.Errorf("不支援的 --service %q（可用：vcs、cos、common）", s)
}

// parseAPIHeaderFlags 把 `--header k=v`（在第一個 "=" 切分）轉成 map；
// x-api-key／x-api-host／User-Agent（不分大小寫）由 transport 自動注入，
// 使用者透過 --header 指定會被拒絕（見 twai.IsReservedRawHeader）。
//
// header 名稱一律以 http.CanonicalHeaderKey 正規化後才存進 map：不同大小寫的同一個
// header（例如 content-type 與 Content-Type）在 http.Header 裡本來就是同一個 key，
// 先在這裡正規化能讓「使用者重複指定同一個 header、只是大小寫不同」時的結果是
// 依 flag 出現順序決定（後面蓋掉前面），而不是留到 (*RawClient).Do 逐一
// req.Header.Set 時才相撞、變成由 map 疊代順序（不確定）決定誰贏。
func parseAPIHeaderFlags(flags []string) (map[string]string, error) {
	headers := make(map[string]string, len(flags))
	for _, f := range flags {
		idx := strings.Index(f, "=")
		if idx < 0 {
			return nil, fmt.Errorf("--header 格式錯誤（需要 k=v）: %q", f)
		}
		name, value := f[:idx], f[idx+1:]
		if twai.IsReservedRawHeader(name) {
			return nil, fmt.Errorf("header %q 由 twaictl 依 settings 自動注入，不可透過 --header 指定", name)
		}
		headers[http.CanonicalHeaderKey(name)] = value
	}
	return headers, nil
}

// readAPIData 依 --data 的值決定 request body 來源：
//   - "@-"：讀取整個 stdin
//   - "@" 開頭的其餘字串：視為檔案路徑，讀取該檔案內容
//   - 其他：視為字面字串（通常是 JSON），原樣送出
func (a *app) readAPIData(data string) ([]byte, error) {
	if data == "@-" {
		body, err := io.ReadAll(a.stdin)
		if err != nil {
			return nil, fmt.Errorf("讀取 stdin 失敗: %w", err)
		}
		return body, nil
	}
	if strings.HasPrefix(data, "@") {
		path := data[1:]
		body, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("讀取 --data 檔案 %s 失敗: %w", path, err)
		}
		return body, nil
	}
	return []byte(data), nil
}

// rawClient 建立 `twaictl api` 逃生口用的 RawClient（需要 API key）。
func (a *app) rawClient() (*twai.RawClient, error) {
	if err := a.settings.RequireAPIKey(); err != nil {
		return nil, err
	}
	return twai.NewRawClient(a.settings, twai.ClientOptions{
		Version:      version.Version,
		Timeout:      a.opts.timeout,
		DryRun:       a.opts.dryRun,
		DryRunWriter: a.stdout,
		Verbose:      a.opts.verbose,
		Logger:       a.logger,
	})
}

// renderAPIResponse 輸出 `twaictl api` 的回應：空 body 不印出內容，改在 stderr 顯示
// 實際的狀態碼提示（避免使用者誤以為命令失敗或卡住）；-o table 視同 json。
//
// 提示訊息印真實狀態碼而非籠統宣稱 204：200/201/202 也可能回空 body，若一律說
// 「HTTP 204 No Content」會誤導只看 stderr 判斷結果的使用者（例如寫腳本比對輸出）。
func (a *app) renderAPIResponse(res twai.RawResponse) error {
	if len(bytes.TrimSpace(res.Raw)) == 0 {
		status := res.Status
		if status == "" {
			status = fmt.Sprintf("%d %s", res.StatusCode, http.StatusText(res.StatusCode))
		}
		_, _ = fmt.Fprintf(a.stderr, "twaictl: HTTP %s（回應沒有內容）\n", status)
		return nil
	}
	return output.RenderRaw(a.stdout, output.Format(a.opts.output), res.Raw)
}
