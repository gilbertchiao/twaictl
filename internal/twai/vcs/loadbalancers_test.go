package vcs

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilbertchiao/twaictl/internal/twai"
)

func TestListLoadBalancersSendsProject(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/loadbalancers/", 200, fixture(t, "loadbalancers"))
	s := newService(t, f)

	res, err := s.ListLoadBalancers(context.Background(), 101)
	require.NoError(t, err)
	require.Len(t, res.Value, 2)
	assert.Equal(t, int64(80), res.Value[0].Listeners[0].ProtocolPort)
	assert.Equal(t, "project=101", f.Requests()[0].Query)
}

func TestGetLoadBalancerDecodesNestedPools(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/loadbalancers/5/", 200, fixture(t, "loadbalancer_detail"))
	s := newService(t, f)

	res, err := s.GetLoadBalancer(context.Background(), 5)
	require.NoError(t, err)
	require.NotNil(t, res.Value.Pools[0].Members)
	assert.Len(t, *res.Value.Pools[0].Members, 2)
	require.NotNil(t, res.Value.Pools[0].Monitor)
	require.NotNil(t, res.Value.Pools[0].Monitor.MonitorType)
	assert.Equal(t, "HTTP", *res.Value.Pools[0].Monitor.MonitorType)
}

func TestDeleteLoadBalancerChecksStatus(t *testing.T) {
	f := newFakeAPI(t)
	f.on("DELETE", vcsBase+"/loadbalancers/5/", 204, nil)
	f.on("DELETE", vcsBase+"/loadbalancers/6/", 404, []byte(`{"message":"LoadBalancer not found"}`))
	s := newService(t, f)

	require.NoError(t, s.DeleteLoadBalancer(context.Background(), 5))
	err := s.DeleteLoadBalancer(context.Background(), 6)
	var apiErr *twai.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusNotFound, apiErr.StatusCode)
	assert.Equal(t, "LoadBalancer not found", apiErr.Message)
}

func TestDecodeLoadBalancerSpecYAMLAndJSON(t *testing.T) {
	yamlSpec := "pools:\n- name: p\n  protocol: HTTP\n  method: ROUND_ROBIN\n  members:\n  - ip: 10.0.0.1\n    port: 80\n" +
		"listeners:\n- name: l\n  pool_name: p\n  protocol: HTTP\n  protocol_port: 80\n"
	jsonSpec := `{"pools":[{"name":"p","protocol":"HTTP","method":"ROUND_ROBIN","members":[{"ip":"10.0.0.1","port":80}]}],` +
		`"listeners":[{"name":"l","pool_name":"p","protocol":"HTTP","protocol_port":80}]}`

	fromYAML, err := DecodeLoadBalancerSpec([]byte(yamlSpec))
	require.NoError(t, err)
	fromJSON, err := DecodeLoadBalancerSpec([]byte(jsonSpec))
	require.NoError(t, err)
	assert.Equal(t, fromJSON, fromYAML)
	require.Len(t, fromYAML.Pools, 1)
	assert.Equal(t, "p", fromYAML.Pools[0].Name)
	require.NotNil(t, fromYAML.Pools[0].Members)
	require.Len(t, *fromYAML.Pools[0].Members, 1)
	assert.Equal(t, "10.0.0.1", *(*fromYAML.Pools[0].Members)[0].Ip)
	require.Len(t, fromYAML.Listeners, 1)
	assert.Equal(t, "l", *fromYAML.Listeners[0].Name)

	_, err = DecodeLoadBalancerSpec([]byte("foo: 1\n"))
	assert.ErrorContains(t, err, "foo")
}

func TestLoadBalancerSpecValidate(t *testing.T) {
	_, err := DecodeLoadBalancerSpec([]byte(`{"pools":[],"listeners":[]}`))
	require.NoError(t, err)
	empty := LoadBalancerSpec{}
	assert.ErrorContains(t, empty.Validate(), "pool")

	badPoolName, err := DecodeLoadBalancerSpec([]byte(
		`{"pools":[{"name":"p","protocol":"HTTP","method":"ROUND_ROBIN"}],` +
			`"listeners":[{"name":"l","pool_name":"nope","protocol":"HTTP","protocol_port":80}]}`))
	require.NoError(t, err)
	assert.ErrorContains(t, badPoolName.Validate(), "pool_name")

	missingPort, err := DecodeLoadBalancerSpec([]byte(
		`{"pools":[{"name":"p","protocol":"HTTP","method":"ROUND_ROBIN","members":[{"ip":"10.0.0.1"}]}],` +
			`"listeners":[{"name":"l","pool_name":"p","protocol":"HTTP","protocol_port":80}]}`))
	require.NoError(t, err)
	assert.Error(t, missingPort.Validate())
}

// TestLoadBalancerSpecValidateRejectsOutOfRangePorts 驗證 listener 的 protocol_port
// 與 member 的 port 都套用 1–65535 的範圍檢查，不是只檢查「有沒有給」。
func TestLoadBalancerSpecValidateRejectsOutOfRangePorts(t *testing.T) {
	badListenerPort, err := DecodeLoadBalancerSpec([]byte(
		`{"pools":[{"name":"p","protocol":"HTTP","method":"ROUND_ROBIN"}],` +
			`"listeners":[{"name":"l","pool_name":"p","protocol":"HTTP","protocol_port":70000}]}`))
	require.NoError(t, err)
	assert.ErrorContains(t, badListenerPort.Validate(), "1–65535")

	badMemberPort, err := DecodeLoadBalancerSpec([]byte(
		`{"pools":[{"name":"p","protocol":"HTTP","method":"ROUND_ROBIN","members":[{"ip":"10.0.0.1","port":0}]}],` +
			`"listeners":[{"name":"l","pool_name":"p","protocol":"HTTP","protocol_port":80}]}`))
	require.NoError(t, err)
	assert.ErrorContains(t, badMemberPort.Validate(), "1–65535")
}

func TestCreateLoadBalancerBodyAndArrayResponse(t *testing.T) {
	spec, err := DecodeLoadBalancerSpec([]byte(
		`{"pools":[{"name":"p","protocol":"HTTP","method":"ROUND_ROBIN"}],` +
			`"listeners":[{"name":"l","pool_name":"p","protocol":"HTTP","protocol_port":80}]}`))
	require.NoError(t, err)

	f := newFakeAPI(t)
	f.on("POST", vcsBase+"/loadbalancers/", 200, []byte(`[{"id":5,"name":"lb1","status":"BUILD"}]`))
	s := newService(t, f)

	res, err := s.CreateLoadBalancer(context.Background(), CreateLoadBalancerInput{
		Name: "lb1", Desc: "d", PrivateNetID: 328291, IPID: 42, Spec: spec,
	})
	require.NoError(t, err)
	req := f.Find("POST", vcsBase+"/loadbalancers/")
	require.NotNil(t, req)
	assert.JSONEq(t, `{"name":"lb1","desc":"d","private_net":328291,"ip":42,`+
		`"pools":[{"name":"p","protocol":"HTTP","method":"ROUND_ROBIN"}],`+
		`"listeners":[{"name":"l","pool_name":"p","protocol":"HTTP","protocol_port":80}]}`, string(req.Body))
	assert.Equal(t, int64(5), res.Value.Id)

	// Raw 必須是「第一筆的原始 bytes」（單一物件），不是整個陣列——這樣 -o json 的
	// 輸出形狀才會跟其他 create 命令、以及本命令加 --wait 後重新 GetLoadBalancer 的
	// 輸出形狀一致（都是單一物件），不會因為 POST 回應是陣列而變成陣列。
	assert.JSONEq(t, `{"id":5,"name":"lb1","status":"BUILD"}`, string(res.Raw))

	f2 := newFakeAPI(t)
	f2.on("POST", vcsBase+"/loadbalancers/", 200, []byte(`{"id":5,"name":"lb1","status":"BUILD"}`))
	s2 := newService(t, f2)
	res2, err := s2.CreateLoadBalancer(context.Background(), CreateLoadBalancerInput{
		Name: "lb1", PrivateNetID: 328291, Spec: spec,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(5), res2.Value.Id)
	assert.JSONEq(t, `{"id":5,"name":"lb1","status":"BUILD"}`, string(res2.Raw))
}

// TestCreateLoadBalancerRejectsEmptyArrayResponse 驗證 POST 回傳空陣列（`[]`）時
// 回傳明確錯誤，而不是靜默把 Value 留成零值。
func TestCreateLoadBalancerRejectsEmptyArrayResponse(t *testing.T) {
	spec, err := DecodeLoadBalancerSpec([]byte(
		`{"pools":[{"name":"p","protocol":"HTTP","method":"ROUND_ROBIN"}],` +
			`"listeners":[{"name":"l","pool_name":"p","protocol":"HTTP","protocol_port":80}]}`))
	require.NoError(t, err)

	f := newFakeAPI(t)
	f.on("POST", vcsBase+"/loadbalancers/", 200, []byte(`[]`))
	s := newService(t, f)

	_, err = s.CreateLoadBalancer(context.Background(), CreateLoadBalancerInput{
		Name: "lb1", PrivateNetID: 328291, Spec: spec,
	})
	assert.ErrorContains(t, err, "空陣列")
}

func TestUpdateLoadBalancerPatchesSpec(t *testing.T) {
	spec, err := DecodeLoadBalancerSpec([]byte(
		`{"pools":[{"name":"p","protocol":"HTTP","method":"ROUND_ROBIN"}],` +
			`"listeners":[{"name":"l","pool_name":"p","protocol":"HTTP","protocol_port":80}]}`))
	require.NoError(t, err)

	f := newFakeAPI(t)
	f.on("PATCH", vcsBase+"/loadbalancers/5/", 200, fixture(t, "loadbalancer_detail"))
	s := newService(t, f)

	_, err = s.UpdateLoadBalancer(context.Background(), 5, spec)
	require.NoError(t, err)
	req := f.Find("PATCH", vcsBase+"/loadbalancers/5/")
	require.NotNil(t, req)
	assert.JSONEq(t, `{"pools":[{"name":"p","protocol":"HTTP","method":"ROUND_ROBIN"}],`+
		`"listeners":[{"name":"l","pool_name":"p","protocol":"HTTP","protocol_port":80}]}`, string(req.Body))
}

func TestLoadBalancerActionBodies(t *testing.T) {
	f := newFakeAPI(t)
	f.on("PUT", vcsBase+"/loadbalancers/5/action/", 202, nil)
	s := newService(t, f)

	require.NoError(t, s.LoadBalancerAction(context.Background(), 5, LoadBalancerActionInput{Action: "associateIP", IPID: 42}))
	req := f.Find("PUT", vcsBase+"/loadbalancers/5/action/")
	require.NotNil(t, req)
	assert.JSONEq(t, `{"action":"associateIP","ip":42}`, string(req.Body))

	require.NoError(t, s.LoadBalancerAction(context.Background(), 5, LoadBalancerActionInput{Action: "disassociateIP"}))
	req = f.Find("PUT", vcsBase+"/loadbalancers/5/action/")
	require.NotNil(t, req)
	assert.JSONEq(t, `{"action":"disassociateIP"}`, string(req.Body))

	timeout := int32(60000)
	require.NoError(t, s.LoadBalancerAction(context.Background(), 5, LoadBalancerActionInput{
		Action: "updateListenerParams", ListenerName: "l", TimeoutClientData: &timeout,
	}))
	req = f.Find("PUT", vcsBase+"/loadbalancers/5/action/")
	require.NotNil(t, req)
	assert.JSONEq(t, `{"action":"updateListenerParams","listener_name":"l","timeout_client_data":60000}`, string(req.Body))
}

// TestLoadBalancerReportQuery 驗證 Begin／End 為 nil 時不送對應查詢參數，
// 給值時以 RFC3339（UTC）字串送出；並驗證 delete_time 為 JSON null 時
// （尚在使用中的 LB，真實值未知）能正常解析為零值時間，不會解析失敗
// （ReportDetailObject.DeleteTime 雖是 time.Time 而非指標，但 encoding/json 對
// time.Time 解析 null 是 no-op、保留零值，見 https://pkg.go.dev/time#Time.UnmarshalJSON，
// 因此不需要另外對 delete_time 加 nullable patch）。
func TestLoadBalancerReportQuery(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/loadbalancers/reports/", 200, fixture(t, "lb_reports"))
	s := newService(t, f)

	res, err := s.LoadBalancerReport(context.Background(), 101, LoadBalancerReportParams{})
	require.NoError(t, err)
	assert.Equal(t, "project=101", f.Requests()[0].Query, "Begin/End 為 nil 時不送 begin_time/end_time")
	require.Len(t, res.Value.Projects, 1)
	require.NotNil(t, res.Value.Projects[0].Detail.LB)
	lbs := *res.Value.Projects[0].Detail.LB
	require.Len(t, lbs, 2)
	assert.True(t, lbs[0].DeleteTime.IsZero(), "delete_time 為 null 時應解析成零值時間，而非解析失敗")
	assert.False(t, lbs[1].DeleteTime.IsZero())

	begin := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	_, err = s.LoadBalancerReport(context.Background(), 101, LoadBalancerReportParams{Begin: &begin, End: &end})
	require.NoError(t, err)
	assert.Equal(t, "project=101&begin_time=2026-08-01T00%3A00%3A00Z&end_time=2026-08-28T00%3A00%3A00Z",
		f.Requests()[1].Query)
}

// TestLoadBalancerMemberReportQueryAndDecode 驗證 member／method／interval／unit 進入
// query，以及回應（真實形狀，非生成的 JSON200）能以 twai.Decode 解析成
// []LoadBalancerMemberReportPoint。
func TestLoadBalancerMemberReportQueryAndDecode(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/loadbalancers/5/reports/", 200, []byte(`[{"bytes_read":37,"time":"2018-09-25T09:55:00.771000"}]`))
	s := newService(t, f)

	res, err := s.LoadBalancerMemberReport(context.Background(), 5, LoadBalancerMemberReportParams{
		Member: "10.0.0.11", Method: "avg", Interval: 10, Unit: "h",
	})
	require.NoError(t, err)
	assert.Equal(t, "member=10.0.0.11&method=avg&interval=10&unit=h", f.Requests()[0].Query)
	require.Len(t, res.Value, 1)
	assert.Equal(t, int64(37), res.Value[0].BytesRead)
	assert.Equal(t, "2018-09-25T09:55:00.771000", res.Value[0].Time)
}
