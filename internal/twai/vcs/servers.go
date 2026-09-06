package vcs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	genvcs "github.com/gilbertchiao/twaictl/internal/gen/vcs"
	"github.com/gilbertchiao/twaictl/internal/twai"
)

// SiteActions 是 `vcs action` 的合法動作（VCS.yaml SiteActionSerializer.status enum）。
var SiteActions = []string{"start", "stop", "reboot", "suspend", "resume", "shelve", "unshelve"}

// Meters 是 `vcs metrics --meter` 的合法值。
var Meters = []string{"cpu", "memory", "disk-read", "disk-write", "net-in", "net-out"}

// ListServers 列出 project 下的所有 server。
func (s *Service) ListServers(ctx context.Context, projectID int64) (twai.Response[[]genvcs.ServerSerializer], error) {
	resp, err := s.api.GetServersWithResponse(ctx, &genvcs.GetServersParams{Project: int(projectID), XApiHost: s.host})
	if err != nil {
		return twai.Response[[]genvcs.ServerSerializer]{}, fmt.Errorf("列出 server 失敗: %w", err)
	}
	return twai.Decode[[]genvcs.ServerSerializer](resp.HTTPResponse, resp.Body)
}

// GetServer 取得單一 server 的詳細資料。
func (s *Service) GetServer(ctx context.Context, id int64) (twai.Response[genvcs.ServerSerializer], error) {
	resp, err := s.api.GetServersServerIdWithResponse(ctx, int(id), &genvcs.GetServersServerIdParams{XApiHost: s.host})
	if err != nil {
		return twai.Response[genvcs.ServerSerializer]{}, fmt.Errorf("取得 server %d 失敗: %w", id, err)
	}
	return twai.Decode[genvcs.ServerSerializer](resp.HTTPResponse, resp.Body)
}

// SiteAction 對 VCS 實例送出動作（PUT /sites/{id}/action/，202 Accepted，非同步）。
func (s *Service) SiteAction(ctx context.Context, siteID int64, action string) error {
	body := genvcs.SiteActionSerializer{Status: genvcs.SiteActionSerializerStatus(action)}
	resp, err := s.api.PutSitesSiteIdActionWithResponse(ctx, int(siteID), &genvcs.PutSitesSiteIdActionParams{XApiHost: s.host}, body)
	if err != nil {
		return fmt.Errorf("對 VCS 實例 %d 送出 %s 動作失敗: %w", siteID, action, err)
	}
	return twai.CheckResponse(resp.HTTPResponse, resp.Body)
}

// SiteEventLogs 取得 VCS 實例的事件紀錄。
func (s *Service) SiteEventLogs(ctx context.Context, siteID int64) (twai.Response[[]genvcs.SiteEventLogSerializer], error) {
	resp, err := s.api.GetSitesSiteIdEventLogsWithResponse(ctx, int(siteID), &genvcs.GetSitesSiteIdEventLogsParams{XApiHost: s.host})
	if err != nil {
		return twai.Response[[]genvcs.SiteEventLogSerializer]{}, fmt.Errorf("取得 VCS 實例 %d 事件紀錄失敗: %w", siteID, err)
	}
	return twai.Decode[[]genvcs.SiteEventLogSerializer](resp.HTTPResponse, resp.Body)
}

// MetricPoint 是四種 metrics 回應（cpu/memory/disk/net）通用解析後的單一資料點；
// Device 只有 disk-read/disk-write 有值（真實回應每個裝置各自一組資料，例如
// "/dev/vda"、"all"，見 docs/api-notes.md A22），其餘 meter 為空字串。
// 數值維持 API 回傳的原始字串（不轉換型別、不做單位換算）。
type MetricPoint struct {
	Device    string
	Timestamp time.Time
	Value     string
	Unit      string
}

// Metrics 查詢 server 的用量指標；meter 為 Meters 其中之一，begin/end 原樣傳給 API
// （對應 query 參數 begin_time/end_time；實測可用格式為 RFC3339，見 docs/api-notes.md A22）。
//
// 注意（已依真實 API 實測結果調整，取代原本照 spec 解析的做法，詳見 docs/api-notes.md A22）：
// VCS.yaml 對 disk-read/disk-write 標示共用同一個扁平 schema ServerDiskReadUtilSerializer、
// net-in/net-out 標示共用同一個扁平 schema ServerNetworkIncomingSerializer，但實測發現
// disk-read/disk-write 的真實回應其實是「每裝置一組」的巢狀結構、net-out 的數值欄位鍵名其實是
// network_outgoing_bytes_rate（不是 spec 標示的 network_incoming_bytes_rate）；net-in 端點
// 本身在伺服器端會回 500（API 自身的 bug，非本工具問題）。四個 serializer 需要整個重構才能對得上
// spec，且 net-in 端點本身壞掉、無法驗證，因此不 patch spec，改由 decodeMetricPoints 通用解析
// 真實回應形狀；四個生成的 Server*Serializer 型別因此不再使用（保留在 internal/gen 不手動刪除）。
func (s *Service) Metrics(ctx context.Context, serverID int64, meter, begin, end string) (twai.Response[[]MetricPoint], error) {
	var beginPtr, endPtr *string
	if begin != "" {
		beginPtr = &begin
	}
	if end != "" {
		endPtr = &end
	}

	switch meter {
	case "cpu":
		resp, err := s.api.GetServersServerIdCpuWithResponse(ctx, int(serverID),
			&genvcs.GetServersServerIdCpuParams{BeginTime: beginPtr, EndTime: endPtr, XApiHost: s.host})
		if err != nil {
			return twai.Response[[]MetricPoint]{}, fmt.Errorf("查詢 server %d cpu 指標失敗: %w", serverID, err)
		}
		return decodeMetricsResponse(resp.HTTPResponse, resp.Body, meter)

	case "memory":
		resp, err := s.api.GetServersServerIdMemoryWithResponse(ctx, int(serverID),
			&genvcs.GetServersServerIdMemoryParams{BeginTime: beginPtr, EndTime: endPtr, XApiHost: s.host})
		if err != nil {
			return twai.Response[[]MetricPoint]{}, fmt.Errorf("查詢 server %d memory 指標失敗: %w", serverID, err)
		}
		return decodeMetricsResponse(resp.HTTPResponse, resp.Body, meter)

	case "disk-read", "disk-write":
		meterName := genvcs.GetServersServerIdDiskParamsMeterNameDiskReadBytesRate
		if meter == "disk-write" {
			meterName = genvcs.GetServersServerIdDiskParamsMeterNameDiskWriteBytesRate
		}
		resp, err := s.api.GetServersServerIdDiskWithResponse(ctx, int(serverID),
			&genvcs.GetServersServerIdDiskParams{MeterName: meterName, BeginTime: beginPtr, EndTime: endPtr, XApiHost: s.host})
		if err != nil {
			return twai.Response[[]MetricPoint]{}, fmt.Errorf("查詢 server %d %s 指標失敗: %w", serverID, meter, err)
		}
		return decodeMetricsResponse(resp.HTTPResponse, resp.Body, meter)

	case "net-in", "net-out":
		meterName := genvcs.GetServersServerIdNetParamsMeterNameNetworkIncomingBytesRate
		if meter == "net-out" {
			meterName = genvcs.GetServersServerIdNetParamsMeterNameNetworkOutgoingBytesRate
		}
		resp, err := s.api.GetServersServerIdNetWithResponse(ctx, int(serverID),
			&genvcs.GetServersServerIdNetParams{MeterName: meterName, BeginTime: beginPtr, EndTime: endPtr, XApiHost: s.host})
		if err != nil {
			return twai.Response[[]MetricPoint]{}, fmt.Errorf("查詢 server %d %s 指標失敗: %w", serverID, meter, err)
		}
		return decodeMetricsResponse(resp.HTTPResponse, resp.Body, meter)

	default:
		return twai.Response[[]MetricPoint]{}, fmt.Errorf("不支援的 meter %q", meter)
	}
}

// decodeMetricsResponse 是四個 metrics 端點共用的收尾：先以 twai.CheckResponse 檢查狀態碼
// （dry-run → twai.ErrDryRun；非 2xx → *twai.APIError，行為與 twai.Decode 一致），
// 再用 decodeMetricPoints 通用解析 body；Raw 維持 API 原始 body（-o json 不受解析規則影響）。
// meter 是這次請求送出的 meter（Meters 其中之一），往下傳給 decodeMetricPoints／
// decodeFlatMetricPoint 決定 value 鍵時只依「這次查的 meter」判斷，不受回應裡其他
// meter 的鍵名干擾（見 meterValueKeys 的說明）。
func decodeMetricsResponse(resp *http.Response, body []byte, meter string) (twai.Response[[]MetricPoint], error) {
	if err := twai.CheckResponse(resp, body); err != nil {
		return twai.Response[[]MetricPoint]{}, err
	}
	points, err := decodeMetricPoints(body, meter)
	if err != nil {
		return twai.Response[[]MetricPoint]{}, err
	}
	return twai.Response[[]MetricPoint]{Value: points, Raw: body}, nil
}

// decodeMetricPoints 通用解析 metrics 端點的真實回應形狀（見 docs/api-notes.md A22）：
//   - 扁平（cpu、memory、net-out；net-in 因伺服器端 500 未能驗證但假設同形狀）：陣列元素
//     直接是一筆資料點，交給 decodeFlatMetricPoint 處理。
//   - 巢狀（disk-read、disk-write）：陣列元素是裝置（含 "name" 與 "utils" 陣列），需要展開
//     "utils" 陣列、把 "name" 標到每一筆 MetricPoint.Device。
func decodeMetricPoints(body []byte, meter string) ([]MetricPoint, error) {
	var elems []json.RawMessage
	if err := json.Unmarshal(body, &elems); err != nil {
		return nil, fmt.Errorf("解析 metrics 回應失敗: %w", err)
	}
	points := make([]MetricPoint, 0, len(elems))
	for _, raw := range elems {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			return nil, fmt.Errorf("解析 metrics 回應元素失敗: %w", err)
		}
		utilsRaw, isNested := fields["utils"]
		if !isNested {
			point, err := decodeFlatMetricPoint(raw, fields, meter)
			if err != nil {
				return nil, err
			}
			points = append(points, point)
			continue
		}

		device := ""
		if nameRaw, ok := fields["name"]; ok {
			if err := json.Unmarshal(nameRaw, &device); err != nil {
				return nil, fmt.Errorf("解析 metrics 裝置名稱失敗: %s", twai.Truncate(raw, 200))
			}
		}
		var utilElems []json.RawMessage
		if err := json.Unmarshal(utilsRaw, &utilElems); err != nil {
			return nil, fmt.Errorf("解析 metrics 裝置 %s 的 utils 失敗: %w", device, err)
		}
		for _, utilRaw := range utilElems {
			var utilFields map[string]json.RawMessage
			if err := json.Unmarshal(utilRaw, &utilFields); err != nil {
				return nil, fmt.Errorf("解析 metrics 裝置 %s 的 utils 元素失敗: %w", device, err)
			}
			point, err := decodeFlatMetricPoint(utilRaw, utilFields, meter)
			if err != nil {
				return nil, err
			}
			point.Device = device
			points = append(points, point)
		}
	}
	return points, nil
}

// meterValueKeys 把 Meters 的每個值對應到該 meter 已知的 value 鍵名（見
// docs/api-notes.md A22）。決定 value 鍵時「只看送出請求的那個 meter 對應哪個鍵」，
// 不能用一份跨 meter 共用、依鍵名字典序排列的清單去猜——例如 disk-write 的回應如果
// 剛好同時帶有 disk_read_bytes_rate 與 disk_write_bytes_rate 兩個鍵（真實 API 目前沒
// 觀察到這種情況，但不能排除未來版本這樣做），字典序 disk_read 排在 disk_write 之前，
// 用「跨 meter 共用清單、依序找第一個存在的鍵」的規則會誤選成 read 的值，即使呼叫端
// 明明查的是 write。
var meterValueKeys = map[string]string{
	"cpu":        "cpu_util",
	"memory":     "memory_usage",
	"disk-read":  "disk_read_bytes_rate",
	"disk-write": "disk_write_bytes_rate",
	"net-in":     "network_incoming_bytes_rate",
	"net-out":    "network_outgoing_bytes_rate",
}

// decodeFlatMetricPoint 解析單一「扁平」資料點：timestamp（RFC3339）、unit（字串，可缺）、
// value（優先取 meter 透過 meterValueKeys 對應到的那一個鍵；該鍵不存在時，退回除了
// timestamp/unit/name/utils 之外、字典序第一個鍵的值——這是給未知 meter（不在
// meterValueKeys 裡）或上游改版新增欄位時的相容退路；字串取其內容，數字則保留原始
// JSON 文字；其他型別視為異常）。raw 只用於錯誤訊息（原始 JSON，截斷至 200 字）。
func decodeFlatMetricPoint(raw json.RawMessage, fields map[string]json.RawMessage, meter string) (MetricPoint, error) {
	var point MetricPoint

	tsRaw, ok := fields["timestamp"]
	if !ok {
		return point, fmt.Errorf("metrics 回應缺少 timestamp 欄位: %s", twai.Truncate(raw, 200))
	}
	var tsStr string
	if err := json.Unmarshal(tsRaw, &tsStr); err != nil {
		return point, fmt.Errorf("metrics 回應的 timestamp 不是字串: %s", twai.Truncate(raw, 200))
	}
	ts, err := time.Parse(time.RFC3339, tsStr)
	if err != nil {
		return point, fmt.Errorf("解析 metrics timestamp %q 失敗: %w", tsStr, err)
	}
	point.Timestamp = ts

	if unitRaw, ok := fields["unit"]; ok {
		if err := json.Unmarshal(unitRaw, &point.Unit); err != nil {
			return point, fmt.Errorf("metrics 回應的 unit 不是字串: %s", twai.Truncate(raw, 200))
		}
	}

	valueKey := ""
	if expected, ok := meterValueKeys[meter]; ok {
		if _, present := fields[expected]; present {
			valueKey = expected
		}
	}
	if valueKey == "" {
		for k := range fields {
			switch k {
			case "timestamp", "unit", "name", "utils":
				continue
			}
			if valueKey == "" || k < valueKey {
				valueKey = k
			}
		}
	}
	if valueKey == "" {
		return point, fmt.Errorf("metrics 回應缺少數值欄位: %s", twai.Truncate(raw, 200))
	}

	valueRaw := fields[valueKey]
	// json.Number 同時接受 JSON 字串與 JSON 數字（前者會拆掉外層引號、保留內容，但要求
	// 字串內容本身是合法數字字面值；後者保留原始文字），剛好對應「字串取其內容、數字保留
	// 原始 JSON 文字」的規則；bool/物件/陣列/null 一律視為異常，直接報錯而非靜默轉成空字串。
	var value json.Number
	trimmed := bytes.TrimSpace(valueRaw)
	switch err := json.Unmarshal(valueRaw, &value); {
	case err == nil && !bytes.Equal(trimmed, []byte("null")):
		point.Value = string(value)
		return point, nil
	case len(trimmed) > 0 && trimmed[0] == '"':
		// 型別本身是字串，但內容不是合法數字字面值（例如 "abc"），與其他異常型別分開
		// 給更精確的錯誤訊息，避免使用者誤以為是型別錯誤而非內容錯誤。
		return point, fmt.Errorf("metrics 回應的數值欄位 %s 不是數字字串: %s", valueKey, twai.Truncate(raw, 200))
	default:
		return point, fmt.Errorf("metrics 回應的數值欄位 %s 型別不是字串或數字: %s", valueKey, twai.Truncate(raw, 200))
	}
}

// WaitServerStatus 輪詢 site 底下「全部」server 的狀態，直到每一台都進入 targets 其中之一
// 才算完成；任一台 server 進入 ERROR 視為失敗；site 沒有任何 server 也視為失敗（無從得知
// 動作是否完成）。
//
// mustLeave 非空時：每一台 server 都要先在某一次輪詢觀察到非空、且不等於 mustLeave 的
// status（代表該台的動作真的已經開始），才會被視為「已離開」；要全部 server 都已離開、
// 且全部回到 targets 內才算完成。這是給 reboot 用的：送出 reboot 後 server 通常仍是
// ACTIVE（重開尚未開始），若不要求先離開 ACTIVE，--wait 會在第一次輪詢就誤判為已完成；
// 多台 server 不會剛好同時重開，因此需要各自追蹤「是否已離開過」，而不是只看某一次輪詢
// 全部 server 是否同時離開。其他動作不需要這個保護，傳空字串即可（維持「status 在
// targets 內就算完成」的原本行為）。
//
// 空字串／null 的 status（srv.Status 未回傳）不算「已離開 mustLeave」：那只代表這次
// 輪詢沒有可用的狀態資訊，不是動作真的已經開始，若誤判為已離開，下一輪只要剛好又回到
// ACTIVE 就會提早判定 reboot 完成。
//
// 注意：若 server 真的重開得比輪詢間隔（--wait-interval，預設 5s）還快，
// 可能完全不會被觀察到離開 mustLeave 的瞬間，此時會等到 --wait-timeout 逾時
// （exit 5）；`vcs action --help` 已對 reboot 註明此限制。
//
// report 每次輪詢會收到全部 server 目前 status 的彙總字串，格式為
// `hostA=STATUS, hostB=STATUS`（未知/null 時該台的 STATUS 為空字串；只有一台 server 時
// 就只有 `host=STATUS`，不含逗號）；hostname 為 nil 時以該台在陣列中的索引字串代替。
func (s *Service) WaitServerStatus(ctx context.Context, siteID int64, targets []string, mustLeave string, interval time.Duration, report func(status string)) error {
	// left 以 server key（hostname，nil 時用索引字串）為 key，記錄該台是否已觀察到
	// 離開 mustLeave；mustLeave 為空字串時不需要這個保護，判定時直接略過檢查。
	left := map[string]bool{}
	return twai.WaitFor(ctx, interval, func(ctx context.Context) (bool, error) {
		res, err := s.GetSite(ctx, siteID)
		if err != nil {
			return false, err
		}
		if res.Value.Servers == nil || len(*res.Value.Servers) == 0 {
			return false, fmt.Errorf("VCS 實例 %d 沒有 server 可供判斷狀態", siteID)
		}
		servers := *res.Value.Servers

		parts := make([]string, len(servers))
		var errKeys []string
		allDone := true
		for i, srv := range servers {
			key := serverKey(srv.Hostname, i)
			status := ""
			if srv.Status != nil {
				status = string(*srv.Status)
			}
			parts[i] = fmt.Sprintf("%s=%s", key, status)

			if status == "ERROR" {
				errKeys = append(errKeys, key)
			}
			if mustLeave != "" && status != "" && status != mustLeave {
				left[key] = true
			}

			inTarget := false
			for _, target := range targets {
				if status == target {
					inTarget = true
					break
				}
			}
			if !inTarget || (mustLeave != "" && !left[key]) {
				allDone = false
			}
		}
		report(strings.Join(parts, ", "))

		if len(errKeys) > 0 {
			// 列出「全部」進入 ERROR 的 server（而非只有第一台），使用者才能一次
			// 得知完整清單、不必等下一輪輪詢才發現還有其他台也失敗了。
			return false, fmt.Errorf("VCS 實例 %d 的 server %s 進入 ERROR 狀態", siteID, strings.Join(errKeys, "、"))
		}
		return allDone, nil
	})
}

// serverKey 回傳 WaitServerStatus 用來識別單一 server 的 key：hostname 非 nil 時直接使用，
// nil 時（真實 API 理論上不會缺這個欄位，但生成型別容許）改用該台在 servers 陣列中的索引字串。
func serverKey(hostname *string, index int) string {
	if hostname != nil {
		return *hostname
	}
	return strconv.Itoa(index)
}
