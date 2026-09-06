package vcs

import (
	"context"
	"fmt"

	gencommon "github.com/gilbertchiao/twaictl/internal/gen/common"
	genvcs "github.com/gilbertchiao/twaictl/internal/gen/vcs"
	"github.com/gilbertchiao/twaictl/internal/twai"
)

// QuotaRow 是 `project quota` 顯示用的單一列（resource 固定為 cpu／gpu／memory／
// floating_ip／static_ip 其中之一）。
type QuotaRow struct {
	Resource     string
	Usage, Quota float64
}

// QuotaRows 把 cpu／gpu／memory（必填）與 floating／static（選填，nil 代表 API 未回傳、
// 略過該列）組成固定順序的顯示列。
func QuotaRows(cpu, gpu, memory genvcs.QuotaUsageSerializer, floating, static *genvcs.QuotaUsageSerializer) []QuotaRow {
	rows := []QuotaRow{
		{Resource: "cpu", Usage: cpu.Usage, Quota: cpu.Quota},
		{Resource: "gpu", Usage: gpu.Usage, Quota: gpu.Quota},
		{Resource: "memory", Usage: memory.Usage, Quota: memory.Quota},
	}
	if floating != nil {
		rows = append(rows, QuotaRow{Resource: "floating_ip", Usage: floating.Usage, Quota: floating.Quota})
	}
	if static != nil {
		rows = append(rows, QuotaRow{Resource: "static_ip", Usage: static.Usage, Quota: static.Quota})
	}
	return rows
}

// ProjectQuota 取得目前 project 的配額（GET /project_quotas/?project= 回傳陣列，
// 取第一筆；空陣列視為錯誤，因為呼叫端無從得知是「無配額資料」還是「project 錯誤」）。
func (s *Service) ProjectQuota(ctx context.Context, projectID int64) (twai.Response[genvcs.ProjectQuotaSerializer], error) {
	resp, err := s.api.GetProjectQuotasWithResponse(ctx, &genvcs.GetProjectQuotasParams{Project: int(projectID), XApiHost: s.host})
	if err != nil {
		return twai.Response[genvcs.ProjectQuotaSerializer]{}, fmt.Errorf("取得 project %d 配額失敗: %w", projectID, err)
	}
	res, err := twai.Decode[[]genvcs.ProjectQuotaSerializer](resp.HTTPResponse, resp.Body)
	if err != nil {
		return twai.Response[genvcs.ProjectQuotaSerializer]{}, err
	}
	if len(res.Value) == 0 {
		return twai.Response[genvcs.ProjectQuotaSerializer]{}, fmt.Errorf("project %d 沒有 quota 資料", projectID)
	}
	return twai.Response[genvcs.ProjectQuotaSerializer]{Value: res.Value[0], Raw: res.Raw}, nil
}

// UserQuotas 取得 project 下所有使用者的配額。
func (s *Service) UserQuotas(ctx context.Context, projectID int64) (twai.Response[[]genvcs.UserQuotaSerializer], error) {
	resp, err := s.api.GetProjectsProjectIdUserQuotasWithResponse(ctx, int(projectID), &genvcs.GetProjectsProjectIdUserQuotasParams{XApiHost: s.host})
	if err != nil {
		return twai.Response[[]genvcs.UserQuotaSerializer]{}, fmt.Errorf("取得 project %d 使用者配額失敗: %w", projectID, err)
	}
	return twai.Decode[[]genvcs.UserQuotaSerializer](resp.HTTPResponse, resp.Body)
}

// projectSolutionsBody 對應 GET /projects/{project_id}/solutions/ 的回應（生成碼為匿名 struct，這裡給它名字）。
type projectSolutionsBody struct {
	Solutions []int `json:"solutions"`
}

// ProjectSolutionIDs 取得 project 已啟用的 solution id 清單。
func (s *Service) ProjectSolutionIDs(ctx context.Context, projectID int64) (twai.Response[[]int], error) {
	resp, err := s.api.GetProjectsProjectIdSolutionsWithResponse(ctx, int(projectID), &genvcs.GetProjectsProjectIdSolutionsParams{XApiHost: s.host})
	if err != nil {
		return twai.Response[[]int]{}, fmt.Errorf("取得 project %d 已啟用的 solution 失敗: %w", projectID, err)
	}
	res, err := twai.Decode[projectSolutionsBody](resp.HTTPResponse, resp.Body)
	if err != nil {
		return twai.Response[[]int]{}, err
	}
	return twai.Response[[]int]{Value: res.Value.Solutions, Raw: res.Raw}, nil
}

// ListSolutions 列出 project 可用的 solution（Common /solutions/?category=os&project=<id>）；
// 從 ResolveSolutionID 抽出，供 `project solutions` 對照 id 與名稱時共用。
func (s *Service) ListSolutions(ctx context.Context, projectID int64) ([]gencommon.SolutionSerializer, error) {
	project := formatID(projectID)
	resp, err := s.common.GetSolutionsWithResponse(ctx, &gencommon.GetSolutionsParams{
		Project: &project, Category: SolutionCategory, XApiHost: s.commonHost,
	})
	if err != nil {
		return nil, fmt.Errorf("列出 solution 失敗: %w", err)
	}
	res, err := twai.Decode[[]gencommon.SolutionSerializer](resp.HTTPResponse, resp.Body)
	if err != nil {
		return nil, err
	}
	return res.Value, nil
}
