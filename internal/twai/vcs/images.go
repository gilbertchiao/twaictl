package vcs

import (
	"context"
	"fmt"

	genvcs "github.com/gilbertchiao/twaictl/internal/gen/vcs"
	"github.com/gilbertchiao/twaictl/internal/twai"
)

// GetImage 取得單一 image 的詳細資料。
func (s *Service) GetImage(ctx context.Context, id int64) (twai.Response[genvcs.ImageDetailSerializer], error) {
	resp, err := s.api.GetImagesImageIdWithResponse(ctx, int(id), &genvcs.GetImagesImageIdParams{XApiHost: s.host})
	if err != nil {
		return twai.Response[genvcs.ImageDetailSerializer]{}, fmt.Errorf("取得 image %d 失敗: %w", id, err)
	}
	return twai.Decode[genvcs.ImageDetailSerializer](resp.HTTPResponse, resp.Body)
}

// DeleteImage 刪除 image（204）。
func (s *Service) DeleteImage(ctx context.Context, id int64) error {
	resp, err := s.api.DeleteImagesImageIdWithResponse(ctx, int(id), &genvcs.DeleteImagesImageIdParams{XApiHost: s.host})
	if err != nil {
		return fmt.Errorf("刪除 image %d 失敗: %w", id, err)
	}
	return twai.CheckResponse(resp.HTTPResponse, resp.Body)
}

// SaveImageInput 是 `vcs image save` 的參數；字串為空代表不送對應欄位（PUT
// /images/{server_id}/save/ 的 body 欄位皆為選填）。
type SaveImageInput struct {
	ServerID                  int64
	Name, OS, OSVersion, Desc string
}

// SaveImage 把 server 目前狀態存成新 image（PUT /images/{server_id}/save/，201 Created）。
func (s *Service) SaveImage(ctx context.Context, in SaveImageInput) (twai.Response[genvcs.ImagePostSerializer], error) {
	body := genvcs.PutImagesServerIdSaveJSONRequestBody{}
	setString(&body.Name, in.Name)
	setString(&body.Os, in.OS)
	setString(&body.OsVersion, in.OSVersion)
	setString(&body.Desc, in.Desc)
	resp, err := s.api.PutImagesServerIdSaveWithResponse(ctx, int(in.ServerID), &genvcs.PutImagesServerIdSaveParams{XApiHost: s.host}, body)
	if err != nil {
		return twai.Response[genvcs.ImagePostSerializer]{}, fmt.Errorf("建立 server %d 的 image 失敗: %w", in.ServerID, err)
	}
	return twai.Decode[genvcs.ImagePostSerializer](resp.HTTPResponse, resp.Body)
}
