package vcs

import (
	"context"
	"fmt"

	genvcs "github.com/gilbertchiao/twaictl/internal/gen/vcs"
	"github.com/gilbertchiao/twaictl/internal/twai"
)

// ListNetworks 列出 project 下的網路（API 要求 project 必填）。
func (s *Service) ListNetworks(ctx context.Context, projectID int64) (twai.Response[[]genvcs.NetworkSerializer], error) {
	resp, err := s.api.GetNetworksWithResponse(ctx, &genvcs.GetNetworksParams{Project: int(projectID), XApiHost: s.host})
	if err != nil {
		return twai.Response[[]genvcs.NetworkSerializer]{}, fmt.Errorf("列出網路失敗: %w", err)
	}
	return twai.Decode[[]genvcs.NetworkSerializer](resp.HTTPResponse, resp.Body)
}

// GetNetwork 取得單一網路的詳細資料。
//
// 注意（spec 疑點，已記錄到 docs/api-notes.md）：VCS.yaml 標示這支 API 成功回應
// 為 204（無 body）+ NetworkDetailSerializer schema，兩者互相矛盾；本專案一律用
// twai.Decode 直接解析 body，不理會生成碼依 spec 產生的 JSON204 欄位命名，
// 因此即使實際回應是常見的 200 也不受影響。
func (s *Service) GetNetwork(ctx context.Context, id int64) (twai.Response[genvcs.NetworkDetailSerializer], error) {
	resp, err := s.api.GetNetworksNetworkIdWithResponse(ctx, int(id), &genvcs.GetNetworksNetworkIdParams{XApiHost: s.host})
	if err != nil {
		return twai.Response[genvcs.NetworkDetailSerializer]{}, fmt.Errorf("取得網路 %d 失敗: %w", id, err)
	}
	return twai.Decode[genvcs.NetworkDetailSerializer](resp.HTTPResponse, resp.Body)
}

// CreateNetworkInput 是 `vcs network create` 的參數；字串為空 / WithRouter 為 false
// 代表不送對應欄位（NetworkPostSerializer 的欄位皆為選填指標）。
//
// 注意：WithRouter 沒有「未指定」與「明確指定 false」的區別（bool 零值即 false），
// 這與 brief 「未指定不出現」的測試期待一致：`--router` 沒帶時不送 with_router。
type CreateNetworkInput struct {
	ProjectID                      int64
	Name, CIDR, Gateway, DNSDomain string
	WithRouter                     bool
}

// CreateNetwork 建立網路（POST /networks/，201 Created）。
func (s *Service) CreateNetwork(ctx context.Context, in CreateNetworkInput) (twai.Response[genvcs.NetworkSerializer], error) {
	project := in.ProjectID
	body := genvcs.PostNetworksJSONRequestBody{Project: &project}
	setString(&body.Name, in.Name)
	setString(&body.Cidr, in.CIDR)
	setString(&body.Gateway, in.Gateway)
	setString(&body.DnsDomain, in.DNSDomain)
	if in.WithRouter {
		withRouter := true
		body.WithRouter = &withRouter
	}
	resp, err := s.api.PostNetworksWithResponse(ctx, &genvcs.PostNetworksParams{XApiHost: s.host}, body)
	if err != nil {
		return twai.Response[genvcs.NetworkSerializer]{}, fmt.Errorf("建立網路失敗: %w", err)
	}
	return twai.Decode[genvcs.NetworkSerializer](resp.HTTPResponse, resp.Body)
}

// DeleteNetwork 刪除網路（202 Accepted，非同步）。
func (s *Service) DeleteNetwork(ctx context.Context, id int64) error {
	resp, err := s.api.DeleteNetworksNetworkIdWithResponse(ctx, int(id), &genvcs.DeleteNetworksNetworkIdParams{XApiHost: s.host})
	if err != nil {
		return fmt.Errorf("刪除網路 %d 失敗: %w", id, err)
	}
	return twai.CheckResponse(resp.HTTPResponse, resp.Body)
}
