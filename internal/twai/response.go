package twai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// ErrDryRun 代表 --dry-run 模式下請求並未送出（transport 回傳了合成的 204）。
// cli 層將它對應為 exit 0 且不印出錯誤。
var ErrDryRun = errors.New("dry-run：請求未送出")

// CheckResponse 是所有 API 呼叫後的統一檢查點：
// dry-run 合成回應 → ErrDryRun；2xx → nil；其他 → *APIError（訊息自 body 解析）。
func CheckResponse(resp *http.Response, body []byte) error {
	if resp == nil {
		return errors.New("沒有收到 HTTP 回應")
	}
	if IsDryRunResponse(resp) {
		return ErrDryRun
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return NewAPIError(resp, body)
}

// Response 同時保留型別化資料與 API 原始 JSON（-o json 直接輸出 Raw）。
type Response[T any] struct {
	Value T
	Raw   []byte
}

// Decode 是所有 GET/POST 呼叫的共用收尾：先以 CheckResponse 檢查狀態碼
// （dry-run → ErrDryRun；非 2xx → *APIError），再把 body 解析成 T；
// JSON 解析失敗時包裝錯誤訊息回傳。
func Decode[T any](resp *http.Response, body []byte) (Response[T], error) {
	var res Response[T]
	if err := CheckResponse(resp, body); err != nil {
		return res, err
	}
	if err := json.Unmarshal(body, &res.Value); err != nil {
		return res, fmt.Errorf("解析 API 回應失敗: %w", err)
	}
	res.Raw = body
	return res, nil
}

// DecodeObjectOrFirst 與 Decode 相同，但額外容忍「spec 標單一物件、真實回應卻是陣列」
// （或反過來）的情況：body 去除前後空白後首字元為 `[` 時解成 []json.RawMessage 取第一筆，
// Raw 為該筆的原始 bytes（不重新 Marshal，保留生成型別沒有定義的欄位與原始格式），
// 空陣列回錯誤；否則行為與 Decode 完全相同。kind 只用於錯誤訊息（例如 "firewall rule"）。
func DecodeObjectOrFirst[T any](resp *http.Response, body []byte, kind string) (Response[T], error) {
	if err := CheckResponse(resp, body); err != nil {
		return Response[T]{}, err
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return Decode[T](resp, body)
	}
	var list []json.RawMessage
	if err := json.Unmarshal(trimmed, &list); err != nil {
		return Response[T]{}, fmt.Errorf("解析 API 回應失敗: %w", err)
	}
	if len(list) == 0 {
		return Response[T]{}, fmt.Errorf("%s 回應為空陣列", kind)
	}
	return Decode[T](resp, list[0])
}

// jsonNull 是 JSON `null` 字面值（去除前後空白後比對用）。
var jsonNull = []byte("null")

// RequireJSONFields 確認 body 是 JSON 物件且含有所有指定的頂層欄位（且值不是 JSON `null`）；
// 缺少或為 null 時回傳錯誤（附 body 前 200 字）。
//
// 生成型別的必填欄位在 Go 端是值型別，json.Unmarshal 對缺失欄位或值為 null 都只會給零值、
// 不會報錯，gateway 對未知路徑回 200 + 其他 JSON 時會被靜默解讀成「用量為 0」之類的假資料，
// 因此需要在 Decode 之外另外檢查回應是否真的含有預期欄位、且該欄位不是顯式的 null。
func RequireJSONFields(body []byte, fields ...string) error {
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fmt.Errorf("API 回應不是 JSON 物件: %w", err)
	}
	for _, name := range fields {
		raw, ok := parsed[name]
		if !ok || bytes.Equal(bytes.TrimSpace(raw), jsonNull) {
			return fmt.Errorf("API 回應缺少欄位 %s（或為 null）: %s", name, Truncate(body, 200))
		}
	}
	return nil
}
