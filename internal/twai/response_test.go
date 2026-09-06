package twai

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeObjectOrFirstAcceptsObjectAndArray(t *testing.T) {
	type item struct {
		ID int `json:"id"`
	}
	resp := &http.Response{StatusCode: 200, Header: http.Header{}}

	obj, err := DecodeObjectOrFirst[item](resp, []byte(`{"id": 7}`), "firewall rule")
	require.NoError(t, err)
	assert.Equal(t, 7, obj.Value.ID)
	assert.JSONEq(t, `{"id": 7}`, string(obj.Raw))

	arr, err := DecodeObjectOrFirst[item](resp, []byte(` [{"id": 8, "extra": true}, {"id": 9}]`), "firewall rule")
	require.NoError(t, err)
	assert.Equal(t, 8, arr.Value.ID)
	assert.JSONEq(t, `{"id": 8, "extra": true}`, string(arr.Raw), "Raw 必須是第一筆的原始 bytes，保留生成型別沒有的欄位")

	_, err = DecodeObjectOrFirst[item](resp, []byte(`[]`), "firewall rule")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "firewall rule 回應為空陣列")

	_, err = DecodeObjectOrFirst[item](&http.Response{StatusCode: 404, Header: http.Header{}}, []byte(`{"message":"x"}`), "firewall rule")
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
}

func TestCheckResponse(t *testing.T) {
	u, _ := url.Parse("https://gw/api/v3/x/sites/")
	mk := func(status int, dry bool) *http.Response {
		resp := &http.Response{StatusCode: status, Header: http.Header{}, Request: &http.Request{Method: "GET", URL: u}}
		if dry {
			resp.Header.Set(HeaderDryRun, "true")
		}
		return resp
	}
	assert.NoError(t, CheckResponse(mk(200, false), nil))
	assert.NoError(t, CheckResponse(mk(201, false), nil))
	assert.NoError(t, CheckResponse(mk(204, false), nil))

	err := CheckResponse(mk(404, false), []byte(`{"message":"not found"}`))
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 404, apiErr.StatusCode)
	assert.Equal(t, "not found", apiErr.Message)

	assert.ErrorIs(t, CheckResponse(mk(204, true), nil), ErrDryRun)
}

func TestDecode(t *testing.T) {
	u, _ := url.Parse("https://gw/api/v3/x/sites/")
	mk := func(status int, dry bool) *http.Response {
		resp := &http.Response{StatusCode: status, Header: http.Header{}, Request: &http.Request{Method: "GET", URL: u}}
		if dry {
			resp.Header.Set(HeaderDryRun, "true")
		}
		return resp
	}

	type item struct {
		Name string `json:"name"`
	}

	t.Run("200 回傳 Value 與 Raw", func(t *testing.T) {
		body := []byte(`{"name":"web-01"}`)
		res, err := Decode[item](mk(200, false), body)
		require.NoError(t, err)
		assert.Equal(t, "web-01", res.Value.Name)
		assert.Equal(t, body, res.Raw)
	})

	t.Run("404 回傳 APIError", func(t *testing.T) {
		res, err := Decode[item](mk(404, false), []byte(`{"message":"not found"}`))
		var apiErr *APIError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, 404, apiErr.StatusCode)
		assert.Equal(t, "not found", apiErr.Message)
		assert.Empty(t, res.Raw, "解析失敗時不應保留 Raw")
	})

	t.Run("dry-run 回傳 ErrDryRun", func(t *testing.T) {
		_, err := Decode[item](mk(204, true), nil)
		assert.ErrorIs(t, err, ErrDryRun)
	})

	t.Run("非法 JSON 包裝錯誤", func(t *testing.T) {
		res, err := Decode[item](mk(200, false), []byte(`not json`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "解析 API 回應失敗")
		assert.Empty(t, res.Raw, "解析失敗時不應保留 Raw")
	})
}

func TestRequireJSONFields(t *testing.T) {
	t.Run("全部欄位存在時回傳 nil", func(t *testing.T) {
		err := RequireJSONFields([]byte(`{"a":1,"b":2}`), "a", "b")
		assert.NoError(t, err)
	})

	t.Run("欄位存在但值為零值仍視為存在", func(t *testing.T) {
		err := RequireJSONFields([]byte(`{"buckets":[]}`), "buckets")
		assert.NoError(t, err)
	})

	t.Run("缺少欄位時回傳錯誤並附上 body 片段", func(t *testing.T) {
		err := RequireJSONFields([]byte(`{"message":"no Route matched"}`), "total_used_space", "total_objects")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "缺少欄位 total_used_space")
		assert.Contains(t, err.Error(), "no Route matched")
	})

	t.Run("欄位值為 JSON null 視同缺少", func(t *testing.T) {
		err := RequireJSONFields([]byte(`{"total_used_space":null,"total_objects":null}`), "total_used_space", "total_objects")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "缺少欄位 total_used_space")

		err = RequireJSONFields([]byte(`{"buckets":null}`), "buckets")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "缺少欄位 buckets")
	})

	t.Run("body 不是 JSON 物件時回傳錯誤", func(t *testing.T) {
		err := RequireJSONFields([]byte(`[1,2,3]`), "a")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "不是 JSON 物件")
	})
}
