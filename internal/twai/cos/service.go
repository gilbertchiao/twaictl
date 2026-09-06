package cos

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gilbertchiao/twaictl/internal/gen/ceph"
	"github.com/gilbertchiao/twaictl/internal/twai"
)

// Service 包裝 Ceph 生成 client：project 解析與 S3 金鑰管理（cos key get/create/renew/rm）。
type Service struct {
	api  *ceph.ClientWithResponses
	host string // Ceph 的 x-api-host
}

// New 由 twai.Client 建立 Service。
func New(client *twai.Client) *Service {
	return &Service{api: client.Ceph, host: client.Settings.APIHosts.COS}
}

// Key 是單一 S3 金鑰（access/secret）。
type Key struct {
	Name      string
	AccessKey string
	SecretKey string
}

// Keys 是 Ceph project 底下的 public 與 private 金鑰集合。
type Keys struct {
	Public  *Key
	Private []Key
}

// ResolveProjectID 把 project code（例如 ENT1）或數字 id 解析為 Ceph 的數字 project id。
//
// 注意：Ceph 的 project id 與 VCS 的 project id 是各自獨立的編號空間（即使 name 相同），
// 因此必須各自向 Ceph 的 `GET /projects/?name=` 查詢，不能沿用 VCS 已解析出的 id。
func (s *Service) ResolveProjectID(ctx context.Context, ref string) (int64, error) {
	if twai.IsNumericRef(ref) {
		return twai.ResolveRef[ceph.GroupSerializer]("project", ref, nil, nil, nil)
	}
	resp, err := s.api.GetProjectsWithResponse(ctx, &ceph.GetProjectsParams{Name: &ref, XApiHost: s.host})
	if err != nil {
		return 0, fmt.Errorf("列出 Ceph project 失敗: %w", err)
	}
	res, err := twai.Decode[[]ceph.GroupSerializer](resp.HTTPResponse, resp.Body)
	if err != nil {
		return 0, err
	}
	return twai.ResolveRef("project", ref, res.Value,
		func(p ceph.GroupSerializer) string { return p.Name },
		func(p ceph.GroupSerializer) int64 { return p.Id })
}

// convertKey 把生成碼的 KeySerializer（欄位皆為指標）轉成值型別的 Key。
func convertKey(in ceph.KeySerializer) Key {
	var k Key
	if in.Name != nil {
		k.Name = *in.Name
	}
	if in.AccessKey != nil {
		k.AccessKey = *in.AccessKey
	}
	if in.SecretKey != nil {
		k.SecretKey = *in.SecretKey
	}
	return k
}

// convertKeys 把生成碼的 ProjectKeySerializer 轉成 Keys。
func convertKeys(in ceph.ProjectKeySerializer) Keys {
	var out Keys
	if in.Public != nil {
		public := convertKey(*in.Public)
		out.Public = &public
	}
	if in.Private != nil {
		out.Private = make([]Key, 0, len(*in.Private))
		for _, k := range *in.Private {
			out.Private = append(out.Private, convertKey(k))
		}
	}
	return out
}

// decodeKeys 是 GetKeys/CreateKey/RenewKey/DeleteKey 共用的收尾：
// 先以 twai.Decode 解析成 ceph.ProjectKeySerializer，再轉換成 Keys（Raw 保留原始 body 不變）。
func decodeKeys(resp *http.Response, body []byte) (twai.Response[Keys], error) {
	raw, err := twai.Decode[ceph.ProjectKeySerializer](resp, body)
	if err != nil {
		return twai.Response[Keys]{}, err
	}
	// 2xx 但 body 不含任何金鑰欄位（例如 gateway 對未知路徑回了 200 + 別的 JSON），
	// 不能靜默回傳空 Keys 讓 cli 印出全空的表格。
	if raw.Value.Public == nil && raw.Value.Private == nil {
		return twai.Response[Keys]{}, fmt.Errorf("API 回應不含金鑰資料（public / private 皆缺）: %s", twai.Truncate(body, 200))
	}
	return twai.Response[Keys]{Value: convertKeys(raw.Value), Raw: raw.Raw}, nil
}

// GetKeys 取得 project 底下的 public/private S3 金鑰。
func (s *Service) GetKeys(ctx context.Context, projectID int64) (twai.Response[Keys], error) {
	resp, err := s.api.GetProjectsProjectIdKeyWithResponse(ctx, int(projectID), &ceph.GetProjectsProjectIdKeyParams{XApiHost: s.host})
	if err != nil {
		return twai.Response[Keys]{}, fmt.Errorf("取得 project %d 的金鑰失敗: %w", projectID, err)
	}
	return decodeKeys(resp.HTTPResponse, resp.Body)
}

// CreateKey 建立新的 private S3 金鑰。
func (s *Service) CreateKey(ctx context.Context, projectID int64, name string) (twai.Response[Keys], error) {
	body := ceph.PostProjectsProjectIdKeyJSONRequestBody{Name: &name}
	resp, err := s.api.PostProjectsProjectIdKeyWithResponse(ctx, int(projectID), &ceph.PostProjectsProjectIdKeyParams{XApiHost: s.host}, body)
	if err != nil {
		return twai.Response[Keys]{}, fmt.Errorf("建立 project %d 的金鑰失敗: %w", projectID, err)
	}
	return decodeKeys(resp.HTTPResponse, resp.Body)
}

// RenewKey 更新（輪替）金鑰：all 為 true 時輪替所有 private 金鑰，name 非空時只輪替該金鑰；
// name 為空字串時不送出 name 欄位（依 spec，PUT body 的 name 可省略，交由 all 決定範圍）。
func (s *Service) RenewKey(ctx context.Context, projectID int64, name string, all bool) (twai.Response[Keys], error) {
	body := ceph.PutProjectsProjectIdKeyJSONRequestBody{}
	if name != "" {
		body.Name = &name
	}
	if all {
		body.All = &all
	}
	resp, err := s.api.PutProjectsProjectIdKeyWithResponse(ctx, int(projectID), &ceph.PutProjectsProjectIdKeyParams{XApiHost: s.host}, body)
	if err != nil {
		return twai.Response[Keys]{}, fmt.Errorf("更新 project %d 的金鑰失敗: %w", projectID, err)
	}
	return decodeKeys(resp.HTTPResponse, resp.Body)
}

// DeleteKey 刪除指定名稱的 private 金鑰。
func (s *Service) DeleteKey(ctx context.Context, projectID int64, name string) (twai.Response[Keys], error) {
	body := ceph.DeleteProjectsProjectIdKeyJSONRequestBody{Name: &name}
	resp, err := s.api.DeleteProjectsProjectIdKeyWithResponse(ctx, int(projectID), &ceph.DeleteProjectsProjectIdKeyParams{XApiHost: s.host}, body)
	if err != nil {
		return twai.Response[Keys]{}, fmt.Errorf("刪除 project %d 的金鑰 %s 失敗: %w", projectID, name, err)
	}
	return decodeKeys(resp.HTTPResponse, resp.Body)
}

// Usage 是 /buckets_util/ 回傳的總用量：某把金鑰（public 或指定的 private）底下所有 bucket 合計。
type Usage struct {
	KeyName        string
	TotalUsedSpace int64 // 單位為 bytes（2026-08-27 已實測確認，見 docs/api-notes.md A16 與「Phase 2a 實測」）
	TotalObjects   int64
}

// BucketUsage 是 /buckets/ 回傳的單一 bucket 用量。
type BucketUsage struct {
	Name         string
	LastModified string // API 原始字串，顯示時交由 output.FormatTimeString
	UsedSpace    int64
	NumObjects   int64
}

// GetUsage 取得金鑰底下所有 bucket 的總用量；keyName 為空時不送 key_name（API 預設回 public 金鑰）。
// 注意：spec 的 `all=true` 在真實環境會 503 逾時（api-notes A17），因此不提供。
func (s *Service) GetUsage(ctx context.Context, projectID int64, keyName string) (twai.Response[Usage], error) {
	params := &ceph.GetProjectsProjectIdBucketsUtilParams{XApiHost: s.host}
	if keyName != "" {
		params.KeyName = &keyName
	}
	resp, err := s.api.GetProjectsProjectIdBucketsUtilWithResponse(ctx, int(projectID), params)
	if err != nil {
		return twai.Response[Usage]{}, fmt.Errorf("取得 project %d 的 COS 用量失敗: %w", projectID, err)
	}
	raw, err := twai.Decode[ceph.ProjectBucketUtilSerializer](resp.HTTPResponse, resp.Body)
	if err != nil {
		return twai.Response[Usage]{}, err
	}
	// json.Unmarshal 對缺失欄位只給零值、不會報錯：gateway 對未知路徑回 200 + 別的 JSON 時，
	// 若不檢查，total_used_space/total_objects 會靜默變成 0，讓 cos usage 顯示錯誤的「0 用量」。
	if err := twai.RequireJSONFields(raw.Raw, "total_used_space", "total_objects"); err != nil {
		return twai.Response[Usage]{}, fmt.Errorf("取得 project %d 的 COS 用量失敗: %w", projectID, err)
	}
	usage := Usage{TotalUsedSpace: raw.Value.TotalUsedSpace, TotalObjects: raw.Value.TotalObjects}
	if raw.Value.KeyName != nil {
		usage.KeyName = *raw.Value.KeyName
	}
	return twai.Response[Usage]{Value: usage, Raw: raw.Raw}, nil
}

// ListBucketUsage 列出金鑰底下每個 bucket 的用量；keyName 語意同 GetUsage。
func (s *Service) ListBucketUsage(ctx context.Context, projectID int64, keyName string) (twai.Response[[]BucketUsage], error) {
	params := &ceph.GetProjectsProjectIdBucketsParams{XApiHost: s.host}
	if keyName != "" {
		params.KeyName = &keyName
	}
	resp, err := s.api.GetProjectsProjectIdBucketsWithResponse(ctx, int(projectID), params)
	if err != nil {
		return twai.Response[[]BucketUsage]{}, fmt.Errorf("列出 project %d 的 bucket 用量失敗: %w", projectID, err)
	}
	raw, err := twai.Decode[ceph.ProjectBucketSerializer](resp.HTTPResponse, resp.Body)
	if err != nil {
		return twai.Response[[]BucketUsage]{}, err
	}
	// buckets 缺失時 json.Unmarshal 只給 nil slice、不會報錯：需與「buckets 存在但為空陣列
	// （真的沒有 bucket）」區分，否則 gateway／路由錯誤會被誤讀成「這個 project 沒有 bucket」。
	if err := twai.RequireJSONFields(raw.Raw, "buckets"); err != nil {
		return twai.Response[[]BucketUsage]{}, fmt.Errorf("列出 project %d 的 bucket 用量失敗: %w", projectID, err)
	}
	buckets := make([]BucketUsage, 0, len(raw.Value.Buckets))
	for _, b := range raw.Value.Buckets {
		buckets = append(buckets, BucketUsage{
			Name: b.BucketName, LastModified: b.LastModifiedTime,
			UsedSpace: int64(b.BucketUsedSpace), NumObjects: int64(b.NumObjects),
		})
	}
	return twai.Response[[]BucketUsage]{Value: buckets, Raw: raw.Raw}, nil
}
