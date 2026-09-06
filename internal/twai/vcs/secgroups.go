package vcs

import (
	"context"
	"fmt"

	genvcs "github.com/gilbertchiao/twaictl/internal/gen/vcs"
	"github.com/gilbertchiao/twaictl/internal/twai"
)

// SecurityRuleDirections 是 `vcs secg add-rule --direction` 的合法值。
var SecurityRuleDirections = []string{"ingress", "egress"}

// SecurityRuleProtocols 是 `vcs secg add-rule --protocol` 的合法值。
var SecurityRuleProtocols = []string{"tcp", "udp", "icmp", "udplite", "sctp", "dccp"}

// ListSecurityGroups 列出 project（可選以 server 篩選）底下的 security group 清單；
// serverID 為 0 代表不篩選（`GetSecurityGroupsParams.Server` 選填）。
func (s *Service) ListSecurityGroups(ctx context.Context, projectID, serverID int64) (twai.Response[[]genvcs.SecurityGroupSerializer], error) {
	params := &genvcs.GetSecurityGroupsParams{Project: int(projectID), XApiHost: s.host}
	if serverID != 0 {
		server := int(serverID)
		params.Server = &server
	}
	resp, err := s.api.GetSecurityGroupsWithResponse(ctx, params)
	if err != nil {
		return twai.Response[[]genvcs.SecurityGroupSerializer]{}, fmt.Errorf("列出 security group 失敗: %w", err)
	}
	return twai.Decode[[]genvcs.SecurityGroupSerializer](resp.HTTPResponse, resp.Body)
}

// SecurityRuleInput 是 `vcs secg add-rule` 的參數；PortMin／PortMax 為 0 代表不送對應欄位，
// RemoteIPPrefix 為空字串代表不送對應欄位。
type SecurityRuleInput struct {
	ProjectID           int64
	Direction, Protocol string
	RemoteIPPrefix      string
	PortMin, PortMax    int
}

// AddSecurityGroupRule 對 security group 新增一條規則（PATCH，成功時 201 無 body）。
func (s *Service) AddSecurityGroupRule(ctx context.Context, sgID string, in SecurityRuleInput) error {
	direction := genvcs.PatchSecurityGroupsSecurityGroupIdJSONBodyDirection(in.Direction)
	protocol := genvcs.PatchSecurityGroupsSecurityGroupIdJSONBodyProtocol(in.Protocol)
	project := int(in.ProjectID)
	body := genvcs.PatchSecurityGroupsSecurityGroupIdJSONRequestBody{
		Direction: &direction,
		Protocol:  &protocol,
		Project:   &project,
	}
	if in.PortMin != 0 {
		body.PortRangeMin = &in.PortMin
	}
	if in.PortMax != 0 {
		body.PortRangeMax = &in.PortMax
	}
	setString(&body.RemoteIpPrefix, in.RemoteIPPrefix)

	resp, err := s.api.PatchSecurityGroupsSecurityGroupIdWithResponse(ctx, sgID, &genvcs.PatchSecurityGroupsSecurityGroupIdParams{XApiHost: s.host}, body)
	if err != nil {
		return fmt.Errorf("對 security group %s 新增規則失敗: %w", sgID, err)
	}
	return twai.CheckResponse(resp.HTTPResponse, resp.Body)
}

// DeleteSecurityGroupRule 刪除 security group 規則（成功時 202 無 body）。
func (s *Service) DeleteSecurityGroupRule(ctx context.Context, projectID int64, ruleID string) error {
	resp, err := s.api.DeleteSecurityGroupRulesRuleIdWithResponse(ctx, ruleID,
		&genvcs.DeleteSecurityGroupRulesRuleIdParams{Project: int(projectID), XApiHost: s.host})
	if err != nil {
		return fmt.Errorf("刪除 security group 規則 %s 失敗: %w", ruleID, err)
	}
	return twai.CheckResponse(resp.HTTPResponse, resp.Body)
}
