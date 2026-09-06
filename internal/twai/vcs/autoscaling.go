package vcs

import (
	"context"
	"fmt"
	"io"
	"strings"

	genvcs "github.com/gilbertchiao/twaictl/internal/gen/vcs"
	"github.com/gilbertchiao/twaictl/internal/twai"
)

// ASPMeterNames 是 `vcs asp create --meter` 短名（與 `vcs metrics --meter` 的 Meters 相同）
// 到 API meter_name（spec enum）的對照。
var ASPMeterNames = map[string]string{
	"cpu":        "cpu_util",
	"memory":     "memory.usage",
	"disk-read":  "disk.read.bytes.rate",
	"disk-write": "disk.write.bytes.rate",
	"net-in":     "network.incoming.bytes.rate",
	"net-out":    "network.outgoing.bytes.rate",
}

// ASPMeterName 把 --meter 的值正規化成 API 的 meter_name：接受短名（cpu、memory…）或
// API 全名（cpu_util、memory.usage…），其他值回錯誤並列出合法短名。
func ASPMeterName(meter string) (string, error) {
	if full, ok := ASPMeterNames[meter]; ok {
		return full, nil
	}
	for _, full := range ASPMeterNames {
		if meter == full {
			return full, nil
		}
	}
	return "", fmt.Errorf("--meter 的值必須是下列其中之一：%s（或對應的 API 名稱，目前為 %q）", strings.Join(Meters, "|"), meter)
}

// ListAutoScalingPolicies 列出 project 下的 auto scaling policy。
func (s *Service) ListAutoScalingPolicies(ctx context.Context, projectID int64) (twai.Response[[]genvcs.AutoScalingPolicySerializer], error) {
	resp, err := s.api.GetAutoScalingPoliciesWithResponse(ctx, &genvcs.GetAutoScalingPoliciesParams{Project: int(projectID), XApiHost: s.host})
	if err != nil {
		return twai.Response[[]genvcs.AutoScalingPolicySerializer]{}, fmt.Errorf("列出 auto scaling policy 失敗: %w", err)
	}
	return twai.Decode[[]genvcs.AutoScalingPolicySerializer](resp.HTTPResponse, resp.Body)
}

// GetAutoScalingPolicy 取得單一 auto scaling policy（路徑已由 patch 補上尾斜線，見 docs/api-notes.md A27）。
//
// 這裡刻意不用 GetAutoScalingPoliciesAutoScalingPolicyIdWithResponse：同 GetFirewall 的理由
// （見 firewalls.go），生成碼的 ParseGetAutoScalingPoliciesAutoScalingPolicyIdResponse 對
// 200 + json 一律強制以單一 AutoScalingPolicySerializer unmarshal，若伺服器實際回陣列
// （spec 標物件、真實回陣列的先例已在 LB／firewall 出現過）會直接在生成碼內部解析失敗、
// 連原始 body 都拿不到，因此改用底層的 GetAutoScalingPoliciesAutoScalingPolicyId
// （回傳原始 *http.Response），自行讀 body 後交給 DecodeObjectOrFirst 判斷形狀。
func (s *Service) GetAutoScalingPolicy(ctx context.Context, id int64) (twai.Response[genvcs.AutoScalingPolicySerializer], error) {
	if err := int32InRange(id); err != nil {
		return twai.Response[genvcs.AutoScalingPolicySerializer]{}, err
	}
	httpResp, err := s.api.GetAutoScalingPoliciesAutoScalingPolicyId(ctx, int32(id), &genvcs.GetAutoScalingPoliciesAutoScalingPolicyIdParams{XApiHost: s.host})
	if err != nil {
		return twai.Response[genvcs.AutoScalingPolicySerializer]{}, fmt.Errorf("取得 auto scaling policy %d 失敗: %w", id, err)
	}
	defer func() { _ = httpResp.Body.Close() }()
	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return twai.Response[genvcs.AutoScalingPolicySerializer]{}, fmt.Errorf("讀取取得 auto scaling policy %d 的回應失敗: %w", id, err)
	}
	return twai.DecodeObjectOrFirst[genvcs.AutoScalingPolicySerializer](httpResp, respBody, "auto scaling policy")
}

// CreateAutoScalingPolicyInput 是 `vcs asp create` 的參數；MeterName 須已是 API 全名
// （由 ASPMeterName 正規化）；ScaleDownThreshold 為 nil 時不送；Desc 空字串不送。
// 注意 Desc 這個 Go 欄位名對應的 request JSON key 是 description（spec 原標 desc，
// 實測必須是 description 才會生效，已以 patch 修正，見 CreateAutoScalingPolicy 註解）。
type CreateAutoScalingPolicyInput struct {
	ProjectID                      int64
	Name, Desc, MeterName          string
	ScaleUpThreshold, ScaleMaxSize int
	ScaleDownThreshold             *int
}

// CreateAutoScalingPolicy 建立 auto scaling policy（POST /auto_scaling_policies/）。
// 注意 spec 的 request 描述欄位原標 desc，但真實環境（2026-08-29 寫入型實測）帶 desc
// 會被伺服器靜默忽略（成功建立但回應 description 為空字串），必須改帶 description
// 才會生效；已以 api/openapi/patches/VCS-asp-create-description.patch 修正 spec，
// 回應（AutoScalingPolicySerializer）也是用 description，見 docs/api-notes.md A34。
//
// 這裡刻意不用 PostAutoScalingPoliciesWithResponse：同 CreateFirewall 的理由（見
// firewalls.go），生成碼的 ParsePostAutoScalingPoliciesResponse 對 200 + json 一律強制以
// 單一 AutoScalingPolicySerializer unmarshal，若伺服器實際回陣列會直接在生成碼內部解析
// 失敗、連原始 body 都拿不到——而 create 失敗在使用者眼中看起來像「沒建立」，但 API
// 端很可能已經建好，遺失原始 body 的代價更高。改用底層的 PostAutoScalingPolicies
// （回傳原始 *http.Response），自行讀 body 後交給 DecodeObjectOrFirst 判斷形狀。
func (s *Service) CreateAutoScalingPolicy(ctx context.Context, in CreateAutoScalingPolicyInput) (twai.Response[genvcs.AutoScalingPolicySerializer], error) {
	if err := int32InRange(in.ProjectID); err != nil {
		return twai.Response[genvcs.AutoScalingPolicySerializer]{}, err
	}
	body := genvcs.PostAutoScalingPoliciesJSONRequestBody{
		Name:               in.Name,
		Project:            int(in.ProjectID),
		MeterName:          genvcs.PostAutoScalingPoliciesJSONBodyMeterName(in.MeterName),
		ScaleupThreshold:   in.ScaleUpThreshold,
		ScaledownThreshold: in.ScaleDownThreshold,
		ScaleMaxSize:       in.ScaleMaxSize,
	}
	setString(&body.Description, in.Desc)
	httpResp, err := s.api.PostAutoScalingPolicies(ctx, &genvcs.PostAutoScalingPoliciesParams{XApiHost: s.host}, body)
	if err != nil {
		return twai.Response[genvcs.AutoScalingPolicySerializer]{}, fmt.Errorf("建立 auto scaling policy 失敗: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()
	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return twai.Response[genvcs.AutoScalingPolicySerializer]{}, fmt.Errorf("讀取建立 auto scaling policy 的回應失敗: %w", err)
	}
	return twai.DecodeObjectOrFirst[genvcs.AutoScalingPolicySerializer](httpResp, respBody, "建立 auto scaling policy")
}

// DeleteAutoScalingPolicy 刪除 auto scaling policy（成功時 204 無 body）。
func (s *Service) DeleteAutoScalingPolicy(ctx context.Context, id int64) error {
	if err := int32InRange(id); err != nil {
		return err
	}
	resp, err := s.api.DeleteAutoScalingPoliciesAutoScalingPolicyIdWithResponse(ctx, int32(id), &genvcs.DeleteAutoScalingPoliciesAutoScalingPolicyIdParams{XApiHost: s.host})
	if err != nil {
		return fmt.Errorf("刪除 auto scaling policy %d 失敗: %w", id, err)
	}
	return twai.CheckResponse(resp.HTTPResponse, resp.Body)
}

// AutoScalingPolicyAttachment 是 POST /servers/{server_id}/auto_scaling_policy/ 的 201 回應
// （spec 為匿名 object {auto_scaling_policy, loadbalancer}，手寫具名型別以便輸出）。
type AutoScalingPolicyAttachment struct {
	AutoScalingPolicy int64  `json:"auto_scaling_policy"`
	LoadBalancer      *int64 `json:"loadbalancer,omitempty"`
}

// AttachAutoScalingPolicyInput 是 `vcs asp attach` 的參數；LoadBalancerID／ProtocolPort 為 0、
// ScaleUpAction／ScaleDownAction 為空字串時不送對應欄位。
type AttachAutoScalingPolicyInput struct {
	PolicyID, ServerID, LoadBalancerID int64
	ProtocolPort                       int
	ScaleUpAction, ScaleDownAction     string
}

// AttachAutoScalingPolicy 把 auto scaling policy 掛到 server（POST /servers/{id}/auto_scaling_policy/，201）。
// 指定 LoadBalancerID 時，擴出的 server 會自動加入該 load balancer（原 server 除外，見 spec 描述）。
//
// 這裡刻意不用 PostServersServerIdAutoScalingPolicyWithResponse：同 CreateAutoScalingPolicy 的
// 理由（見上），生成碼的 ParsePostServersServerIdAutoScalingPolicyResponse 對 201 + json 一律
// 強制以單一匿名 struct unmarshal，若伺服器實際回陣列會直接在生成碼內部解析失敗、連原始 body
// 都拿不到——而 attach 失敗在使用者眼中看起來像「沒掛上」，但 API 端很可能已經掛好，遺失原始
// body 的代價更高。改用底層的 PostServersServerIdAutoScalingPolicy（回傳原始 *http.Response），
// 自行讀 body 後交給 DecodeObjectOrFirst 判斷形狀。
func (s *Service) AttachAutoScalingPolicy(ctx context.Context, in AttachAutoScalingPolicyInput) (twai.Response[AutoScalingPolicyAttachment], error) {
	// 只對 PolicyID 做 int32 範圍檢查：它對應 API 的 int32 auto_scaling_policy 欄位
	// （下面 int32(in.PolicyID) 的轉型）。ServerID 對應生成碼的 ServerIDParam
	// （= int，不是 int32，見 vcs.gen.go 的 PostServersServerIdAutoScalingPolicy 簽名），
	// 呼叫端 parseServerFlag／ResolveServerIDForSite 已保證是正整數，這裡不需要也不應該
	// 再用 int32InRange 檢查，否則會誤擋合法但超過 int32 上限的 server id。
	if err := int32InRange(in.PolicyID); err != nil {
		return twai.Response[AutoScalingPolicyAttachment]{}, err
	}
	body := genvcs.PostServersServerIdAutoScalingPolicyJSONRequestBody{AutoScalingPolicy: int32(in.PolicyID)}
	if in.LoadBalancerID != 0 {
		if err := int32InRange(in.LoadBalancerID); err != nil {
			return twai.Response[AutoScalingPolicyAttachment]{}, err
		}
		lb := int32(in.LoadBalancerID)
		body.Loadbalancer = &lb
	}
	if in.ProtocolPort != 0 {
		// ProtocolPort 是 int，轉成 API 的 int32 之前先確認落在合法 TCP/UDP port 範圍，
		// 避免超出範圍的值在轉型時被靜默截斷（CLI 層雖然已有 --port 檢查，但 service 層
		// 是這個型別轉換實際發生的地方，不能只依賴呼叫端）。
		if err := ValidatePort("protocol port", int64(in.ProtocolPort)); err != nil {
			return twai.Response[AutoScalingPolicyAttachment]{}, err
		}
		port := int32(in.ProtocolPort)
		body.ProtocolPort = &port
	}
	setString(&body.ScaleupAction, in.ScaleUpAction)
	setString(&body.ScaledownAction, in.ScaleDownAction)
	httpResp, err := s.api.PostServersServerIdAutoScalingPolicy(ctx, int(in.ServerID), &genvcs.PostServersServerIdAutoScalingPolicyParams{XApiHost: s.host}, body)
	if err != nil {
		return twai.Response[AutoScalingPolicyAttachment]{}, fmt.Errorf("將 auto scaling policy %d 掛到 server %d 失敗: %w", in.PolicyID, in.ServerID, err)
	}
	defer func() { _ = httpResp.Body.Close() }()
	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return twai.Response[AutoScalingPolicyAttachment]{}, fmt.Errorf("讀取將 auto scaling policy %d 掛到 server %d 的回應失敗: %w", in.PolicyID, in.ServerID, err)
	}
	return twai.DecodeObjectOrFirst[AutoScalingPolicyAttachment](httpResp, respBody, "掛載 auto scaling policy")
}

// DetachAutoScalingPolicy 移除 server 上的 auto scaling policy（DELETE，成功時 204 無 body）。
func (s *Service) DetachAutoScalingPolicy(ctx context.Context, serverID int64) error {
	resp, err := s.api.DeleteServersServerIdAutoScalingPolicyWithResponse(ctx, int(serverID), &genvcs.DeleteServersServerIdAutoScalingPolicyParams{XApiHost: s.host})
	if err != nil {
		return fmt.Errorf("移除 server %d 的 auto scaling policy 失敗: %w", serverID, err)
	}
	return twai.CheckResponse(resp.HTTPResponse, resp.Body)
}
