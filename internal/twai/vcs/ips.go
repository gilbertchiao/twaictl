package vcs

import (
	"context"
	"fmt"
	"math"

	genvcs "github.com/gilbertchiao/twaictl/internal/gen/vcs"
	"github.com/gilbertchiao/twaictl/internal/twai"
)

// ListIPs 列出 project 下的浮動 IP／固定 IP（API 要求 project 必填）；
// address 非空時以 address 查詢參數篩選。
func (s *Service) ListIPs(ctx context.Context, projectID int64, address string) (twai.Response[[]genvcs.IPSerializer], error) {
	params := &genvcs.GetIpsParams{Project: int(projectID), XApiHost: s.host}
	setString(&params.Address, address)
	resp, err := s.api.GetIpsWithResponse(ctx, params)
	if err != nil {
		return twai.Response[[]genvcs.IPSerializer]{}, fmt.Errorf("列出 IP 失敗: %w", err)
	}
	return twai.Decode[[]genvcs.IPSerializer](resp.HTTPResponse, resp.Body)
}

// GetIP 取得單一 IP 的詳細資料。
func (s *Service) GetIP(ctx context.Context, id int64) (twai.Response[genvcs.IPSerializer], error) {
	resp, err := s.api.GetIpsIpIdWithResponse(ctx, int(id), &genvcs.GetIpsIpIdParams{XApiHost: s.host})
	if err != nil {
		return twai.Response[genvcs.IPSerializer]{}, fmt.Errorf("取得 IP %d 失敗: %w", id, err)
	}
	return twai.Decode[genvcs.IPSerializer](resp.HTTPResponse, resp.Body)
}

// CreateIP 申請新的浮動 IP（POST /ips/ {project}，200 OK）。
func (s *Service) CreateIP(ctx context.Context, projectID int64) (twai.Response[genvcs.IPSerializer], error) {
	// PostIpsJSONRequestBody.Project 是生成碼的 int32；projectID 超出上限時直接 int32()
	// 轉型會靜默溢位、送出錯誤的 id，必須在轉型前擋下；0 或負數雖不會溢位，
	// 但對 API 而言同樣不是合法的 project id，一併視為「超出範圍」擋下。
	if projectID <= 0 || projectID > math.MaxInt32 {
		return twai.Response[genvcs.IPSerializer]{}, fmt.Errorf("id %d 不是合法的 id（必須介於 1 與 %d 之間）", projectID, int32(math.MaxInt32))
	}
	body := genvcs.PostIpsJSONRequestBody{Project: int32(projectID)}
	resp, err := s.api.PostIpsWithResponse(ctx, &genvcs.PostIpsParams{XApiHost: s.host}, body)
	if err != nil {
		return twai.Response[genvcs.IPSerializer]{}, fmt.Errorf("建立 IP 失敗: %w", err)
	}
	return twai.Decode[genvcs.IPSerializer](resp.HTTPResponse, resp.Body)
}

// UpdateIPDesc 更新 IP 的描述（PATCH /ips/{id}/）。
func (s *Service) UpdateIPDesc(ctx context.Context, id int64, desc string) (twai.Response[genvcs.IPSerializer], error) {
	body := genvcs.PatchIpsIpIdJSONRequestBody{Desc: &desc}
	resp, err := s.api.PatchIpsIpIdWithResponse(ctx, int(id), &genvcs.PatchIpsIpIdParams{XApiHost: s.host}, body)
	if err != nil {
		return twai.Response[genvcs.IPSerializer]{}, fmt.Errorf("更新 IP %d 描述失敗: %w", id, err)
	}
	return twai.Decode[genvcs.IPSerializer](resp.HTTPResponse, resp.Body)
}

// DeleteIP 刪除 IP（204 No Content）。
func (s *Service) DeleteIP(ctx context.Context, id int64) error {
	resp, err := s.api.DeleteIpsIpIdWithResponse(ctx, int(id), &genvcs.DeleteIpsIpIdParams{XApiHost: s.host})
	if err != nil {
		return fmt.Errorf("刪除 IP %d 失敗: %w", id, err)
	}
	return twai.CheckResponse(resp.HTTPResponse, resp.Body)
}
