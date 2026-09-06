package twai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"sort"
	"strings"

	"github.com/gilbertchiao/twaictl/internal/config"
)

// HTTP header 名稱。
const (
	HeaderAPIKey  = "x-api-key"
	HeaderAPIHost = "x-api-host"
	// HeaderDryRun 標記 dryRun() 合成的回應，供 IsDryRunResponse 判斷。
	HeaderDryRun = "X-Twaictl-Dry-Run"
)

// UserAgent 產生 `twaictl/<version> (<os>/<arch>)`。
func UserAgent(version string) string {
	return fmt.Sprintf("twaictl/%s (%s/%s)", version, runtime.GOOS, runtime.GOARCH)
}

// shellQuote 以單引號包住字串；字串內若含單引號，則先結束引號、
// 以反斜線跳脫該單引號、再重新開始引號（shell 標準寫法）。
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// maskedHeaders 是 BuildCurl 印出時一律以 config.MaskedValue 取代其值的 header（小寫比對）。
// 除了 x-api-key，vcs create --password 也會放進 x-extra-property-password header；
// --dry-run 印出的 curl 命令常被貼進 issue / log，明文密碼不該出現在裡面
// （curl 本就因 *** 不可直接執行，遮蔽不損失功能）。
var maskedHeaders = map[string]bool{
	strings.ToLower(HeaderAPIKey): true,
	"x-extra-property-password":   true,
}

// maskedBodyFields 是 maskJSONBody 遮蔽的 JSON body 欄位名稱（不分深度）：
// `payload`（secret 內容，例如 TLS 憑證私鑰）、`password`（目前雖無 API 走 JSON body
// 傳密碼，未來若有也一併涵蓋），避免 --dry-run 印出的 curl 命令把明文機密貼進 issue / log。
// 比對時統一轉小寫（見 maskJSONValue），與 maskedHeaders 的慣例一致；
// 這裡的 key 本身也一律用小寫寫死。
var maskedBodyFields = map[string]bool{
	"payload":  true,
	"password": true,
}

// maskJSONBody 把 body 解成單一 JSON value 後，遞迴把任何深度的 object 內、名稱命中
// maskedBodyFields（不分大小寫）的欄位值改為 config.MaskedValue；只有真的遮蔽到欄位時才
// 重新序列化（理由見 maskJSONValue 下方說明）。body 不是**單一**合法 JSON value（不是合法
// JSON、或合法 JSON 後面還有多餘內容，例如 NDJSON 的第二筆、或尾端多打的字元）時原樣回傳。
func maskJSONBody(body []byte) []byte {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber() // 保留數字原始寫法（避免 1e3 變 1000 之類的改寫）
	var value any
	if err := dec.Decode(&value); err != nil {
		return body
	}
	// 第一個 value 之後只允許空白：再 Decode 一次必須得到 io.EOF，否則 body 不是單一
	// JSON value（例如 NDJSON 第二筆、尾端多打的字元、多餘的 `}`／`]`），比照非 JSON
	// 原樣回傳，避免命中遮蔽時只序列化第一個 value、其餘內容從 --dry-run 輸出中靜默
	// 消失（--dry-run 印出的 curl 是「等價命令」的承諾，不能悄悄少送一段 body）。
	// 不能用 dec.More()：它只回答「目前容器（最外層 array／object）是否還有下一個
	// 元素」，遇到緊接在後的 `}`／`]`（屬於外層容器、不是這個 value 的一部分）會直接
	// 回 false，讓帶有多餘收尾符號的 body 被誤判成「只有這一個 value」而遮蔽後截斷。
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return body
	}
	masked := maskJSONValue(value)
	if !masked {
		// 沒有機密可遮蔽時原樣回傳：重新序列化會讓 key 變字典序、把 <>& HTML-escape，
		// 使 `twaictl api --dry-run` 印出的內容與使用者輸入不同（即使語意等價）。
		return body
	}
	out, err := json.Marshal(value)
	if err != nil {
		return body
	}
	return out
}

// maskJSONValue 遞迴走訪 value（map / slice 皆進入），命中的欄位值就地改為 config.MaskedValue；
// 回傳是否有任何欄位被遮蔽。
func maskJSONValue(value any) bool {
	masked := false
	switch v := value.(type) {
	case map[string]any:
		for name, child := range v {
			if maskedBodyFields[strings.ToLower(name)] {
				v[name] = config.MaskedValue
				masked = true
				continue
			}
			if maskJSONValue(child) {
				masked = true
			}
		}
	case []any:
		for _, child := range v {
			if maskJSONValue(child) {
				masked = true
			}
		}
	}
	return masked
}

// BuildCurl 產生與 req 等價的 curl 命令，供 --dry-run 使用。
// maskedHeaders 命中的 header 值一律以 *** 取代；header 依名稱排序讓輸出穩定。
// body 若是合法 JSON，maskedBodyFields 命中的欄位值（不限深度）也會經 maskJSONBody
// 以 *** 取代（見該函式說明），避免 payload/password 等機密內容明文出現在
// dry-run 輸出中；沒有命中任何欄位時 body 原樣輸出。
func BuildCurl(req *http.Request, body []byte) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("curl -X %s %s", req.Method, shellQuote(req.URL.String())))

	names := make([]string, 0, len(req.Header))
	for name := range req.Header {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		value := req.Header.Get(name)
		if maskedHeaders[strings.ToLower(name)] {
			value = config.MaskedValue
		}
		// 自訂的 x-* header（x-api-*、x-extra-property-* 等）一律以小寫顯示，與 OpenAPI 文件一致；
		// Go 的 http.Header 會把 Set 進去的 header 名稱正規化成 Content-Type 這種大小寫（textproto
		// canonical form），若不特別處理，curl 命令印出的會是 X-Extra-Property-Password 而非
		// spec 使用的 x-extra-property-password。標準 header（Content-Type、User-Agent 等）
		// 則維持原本的大小寫。
		displayName := name
		if strings.HasPrefix(strings.ToLower(name), "x-") {
			displayName = strings.ToLower(name)
		}
		lines = append(lines, fmt.Sprintf("  -H %s", shellQuote(displayName+": "+value)))
	}
	if len(body) > 0 {
		lines = append(lines, fmt.Sprintf("  -d %s", shellQuote(string(maskJSONBody(body)))))
	}
	return strings.Join(lines, " \\\n")
}
