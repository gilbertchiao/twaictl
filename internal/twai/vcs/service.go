// Package vcs 是 VCS（Virtual Compute Service）API 的手寫 wrapper：
// 把生成 client 包成命令層好用的方法、統一 x-api-host、錯誤檢查與名稱解析。
package vcs

import (
	"context"
	"fmt"
	"strconv"
	"time"

	gencommon "github.com/gilbertchiao/twaictl/internal/gen/common"
	genvcs "github.com/gilbertchiao/twaictl/internal/gen/vcs"
	"github.com/gilbertchiao/twaictl/internal/twai"
)

// SolutionCategory 是 VCS 解決方案在 Common /solutions/ 的分類值（spec 只在 example 出現）。
const SolutionCategory = "os"

// DefaultWaitInterval 是 --wait 的輪詢間隔。
const DefaultWaitInterval = 5 * time.Second

// Service 包裝 VCS 與 Common 的生成 client。
type Service struct {
	api        *genvcs.ClientWithResponses
	common     *gencommon.ClientWithResponses
	host       string // VCS 的 x-api-host
	commonHost string // Common 的 x-api-host
}

// New 由 twai.Client 建立 Service。
func New(client *twai.Client) *Service {
	return &Service{
		api:        client.VCS,
		common:     client.Common,
		host:       client.Settings.APIHosts.VCS,
		commonHost: client.Settings.APIHosts.Common,
	}
}

// KeypairDetail 對應 GET /keypairs/{key_name}/ 的回應（生成碼為匿名 struct，這裡給它名字）。
//
// CreateTime 為 *string 而非 *time.Time：真實 API（2026-08-27 實測）回傳無時區的
// 本地時間字串（例如 "2023-01-23T16:17:14"），不符合 spec 標的 date-time 格式，
// 已透過 patch 修正 spec（見 api/openapi/patches/VCS-keypair-create-time-string.patch）；
// 顯示時交由 output.FormatTimeString 依序嘗試多種格式解析。
type KeypairDetail struct {
	Name        string                `json:"name"`
	Fingerprint string                `json:"fingerprint"`
	Platform    string                `json:"platform"`
	PublicKey   *string               `json:"public_key,omitempty"`
	CreateTime  *string               `json:"create_time,omitempty"`
	User        genvcs.UserSerializer `json:"user"`
}

// ListProjects 列出使用者可用的 project。
func (s *Service) ListProjects(ctx context.Context) (twai.Response[[]genvcs.GroupSerializer], error) {
	return s.listProjects(ctx, nil)
}

// listProjects 是 ListProjects 與 ResolveProjectID 共用的實作，name 非 nil 時以 name 篩選。
func (s *Service) listProjects(ctx context.Context, name *string) (twai.Response[[]genvcs.GroupSerializer], error) {
	resp, err := s.api.GetProjectsWithResponse(ctx, &genvcs.GetProjectsParams{Name: name, XApiHost: s.host})
	if err != nil {
		return twai.Response[[]genvcs.GroupSerializer]{}, fmt.Errorf("列出 project 失敗: %w", err)
	}
	return twai.Decode[[]genvcs.GroupSerializer](resp.HTTPResponse, resp.Body)
}

// GetProject 取得單一 project 詳細資料。
func (s *Service) GetProject(ctx context.Context, id int64) (twai.Response[genvcs.GroupDetailSerializer], error) {
	resp, err := s.api.GetProjectsProjectIdWithResponse(ctx, int(id), &genvcs.GetProjectsProjectIdParams{XApiHost: s.host})
	if err != nil {
		return twai.Response[genvcs.GroupDetailSerializer]{}, fmt.Errorf("取得 project %d 失敗: %w", id, err)
	}
	return twai.Decode[genvcs.GroupDetailSerializer](resp.HTTPResponse, resp.Body)
}

// ListSites 列出 project 下的 VCS 實例；allUsers 對應 all_users=1（列出所有使用者的資源）。
func (s *Service) ListSites(ctx context.Context, projectID int64, allUsers bool) (twai.Response[[]genvcs.SiteSerializer], error) {
	params := &genvcs.GetSitesParams{Project: int(projectID), XApiHost: s.host}
	if allUsers {
		all := genvcs.N1
		params.AllUsers = &all
	}
	resp, err := s.api.GetSitesWithResponse(ctx, params)
	if err != nil {
		return twai.Response[[]genvcs.SiteSerializer]{}, fmt.Errorf("列出 VCS 實例失敗: %w", err)
	}
	return twai.Decode[[]genvcs.SiteSerializer](resp.HTTPResponse, resp.Body)
}

// GetSite 取得單一 VCS 實例。
func (s *Service) GetSite(ctx context.Context, id int64) (twai.Response[genvcs.SiteSerializer], error) {
	resp, err := s.api.GetSitesSiteIdWithResponse(ctx, int(id), &genvcs.GetSitesSiteIdParams{XApiHost: s.host})
	if err != nil {
		return twai.Response[genvcs.SiteSerializer]{}, fmt.Errorf("取得 VCS 實例 %d 失敗: %w", id, err)
	}
	return twai.Decode[genvcs.SiteSerializer](resp.HTTPResponse, resp.Body)
}

// CreateSiteInput 是 vcs create 的參數；字串為空 / 數值為 0 代表不送對應 header。
type CreateSiteInput struct {
	Name, Desc                                       string
	ProjectID, SolutionID                            int64
	Image, Flavor, Keypair, Password, PrivateNetwork string
	FloatingIP                                       bool
	VolumeSize, SystemVolumeSize                     int
	VolumeType, SystemVolumeType, AvailabilityZone   string
}

// CreateSite 建立 VCS 實例。body 只有 name/project/solution/desc，其餘依 spec 以 x-extra-property-* header 傳遞。
func (s *Service) CreateSite(ctx context.Context, in CreateSiteInput) (twai.Response[genvcs.SiteSerializer], error) {
	body := genvcs.PostSitesJSONRequestBody{Name: in.Name, Project: int(in.ProjectID), Solution: int(in.SolutionID)}
	if in.Desc != "" {
		body.Desc = &in.Desc
	}
	params := &genvcs.PostSitesParams{XApiHost: s.host}
	setString(&params.XExtraPropertyImage, in.Image)
	setString(&params.XExtraPropertyFlavor, in.Flavor)
	setString(&params.XExtraPropertyKeypair, in.Keypair)
	setString(&params.XExtraPropertyPassword, in.Password)
	setString(&params.XExtraPropertyPrivateNetwork, in.PrivateNetwork)
	setString(&params.XExtraPropertyAvailabilityZone, in.AvailabilityZone)

	floating := genvcs.PostSitesParamsXExtraPropertyFloatingIp("nofloating")
	if in.FloatingIP {
		floating = "floating"
	}
	params.XExtraPropertyFloatingIp = &floating

	if in.VolumeSize > 0 {
		params.XExtraPropertyVolumeSize = &in.VolumeSize
	}
	if in.VolumeType != "" {
		v := genvcs.PostSitesParamsXExtraPropertyVolumeType(in.VolumeType)
		params.XExtraPropertyVolumeType = &v
	}
	if in.SystemVolumeSize > 0 {
		params.XExtraPropertySystemVolumeSize = &in.SystemVolumeSize
	}
	if in.SystemVolumeType != "" {
		v := genvcs.PostSitesParamsXExtraPropertySystemVolumeType(in.SystemVolumeType)
		params.XExtraPropertySystemVolumeType = &v
	}

	resp, err := s.api.PostSitesWithResponse(ctx, params, body)
	if err != nil {
		return twai.Response[genvcs.SiteSerializer]{}, fmt.Errorf("建立 VCS 實例失敗: %w", err)
	}
	return twai.Decode[genvcs.SiteSerializer](resp.HTTPResponse, resp.Body)
}

// setString 只在 value 非空時把指標指向它。
func setString(target **string, value string) {
	if value != "" {
		v := value
		*target = &v
	}
}

// DeleteSite 刪除 VCS 實例（204）。
func (s *Service) DeleteSite(ctx context.Context, id int64) error {
	resp, err := s.api.DeleteSitesSiteIdWithResponse(ctx, int(id), &genvcs.DeleteSitesSiteIdParams{XApiHost: s.host})
	if err != nil {
		return fmt.Errorf("刪除 VCS 實例 %d 失敗: %w", id, err)
	}
	return twai.CheckResponse(resp.HTTPResponse, resp.Body)
}

// ListFlavors 列出 flavor；projectID 非 nil 時以 project 過濾。
func (s *Service) ListFlavors(ctx context.Context, projectID *int64) (twai.Response[[]genvcs.FlavorSerializer], error) {
	params := &genvcs.GetFlavorsParams{XApiHost: s.host}
	if projectID != nil {
		p := int32(*projectID)
		params.Project = &p
	}
	resp, err := s.api.GetFlavorsWithResponse(ctx, params)
	if err != nil {
		return twai.Response[[]genvcs.FlavorSerializer]{}, fmt.Errorf("列出 flavor 失敗: %w", err)
	}
	return twai.Decode[[]genvcs.FlavorSerializer](resp.HTTPResponse, resp.Body)
}

// ListImages 列出 project 下的 image（API 要求 project 必填）。
func (s *Service) ListImages(ctx context.Context, projectID int64) (twai.Response[[]genvcs.ImageSerializer], error) {
	resp, err := s.api.GetImagesWithResponse(ctx, &genvcs.GetImagesParams{Project: int(projectID), XApiHost: s.host})
	if err != nil {
		return twai.Response[[]genvcs.ImageSerializer]{}, fmt.Errorf("列出 image 失敗: %w", err)
	}
	return twai.Decode[[]genvcs.ImageSerializer](resp.HTTPResponse, resp.Body)
}

// ListKeypairs 列出金鑰對。
func (s *Service) ListKeypairs(ctx context.Context) (twai.Response[[]genvcs.KeypairSerializer], error) {
	resp, err := s.api.GetKeypairsWithResponse(ctx, &genvcs.GetKeypairsParams{XApiHost: s.host})
	if err != nil {
		return twai.Response[[]genvcs.KeypairSerializer]{}, fmt.Errorf("列出金鑰對失敗: %w", err)
	}
	return twai.Decode[[]genvcs.KeypairSerializer](resp.HTTPResponse, resp.Body)
}

// GetKeypair 取得金鑰對詳細資料（含 public key，不含 private key）。
func (s *Service) GetKeypair(ctx context.Context, name string) (twai.Response[KeypairDetail], error) {
	resp, err := s.api.GetKeypairsKeyNameWithResponse(ctx, name, &genvcs.GetKeypairsKeyNameParams{XApiHost: s.host})
	if err != nil {
		return twai.Response[KeypairDetail]{}, fmt.Errorf("取得金鑰對 %s 失敗: %w", name, err)
	}
	return twai.Decode[KeypairDetail](resp.HTTPResponse, resp.Body)
}

// CreateKeypair 建立（或以 publicKey 匯入）金鑰對。API 回應為純文字（新建時為 private key PEM），原樣回傳。
func (s *Service) CreateKeypair(ctx context.Context, name, publicKey string) (string, error) {
	body := genvcs.PostKeypairsJSONRequestBody{Name: name}
	if publicKey != "" {
		body.PublicKey = &publicKey
	}
	resp, err := s.api.PostKeypairsWithResponse(ctx, &genvcs.PostKeypairsParams{XApiHost: s.host}, body)
	if err != nil {
		return "", fmt.Errorf("建立金鑰對失敗: %w", err)
	}
	if err := twai.CheckResponse(resp.HTTPResponse, resp.Body); err != nil {
		return "", err
	}
	return string(resp.Body), nil
}

// DeleteKeypair 刪除金鑰對（204）。
func (s *Service) DeleteKeypair(ctx context.Context, name string) error {
	resp, err := s.api.DeleteKeypairsKeyNameWithResponse(ctx, name, &genvcs.DeleteKeypairsKeyNameParams{XApiHost: s.host})
	if err != nil {
		return fmt.Errorf("刪除金鑰對 %s 失敗: %w", name, err)
	}
	return twai.CheckResponse(resp.HTTPResponse, resp.Body)
}

// formatID 把數字 id 轉成字串，供需要字串型 id 的生成參數使用（例如 Common 的 project 查詢參數）。
func formatID(id int64) string { return strconv.FormatInt(id, 10) }
