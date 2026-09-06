package vcs

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestASPMeterName(t *testing.T) {
	assert.Len(t, ASPMeterNames, len(Meters), "ASPMeterNames 須與 vcs metrics --meter 的 Meters 一一對應")
	for _, short := range Meters {
		_, ok := ASPMeterNames[short]
		assert.True(t, ok, "Meters 短名 %q 應存在於 ASPMeterNames", short)
	}
	for short, full := range ASPMeterNames {
		got, err := ASPMeterName(short)
		require.NoError(t, err)
		assert.Equal(t, full, got)
		got, err = ASPMeterName(full)
		require.NoError(t, err)
		assert.Equal(t, full, got)
	}
	_, err := ASPMeterName("gpu")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cpu|memory|disk-read|disk-write|net-in|net-out")
}

func TestGetAutoScalingPolicyUsesTrailingSlash(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/auto_scaling_policies/701/", 200, fixture(t, "asp_detail"))
	s := newService(t, f)
	res, err := s.GetAutoScalingPolicy(context.Background(), 701)
	require.NoError(t, err)
	assert.Equal(t, "web-asp", res.Value.Name)
	assert.NotNil(t, f.Find("GET", vcsBase+"/auto_scaling_policies/701/"), "路徑須含尾斜線（Task 1 patch）")
}

// TestGetAutoScalingPolicyAcceptsArrayResponse 比照 GetFirewall：GetAutoScalingPolicy 改用
// 底層 raw client + DecodeObjectOrFirst，即使伺服器回陣列也要能解析成功並取第一筆。
func TestGetAutoScalingPolicyAcceptsArrayResponse(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/auto_scaling_policies/701/", 200, []byte(`[{"id": 701, "name": "web-asp", "meter_name": "cpu_util"}]`))
	s := newService(t, f)
	res, err := s.GetAutoScalingPolicy(context.Background(), 701)
	require.NoError(t, err)
	assert.Equal(t, int64(701), res.Value.Id)
	assert.JSONEq(t, `{"id": 701, "name": "web-asp", "meter_name": "cpu_util"}`, string(res.Raw), "Raw 為陣列第一筆的原始 bytes")
}

func TestCreateAutoScalingPolicyBody(t *testing.T) {
	f := newFakeAPI(t)
	f.on("POST", vcsBase+"/auto_scaling_policies/", 200, fixture(t, "asp_detail"))
	s := newService(t, f)
	down := 20
	_, err := s.CreateAutoScalingPolicy(context.Background(), CreateAutoScalingPolicyInput{
		ProjectID: 101, Name: "web-asp", Desc: "scale web tier", MeterName: "cpu_util", ScaleUpThreshold: 80, ScaleDownThreshold: &down, ScaleMaxSize: 5,
	})
	require.NoError(t, err)
	req := f.Find("POST", vcsBase+"/auto_scaling_policies/")
	require.NotNil(t, req)
	assert.JSONEq(t, `{"name":"web-asp","project":101,"description":"scale web tier","meter_name":"cpu_util","scaleup_threshold":80,"scaledown_threshold":20,"scale_max_size":5}`, string(req.Body))
}

// TestCreateAutoScalingPolicyAcceptsArrayResponse 比照 CreateFirewall：CreateAutoScalingPolicy
// 改用底層 raw client + DecodeObjectOrFirst，伺服器回陣列也要能解析成功。
func TestCreateAutoScalingPolicyAcceptsArrayResponse(t *testing.T) {
	f := newFakeAPI(t)
	f.on("POST", vcsBase+"/auto_scaling_policies/", 200, []byte(`[{"id": 701, "name": "web-asp", "meter_name": "cpu_util"}]`))
	s := newService(t, f)
	res, err := s.CreateAutoScalingPolicy(context.Background(), CreateAutoScalingPolicyInput{
		ProjectID: 101, Name: "web-asp", MeterName: "cpu_util", ScaleUpThreshold: 80, ScaleMaxSize: 5,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(701), res.Value.Id)
	assert.JSONEq(t, `{"id": 701, "name": "web-asp", "meter_name": "cpu_util"}`, string(res.Raw), "Raw 為陣列第一筆的原始 bytes")
}

func TestDeleteAutoScalingPolicy(t *testing.T) {
	f := newFakeAPI(t)
	f.onText("DELETE", vcsBase+"/auto_scaling_policies/701/", 204, "")
	s := newService(t, f)
	err := s.DeleteAutoScalingPolicy(context.Background(), 701)
	require.NoError(t, err)
	assert.NotNil(t, f.Find("DELETE", vcsBase+"/auto_scaling_policies/701/"))
}

func TestResolveAutoScalingPolicyID(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/auto_scaling_policies/", 200, fixture(t, "auto_scaling_policies"))
	s := newService(t, f)
	id, err := s.ResolveAutoScalingPolicyID(context.Background(), 101, "web-asp")
	require.NoError(t, err)
	assert.Equal(t, int64(701), id)
}

func TestAttachAutoScalingPolicyBody(t *testing.T) {
	f := newFakeAPI(t)
	f.on("POST", vcsBase+"/servers/3658462/auto_scaling_policy/", 201, fixture(t, "asp_attached"))
	s := newService(t, f)
	res, err := s.AttachAutoScalingPolicy(context.Background(), AttachAutoScalingPolicyInput{
		PolicyID: 701, ServerID: 3658462, LoadBalancerID: 313004, ProtocolPort: 8080, ScaleUpAction: "https://hook/up",
	})
	require.NoError(t, err)
	req := f.Find("POST", vcsBase+"/servers/3658462/auto_scaling_policy/")
	require.NotNil(t, req)
	assert.JSONEq(t, `{"auto_scaling_policy":701,"loadbalancer":313004,"protocol_port":8080,"scaleup_action":"https://hook/up"}`, string(req.Body))
	assert.Equal(t, int64(701), res.Value.AutoScalingPolicy)
	require.NotNil(t, res.Value.LoadBalancer)
	assert.Equal(t, int64(313004), *res.Value.LoadBalancer)
}

func TestAttachAutoScalingPolicyMinimalBody(t *testing.T) {
	f := newFakeAPI(t)
	f.on("POST", vcsBase+"/servers/3658462/auto_scaling_policy/", 201, []byte(`{"auto_scaling_policy": 701}`))
	s := newService(t, f)
	_, err := s.AttachAutoScalingPolicy(context.Background(), AttachAutoScalingPolicyInput{PolicyID: 701, ServerID: 3658462})
	require.NoError(t, err)
	assert.JSONEq(t, `{"auto_scaling_policy":701}`, string(f.Find("POST", vcsBase+"/servers/3658462/auto_scaling_policy/").Body))
}

// TestAttachAutoScalingPolicyAcceptsArrayResponse 比照 GetAutoScalingPolicy／CreateAutoScalingPolicy：
// AttachAutoScalingPolicy 改用底層 raw client + DecodeObjectOrFirst，即使伺服器回陣列也要能解析
// 成功，Raw 須為陣列第一筆的原始 bytes。
func TestAttachAutoScalingPolicyAcceptsArrayResponse(t *testing.T) {
	f := newFakeAPI(t)
	f.on("POST", vcsBase+"/servers/3658462/auto_scaling_policy/", 201, []byte(`[{"auto_scaling_policy": 701, "loadbalancer": 313004}]`))
	s := newService(t, f)
	res, err := s.AttachAutoScalingPolicy(context.Background(), AttachAutoScalingPolicyInput{PolicyID: 701, ServerID: 3658462})
	require.NoError(t, err)
	assert.Equal(t, int64(701), res.Value.AutoScalingPolicy)
	assert.JSONEq(t, `{"auto_scaling_policy": 701, "loadbalancer": 313004}`, string(res.Raw), "Raw 為陣列第一筆的原始 bytes")
}

// TestAttachAutoScalingPolicyLargeServerIDIsAllowed 驗證 ServerID 對應生成碼的
// ServerIDParam（= int，不是 int32；只有 PolicyID／LoadBalancerID 才是 int32 欄位），
// 超過 math.MaxInt32 的 server id 不應被 int32InRange 誤擋，請求仍要照常送到對應路徑
// （Codex P2：AttachAutoScalingPolicy 原本對 ServerID 也做了 int32InRange，brief 的迴圈是
// 計畫缺陷，controller ruling 已裁定接受修正）。
func TestAttachAutoScalingPolicyLargeServerIDIsAllowed(t *testing.T) {
	f := newFakeAPI(t)
	f.on("POST", vcsBase+"/servers/2147483648/auto_scaling_policy/", 201, []byte(`{"auto_scaling_policy": 701}`))
	s := newService(t, f)
	_, err := s.AttachAutoScalingPolicy(context.Background(), AttachAutoScalingPolicyInput{
		PolicyID: 701, ServerID: 2147483648,
	})
	require.NoError(t, err)
	assert.NotNil(t, f.Find("POST", vcsBase+"/servers/2147483648/auto_scaling_policy/"),
		"ServerID 超過 int32 上限時請求仍應送出到對應路徑")
}

// TestAttachAutoScalingPolicyInvalidProtocolPort 驗證 ProtocolPort 超出合法 TCP/UDP port
// 範圍（1–65535）時，在 int → int32 轉型、送出任何請求之前就報錯（Codex M-2）。
func TestAttachAutoScalingPolicyInvalidProtocolPort(t *testing.T) {
	f := newFakeAPI(t)
	s := newService(t, f)
	_, err := s.AttachAutoScalingPolicy(context.Background(), AttachAutoScalingPolicyInput{
		PolicyID: 701, ServerID: 3658462, ProtocolPort: 70000,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "1–65535")
	assert.Nil(t, f.Find("POST", vcsBase+"/servers/3658462/auto_scaling_policy/"), "protocol port 不合法時不應送出請求")
}

func TestDetachAutoScalingPolicy(t *testing.T) {
	f := newFakeAPI(t)
	f.onText("DELETE", vcsBase+"/servers/3658462/auto_scaling_policy/", 204, "")
	s := newService(t, f)
	err := s.DetachAutoScalingPolicy(context.Background(), 3658462)
	require.NoError(t, err)
	assert.NotNil(t, f.Find("DELETE", vcsBase+"/servers/3658462/auto_scaling_policy/"))
}
