package vcs

import (
	"context"
	"fmt"
	"io"

	genvcs "github.com/gilbertchiao/twaictl/internal/gen/vcs"
	"github.com/gilbertchiao/twaictl/internal/twai"
)

// FirewallRuleProtocols 是 `vcs firewall rule create/update --protocol` 的合法值（spec enum）。
var FirewallRuleProtocols = []string{"icmp", "tcp", "udp"}

// FirewallRuleActions 是 `vcs firewall rule create/update --action` 的合法值（spec enum）。
var FirewallRuleActions = []string{"allow", "deny", "reject"}

// ListFirewallRules 列出 project 下的 firewall rule（spec 描述僅 tenant admin 可用；
// 真實環境 2026-08-28 對非 admin 的 project 回 200 []，未回 403）。
func (s *Service) ListFirewallRules(ctx context.Context, projectID int64) (twai.Response[[]genvcs.FirewallRuleSerializer], error) {
	resp, err := s.api.GetFirewallRulesWithResponse(ctx, &genvcs.GetFirewallRulesParams{Project: int(projectID), XApiHost: s.host})
	if err != nil {
		return twai.Response[[]genvcs.FirewallRuleSerializer]{}, fmt.Errorf("列出 firewall rule 失敗: %w", err)
	}
	return twai.Decode[[]genvcs.FirewallRuleSerializer](resp.HTTPResponse, resp.Body)
}

// GetFirewallRule 取得單一 firewall rule。spec 把 GET /firewall_rules/{id}/ 的 200 回應標成
// 陣列（同 path 的 PATCH 回應卻標單一物件，疑為抄 list 的筆誤，見 docs/api-notes.md A33），
// 真實形狀未經寫入型實測確認，故以 DecodeObjectOrFirst 同時接受單一物件或陣列第一筆。
//
// 這裡刻意不用 GetFirewallRulesFirewallRuleIdWithResponse：生成碼的
// ParseGetFirewallRulesFirewallRuleIdResponse 對 200 + json 一律強制以
// []FirewallRuleDetailSerializer unmarshal，若伺服器實際回單一物件會直接在生成碼
// 內部解析失敗、連原始 body 都拿不到，因此改用底層的 GetFirewallRulesFirewallRuleId
// （回傳原始 *http.Response），自行讀 body 後交給 DecodeObjectOrFirst 判斷形狀
// （同 CreateLoadBalancer 的處理方式）。
func (s *Service) GetFirewallRule(ctx context.Context, id int64) (twai.Response[genvcs.FirewallRuleDetailSerializer], error) {
	httpResp, err := s.api.GetFirewallRulesFirewallRuleId(ctx, int(id), &genvcs.GetFirewallRulesFirewallRuleIdParams{XApiHost: s.host})
	if err != nil {
		return twai.Response[genvcs.FirewallRuleDetailSerializer]{}, fmt.Errorf("取得 firewall rule %d 失敗: %w", id, err)
	}
	defer func() { _ = httpResp.Body.Close() }()
	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return twai.Response[genvcs.FirewallRuleDetailSerializer]{}, fmt.Errorf("讀取取得 firewall rule %d 的回應失敗: %w", id, err)
	}
	return twai.DecodeObjectOrFirst[genvcs.FirewallRuleDetailSerializer](httpResp, respBody, "firewall rule")
}

// FirewallRuleInput 是 `vcs firewall rule create` 的參數；除 ProjectID／Name 外，空字串代表不送該欄位。
type FirewallRuleInput struct {
	ProjectID                                                                    int64
	Name, Protocol, Action, SourceIP, SourcePort, DestinationIP, DestinationPort string
}

// CreateFirewallRule 建立 firewall rule（POST /firewall_rules/；body 依 spec 必帶 project）。
//
// 這裡刻意不用 PostFirewallRulesWithResponse：生成碼的 ParsePostFirewallRulesResponse
// 對 200 + json 一律強制以單一 FirewallRuleSerializer unmarshal，若伺服器實際回陣列
// （同 GetFirewallRule 的 A33 疑慮）會直接在生成碼內部解析失敗、連原始 body 都拿不到——
// 而 create 失敗在使用者眼中看起來像「規則沒建立」，但 API 端很可能已經建好規則，
// 這種情況下遺失原始 body 的代價比 GetFirewallRule 更高。改用底層的
// PostFirewallRules（回傳原始 *http.Response），自行讀 body 後交給
// DecodeObjectOrFirst 判斷形狀（同 GetFirewallRule／UpdateFirewallRule 的處理方式）。
func (s *Service) CreateFirewallRule(ctx context.Context, in FirewallRuleInput) (twai.Response[genvcs.FirewallRuleSerializer], error) {
	if err := int32InRange(in.ProjectID); err != nil {
		return twai.Response[genvcs.FirewallRuleSerializer]{}, err
	}
	body := genvcs.PostFirewallRulesJSONRequestBody{Name: in.Name, Project: int32(in.ProjectID)}
	if in.Protocol != "" {
		protocol := genvcs.PostFirewallRulesJSONBodyProtocol(in.Protocol)
		body.Protocol = &protocol
	}
	if in.Action != "" {
		action := genvcs.PostFirewallRulesJSONBodyAction(in.Action)
		body.Action = &action
	}
	setString(&body.SourceIpAddress, in.SourceIP)
	setString(&body.SourcePort, in.SourcePort)
	setString(&body.DestinationIpAddress, in.DestinationIP)
	setString(&body.DestinationPort, in.DestinationPort)
	httpResp, err := s.api.PostFirewallRules(ctx, &genvcs.PostFirewallRulesParams{XApiHost: s.host}, body)
	if err != nil {
		return twai.Response[genvcs.FirewallRuleSerializer]{}, fmt.Errorf("建立 firewall rule 失敗: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()
	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return twai.Response[genvcs.FirewallRuleSerializer]{}, fmt.Errorf("讀取建立 firewall rule 的回應失敗: %w", err)
	}
	return twai.DecodeObjectOrFirst[genvcs.FirewallRuleSerializer](httpResp, respBody, "建立 firewall rule")
}

// UpdateFirewallRuleInput 是 `vcs firewall rule update` 的參數：nil 代表不送該欄位（維持原值），
// 非 nil 的空字串會原樣送出（用於清除 IP／port 限制）。
type UpdateFirewallRuleInput struct {
	Name, Protocol, Action, SourceIP, SourcePort, DestinationIP, DestinationPort *string
}

// UpdateFirewallRule 更新 firewall rule（PATCH /firewall_rules/{id}/），回應同 GetFirewallRule
// 寬鬆解析（原因同上：生成碼的 ParsePatchFirewallRulesFirewallRuleIdResponse 對 200 + json
// 一律強制以單一 FirewallRuleDetailSerializer unmarshal，若伺服器實際回陣列會解析失敗，
// 因此同樣改用底層的 PatchFirewallRulesFirewallRuleId 自行讀 body）。
func (s *Service) UpdateFirewallRule(ctx context.Context, id int64, in UpdateFirewallRuleInput) (twai.Response[genvcs.FirewallRuleDetailSerializer], error) {
	body := genvcs.PatchFirewallRulesFirewallRuleIdJSONRequestBody{
		Name: in.Name, SourceIpAddress: in.SourceIP, SourcePort: in.SourcePort,
		DestinationIpAddress: in.DestinationIP, DestinationPort: in.DestinationPort,
	}
	if in.Protocol != nil {
		protocol := genvcs.PatchFirewallRulesFirewallRuleIdJSONBodyProtocol(*in.Protocol)
		body.Protocol = &protocol
	}
	if in.Action != nil {
		action := genvcs.PatchFirewallRulesFirewallRuleIdJSONBodyAction(*in.Action)
		body.Action = &action
	}
	httpResp, err := s.api.PatchFirewallRulesFirewallRuleId(ctx, int(id), &genvcs.PatchFirewallRulesFirewallRuleIdParams{XApiHost: s.host}, body)
	if err != nil {
		return twai.Response[genvcs.FirewallRuleDetailSerializer]{}, fmt.Errorf("更新 firewall rule %d 失敗: %w", id, err)
	}
	defer func() { _ = httpResp.Body.Close() }()
	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return twai.Response[genvcs.FirewallRuleDetailSerializer]{}, fmt.Errorf("讀取更新 firewall rule %d 的回應失敗: %w", id, err)
	}
	return twai.DecodeObjectOrFirst[genvcs.FirewallRuleDetailSerializer](httpResp, respBody, "firewall rule")
}

// DeleteFirewallRule 刪除 firewall rule（成功時 204 無 body）。
func (s *Service) DeleteFirewallRule(ctx context.Context, id int64) error {
	resp, err := s.api.DeleteFirewallRulesFirewallRuleIdWithResponse(ctx, int(id), &genvcs.DeleteFirewallRulesFirewallRuleIdParams{XApiHost: s.host})
	if err != nil {
		return fmt.Errorf("刪除 firewall rule %d 失敗: %w", id, err)
	}
	return twai.CheckResponse(resp.HTTPResponse, resp.Body)
}

// ListFirewalls 列出 project 下的 firewall（spec 描述僅 tenant admin 可用）。
func (s *Service) ListFirewalls(ctx context.Context, projectID int64) (twai.Response[[]genvcs.FirewallSerializer], error) {
	resp, err := s.api.GetFirewallsWithResponse(ctx, &genvcs.GetFirewallsParams{Project: int(projectID), XApiHost: s.host})
	if err != nil {
		return twai.Response[[]genvcs.FirewallSerializer]{}, fmt.Errorf("列出 firewall 失敗: %w", err)
	}
	return twai.Decode[[]genvcs.FirewallSerializer](resp.HTTPResponse, resp.Body)
}

// GetFirewall 取得單一 firewall 的詳細資料（含 rules 與 associate_networks）。
//
// 這裡刻意不用 GetFirewallsFirewallIdWithResponse：同 GetFirewallRule 的理由（見上），
// 生成碼的 ParseGetFirewallsFirewallIdResponse 對 200 + json 一律強制以
// FirewallDetailSerializer unmarshal，若伺服器實際回陣列會直接在生成碼內部解析失敗、
// 連原始 body 都拿不到，因此改用底層的 GetFirewallsFirewallId（回傳原始 *http.Response），
// 自行讀 body 後交給 DecodeObjectOrFirst 判斷形狀。
func (s *Service) GetFirewall(ctx context.Context, id int64) (twai.Response[genvcs.FirewallDetailSerializer], error) {
	httpResp, err := s.api.GetFirewallsFirewallId(ctx, int(id), &genvcs.GetFirewallsFirewallIdParams{XApiHost: s.host})
	if err != nil {
		return twai.Response[genvcs.FirewallDetailSerializer]{}, fmt.Errorf("取得 firewall %d 失敗: %w", id, err)
	}
	defer func() { _ = httpResp.Body.Close() }()
	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return twai.Response[genvcs.FirewallDetailSerializer]{}, fmt.Errorf("讀取取得 firewall %d 的回應失敗: %w", id, err)
	}
	return twai.DecodeObjectOrFirst[genvcs.FirewallDetailSerializer](httpResp, respBody, "firewall")
}

// int32IDs 把 id 清單轉成 API 要的 int32 slice；任一 id 超出 int32 範圍即回錯誤。
// 對空 slice 回 []int32{}（非 nil），確保 json.Marshal 產生 [] 而非 null。
func int32IDs(ids []int64) ([]int32, error) {
	out := make([]int32, 0, len(ids))
	for _, id := range ids {
		if err := int32InRange(id); err != nil {
			return nil, err
		}
		out = append(out, int32(id))
	}
	return out, nil
}

// CreateFirewallInput 是 `vcs firewall create` 的參數；RuleIDs／NetworkIDs 為空時不送對應欄位。
type CreateFirewallInput struct {
	ProjectID           int64
	Name, Desc          string
	RuleIDs, NetworkIDs []int64
}

// CreateFirewall 建立 firewall（POST /firewalls/）。
//
// 這裡刻意不用 PostFirewallsWithResponse：同 CreateFirewallRule 的理由（見上），生成碼的
// ParsePostFirewallsResponse 對 200 + json 一律強制以單一 FirewallSerializer unmarshal，
// 若伺服器實際回陣列（LB 的 POST 先例就是 spec 標 array、真實回 object，反過來的情況
// 同樣可能發生）會直接在生成碼內部解析失敗、連原始 body 都拿不到——而 create 失敗在
// 使用者眼中看起來像「沒建立」，但 API 端很可能已經建好，遺失原始 body 的代價更高。
// 改用底層的 PostFirewalls（回傳原始 *http.Response），自行讀 body 後交給
// DecodeObjectOrFirst 判斷形狀。
func (s *Service) CreateFirewall(ctx context.Context, in CreateFirewallInput) (twai.Response[genvcs.FirewallSerializer], error) {
	if err := int32InRange(in.ProjectID); err != nil {
		return twai.Response[genvcs.FirewallSerializer]{}, err
	}
	project := int32(in.ProjectID)
	body := genvcs.PostFirewallsJSONRequestBody{Name: &in.Name, Project: &project}
	setString(&body.Desc, in.Desc)
	if len(in.RuleIDs) > 0 {
		rules, err := int32IDs(in.RuleIDs)
		if err != nil {
			return twai.Response[genvcs.FirewallSerializer]{}, fmt.Errorf("rule id: %w", err)
		}
		body.Rules = &rules
	}
	if len(in.NetworkIDs) > 0 {
		networks, err := int32IDs(in.NetworkIDs)
		if err != nil {
			return twai.Response[genvcs.FirewallSerializer]{}, fmt.Errorf("network id: %w", err)
		}
		body.AssociateNetworks = &networks
	}
	httpResp, err := s.api.PostFirewalls(ctx, &genvcs.PostFirewallsParams{XApiHost: s.host}, body)
	if err != nil {
		return twai.Response[genvcs.FirewallSerializer]{}, fmt.Errorf("建立 firewall 失敗: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()
	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return twai.Response[genvcs.FirewallSerializer]{}, fmt.Errorf("讀取建立 firewall 的回應失敗: %w", err)
	}
	return twai.DecodeObjectOrFirst[genvcs.FirewallSerializer](httpResp, respBody, "建立 firewall")
}

// UpdateFirewallInput 是 `vcs firewall update` 的參數：nil 代表不送該欄位；
// RuleIDs／NetworkIDs 指向空 slice 時送 []（清空清單，PATCH 為整份取代語意）。
type UpdateFirewallInput struct {
	Desc                *string
	RuleIDs, NetworkIDs *[]int64
}

// UpdateFirewall 更新 firewall（PATCH /firewalls/{id}/，rules／associate_networks 為整份取代）。
//
// 這裡刻意不用 PatchFirewallsFirewallIdWithResponse：理由同 CreateFirewall，改用底層的
// PatchFirewallsFirewallId 自行讀 body 後交給 DecodeObjectOrFirst 判斷形狀。
func (s *Service) UpdateFirewall(ctx context.Context, id int64, in UpdateFirewallInput) (twai.Response[genvcs.FirewallDetailSerializer], error) {
	body := genvcs.PatchFirewallsFirewallIdJSONRequestBody{Desc: in.Desc}
	if in.RuleIDs != nil {
		rules, err := int32IDs(*in.RuleIDs)
		if err != nil {
			return twai.Response[genvcs.FirewallDetailSerializer]{}, fmt.Errorf("rule id: %w", err)
		}
		body.Rules = &rules
	}
	if in.NetworkIDs != nil {
		networks, err := int32IDs(*in.NetworkIDs)
		if err != nil {
			return twai.Response[genvcs.FirewallDetailSerializer]{}, fmt.Errorf("network id: %w", err)
		}
		body.AssociateNetworks = &networks
	}
	httpResp, err := s.api.PatchFirewallsFirewallId(ctx, int(id), &genvcs.PatchFirewallsFirewallIdParams{XApiHost: s.host}, body)
	if err != nil {
		return twai.Response[genvcs.FirewallDetailSerializer]{}, fmt.Errorf("更新 firewall %d 失敗: %w", id, err)
	}
	defer func() { _ = httpResp.Body.Close() }()
	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return twai.Response[genvcs.FirewallDetailSerializer]{}, fmt.Errorf("讀取更新 firewall %d 的回應失敗: %w", id, err)
	}
	return twai.DecodeObjectOrFirst[genvcs.FirewallDetailSerializer](httpResp, respBody, "更新 firewall")
}

// DeleteFirewall 刪除 firewall（成功時 204 無 body）。
func (s *Service) DeleteFirewall(ctx context.Context, id int64) error {
	resp, err := s.api.DeleteFirewallsFirewallIdWithResponse(ctx, int(id), &genvcs.DeleteFirewallsFirewallIdParams{XApiHost: s.host})
	if err != nil {
		return fmt.Errorf("刪除 firewall %d 失敗: %w", id, err)
	}
	return twai.CheckResponse(resp.HTTPResponse, resp.Body)
}
