package vcs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gilbertchiao/twaictl/internal/twai"
)

// 對應 SiteSerializer.status 的 enum（VCS.yaml SiteSerializer）。
const (
	SiteStatusReady    = "Ready"
	SiteStatusNotReady = "NotReady"
	SiteStatusError    = "Error"
	SiteStatusDeleted  = "Deleted"
)

// WaitSiteSettled 輪詢直到 site 狀態離開過渡狀態（Stopping／Starting／Rebooting／Suspending／
// Resuming／Shelving／Unshelving …）、進入 Ready 或 NotReady；進入 Error 視為失敗。
// 真實環境的 site action API 只在 site 為 Ready／NotReady 時接受下一個動作，而 server 狀態
// 到達目標後 site 狀態仍會落後 45～120 秒（見 docs/api-notes.md A39），`vcs action --wait`
// 在 server 狀態到位後再呼叫本函式，確保回傳時可以立即接續下一個動作。
func (s *Service) WaitSiteSettled(ctx context.Context, id int64, interval time.Duration, report func(status string)) error {
	return twai.WaitFor(ctx, interval, func(ctx context.Context) (bool, error) {
		res, err := s.GetSite(ctx, id)
		if err != nil {
			return false, err
		}
		status := string(res.Value.Status)
		report(status)
		switch status {
		case SiteStatusReady, SiteStatusNotReady:
			return true, nil
		case SiteStatusError:
			return false, fmt.Errorf("VCS 實例 %d 進入 %s 狀態", id, status)
		}
		return false, nil
	})
}

// WaitSiteReady 輪詢直到 site 進入 Ready；進入 Error 視為失敗。report 每次輪詢會收到目前 status。
func (s *Service) WaitSiteReady(ctx context.Context, id int64, interval time.Duration, report func(status string)) error {
	return twai.WaitFor(ctx, interval, func(ctx context.Context) (bool, error) {
		res, err := s.GetSite(ctx, id)
		if err != nil {
			return false, err
		}
		status := string(res.Value.Status)
		report(status)
		switch status {
		case SiteStatusReady:
			return true, nil
		case SiteStatusError:
			return false, fmt.Errorf("VCS 實例 %d 進入 %s 狀態", id, status)
		}
		return false, nil
	})
}

// WaitSiteDeleted 輪詢直到 site 回 404 或 status 為 Deleted。
func (s *Service) WaitSiteDeleted(ctx context.Context, id int64, interval time.Duration, report func(status string)) error {
	return twai.WaitFor(ctx, interval, func(ctx context.Context) (bool, error) {
		res, err := s.GetSite(ctx, id)
		var apiErr *twai.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			report(SiteStatusDeleted)
			return true, nil
		}
		if err != nil {
			return false, err
		}
		status := string(res.Value.Status)
		report(status)
		return status == SiteStatusDeleted, nil
	})
}

// WaitLoadBalancerActive 輪詢直到 load balancer 進入 ACTIVE；status 含 "ERROR"
// 子字串視為失敗（spec 未定義 status enum，見 LoadBalancerStatusActive／LoadBalancerStatusError
// 的說明），錯誤訊息在 StatusReason 非空時附上原因。
func (s *Service) WaitLoadBalancerActive(ctx context.Context, id int64, interval time.Duration, report func(status string)) error {
	return twai.WaitFor(ctx, interval, func(ctx context.Context) (bool, error) {
		res, err := s.GetLoadBalancer(ctx, id)
		if err != nil {
			return false, err
		}
		status := res.Value.Status
		report(status)
		if status == LoadBalancerStatusActive {
			return true, nil
		}
		if strings.Contains(status, LoadBalancerStatusError) {
			reason := ""
			if res.Value.StatusReason != nil && *res.Value.StatusReason != "" {
				reason = "：" + *res.Value.StatusReason
			}
			return false, fmt.Errorf("load balancer %d 進入 %s 狀態%s", id, status, reason)
		}
		return false, nil
	})
}

// WaitLoadBalancerUpdated 輪詢直到 load balancer 完成一次 action（associate-ip／
// disassociate-ip／update-listener）造成的更新。送出 action 後 load balancer 通常仍是
// ACTIVE（更新尚未真的開始），若不做任何保護，--wait 第一次輪詢就可能讀到舊的 ACTIVE、
// 誤判為已經完成；因此必須先觀察到 status 離開 ACTIVE（進入非空、非 ACTIVE 的中繼狀態，
// 例如 UPDATING，見 LoadBalancerStatusUpdating）才算「這次動作真的開始了」，之後再等它
// 回到 ACTIVE 才算完成
// （比照 internal/twai/vcs/servers.go WaitServerStatus 的 mustLeave 語意，這裡只有單一
// 資源、不需要 per-server 的 left map）。status 含 "ERROR" 子字串視為失敗（同
// WaitLoadBalancerActive，錯誤訊息在 StatusReason 非空時附上原因）。
//
// 注意（與 WaitServerStatus 相同的已知限制）：若整個更新過程比 --wait-interval
// （預設 5s）還快、沒有輪詢到「已離開 ACTIVE」的瞬間，會持續等到 --wait-timeout 逾時
// （exit 5），此時實際上動作多半已經完成，只是 --wait 沒能觀察到。
func (s *Service) WaitLoadBalancerUpdated(ctx context.Context, id int64, interval time.Duration, report func(status string)) error {
	left := false
	return twai.WaitFor(ctx, interval, func(ctx context.Context) (bool, error) {
		res, err := s.GetLoadBalancer(ctx, id)
		if err != nil {
			return false, err
		}
		status := res.Value.Status
		report(status)
		if strings.Contains(status, LoadBalancerStatusError) {
			reason := ""
			if res.Value.StatusReason != nil && *res.Value.StatusReason != "" {
				reason = "：" + *res.Value.StatusReason
			}
			return false, fmt.Errorf("load balancer %d 進入 %s 狀態%s", id, status, reason)
		}
		if status != "" && status != LoadBalancerStatusActive {
			left = true
		}
		return left && status == LoadBalancerStatusActive, nil
	})
}

// WaitLoadBalancerDeleted 輪詢直到 load balancer 回 404，或 status 進入
// LoadBalancerStatusDeleted（部分情況刪除完成後不會馬上變成 404，見該常數的說明）。
func (s *Service) WaitLoadBalancerDeleted(ctx context.Context, id int64, interval time.Duration, report func(status string)) error {
	return twai.WaitFor(ctx, interval, func(ctx context.Context) (bool, error) {
		res, err := s.GetLoadBalancer(ctx, id)
		var apiErr *twai.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			report(SiteStatusDeleted)
			return true, nil
		}
		if err != nil {
			return false, err
		}
		status := res.Value.Status
		report(status)
		return status == LoadBalancerStatusDeleted, nil
	})
}
