package vcs

import (
	"context"
	"fmt"
	"math"

	genvcs "github.com/gilbertchiao/twaictl/internal/gen/vcs"
	"github.com/gilbertchiao/twaictl/internal/twai"
)

// ListSnapshots 列出 project 下的 snapshot（API 要求 project 必填）；
// status 非空時以 status 查詢參數篩選。
func (s *Service) ListSnapshots(ctx context.Context, projectID int64, status string) (twai.Response[[]genvcs.SnapshotSerializer], error) {
	params := &genvcs.GetSnapshotsParams{Project: int(projectID), XApiHost: s.host}
	setString(&params.Status, status)
	resp, err := s.api.GetSnapshotsWithResponse(ctx, params)
	if err != nil {
		return twai.Response[[]genvcs.SnapshotSerializer]{}, fmt.Errorf("列出 snapshot 失敗: %w", err)
	}
	return twai.Decode[[]genvcs.SnapshotSerializer](resp.HTTPResponse, resp.Body)
}

// GetSnapshot 取得單一 snapshot 的詳細資料。
func (s *Service) GetSnapshot(ctx context.Context, id int64) (twai.Response[genvcs.SnapshotSerializer], error) {
	resp, err := s.api.GetSnapshotsSnapshotIdWithResponse(ctx, int(id), &genvcs.GetSnapshotsSnapshotIdParams{XApiHost: s.host})
	if err != nil {
		return twai.Response[genvcs.SnapshotSerializer]{}, fmt.Errorf("取得 snapshot %d 失敗: %w", id, err)
	}
	return twai.Decode[genvcs.SnapshotSerializer](resp.HTTPResponse, resp.Body)
}

// CreateSnapshot 建立 snapshot（POST /snapshots/，200 OK）；desc 為空代表不送對應欄位。
func (s *Service) CreateSnapshot(ctx context.Context, name string, volumeID int64, desc string) (twai.Response[genvcs.SnapshotSerializer], error) {
	// PostSnapshotsJSONRequestBody.Volume 是生成碼的 int32；volumeID 超出上限時直接
	// int32() 轉型會靜默溢位、送出錯誤的 id，必須在轉型前擋下；0 或負數雖不會溢位，
	// 但對 API 而言同樣不是合法的 volume id，一併視為「超出範圍」擋下。
	if volumeID <= 0 || volumeID > math.MaxInt32 {
		return twai.Response[genvcs.SnapshotSerializer]{}, fmt.Errorf("id %d 不是合法的 id（必須介於 1 與 %d 之間）", volumeID, int32(math.MaxInt32))
	}
	body := genvcs.PostSnapshotsJSONRequestBody{Name: name, Volume: int32(volumeID)}
	setString(&body.Desc, desc)
	resp, err := s.api.PostSnapshotsWithResponse(ctx, &genvcs.PostSnapshotsParams{XApiHost: s.host}, body)
	if err != nil {
		return twai.Response[genvcs.SnapshotSerializer]{}, fmt.Errorf("建立 snapshot 失敗: %w", err)
	}
	return twai.Decode[genvcs.SnapshotSerializer](resp.HTTPResponse, resp.Body)
}

// DeleteSnapshot 刪除 snapshot（204 No Content）。
func (s *Service) DeleteSnapshot(ctx context.Context, id int64) error {
	resp, err := s.api.DeleteSnapshotsSnapshotIdWithResponse(ctx, int(id), &genvcs.DeleteSnapshotsSnapshotIdParams{XApiHost: s.host})
	if err != nil {
		return fmt.Errorf("刪除 snapshot %d 失敗: %w", id, err)
	}
	return twai.CheckResponse(resp.HTTPResponse, resp.Body)
}
