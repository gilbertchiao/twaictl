package vcs

import (
	"bytes"
	"context"
	"fmt"
	"io"

	genvcs "github.com/gilbertchiao/twaictl/internal/gen/vcs"
	"github.com/gilbertchiao/twaictl/internal/twai"
)

// VolumeActions 是 `vcs volume action` 的合法動作。
var VolumeActions = []string{"attach", "detach", "extend"}

// ListVolumes 列出 project 下的 volume（API 要求 project 必填）。
func (s *Service) ListVolumes(ctx context.Context, projectID int64) (twai.Response[[]genvcs.VolumeSerializer], error) {
	resp, err := s.api.GetVolumesWithResponse(ctx, &genvcs.GetVolumesParams{Project: int(projectID), XApiHost: s.host})
	if err != nil {
		return twai.Response[[]genvcs.VolumeSerializer]{}, fmt.Errorf("列出 volume 失敗: %w", err)
	}
	return twai.Decode[[]genvcs.VolumeSerializer](resp.HTTPResponse, resp.Body)
}

// GetVolume 取得單一 volume 的詳細資料。
func (s *Service) GetVolume(ctx context.Context, id int64) (twai.Response[genvcs.VolumeSerializer], error) {
	resp, err := s.api.GetVolumesVolumeIdWithResponse(ctx, int(id), &genvcs.GetVolumesVolumeIdParams{XApiHost: s.host})
	if err != nil {
		return twai.Response[genvcs.VolumeSerializer]{}, fmt.Errorf("取得 volume %d 失敗: %w", id, err)
	}
	return twai.Decode[genvcs.VolumeSerializer](resp.HTTPResponse, resp.Body)
}

// CreateVolumeInput 是 `vcs volume create` 的參數；VolumeType 為空代表不送對應欄位。
type CreateVolumeInput struct {
	ProjectID        int64
	Name             string
	SizeGB           int64
	VolumeType, Desc string
}

// VolumeCreated 對應 POST /volumes/ 成功回應（生成碼為匿名 struct，這裡給它名字）。
type VolumeCreated struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Platform   string `json:"platform"`
	Project    int64  `json:"project"`
	SizeGB     int64  `json:"size"`
	VolumeType string `json:"volume_type"`
}

// CreateVolume 建立 volume（POST /volumes/，200 OK）。
func (s *Service) CreateVolume(ctx context.Context, in CreateVolumeInput) (twai.Response[VolumeCreated], error) {
	body := genvcs.PostVolumesJSONRequestBody{
		Name:    in.Name,
		Project: in.ProjectID,
		Size:    in.SizeGB,
	}
	setString(&body.VolumeType, in.VolumeType)
	setString(&body.Desc, in.Desc)
	resp, err := s.api.PostVolumesWithResponse(ctx, &genvcs.PostVolumesParams{XApiHost: s.host}, body)
	if err != nil {
		return twai.Response[VolumeCreated]{}, fmt.Errorf("建立 volume 失敗: %w", err)
	}
	return twai.Decode[VolumeCreated](resp.HTTPResponse, resp.Body)
}

// DeleteVolume 刪除 volume（204 No Content）。
func (s *Service) DeleteVolume(ctx context.Context, id int64) error {
	resp, err := s.api.DeleteVolumesVolumeIdWithResponse(ctx, int(id), &genvcs.DeleteVolumesVolumeIdParams{XApiHost: s.host})
	if err != nil {
		return fmt.Errorf("刪除 volume %d 失敗: %w", id, err)
	}
	return twai.CheckResponse(resp.HTTPResponse, resp.Body)
}

// VolumeActionResult 對應 PUT /volumes/{id}/action/ 成功回應（生成碼為匿名 struct，這裡給它名字）。
// VolumeID／ServerID 用 FlexibleID（secrets.go）：spec 皆標 integer，真實 attach 回應卻都是字串
// （見 docs/api-notes.md A38）。
type VolumeActionResult struct {
	VolumeID   FlexibleID `json:"volume_id"`
	ServerID   FlexibleID `json:"server_id"`
	Mountpoint string     `json:"mountpoint"`
}

// VolumeAction 對 volume 執行 attach／detach／extend；serverID／sizeGB 為 0 時不送對應欄位
// （attach/detach 只送 server、extend 只送 size）。
func (s *Service) VolumeAction(ctx context.Context, id int64, action string, serverID, sizeGB int64) (twai.Response[VolumeActionResult], error) {
	status := genvcs.PutVolumesVolumeIdActionJSONBodyStatus(action)
	body := genvcs.PutVolumesVolumeIdActionJSONRequestBody{Status: &status}
	if serverID != 0 {
		server := int(serverID)
		body.Server = &server
	}
	if sizeGB != 0 {
		size := int(sizeGB)
		body.Size = &size
	}
	// 不用生成的 ...WithResponse：它會依 spec 把 200 回應解析成 server_id 為 int 的匿名 struct，
	// 真實 attach 回應的 server_id 是字串，會在生成碼內就解析失敗（見 docs/api-notes.md A38）；
	// 改用底層方法拿原始 *http.Response 自行解析成 VolumeActionResult。
	httpResp, err := s.api.PutVolumesVolumeIdAction(ctx, int(id), &genvcs.PutVolumesVolumeIdActionParams{XApiHost: s.host}, body)
	if err != nil {
		return twai.Response[VolumeActionResult]{}, fmt.Errorf("對 volume %d 執行 %s 失敗: %w", id, action, err)
	}
	defer func() { _ = httpResp.Body.Close() }()
	raw, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return twai.Response[VolumeActionResult]{}, fmt.Errorf("讀取 volume %d 的 %s 回應失敗: %w", id, action, err)
	}
	// 真實環境 detach 成功時回 200 但 body 為空（A38）：先確認狀態碼，body 為空就回傳
	// 零值結果（Raw 為空），不當成解析錯誤。
	if err := twai.CheckResponse(httpResp, raw); err != nil {
		return twai.Response[VolumeActionResult]{}, err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return twai.Response[VolumeActionResult]{}, nil
	}
	return twai.Decode[VolumeActionResult](httpResp, raw)
}
