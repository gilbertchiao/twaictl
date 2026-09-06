package twai

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCurlMasksAPIKeyAndIncludesBody(t *testing.T) {
	body := []byte(`{"name":"demo"}`)
	req, err := http.NewRequest(http.MethodPost, "https://gw/api/v3/openstack-taichung-default-2/sites/", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set(HeaderAPIKey, "super-secret")
	req.Header.Set(HeaderAPIHost, "openstack-taichung-default-2")
	req.Header.Set("Content-Type", "application/json")

	got := BuildCurl(req, body)

	assert.Equal(t, `curl -X POST 'https://gw/api/v3/openstack-taichung-default-2/sites/' \
  -H 'Content-Type: application/json' \
  -H 'x-api-host: openstack-taichung-default-2' \
  -H 'x-api-key: ***' \
  -d '{"name":"demo"}'`, got)
	assert.NotContains(t, got, "super-secret")
}

func TestBuildCurlGetWithoutBody(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "https://gw/api/v3/projects/?name=a", nil)
	req.Header.Set(HeaderAPIKey, "k")

	got := BuildCurl(req, nil)

	assert.Equal(t, `curl -X GET 'https://gw/api/v3/projects/?name=a' \
  -H 'x-api-key: ***'`, got)
}

func TestBuildCurlMasksPasswordHeader(t *testing.T) {
	// vcs create --password 的值只會放進 x-extra-property-password header；--dry-run 印出的
	// curl 命令常被貼進 issue / log，明文密碼不該出現在裡面（curl 本就因 *** 不可直接執行，
	// 遮蔽不損失功能，見 internal/cli/vcs_create.go 的說明）。
	req, err := http.NewRequest(http.MethodPost, "https://gw/x", nil)
	require.NoError(t, err)
	req.Header.Set("x-extra-property-password", "S3cret")
	req.Header.Set("x-extra-property-image", "Ubuntu 22.04")

	got := BuildCurl(req, nil)

	assert.Contains(t, got, "-H 'x-extra-property-password: ***'")
	assert.NotContains(t, got, "S3cret")
	assert.Contains(t, got, "-H 'x-extra-property-image: Ubuntu 22.04'", "其他 x-extra-property-* header 原樣顯示")
}

func TestBuildCurlLowercasesCustomXHeaders(t *testing.T) {
	// http.Header.Set 會把 header 名稱正規化成 textproto canonical form
	// （例如 X-Extra-Property-Network），但 spec 與 OpenAPI 文件一律使用小寫；
	// BuildCurl 必須把所有 x-* 自訂 header 印成小寫，不能只處理 x-api-key/x-api-host。
	req, err := http.NewRequest(http.MethodPost, "https://gw/api/v3/openstack-taichung-default-2/sites/", nil)
	require.NoError(t, err)
	req.Header.Set("x-extra-property-network", "priv-net")
	req.Header.Set("Content-Type", "application/json")

	got := BuildCurl(req, nil)

	assert.Contains(t, got, "-H 'x-extra-property-network: priv-net'")
	assert.Contains(t, got, "-H 'Content-Type: application/json'", "標準 header 維持原本大小寫")
}

func TestBuildCurlEscapesSingleQuotesInBody(t *testing.T) {
	body := []byte(`{"cmd":"echo 'hi'"}`)
	req, _ := http.NewRequest(http.MethodPost, "https://gw/x", bytes.NewReader(body))
	got := BuildCurl(req, body)
	assert.Contains(t, got, `-d '{"cmd":"echo '\''hi'\''"}'`)
}

func TestBuildCurlMasksSensitiveJSONBodyFields(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "https://gw/api/v3/x/secrets/?project=1", nil)
	out := BuildCurl(req, []byte(`{"name":"s1","payload":"aGVsbG8=","desc":"d"}`))
	assert.NotContains(t, out, "aGVsbG8=")
	assert.Contains(t, out, `"payload":"***"`)
	assert.Contains(t, out, `"name":"s1"`)
	// 非 JSON body 原樣輸出
	assert.Contains(t, BuildCurl(req, []byte(`not-json payload`)), "not-json payload")
}

// TestBuildCurlLeavesBodyUntouchedWhenNoSensitiveFieldMatches 確認沒有命中任何
// maskedBodyFields 時，body 原樣輸出，不會被重新序列化——重新序列化會讓多鍵
// object 的 key 順序變成字典序，且 json.Marshal 預設會把 <、>、& 轉成 HTML
// escape，兩者都會讓 `twaictl api --data '<原始 JSON>' --dry-run` 印出的內容
// 跟使用者實際送出的不一樣。這裡用非字典序（zebra, apple）與需要 HTML-escape
// 的字元（<, >）確認完全逐位元組相同，才能真正證明沒有重新序列化過。
func TestBuildCurlLeavesBodyUntouchedWhenNoSensitiveFieldMatches(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "https://gw/x", nil)
	body := []byte(`{"zebra":"<a&b>","apple":1}`)

	got := BuildCurl(req, body)

	assert.Contains(t, got, `-d '{"zebra":"<a&b>","apple":1}'`)
}

// TestBuildCurlMasksSensitiveJSONBodyFieldsCaseInsensitively 確認 maskedBodyFields
// 的比對比照 maskedHeaders 的慣例採大小寫不敏感（例如 `Payload`/`PASSWORD`
// 也要被遮蔽），而不是要求呼叫端剛好用小寫欄位名稱。
func TestBuildCurlMasksSensitiveJSONBodyFieldsCaseInsensitively(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "https://gw/x", nil)
	got := BuildCurl(req, []byte(`{"name":"s1","Payload":"aGVsbG8="}`))
	assert.NotContains(t, got, "aGVsbG8=")
	assert.Contains(t, got, `"Payload":"***"`)
}

// TestMaskJSONBodyMasksNestedFields 驗證 maskJSONBody 遮蔽任何深度（不限頂層）的
// payload／password 欄位，包含陣列元素內的 object、巢狀層級的大小寫不敏感比對（"Password"
// 而非全小寫），以及頂層本身就是陣列的情況；沒有任何欄位命中時原樣回傳（不重新序列化，
// 理由見 maskJSONBody 說明）。
func TestMaskJSONBodyMasksNestedFields(t *testing.T) {
	body := []byte(`{"name":"x","spec":{"Password":"p","items":[{"payload":"s","keep":1}]},"payload":"top"}`)
	got := maskJSONBody(body)
	assert.JSONEq(t, `{"name":"x","spec":{"Password":"***","items":[{"payload":"***","keep":1}]},"payload":"***"}`, string(got))

	// 沒有任何敏感欄位時原樣回傳（不重新序列化）
	plain := []byte(`{"b":1, "a":"<x>"}`)
	assert.Equal(t, plain, maskJSONBody(plain))
	// 頂層是陣列也要遞迴
	arr := []byte(`[{"password":"p"}]`)
	assert.JSONEq(t, `[{"password":"***"}]`, string(maskJSONBody(arr)))
	// 命中欄位的值本身是 object／array 時，整棵子樹被 *** 取代（不是只遮蔽子樹內部
	// 剛好也叫 password 的欄位，而是命中的那一層直接整個換成 config.MaskedValue）。
	subtree := []byte(`{"payload":{"deep":{"password":"x"}}}`)
	assert.JSONEq(t, `{"payload":"***"}`, string(maskJSONBody(subtree)))
}

// TestMaskJSONBodyRejectsTrailingData 是 review Important 1 的迴歸測試：
// json.Decoder.Decode 只消費第一個 JSON value 就成功回傳，不代表整段 body 只有這一個
// value；若照樣只序列化第一個 value，NDJSON 的第二筆、尾端多打的字元、或多餘的收尾
// `}`／`]` 都會從 --dry-run 輸出中靜默消失。maskJSONBody 必須偵測「Decode 之後還有
// 更多內容」（第二次 Decode 未得到 io.EOF）並比照非 JSON body 原樣回傳；但尾端只有
// 空白／換行（第二次 Decode 得到 io.EOF）時仍要照常遮蔽，不能因為多了個換行就整段
// 放棄遮蔽。
//
// 特別涵蓋 dec.More() 這個看似合理但錯誤的替代方案會漏掉的兩種情況：More() 只回答
// 「目前容器是否還有下一個元素」，遇到緊接在後的 `}`／`]` 會直接回 false（誤判成
// 「只有這一個 value」），與「後面還有非收尾符號的字元」（More() 能正確偵測）刻意
// 放在同一個測試裡對照。
func TestMaskJSONBodyRejectsTrailingData(t *testing.T) {
	// 合法 JSON 後面還有非空白的多餘內容：原樣回傳（byte 比對，不能只序列化第一個 value）。
	trailingGarbage := []byte(`{"password":"p"} trailing`)
	assert.Equal(t, trailingGarbage, maskJSONBody(trailingGarbage))

	// 尾端多了一個 `}`：dec.More() 對此會誤判為 false（因為它只看「目前容器」，`}`
	// 屬於已經結束的外層，不代表這個 value 之後沒有更多內容）；正確行為仍是原樣回傳。
	trailingCloseBrace := []byte(`{"password":"p"}}`)
	assert.Equal(t, trailingCloseBrace, maskJSONBody(trailingCloseBrace))

	// 尾端多了一個 `]`：與上面同理，dec.More() 也會誤判為 false。
	trailingCloseBracket := []byte(`{"password":"p"} ]`)
	assert.Equal(t, trailingCloseBracket, maskJSONBody(trailingCloseBracket))

	// NDJSON（兩個以換行分隔的 JSON value）：同樣視為「不是單一 JSON value」，原樣回傳。
	ndjson := []byte("{\"password\":\"p\"}\n{\"password\":\"q\"}")
	assert.Equal(t, ndjson, maskJSONBody(ndjson))

	// 尾端只有空白／換行、沒有下一個 token：仍是單一 JSON value，照常遮蔽。
	trailingNewline := []byte("{\"password\":\"p\"}\n")
	assert.JSONEq(t, `{"password":"***"}`, string(maskJSONBody(trailingNewline)))
}

func TestUserAgentFormat(t *testing.T) {
	ua := UserAgent("v1.2.3")
	assert.Regexp(t, `^twaictl/v1\.2\.3 \([a-z0-9]+/[a-z0-9]+\)$`, ua)
}
