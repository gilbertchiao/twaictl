package output

import (
	"bytes"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilbertchiao/twaictl/internal/testutil"
)

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

type site struct {
	ID      int64
	Name    string
	Status  string
	Created time.Time
}

var siteColumns = []Column[site]{
	{Name: "ID", Default: true, Value: func(s site) string { return itoa(s.ID) }},
	{Name: "NAME", Default: true, Value: func(s site) string { return s.Name }},
	{Name: "STATUS", Default: true, Value: func(s site) string { return s.Status }},
	{Name: "CREATED", Default: false, Value: func(s site) string { return FormatTime(s.Created) }},
}

var sampleSites = []site{
	{1, "web-01", "Ready", time.Date(2026, 8, 27, 1, 2, 3, 0, time.UTC)},
	{22, "db", "Initializing", time.Date(2026, 1, 1, 16, 0, 0, 0, time.UTC)},
}

const sampleRaw = `[{"id":1,"name":"web-01","status":"Ready","create_time":"2026-08-27T01:02:03Z"},{"id":22,"name":"db","status":"Initializing","create_time":"2026-01-01T16:00:00Z"}]`

func TestRenderTableDefaultColumnsGolden(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, Render(&buf, Options{Format: FormatTable}, []byte(sampleRaw), sampleSites, siteColumns))
	testutil.AssertGolden(t, "output_table.txt", buf.Bytes())
}

func TestRenderTableSelectedColumnsAndNoHeader(t *testing.T) {
	var buf bytes.Buffer
	opts := Options{Format: FormatTable, Columns: []string{"name", "created"}, NoHeader: true}
	require.NoError(t, Render(&buf, opts, nil, sampleSites, siteColumns))
	assert.Equal(t, "web-01  2026-08-27 09:02:03\ndb      2026-01-02 00:00:00\n", buf.String())
}

func TestRenderTableUnknownColumn(t *testing.T) {
	err := Render(&bytes.Buffer{}, Options{Format: FormatTable, Columns: []string{"nope"}}, nil, sampleSites, siteColumns)
	require.ErrorIs(t, err, ErrUnknownColumn)
	assert.Contains(t, err.Error(), "ID, NAME, STATUS, CREATED")
}

func TestRenderTableEmptyPrintsHeaderOnly(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, Render(&buf, Options{Format: FormatTable}, []byte(`[]`), []site{}, siteColumns))
	assert.Equal(t, "ID  NAME  STATUS\n", buf.String())
}

func TestRenderJSONUsesRawResponseIndented(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, Render(&buf, Options{Format: FormatJSON}, []byte(sampleRaw), sampleSites, siteColumns))
	assert.Equal(t, "[\n  {\n    \"id\": 1,\n", buf.String()[:19])
	assert.Contains(t, buf.String(), `"create_time": "2026-08-27T01:02:03Z"`, "json 保留 API 原始值，不轉時區")
	assert.True(t, bytes.HasSuffix(buf.Bytes(), []byte("\n")))
}

func TestRenderYAMLFromRaw(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, Render(&buf, Options{Format: FormatYAML}, []byte(`{"id":1,"name":"x"}`), []site{}, siteColumns))
	assert.Equal(t, "id: 1\nname: x\n", buf.String())
}

func TestRenderJSONFallsBackToItemsWhenRawEmpty(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, Render(&buf, Options{Format: FormatJSON}, nil, sampleSites[:1], siteColumns))
	assert.Contains(t, buf.String(), `"Name": "web-01"`)
}

func TestRenderYAMLPreservesLargeIntegers(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, Render(&buf, Options{Format: FormatYAML}, []byte(`{"id":9007199254740993,"ratio":1.5,"n":-3}`), []site{}, siteColumns))
	assert.Equal(t, "id: 9007199254740993\n\"n\": -3\nratio: 1.5\n", buf.String())
}

func TestRenderYAMLFallbackUsesJSONTags(t *testing.T) {
	type tagged struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
		Skip string `json:"skip,omitempty"`
	}
	var buf bytes.Buffer
	require.NoError(t, Render(&buf, Options{Format: FormatYAML}, nil, []tagged{{ID: 1, Name: "x"}}, nil))
	assert.Equal(t, "- id: 1\n  name: x\n", buf.String())
}

func TestRenderYAMLPreservesIntegersBeyondInt64(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, Render(&buf, Options{Format: FormatYAML}, []byte(`{"big":18446744073709551615,"huge":123456789012345678901234567890,"f":1.5}`), []site{}, siteColumns))
	assert.Equal(t, "big: 18446744073709551615\nf: 1.5\nhuge: !!int 123456789012345678901234567890\n", buf.String())
}

func TestRenderYAMLRejectsTrailingContent(t *testing.T) {
	err := Render(&bytes.Buffer{}, Options{Format: FormatYAML}, []byte(`{"a":1} {"b":2}`), []site{}, siteColumns)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "多餘內容")
}

// TestRenderRawJSONReindentsValidJSON 驗證 json 格式對合法 JSON body 重新縮排。
func TestRenderRawJSONReindentsValidJSON(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, RenderRaw(&buf, FormatJSON, []byte(`{"a":1}`)))
	assert.Equal(t, "{\n  \"a\": 1\n}\n", buf.String())
}

// TestRenderRawJSONPassesThroughInvalidJSON 驗證 json 格式對非 JSON body（例如逃生口打到
// 回傳純文字的 endpoint）原樣輸出，而不是報錯；body 結尾沒有換行時會補上一個。
func TestRenderRawJSONPassesThroughInvalidJSON(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, RenderRaw(&buf, FormatJSON, []byte("plain text, not json")))
	assert.Equal(t, "plain text, not json\n", buf.String())
}

// TestRenderRawJSONPassThroughDoesNotDoubleNewline 驗證 body 結尾已經有換行時不會
// 再多補一個。
func TestRenderRawJSONPassThroughDoesNotDoubleNewline(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, RenderRaw(&buf, FormatJSON, []byte("plain text, not json\n")))
	assert.Equal(t, "plain text, not json\n", buf.String())
}

// TestRenderRawTableSameAsJSON 驗證 table 格式視同 json（逃生口沒有欄位定義可用來畫表格）。
func TestRenderRawTableSameAsJSON(t *testing.T) {
	var jsonBuf, tableBuf bytes.Buffer
	require.NoError(t, RenderRaw(&jsonBuf, FormatJSON, []byte(`{"a":1}`)))
	require.NoError(t, RenderRaw(&tableBuf, FormatTable, []byte(`{"a":1}`)))
	assert.Equal(t, jsonBuf.String(), tableBuf.String())
}

// TestRenderRawYAMLConvertsValidJSON 驗證 yaml 格式把合法 JSON body 轉成 YAML。
func TestRenderRawYAMLConvertsValidJSON(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, RenderRaw(&buf, FormatYAML, []byte(`{"a":1}`)))
	assert.Equal(t, "a: 1\n", buf.String())
}

// TestRenderRawYAMLRejectsInvalidJSON 驗證 yaml 格式對非 JSON body 回傳錯誤——
// 轉換成 YAML 的前提是合法 JSON，任意文字沒有辦法轉換。
func TestRenderRawYAMLRejectsInvalidJSON(t *testing.T) {
	err := RenderRaw(&bytes.Buffer{}, FormatYAML, []byte("plain text, not json"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "解析 JSON 回應失敗")
}

func TestParseFormat(t *testing.T) {
	for _, s := range []string{"table", "json", "yaml"} {
		f, err := ParseFormat(s)
		require.NoError(t, err)
		assert.Equal(t, Format(s), f)
	}
	_, err := ParseFormat("xml")
	assert.Error(t, err)
}

func TestHelpers(t *testing.T) {
	assert.Equal(t, "2026-08-27 09:02:03", FormatTime(time.Date(2026, 8, 27, 1, 2, 3, 0, time.UTC)))
	assert.Equal(t, "", TimePtr(nil))
	assert.Equal(t, "", Str(nil))
	s := "x"
	assert.Equal(t, "x", Str(&s))
	assert.Equal(t, "", Int64(nil))
	n := int64(7)
	assert.Equal(t, "7", Int64(&n))
	assert.Equal(t, "", Float(nil))
	f := 0.25
	assert.Equal(t, "0.25", Float(&f))
	one := 1.0
	assert.Equal(t, "1", Float(&one))
	assert.Equal(t, "yes", Bool(true))
	assert.Equal(t, "no", Bool(false))
	assert.Equal(t, "0 B", Bytes(0))
	assert.Equal(t, "512 B", Bytes(512))
	assert.Equal(t, "1.0 KiB", Bytes(1024))
	assert.Equal(t, "1.5 GiB", Bytes(1610612736))
}

func TestFormatTimeString(t *testing.T) {
	// RFC3339（含時區）。
	assert.Equal(t, "2026-08-27 09:02:03", FormatTimeString("2026-08-27T01:02:03Z"))
	// 無時區的 "T" 分隔格式（keypair create_time 實測值），視為 UTC。
	assert.Equal(t, "2023-01-24 00:17:14", FormatTimeString("2023-01-23T16:17:14"))
	// 無時區的空格分隔格式，視為 UTC。
	assert.Equal(t, "2023-01-24 00:17:14", FormatTimeString("2023-01-23 16:17:14"))
	// 無時區且含微秒的 "T" 分隔格式（vcs lb report --member 實測值，見
	// docs/api-notes.md），視為 UTC：Go 的 time.Parse／ParseInLocation 即使 layout
	// 沒有宣告小數秒位，仍會接受輸入內的小數秒（reference time 文件的既有行為），
	// 因此沿用既有的 "2006-01-02T15:04:05" layout 即可解析，不需要另外新增 layout。
	assert.Equal(t, "2018-09-25 17:55:00", FormatTimeString("2018-09-25T09:55:00.771000"))
	// 無法解析的字串原樣回傳。
	assert.Equal(t, "not-a-time", FormatTimeString("not-a-time"))
	// 空字串回傳空字串。
	assert.Equal(t, "", FormatTimeString(""))
}

func TestRenderKeyValues(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, RenderKeyValues(&buf, [][2]string{{"API version", "v5"}, {"Profile", "default"}}))
	assert.Equal(t, "API version:  v5\nProfile:      default\n", buf.String())
}

func TestSelectColumnsExported(t *testing.T) {
	defaults, err := SelectColumns(nil, siteColumns)
	require.NoError(t, err)
	require.Len(t, defaults, 3)
	for _, c := range defaults {
		assert.True(t, c.Default)
	}

	selected, err := SelectColumns([]string{"created", "NAME"}, siteColumns)
	require.NoError(t, err)
	require.Len(t, selected, 2)
	assert.Equal(t, "CREATED", selected[0].Name)
	assert.Equal(t, "NAME", selected[1].Name)

	_, err = SelectColumns([]string{"nope"}, siteColumns)
	assert.ErrorIs(t, err, ErrUnknownColumn)
}
