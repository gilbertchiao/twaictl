package vcs

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlexibleIDAcceptsNumberAndString(t *testing.T) {
	var a, b struct {
		ID FlexibleID `json:"id"`
	}
	require.NoError(t, json.Unmarshal([]byte(`{"id": 123}`), &a))
	require.NoError(t, json.Unmarshal([]byte(`{"id": "c1d2-uuid"}`), &b))
	assert.Equal(t, FlexibleID("123"), a.ID)
	assert.Equal(t, FlexibleID("c1d2-uuid"), b.ID)
	var c struct {
		ID FlexibleID `json:"id"`
	}
	require.NoError(t, json.Unmarshal([]byte(`{"id": null}`), &c))
	assert.Equal(t, FlexibleID(""), c.ID, "null 視為空字串")
	require.Error(t, json.Unmarshal([]byte(`{"id": true}`), &c), "bool 仍應視為錯誤")
	assert.Error(t, json.Unmarshal([]byte(`{"id": true}`), &a))
}

func TestCreateSecretEncodesPayloadAndSendsProjectInQueryAndBody(t *testing.T) {
	f := newFakeAPI(t)
	f.on("POST", vcsBase+"/secrets/", 200, []byte(`{"id": 7, "name": "s1", "status": "ACTIVE", "desc": "", "create_time": "2026-08-28T01:02:03Z", "expire_time": "2027-01-01T00:00:00Z"}`))
	s := newService(t, f)
	expire := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	res, err := s.CreateSecret(context.Background(), CreateSecretInput{ProjectID: 101, Name: "s1", Payload: []byte("hello"), ExpireTime: &expire})
	require.NoError(t, err)
	req := f.Find("POST", vcsBase+"/secrets/")
	require.NotNil(t, req)
	assert.Equal(t, "project=101", req.Query)
	assert.JSONEq(t, `{"name":"s1","payload":"aGVsbG8=","expire_time":"2027-01-01T00:00:00Z","project":101}`, string(req.Body))
	assert.Equal(t, FlexibleID("7"), res.Value.ID)
}

// TestCreateSecretRejectsInvalidProjectID 驗證 ProjectID 為 0 或負數時，在送出請求前就
// 由 int32InRange 回傳明確錯誤，不會呼叫 API（與 CreateSnapshot／CreateIP 對非法 id 的
// 檢查方式一致）。
func TestCreateSecretRejectsInvalidProjectID(t *testing.T) {
	f := newFakeAPI(t)
	s := newService(t, f)

	for _, id := range []int64{0, -1} {
		_, err := s.CreateSecret(context.Background(), CreateSecretInput{ProjectID: id, Name: "s1", Payload: []byte("hello")})
		assert.ErrorContains(t, err, "不是合法的 id", "id=%d", id)
		assert.Nil(t, f.Find("POST", vcsBase+"/secrets/"), "id=%d", id)
	}
}

// TestGetSecretSendsNonNumericIDInPath 端到端證明 SecretIDParam 的 spec patch
// （integer → string）真的生效：path 參數可以放非數字字串（例如 UUID），不會被
// oapi-codegen 生成的 client 擋在請求送出之前。先前的測試都只用數字 id 打過
// GetSecret，或雖然解析出非數字 id（TestResolveSecretIDByIDThenName 的 "abc"）
// 但沒有拿它去發過請求，patch 是否真的生效只有「編譯期型別變成 string」這個
// 間接證據；這裡直接用非數字 id 呼叫 GetSecret，斷言請求路徑正確帶有該 id。
func TestGetSecretSendsNonNumericIDInPath(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/secrets/c1d2-uuid/", 200, []byte(`{"id": "c1d2-uuid", "name": "tls-cert", "status": "ACTIVE", "desc": ""}`))
	s := newService(t, f)

	res, err := s.GetSecret(context.Background(), "c1d2-uuid")
	require.NoError(t, err)
	require.NotNil(t, f.Find("GET", vcsBase+"/secrets/c1d2-uuid/"))
	assert.Equal(t, FlexibleID("c1d2-uuid"), res.Value.ID)
}

func TestResolveSecretIDByIDThenName(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/secrets/", 200, []byte(`[{"id": 7, "name": "s1"}, {"id": "abc", "name": "s2"}, {"id": 9, "name": "dup"}, {"id": 10, "name": "dup"}]`))
	s := newService(t, f)

	id, err := s.ResolveSecretID(context.Background(), 101, "7")
	require.NoError(t, err)
	assert.Equal(t, "7", id)

	id, err = s.ResolveSecretID(context.Background(), 101, "abc")
	require.NoError(t, err)
	assert.Equal(t, "abc", id)

	id, err = s.ResolveSecretID(context.Background(), 101, "s2")
	require.NoError(t, err)
	assert.Equal(t, "abc", id)

	_, err = s.ResolveSecretID(context.Background(), 101, "dup")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "不只一個")

	_, err = s.ResolveSecretID(context.Background(), 101, "nope")
	require.Error(t, err)
}
