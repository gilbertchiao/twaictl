package vcs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"time"

	genvcs "github.com/gilbertchiao/twaictl/internal/gen/vcs"
	"github.com/gilbertchiao/twaictl/internal/twai"
	"gopkg.in/yaml.v3"
)

// 對應 LBSerializer.status／LBDetailSerializer.status 的可能值。spec 未定義 enum，
// 2026-08-28 實測（見 docs/api-notes.md A29）狀態機為：建立中為 BUILD、建立完成／
// 更新完成為 ACTIVE、更新中為 UPDATING、刪除中為 DELETING；DELETED 未觀察到
// （刪除完成後直接變成 404，見 WaitLoadBalancerDeleted），保留該分支無害。
// 含 "ERROR" 子字串一律視為失敗（見 WaitLoadBalancerActive）。
const (
	LoadBalancerStatusActive = "ACTIVE"
	LoadBalancerStatusError  = "ERROR"
	// LoadBalancerStatusBuild 是建立中的中繼狀態（2026-08-28 實測值，約 2.5 分鐘後
	// 轉為 ACTIVE）；僅供註解／測試序列引用，WaitLoadBalancerActive 只判斷是否已
	// 到達 ACTIVE，不需要逐一列舉中繼狀態。
	LoadBalancerStatusBuild = "BUILD"
	// LoadBalancerStatusUpdating 是更新中的中繼狀態（2026-08-28 實測值）；僅供
	// 註解／測試序列引用，WaitLoadBalancerUpdated 的 mustLeave 語意（先離開
	// ACTIVE 再等回到 ACTIVE）已涵蓋此狀態、不需要另外判斷。
	LoadBalancerStatusUpdating = "UPDATING"
	// LoadBalancerStatusDeleting 是刪除中的中繼狀態（2026-08-28 實測值）。
	LoadBalancerStatusDeleting = "DELETING"
	// LoadBalancerStatusDeleted 是刪除完成後可能停留的 status（而非直接變成 404）；
	// 2026-08-28 實測未觀察到此狀態（刪除完成直接 404），保留此分支無害
	// （見 WaitLoadBalancerDeleted）。
	LoadBalancerStatusDeleted = "DELETED"
)

// LoadBalancerSpec 是 create／update 共用的 pools／listeners 描述；
// `--spec` 檔案（YAML 或 JSON）直接對應這個結構。
type LoadBalancerSpec struct {
	Pools     []genvcs.PoolData     `json:"pools"`
	Listeners []genvcs.ListenerData `json:"listeners"`
}

// DecodeLoadBalancerSpec 解析 `--spec` 檔案內容：YAML 是主要格式，JSON 是 YAML 的子集，
// 因此一律先以 yaml.Unmarshal 解成 any（yaml.v3 對 mapping 會解成 map[string]any，
// 與 JSON 相容），再轉成 JSON 位元組後以 DisallowUnknownFields 解成 LoadBalancerSpec，
// 讓使用者打錯欄位名稱時能立刻得到錯誤，而不是被靜默忽略。
func DecodeLoadBalancerSpec(data []byte) (LoadBalancerSpec, error) {
	var raw any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return LoadBalancerSpec{}, fmt.Errorf("解析 load balancer spec 失敗: %w", err)
	}
	converted, err := json.Marshal(raw)
	if err != nil {
		return LoadBalancerSpec{}, fmt.Errorf("解析 load balancer spec 失敗: %w", err)
	}
	var spec LoadBalancerSpec
	dec := json.NewDecoder(bytes.NewReader(converted))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&spec); err != nil {
		return LoadBalancerSpec{}, fmt.Errorf("解析 load balancer spec 失敗: %w", err)
	}
	return spec, nil
}

// Validate 檢查 spec 語意是否完整：至少一個 pool 與一個 listener；pool 的
// name／protocol／method 必填；listener 的 name／pool_name／protocol／protocol_port
// 必填且 pool_name 必須存在於 pools；member 的 ip／port 必填。
func (spec LoadBalancerSpec) Validate() error {
	if len(spec.Pools) == 0 {
		return fmt.Errorf("load balancer spec 至少需要一個 pool")
	}
	if len(spec.Listeners) == 0 {
		return fmt.Errorf("load balancer spec 至少需要一個 listener")
	}
	poolNames := make(map[string]bool, len(spec.Pools))
	for i, p := range spec.Pools {
		if p.Name == "" {
			return fmt.Errorf("pool[%d] 缺少 name", i)
		}
		if p.Protocol == "" {
			return fmt.Errorf("pool %q 缺少 protocol", p.Name)
		}
		if p.Method == "" {
			return fmt.Errorf("pool %q 缺少 method", p.Name)
		}
		if p.Members != nil {
			for j, m := range *p.Members {
				if m.Ip == nil || *m.Ip == "" {
					return fmt.Errorf("pool %q member[%d] 缺少 ip", p.Name, j)
				}
				if m.Port == nil {
					return fmt.Errorf("pool %q member[%d] 缺少 port", p.Name, j)
				}
				if err := ValidatePort(fmt.Sprintf("pool %q member[%d] 的 port", p.Name, j), *m.Port); err != nil {
					return err
				}
			}
		}
		poolNames[p.Name] = true
	}
	for i, l := range spec.Listeners {
		if l.Name == nil || *l.Name == "" {
			return fmt.Errorf("listener[%d] 缺少 name", i)
		}
		if l.PoolName == nil || *l.PoolName == "" {
			return fmt.Errorf("listener %q 缺少 pool_name", *l.Name)
		}
		if l.Protocol == nil || *l.Protocol == "" {
			return fmt.Errorf("listener %q 缺少 protocol", *l.Name)
		}
		if l.ProtocolPort == nil {
			return fmt.Errorf("listener %q 缺少 protocol_port", *l.Name)
		}
		if err := ValidatePort(fmt.Sprintf("listener %q 的 protocol_port", *l.Name), int64(*l.ProtocolPort)); err != nil {
			return err
		}
		if !poolNames[*l.PoolName] {
			return fmt.Errorf("listener %q 的 pool_name %q 不存在於 pools 內的任何 pool", *l.Name, *l.PoolName)
		}
	}
	return nil
}

// int32InRange 檢查 id 是否落在 int32 合法範圍內（同 Phase 2c 慣例：0 或負數、
// 超過 math.MaxInt32 都視為不合法，避免轉型時靜默溢位）。
func int32InRange(id int64) error {
	if id <= 0 || id > math.MaxInt32 {
		return fmt.Errorf("id %d 不是合法的 id（必須介於 1 與 %d 之間）", id, int32(math.MaxInt32))
	}
	return nil
}

// ValidatePort 檢查 port 是否落在合法的 TCP/UDP port 範圍（1–65535）。field 是用於
// 錯誤訊息的欄位名稱，讓共用這個檢查的每個呼叫端（`--member` 的 port、`--port` 這個
// listener port、spec 內的 protocol_port／member port）都能在錯誤訊息裡指出是哪個欄位
// 不合法，而不是丟一句籠統的「port 不合法」讓使用者自己猜。
func ValidatePort(field string, port int64) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("%s 必須介於 1–65535（目前 %d）", field, port)
	}
	return nil
}

// ListLoadBalancers 列出 project 下的 load balancer（API 要求 project 必填）。
func (s *Service) ListLoadBalancers(ctx context.Context, projectID int64) (twai.Response[[]genvcs.LBSerializer], error) {
	resp, err := s.api.GetLoadbalancersWithResponse(ctx, &genvcs.GetLoadbalancersParams{Project: int(projectID), XApiHost: s.host})
	if err != nil {
		return twai.Response[[]genvcs.LBSerializer]{}, fmt.Errorf("列出 load balancer 失敗: %w", err)
	}
	return twai.Decode[[]genvcs.LBSerializer](resp.HTTPResponse, resp.Body)
}

// GetLoadBalancer 取得單一 load balancer 的詳細資料。
func (s *Service) GetLoadBalancer(ctx context.Context, id int64) (twai.Response[genvcs.LBDetailSerializer], error) {
	resp, err := s.api.GetLoadbalancersLoadbalancerIdWithResponse(ctx, int(id), &genvcs.GetLoadbalancersLoadbalancerIdParams{XApiHost: s.host})
	if err != nil {
		return twai.Response[genvcs.LBDetailSerializer]{}, fmt.Errorf("取得 load balancer %d 失敗: %w", id, err)
	}
	return twai.Decode[genvcs.LBDetailSerializer](resp.HTTPResponse, resp.Body)
}

// DeleteLoadBalancer 刪除 load balancer（204 No Content）。
func (s *Service) DeleteLoadBalancer(ctx context.Context, id int64) error {
	resp, err := s.api.DeleteLoadbalancersLoadbalancerIdWithResponse(ctx, int(id), &genvcs.DeleteLoadbalancersLoadbalancerIdParams{XApiHost: s.host})
	if err != nil {
		return fmt.Errorf("刪除 load balancer %d 失敗: %w", id, err)
	}
	return twai.CheckResponse(resp.HTTPResponse, resp.Body)
}

// CreateLoadBalancerInput 是 CreateLoadBalancer 的參數。
type CreateLoadBalancerInput struct {
	Name, Desc   string
	PrivateNetID int64
	IPID         int64 // 0 不送 ip（不綁 floating IP）
	Spec         LoadBalancerSpec
}

// CreateLoadBalancer 建立 load balancer（POST /loadbalancers/）。spec 標回應為
// array，但實測前無法確認一律如此，因此同時接受單一物件或陣列——見 twai.DecodeObjectOrFirst。
//
// 這裡刻意不用 PostLoadbalancersWithResponse：生成碼的
// ParsePostLoadbalancersResponse 對 200 + json 一律強制以 `[]LBSerializer`
// unmarshal，若伺服器實際回單一物件會直接在生成碼內部解析失敗、連原始 body
// 都拿不到（回傳的 *PostLoadbalancersResponse 為 nil），因此改用底層的
// PostLoadbalancers（回傳原始 *http.Response），自行讀 body 後依實際內容判斷形狀。
func (s *Service) CreateLoadBalancer(ctx context.Context, in CreateLoadBalancerInput) (twai.Response[genvcs.LBSerializer], error) {
	if err := int32InRange(in.PrivateNetID); err != nil {
		return twai.Response[genvcs.LBSerializer]{}, err
	}
	if in.IPID != 0 {
		if err := int32InRange(in.IPID); err != nil {
			return twai.Response[genvcs.LBSerializer]{}, err
		}
	}
	privateNet := int32(in.PrivateNetID)
	body := genvcs.PostLoadbalancersJSONRequestBody{
		Name:       &in.Name,
		PrivateNet: &privateNet,
		Pools:      &in.Spec.Pools,
		Listeners:  &in.Spec.Listeners,
	}
	setString(&body.Desc, in.Desc)
	if in.IPID != 0 {
		ip := int32(in.IPID)
		body.Ip = &ip
	}
	httpResp, err := s.api.PostLoadbalancers(ctx, &genvcs.PostLoadbalancersParams{XApiHost: s.host}, body)
	if err != nil {
		return twai.Response[genvcs.LBSerializer]{}, fmt.Errorf("建立 load balancer 失敗: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()
	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return twai.Response[genvcs.LBSerializer]{}, fmt.Errorf("讀取建立 load balancer 的回應失敗: %w", err)
	}
	return twai.DecodeObjectOrFirst[genvcs.LBSerializer](httpResp, respBody, "建立 load balancer")
}

// UpdateLoadBalancer 以 spec 內容整份取代 load balancer 的 pools／listeners
// （PATCH /loadbalancers/{id}/）。
func (s *Service) UpdateLoadBalancer(ctx context.Context, id int64, spec LoadBalancerSpec) (twai.Response[genvcs.LBDetailSerializer], error) {
	body := genvcs.PatchLoadbalancersLoadbalancerIdJSONRequestBody{
		Pools:     &spec.Pools,
		Listeners: &spec.Listeners,
	}
	resp, err := s.api.PatchLoadbalancersLoadbalancerIdWithResponse(ctx, int(id), &genvcs.PatchLoadbalancersLoadbalancerIdParams{XApiHost: s.host}, body)
	if err != nil {
		return twai.Response[genvcs.LBDetailSerializer]{}, fmt.Errorf("更新 load balancer %d 失敗: %w", id, err)
	}
	return twai.Decode[genvcs.LBDetailSerializer](resp.HTTPResponse, resp.Body)
}

// LoadBalancerActionInput 是 LoadBalancerAction 的參數；哪些欄位有效依 Action 而定
// （見 PutLoadbalancersLoadbalancerIdActionJSONBody 的欄位註解）。
type LoadBalancerActionInput struct {
	Action                                                                        string // associateIP | disassociateIP | updateListenerParams
	IPID                                                                          int64  // associateIP
	ListenerName                                                                  string // updateListenerParams
	TimeoutClientData, TimeoutMemberConnect, TimeoutMemberData, TimeoutTCPInspect *int32
}

// LoadBalancerAction 對 load balancer 送出動作（PUT /loadbalancers/{id}/action/，202 Accepted）。
func (s *Service) LoadBalancerAction(ctx context.Context, id int64, in LoadBalancerActionInput) error {
	body := genvcs.PutLoadbalancersLoadbalancerIdActionJSONRequestBody{
		Action: genvcs.PutLoadbalancersLoadbalancerIdActionJSONBodyAction(in.Action),
	}
	if in.Action == string(genvcs.AssociateIP) {
		if err := int32InRange(in.IPID); err != nil {
			return err
		}
		ip := int32(in.IPID)
		body.Ip = &ip
	}
	setString(&body.ListenerName, in.ListenerName)
	body.TimeoutClientData = in.TimeoutClientData
	body.TimeoutMemberConnect = in.TimeoutMemberConnect
	body.TimeoutMemberData = in.TimeoutMemberData
	body.TimeoutTcpInspect = in.TimeoutTCPInspect
	resp, err := s.api.PutLoadbalancersLoadbalancerIdActionWithResponse(ctx, int(id), &genvcs.PutLoadbalancersLoadbalancerIdActionParams{XApiHost: s.host}, body)
	if err != nil {
		return fmt.Errorf("對 load balancer %d 送出 %s 動作失敗: %w", id, in.Action, err)
	}
	return twai.CheckResponse(resp.HTTPResponse, resp.Body)
}

// LoadBalancerReportParams 是 LoadBalancerReport 的參數；Begin／End 為 nil 時不送對應
// 查詢參數（API 預設 end=now、begin=一個月前）。
type LoadBalancerReportParams struct {
	Begin, End *time.Time
}

// LoadBalancerReport 查詢 project 下所有 load balancer 的用量報表
// （GET /loadbalancers/reports/）。Begin／End 一律以 RFC3339（UTC）字串送出。
func (s *Service) LoadBalancerReport(ctx context.Context, projectID int64, p LoadBalancerReportParams) (twai.Response[genvcs.LBReportSerializer], error) {
	params := &genvcs.GetLoadbalancersReportsParams{Project: int(projectID), XApiHost: s.host}
	if p.Begin != nil {
		begin := p.Begin.UTC().Format(time.RFC3339)
		params.BeginTime = &begin
	}
	if p.End != nil {
		end := p.End.UTC().Format(time.RFC3339)
		params.EndTime = &end
	}
	resp, err := s.api.GetLoadbalancersReportsWithResponse(ctx, params)
	if err != nil {
		return twai.Response[genvcs.LBReportSerializer]{}, fmt.Errorf("查詢 load balancer 報表失敗: %w", err)
	}
	return twai.Decode[genvcs.LBReportSerializer](resp.HTTPResponse, resp.Body)
}

// LoadBalancerMemberReportPoint 是 `GET /loadbalancers/{id}/reports/` 的真實回應形狀
// （spec 只有 example、沒有對應的 schema，生成碼因此誤把 JSON200 型別標成
// LBDetailSerializer，見 GetLoadbalancersLoadbalancerIdReportsResponse；本函式改用
// twai.Decode 直接依這個手寫型別解析，不採用生成的 JSON200）：
// [{"bytes_read": 37, "time": "2018-09-25T09:55:00.771000"}]；time 不含時區，
// 顯示時交由 output.FormatTimeString 處理。
type LoadBalancerMemberReportPoint struct {
	BytesRead int64  `json:"bytes_read"`
	Time      string `json:"time"`
}

// LoadBalancerMemberReportParams 是 LoadBalancerMemberReport 的參數；
// Method／Unit 為空字串、Interval 為 0 時皆不送對應查詢參數。
type LoadBalancerMemberReportParams struct {
	Member     string // 必填：pool member 的 ip
	Begin, End *time.Time
	Method     string // sum｜avg，空字串不送
	Interval   int32  // 0 不送
	Unit       string // s｜m｜h｜d｜w｜M｜y，空字串不送
}

// LoadBalancerMemberReport 查詢單一 load balancer 底下指定 member 的流量報表
// （GET /loadbalancers/{id}/reports/）。Begin／End 一律以 RFC3339（UTC）字串送出。
//
// 這裡刻意不用 GetLoadbalancersLoadbalancerIdReportsWithResponse：spec 對這個
// endpoint 只給了 example（陣列）、沒有對應的 schema，生成碼因此誤把 200 回應的
// 型別標成單一物件的 LBDetailSerializer（見 GetLoadbalancersLoadbalancerIdReportsResponse
// 與其 ParseGetLoadbalancersLoadbalancerIdReportsResponse），實際回應是陣列時會在生成碼
// 內部 json.Unmarshal 失敗、直接回傳 nil 回應（*GetLoadbalancersLoadbalancerIdReportsResponse
// 連原始 body 都拿不到）。因此改用底層的 GetLoadbalancersLoadbalancerIdReports
// （回傳原始 *http.Response），自行讀 body 後依 LoadBalancerMemberReportPoint 解析，
// 與 CreateLoadBalancer 對生成碼強制型別不符的處理方式一致。
func (s *Service) LoadBalancerMemberReport(ctx context.Context, id int64, p LoadBalancerMemberReportParams) (twai.Response[[]LoadBalancerMemberReportPoint], error) {
	params := &genvcs.GetLoadbalancersLoadbalancerIdReportsParams{Member: p.Member, XApiHost: s.host}
	if p.Begin != nil {
		begin := p.Begin.UTC().Format(time.RFC3339)
		params.BeginTime = &begin
	}
	if p.End != nil {
		end := p.End.UTC().Format(time.RFC3339)
		params.EndTime = &end
	}
	if p.Method != "" {
		method := genvcs.GetLoadbalancersLoadbalancerIdReportsParamsMethod(p.Method)
		params.Method = &method
	}
	if p.Interval != 0 {
		interval := p.Interval
		params.Interval = &interval
	}
	if p.Unit != "" {
		unit := genvcs.GetLoadbalancersLoadbalancerIdReportsParamsUnit(p.Unit)
		params.Unit = &unit
	}
	httpResp, err := s.api.GetLoadbalancersLoadbalancerIdReports(ctx, int(id), params)
	if err != nil {
		return twai.Response[[]LoadBalancerMemberReportPoint]{}, fmt.Errorf("查詢 load balancer %d member 報表失敗: %w", id, err)
	}
	defer func() { _ = httpResp.Body.Close() }()
	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return twai.Response[[]LoadBalancerMemberReportPoint]{}, fmt.Errorf("讀取 load balancer %d member 報表回應失敗: %w", id, err)
	}
	return twai.Decode[[]LoadBalancerMemberReportPoint](httpResp, respBody)
}
