package vcs

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	gencommon "github.com/gilbertchiao/twaictl/internal/gen/common"
	genvcs "github.com/gilbertchiao/twaictl/internal/gen/vcs"
	"github.com/gilbertchiao/twaictl/internal/twai"
)

// ResolveProjectID 把 project code（例如 ENT000000）或數字 id 解析為數字 id。
// code 走 GET /projects/?name=<code> 後以 name 完全比對。
func (s *Service) ResolveProjectID(ctx context.Context, ref string) (int64, error) {
	if twai.IsNumericRef(ref) {
		return twai.ResolveRef[genvcs.GroupSerializer]("project", ref, nil, nil, nil)
	}
	res, err := s.listProjects(ctx, &ref)
	if err != nil {
		return 0, err
	}
	return twai.ResolveRef("project", ref, res.Value,
		func(p genvcs.GroupSerializer) string { return p.Name },
		func(p genvcs.GroupSerializer) int64 { return p.Id })
}

// ResolveSiteID 把 <id|name> 解析為 site id（name 以 project 下的清單比對）。
func (s *Service) ResolveSiteID(ctx context.Context, projectID int64, ref string) (int64, error) {
	if twai.IsNumericRef(ref) {
		return twai.ResolveRef[genvcs.SiteSerializer]("site", ref, nil, nil, nil)
	}
	res, err := s.ListSites(ctx, projectID, false)
	if err != nil {
		return 0, err
	}
	return twai.ResolveRef("site", ref, res.Value,
		func(x genvcs.SiteSerializer) string { return x.Name },
		func(x genvcs.SiteSerializer) int64 { return x.Id })
}

// ResolveServerIDForSite 從 project 底下的 server 清單中，篩出 Site 欄位等於 siteID 的 server；
// `vcs metrics` 未指定 --server 時用它把 site 解析成唯一的 server id
// （多台 server 的 site 目前無法自動判斷要查哪一台，需要使用者以 --server 明確指定）。
func (s *Service) ResolveServerIDForSite(ctx context.Context, projectID, siteID int64) (int64, error) {
	res, err := s.ListServers(ctx, projectID)
	if err != nil {
		return 0, err
	}
	var matches []int64
	for _, srv := range res.Value {
		if srv.Site == siteID {
			matches = append(matches, srv.Id)
		}
	}
	switch len(matches) {
	case 0:
		return 0, fmt.Errorf("site %d 沒有 server", siteID)
	case 1:
		return matches[0], nil
	}
	ids := make([]string, 0, len(matches))
	for _, m := range matches {
		ids = append(ids, strconv.FormatInt(m, 10))
	}
	return 0, fmt.Errorf("site %d 有多個 server（id: %s），請以 --server 指定", siteID, strings.Join(ids, ", "))
}

// ResolveImageID 把 <id|name> 解析為 image id（name 以 project 下的清單比對）。
func (s *Service) ResolveImageID(ctx context.Context, projectID int64, ref string) (int64, error) {
	if twai.IsNumericRef(ref) {
		return twai.ResolveRef[genvcs.ImageSerializer]("image", ref, nil, nil, nil)
	}
	res, err := s.ListImages(ctx, projectID)
	if err != nil {
		return 0, err
	}
	return twai.ResolveRef("image", ref, res.Value,
		func(x genvcs.ImageSerializer) string { return x.Name },
		func(x genvcs.ImageSerializer) int64 { return x.Id })
}

// ResolveNetworkID 把 <id|name> 解析為 network id（name 以 project 下的清單比對）。
func (s *Service) ResolveNetworkID(ctx context.Context, projectID int64, ref string) (int64, error) {
	if twai.IsNumericRef(ref) {
		return twai.ResolveRef[genvcs.NetworkSerializer]("network", ref, nil, nil, nil)
	}
	res, err := s.ListNetworks(ctx, projectID)
	if err != nil {
		return 0, err
	}
	return twai.ResolveRef("network", ref, res.Value,
		func(x genvcs.NetworkSerializer) string { return x.Name },
		func(x genvcs.NetworkSerializer) int64 { return x.Id })
}

// ResolveLoadBalancerID 把 <id|name> 解析為 load balancer id（name 以 project 下的清單比對）。
func (s *Service) ResolveLoadBalancerID(ctx context.Context, projectID int64, ref string) (int64, error) {
	if twai.IsNumericRef(ref) {
		return twai.ResolveRef[genvcs.LBSerializer]("load balancer", ref, nil, nil, nil)
	}
	res, err := s.ListLoadBalancers(ctx, projectID)
	if err != nil {
		return 0, err
	}
	return twai.ResolveRef("load balancer", ref, res.Value,
		func(x genvcs.LBSerializer) string { return x.Name },
		func(x genvcs.LBSerializer) int64 { return x.Id })
}

// ResolveFirewallRuleID 把 <id|name> 解析為 firewall rule id（name 以 project 下的清單比對）。
func (s *Service) ResolveFirewallRuleID(ctx context.Context, projectID int64, ref string) (int64, error) {
	if twai.IsNumericRef(ref) {
		return twai.ResolveRef[genvcs.FirewallRuleSerializer]("firewall rule", ref, nil, nil, nil)
	}
	res, err := s.ListFirewallRules(ctx, projectID)
	if err != nil {
		return 0, err
	}
	return twai.ResolveRef("firewall rule", ref, res.Value,
		func(x genvcs.FirewallRuleSerializer) string { return x.Name },
		func(x genvcs.FirewallRuleSerializer) int64 { return x.Id })
}

// ResolveFirewallID 把 <id|name> 解析為 firewall id（name 以 project 下的清單比對）。
func (s *Service) ResolveFirewallID(ctx context.Context, projectID int64, ref string) (int64, error) {
	if twai.IsNumericRef(ref) {
		return twai.ResolveRef[genvcs.FirewallSerializer]("firewall", ref, nil, nil, nil)
	}
	res, err := s.ListFirewalls(ctx, projectID)
	if err != nil {
		return 0, err
	}
	return twai.ResolveRef("firewall", ref, res.Value,
		func(x genvcs.FirewallSerializer) string { return x.Name },
		func(x genvcs.FirewallSerializer) int64 { return x.Id })
}

// ResolveAutoScalingPolicyID 把 <id|name> 解析為 auto scaling policy id（name 以 project 下的清單比對）。
func (s *Service) ResolveAutoScalingPolicyID(ctx context.Context, projectID int64, ref string) (int64, error) {
	if twai.IsNumericRef(ref) {
		return twai.ResolveRef[genvcs.AutoScalingPolicySerializer]("auto scaling policy", ref, nil, nil, nil)
	}
	res, err := s.ListAutoScalingPolicies(ctx, projectID)
	if err != nil {
		return 0, err
	}
	return twai.ResolveRef("auto scaling policy", ref, res.Value,
		func(x genvcs.AutoScalingPolicySerializer) string { return x.Name },
		func(x genvcs.AutoScalingPolicySerializer) int64 { return x.Id })
}

// ResolveIPID 把 <id|address> 解析為 IP id：純數字直接當 id；否則列出 project 下的
// IP（不帶 address 查詢參數，避免多依賴一種 API 行為）後以 Address 完全比對。
func (s *Service) ResolveIPID(ctx context.Context, projectID int64, ref string) (int64, error) {
	if twai.IsNumericRef(ref) {
		return twai.ResolveRef[genvcs.IPSerializer]("ip", ref, nil, nil, nil)
	}
	res, err := s.ListIPs(ctx, projectID, "")
	if err != nil {
		return 0, err
	}
	return twai.ResolveRef("ip", ref, res.Value,
		func(x genvcs.IPSerializer) string { return x.Address },
		func(x genvcs.IPSerializer) int64 { return int64Value(x.Id) })
}

// int64Value 把 *int64 轉成數值，nil 回傳 0（IPSerializer.Id 依 spec 為選填指標，
// 正常資料一定會有值，這裡只是避免 nil 造成 panic）。
func int64Value(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

// ResolveVolumeID 把 <id|name> 解析為 volume id（name 以 project 下的清單比對）。
func (s *Service) ResolveVolumeID(ctx context.Context, projectID int64, ref string) (int64, error) {
	if twai.IsNumericRef(ref) {
		return twai.ResolveRef[genvcs.VolumeSerializer]("volume", ref, nil, nil, nil)
	}
	res, err := s.ListVolumes(ctx, projectID)
	if err != nil {
		return 0, err
	}
	return twai.ResolveRef("volume", ref, res.Value,
		func(x genvcs.VolumeSerializer) string { return x.Name },
		func(x genvcs.VolumeSerializer) int64 { return x.Id })
}

// ResolveSnapshotID 把 <id|name> 解析為 snapshot id（name 以 project 下的清單比對）。
func (s *Service) ResolveSnapshotID(ctx context.Context, projectID int64, ref string) (int64, error) {
	if twai.IsNumericRef(ref) {
		return twai.ResolveRef[genvcs.SnapshotSerializer]("snapshot", ref, nil, nil, nil)
	}
	res, err := s.ListSnapshots(ctx, projectID, "")
	if err != nil {
		return 0, err
	}
	return twai.ResolveRef("snapshot", ref, res.Value,
		func(x genvcs.SnapshotSerializer) string { return x.Name },
		func(x genvcs.SnapshotSerializer) int64 { return x.Id })
}

// ResolveSolutionID 把 solution <id|name> 解析為 id；清單來自 Common /solutions/?category=os&project=<id>
// （ListSolutions，供本函式與 `project solutions` 共用）。
func (s *Service) ResolveSolutionID(ctx context.Context, projectID int64, ref string) (int64, error) {
	if twai.IsNumericRef(ref) {
		return twai.ResolveRef[gencommon.SolutionSerializer]("solution", ref, nil, nil, nil)
	}
	items, err := s.ListSolutions(ctx, projectID)
	if err != nil {
		return 0, err
	}
	return twai.ResolveRef("solution", ref, items,
		func(x gencommon.SolutionSerializer) string { return x.Name },
		func(x gencommon.SolutionSerializer) int64 { return x.Id })
}
