// Package output 負責 table / json / yaml 三種輸出格式的渲染、欄位選擇與時區轉換。
package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"gopkg.in/yaml.v3"
)

// Format 是輸出格式。
type Format string

// 支援的輸出格式。
const (
	FormatTable Format = "table"
	FormatJSON  Format = "json"
	FormatYAML  Format = "yaml"
)

// ParseFormat 驗證 -o 的值。
func ParseFormat(s string) (Format, error) {
	switch Format(s) {
	case FormatTable, FormatJSON, FormatYAML:
		return Format(s), nil
	}
	return "", fmt.Errorf("不支援的輸出格式 %q（可用：table、json、yaml）", s)
}

// Options 是渲染選項，直接對應全域 flags。
type Options struct {
	Format   Format
	Columns  []string // --columns，空代表用 Default 欄位
	NoHeader bool     // --no-header
}

// Column 定義 table 的一個欄位。
type Column[T any] struct {
	Name    string         // 表頭；--columns 以此比對（不分大小寫）
	Default bool           // 未指定 --columns 時是否顯示
	Value   func(T) string // 從資料列取出顯示文字
}

// ErrUnknownColumn 代表 --columns 指定了不存在的欄位。
var ErrUnknownColumn = errors.New("不支援的欄位")

// taipei 是顯示用時區；台灣無 DST，用固定偏移避免依賴 tzdata（Windows 可能沒有）。
var taipei = time.FixedZone("Asia/Taipei", 8*60*60)

// Render 依 opts.Format 輸出。table 用 items + columns；json 直接輸出 API 原始回應（rawJSON）
// 並重新縮排；yaml 把 rawJSON 轉成 YAML。rawJSON 為空時（例如本地組合的資料）json/yaml 改 marshal items。
func Render[T any](w io.Writer, opts Options, rawJSON []byte, items []T, columns []Column[T]) error {
	switch opts.Format {
	case FormatJSON:
		return renderJSON(w, rawJSON, items)
	case FormatYAML:
		return renderYAML(w, rawJSON, items)
	default:
		return renderTable(w, opts, items, columns)
	}
}

func renderJSON[T any](w io.Writer, rawJSON []byte, items []T) error {
	var out bytes.Buffer
	if len(bytes.TrimSpace(rawJSON)) == 0 {
		data, err := json.Marshal(items)
		if err != nil {
			return fmt.Errorf("序列化為 JSON 失敗: %w", err)
		}
		rawJSON = data
	}
	if err := json.Indent(&out, rawJSON, "", "  "); err != nil {
		return fmt.Errorf("格式化 JSON 回應失敗: %w", err)
	}
	out.WriteByte('\n')
	_, err := w.Write(out.Bytes())
	return err
}

func renderYAML[T any](w io.Writer, rawJSON []byte, items []T) error {
	if len(bytes.TrimSpace(rawJSON)) == 0 {
		// rawJSON 為空時（本地組合的資料）先 marshal 成 JSON，
		// 這樣才會套用 json tag（例如欄位名稱、omitempty），
		// 讓 yaml 輸出的欄位名稱與 schema 跟 json 輸出保持一致，
		// 而不是直接用 yaml.Marshal(items) 產生的 Go 欄位名稱。
		data, err := json.Marshal(items)
		if err != nil {
			return fmt.Errorf("序列化為 JSON 失敗: %w", err)
		}
		rawJSON = data
	}
	data, err := jsonToYAML(rawJSON)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// jsonToYAML 把 JSON bytes 轉成 YAML。使用 json.Decoder 搭配 UseNumber()，
// 避免數字先被解成 float64 而讓超過 2^53 的整數（例如 API 回傳的大 ID）失真；
// 解出的 json.Number 交由 normalizeJSONNumbers 還原成適當型別後再 marshal。
// 另外多做一次 Decode 確認第一個 JSON 值之後沒有多餘內容（json.Decoder.Decode
// 預設只讀取第一個值，尾隨內容會被悄悄忽略）。
func jsonToYAML(rawJSON []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(rawJSON))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, fmt.Errorf("解析 JSON 回應失敗: %w", err)
	}
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("解析 JSON 回應失敗: 第一個 JSON 值之後有多餘內容")
		}
		return nil, fmt.Errorf("解析 JSON 回應失敗: 第一個 JSON 值之後有多餘內容: %w", err)
	}
	data, err := yaml.Marshal(normalizeJSONNumbers(value))
	if err != nil {
		return nil, fmt.Errorf("轉換為 YAML 失敗: %w", err)
	}
	return data, nil
}

// normalizeJSONNumbers 遞迴走訪 map[string]any / []any，把其中的 json.Number 轉成
// 適合 yaml.Marshal 呈現的型別：
//   - 整數字面（不含 '.'、'e'、'E'）一律以 *yaml.Node（Tag "!!int"）保留原始數字文字，
//     不論位數多長都不失真（一般 int64/uint64 會超出範圍，改用字串再轉數字仍可能溢位或
//     被迫加引號輸出成字串，因此改用 Node 讓 yaml.v3 直接把原始文字當數字純量輸出）。
//   - 其餘（含科學記號、小數）用 Float64() 轉成 float64；失敗時保留原始字串，避免完全失真。
func normalizeJSONNumbers(v any) any {
	switch val := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, item := range val {
			out[k] = normalizeJSONNumbers(item)
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = normalizeJSONNumbers(item)
		}
		return out
	case json.Number:
		s := val.String()
		if !strings.ContainsAny(s, ".eE") {
			return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: s}
		}
		if f, err := val.Float64(); err == nil {
			return f
		}
		return s
	default:
		return v
	}
}

func renderTable[T any](w io.Writer, opts Options, items []T, columns []Column[T]) error {
	selected, err := SelectColumns(opts.Columns, columns)
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if !opts.NoHeader {
		names := make([]string, len(selected))
		for i, c := range selected {
			names[i] = strings.ToUpper(c.Name)
		}
		_, _ = fmt.Fprintln(tw, strings.Join(names, "\t"))
	}
	for _, item := range items {
		cells := make([]string, len(selected))
		for i, c := range selected {
			cells[i] = c.Value(item)
		}
		_, _ = fmt.Fprintln(tw, strings.Join(cells, "\t"))
	}
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("輸出表格失敗: %w", err)
	}
	return nil
}

// SelectColumns 依 --columns 挑選欄位（不分大小寫、保留使用者指定的順序）；未指定則用 Default 欄位。
// 匯出供 cli 層的 renderOne 這類「不經過 renderTable」的單一物件輸出重用同一套欄位選擇規則。
func SelectColumns[T any](requested []string, columns []Column[T]) ([]Column[T], error) {
	if len(requested) == 0 {
		var defaults []Column[T]
		for _, c := range columns {
			if c.Default {
				defaults = append(defaults, c)
			}
		}
		return defaults, nil
	}
	var selected []Column[T]
	for _, want := range requested {
		found := false
		for _, c := range columns {
			if strings.EqualFold(strings.TrimSpace(want), c.Name) {
				selected = append(selected, c)
				found = true
				break
			}
		}
		if !found {
			names := make([]string, len(columns))
			for i, c := range columns {
				names[i] = c.Name
			}
			return nil, fmt.Errorf("%w %q（可用：%s）", ErrUnknownColumn, want, strings.Join(names, ", "))
		}
	}
	return selected, nil
}

// RenderRaw 輸出 `twaictl api` 逃生口取得的原始 body（沒有欄位定義、不保證是 JSON）：
// json 若 body 是合法 JSON 則重新縮排、否則原樣輸出（逃生口打到的 endpoint 不保證回應
// 一定是 JSON，例如純文字錯誤頁）；yaml 一律要求 body 是合法 JSON 再轉成 YAML，非 JSON
// 直接回傳錯誤（沒有「原樣輸出」的意義）；table 沒有欄位可畫表格，行為同 json。
func RenderRaw(w io.Writer, format Format, body []byte) error {
	if format == FormatYAML {
		data, err := jsonToYAML(body)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	}

	var out bytes.Buffer
	if err := json.Indent(&out, body, "", "  "); err != nil {
		// body 不是合法 JSON：原樣輸出，不視為錯誤；若原本結尾沒有換行則補上一個，
		// 讓輸出在終端機／被接到其他行工具時行為與一般命令列工具一致。
		if _, err := w.Write(body); err != nil {
			return err
		}
		if len(body) > 0 && body[len(body)-1] != '\n' {
			_, err := w.Write([]byte("\n"))
			return err
		}
		return nil
	}
	out.WriteByte('\n')
	_, err := w.Write(out.Bytes())
	return err
}

// RenderKeyValues 輸出「Key:  value」對齊的清單，供 whoami 這類非表格資料使用。
func RenderKeyValues(w io.Writer, pairs [][2]string) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, p := range pairs {
		_, _ = fmt.Fprintf(tw, "%s:\t%s\n", p[0], p[1])
	}
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("輸出失敗: %w", err)
	}
	return nil
}

// FormatTime 把 API 的 UTC 時間轉成 Asia/Taipei 顯示。
func FormatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(taipei).Format("2006-01-02 15:04:05")
}

// TimePtr 是 FormatTime 的指標版本，nil 顯示為空字串。
func TimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return FormatTime(*t)
}

// timeStringLayouts 是 FormatTimeString 依序嘗試解析的時間格式：
// 標準 RFC3339（含時區），以及實測發現的兩種無時區格式（一律視為 UTC）。
var timeStringLayouts = []string{
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
}

// FormatTimeString 把 API 回傳的時間字串轉成 Asia/Taipei 顯示。
//
// 部分 API 回應（例如 keypair 的 create_time，見 docs/api-notes.md）不是標準
// RFC3339，而是無時區資訊的本地時間字串；此函式依序嘗試 RFC3339 與兩種常見的
// 無時區格式（皆視為 UTC）解析，全部失敗時原樣回傳輸入字串，避免整個命令因
// 無法解析而失敗。
func FormatTimeString(s string) string {
	if s == "" {
		return ""
	}
	// RFC3339 本身帶時區資訊，直接用 time.Parse；其餘格式一律視為 UTC。
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return FormatTime(t)
	}
	for _, layout := range timeStringLayouts[1:] {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return FormatTime(t)
		}
	}
	return s
}

// Str 把 *string 轉成顯示文字，nil 為空字串。
func Str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// Int64 把 *int64 轉成顯示文字，nil 為空字串。
func Int64(p *int64) string {
	if p == nil {
		return ""
	}
	return strconv.FormatInt(*p, 10)
}

// Float 把 *float64 轉成顯示文字，nil 為空字串；使用最短且不失真的表示法
// （例如 0.25 顯示為 "0.25"、1 顯示為 "1"，不會像 %f 一樣補上多餘的 0）。
func Float(p *float64) string {
	if p == nil {
		return ""
	}
	return strconv.FormatFloat(*p, 'f', -1, 64)
}

// Bool 以 yes / no 顯示布林值。
func Bool(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// Bytes 以二進位單位顯示位元組數（B、KiB、MiB、GiB、TiB）。
func Bytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	value := float64(n)
	suffixes := []string{"KiB", "MiB", "GiB", "TiB", "PiB"}
	i := -1
	for value >= unit && i < len(suffixes)-1 {
		value /= unit
		i++
	}
	return fmt.Sprintf("%.1f %s", value, suffixes[i])
}
