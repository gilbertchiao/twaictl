package vcs

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	genvcs "github.com/gilbertchiao/twaictl/internal/gen/vcs"
	"github.com/gilbertchiao/twaictl/internal/twai"
)

// FlexibleID 解 JSON 時同時接受數字（原樣轉成十進位字串）、字串或 null（空字串）：
// spec 的 SecretSerializer.id 標 string、SecretIDParam 標 integer 自相矛盾（見
// docs/api-notes.md A28）；volume attach 回應的 server_id 則是 spec 標 integer、真實
// 回應為字串（A38）。以此型別寬鬆解析，避免任一種真實回應讓整個指令壞掉。
type FlexibleID string

// UnmarshalJSON 依 JSON 值的實際型別解析：字串直接採用；數字轉成十進位字串
// （用 json.Number 保留精度，不經過浮點數）；null 視為空字串；其他型別（例如 bool、物件）視為錯誤。
func (f *FlexibleID) UnmarshalJSON(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return fmt.Errorf("識別碼必須是數字或字串: %s: %w", b, err)
	}
	switch value := v.(type) {
	case string:
		*f = FlexibleID(value)
	case json.Number:
		*f = FlexibleID(value.String())
	case nil:
		*f = ""
	default:
		return fmt.Errorf("識別碼必須是數字或字串: %s", b)
	}
	return nil
}

// Secret 是手寫的 secret 回應型別：spec 的 SecretSerializer.id 標 string、
// SecretIDParam 標 integer 自相矛盾，真實值型別未確認（見 docs/api-notes.md A28），
// ID 以 FlexibleID 同時接受 JSON 數字與字串。
type Secret struct {
	ID         FlexibleID                     `json:"id"`
	Name       string                         `json:"name"`
	Status     string                         `json:"status"`
	Desc       string                         `json:"desc"`
	CreateTime *string                        `json:"create_time,omitempty"` // 顯示用 output.FormatTimeString
	ExpireTime *string                        `json:"expire_time,omitempty"`
	Project    *genvcs.ProjectFieldSerializer `json:"project,omitempty"`
	User       *genvcs.UserSerializer         `json:"user,omitempty"`
}

// ListSecrets 列出 project 下的 secret（API 要求 project 必填）。
func (s *Service) ListSecrets(ctx context.Context, projectID int64) (twai.Response[[]Secret], error) {
	resp, err := s.api.GetSecretsWithResponse(ctx, &genvcs.GetSecretsParams{Project: int(projectID), XApiHost: s.host})
	if err != nil {
		return twai.Response[[]Secret]{}, fmt.Errorf("列出 secret 失敗: %w", err)
	}
	return twai.Decode[[]Secret](resp.HTTPResponse, resp.Body)
}

// GetSecret 取得單一 secret 的詳細資料。
func (s *Service) GetSecret(ctx context.Context, id string) (twai.Response[Secret], error) {
	resp, err := s.api.GetSecretsSecretIdWithResponse(ctx, id, &genvcs.GetSecretsSecretIdParams{XApiHost: s.host})
	if err != nil {
		return twai.Response[Secret]{}, fmt.Errorf("取得 secret %s 失敗: %w", id, err)
	}
	return twai.Decode[Secret](resp.HTTPResponse, resp.Body)
}

// CreateSecretInput 是 `vcs secret create` 的參數；Payload 為原始內容，
// CreateSecret 內做 base64 編碼後才送出；ExpireTime 為 nil 時不送該欄位。
type CreateSecretInput struct {
	ProjectID  int64
	Name, Desc string
	Payload    []byte
	ExpireTime *time.Time
}

// CreateSecret 建立 secret（POST /secrets/）。真實環境實測（見 docs/api-notes.md
// A28）：spec 只把 project 標在 query，但 body 缺 project 會被 API 回 400
// `{'project': ['This field is required.']}`，因此 body 與 query 都要帶 project。
func (s *Service) CreateSecret(ctx context.Context, in CreateSecretInput) (twai.Response[Secret], error) {
	if err := int32InRange(in.ProjectID); err != nil {
		return twai.Response[Secret]{}, err
	}
	body := genvcs.PostSecretsJSONRequestBody{
		Name:    in.Name,
		Payload: base64.StdEncoding.EncodeToString(in.Payload),
		Project: int32(in.ProjectID),
	}
	setString(&body.Desc, in.Desc)
	body.ExpireTime = in.ExpireTime
	resp, err := s.api.PostSecretsWithResponse(ctx, &genvcs.PostSecretsParams{Project: int(in.ProjectID), XApiHost: s.host}, body)
	if err != nil {
		return twai.Response[Secret]{}, fmt.Errorf("建立 secret 失敗: %w", err)
	}
	return twai.Decode[Secret](resp.HTTPResponse, resp.Body)
}

// DeleteSecret 刪除 secret。
func (s *Service) DeleteSecret(ctx context.Context, id string) error {
	resp, err := s.api.DeleteSecretsSecretIdWithResponse(ctx, id, &genvcs.DeleteSecretsSecretIdParams{XApiHost: s.host})
	if err != nil {
		return fmt.Errorf("刪除 secret %s 失敗: %w", id, err)
	}
	return twai.CheckResponse(resp.HTTPResponse, resp.Body)
}

// ResolveSecretID 把 <id|name> 解析為 secret id：ref 與某筆 id 完全相等時直接回傳
// （不論 ref 本身是否為純數字——secret id 可能是數字也可能是字串，見 FlexibleID）；
// 否則以 name 唯一比對，找不到或重複皆回錯誤。
func (s *Service) ResolveSecretID(ctx context.Context, projectID int64, ref string) (string, error) {
	res, err := s.ListSecrets(ctx, projectID)
	if err != nil {
		return "", err
	}
	for _, secret := range res.Value {
		if string(secret.ID) == ref {
			return string(secret.ID), nil
		}
	}
	var matches []string
	for _, secret := range res.Value {
		if secret.Name == ref {
			matches = append(matches, string(secret.ID))
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("找不到 secret %q", ref)
	case 1:
		return matches[0], nil
	}
	return "", fmt.Errorf("secret 名稱 %q 對應不只一個（%s），請改用 id", ref, strings.Join(matches, ", "))
}
