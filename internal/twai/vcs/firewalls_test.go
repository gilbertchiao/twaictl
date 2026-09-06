package vcs

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetFirewallRuleAcceptsArrayOrObject(t *testing.T) {
	// 陣列（spec 形狀）
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/firewall_rules/501/", 200, fixture(t, "firewall_rule_detail"))
	s := newService(t, f)
	res, err := s.GetFirewallRule(context.Background(), 501)
	require.NoError(t, err)
	assert.Equal(t, int64(501), res.Value.Id)
	assert.Equal(t, "allow-ssh", res.Value.Name)
	assert.Equal(t, 2026, res.Value.CreateTime.Year())
	assert.JSONEq(t, string(res.Raw), `{"id": 501, "name": "allow-ssh", "project": 101, "platform": "openstack-taichung-default-2", "ip_version": 4, "protocol": "tcp", "action": "allow", "source_ip_address": "0.0.0.0/0", "source_port": "", "destination_ip_address": "192.168.0.0/24", "destination_port": "22", "create_time": "2026-08-28T01:02:03Z"}`, "Raw 為陣列第一筆的原始 bytes")

	// 單一物件（PATCH 回應形狀）
	f2 := newFakeAPI(t)
	f2.on("GET", vcsBase+"/firewall_rules/501/", 200, []byte(`{"id": 501, "name": "allow-ssh", "action": "allow"}`))
	s2 := newService(t, f2)
	res, err = s2.GetFirewallRule(context.Background(), 501)
	require.NoError(t, err)
	assert.Equal(t, "allow-ssh", res.Value.Name)
}

func TestCreateFirewallRuleBodyOmitsEmptyFields(t *testing.T) {
	f := newFakeAPI(t)
	f.on("POST", vcsBase+"/firewall_rules/", 200, []byte(`{"id": 503, "name": "r"}`))
	s := newService(t, f)
	_, err := s.CreateFirewallRule(context.Background(), FirewallRuleInput{ProjectID: 101, Name: "r", Protocol: "tcp", Action: "allow", DestinationPort: "22"})
	require.NoError(t, err)
	req := f.Find("POST", vcsBase+"/firewall_rules/")
	require.NotNil(t, req)
	assert.JSONEq(t, `{"name":"r","project":101,"protocol":"tcp","action":"allow","destination_port":"22"}`, string(req.Body))
}

// TestCreateFirewallRuleAcceptsArrayResponse 證明 CreateFirewallRule 跟 GetFirewallRule
// 一樣不會在伺服器回陣列時整條命令連原始 body 都拿不到：POST 成功但回應是陣列
// （同 A33 的疑慮——spec 標單一物件，但既然 GET 同一資源的 200 已知會回陣列，
// create 也可能如此，且 create 失敗時 API 端很可能已經建好規則，遺失原始 body 的
// 代價比 GetFirewallRule 更高），仍要能解析成功並取第一筆。
func TestCreateFirewallRuleAcceptsArrayResponse(t *testing.T) {
	f := newFakeAPI(t)
	f.on("POST", vcsBase+"/firewall_rules/", 200, []byte(`[{"id": 503, "name": "r"}]`))
	s := newService(t, f)
	res, err := s.CreateFirewallRule(context.Background(), FirewallRuleInput{ProjectID: 101, Name: "r"})
	require.NoError(t, err)
	assert.Equal(t, int64(503), res.Value.Id)
	assert.JSONEq(t, string(res.Raw), `{"id": 503, "name": "r"}`, "Raw 為陣列第一筆的原始 bytes")
}

func TestResolveFirewallRuleID(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/firewall_rules/", 200, fixture(t, "firewall_rules"))
	s := newService(t, f)
	id, err := s.ResolveFirewallRuleID(context.Background(), 101, "deny-all")
	require.NoError(t, err)
	assert.Equal(t, int64(502), id)
	id, err = s.ResolveFirewallRuleID(context.Background(), 101, "999")
	require.NoError(t, err)
	assert.Equal(t, int64(999), id)
	_, err = s.ResolveFirewallRuleID(context.Background(), 101, "nope")
	require.Error(t, err)
}

func TestListFirewalls(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/firewalls/", 200, fixture(t, "firewalls"))
	s := newService(t, f)
	res, err := s.ListFirewalls(context.Background(), 101)
	require.NoError(t, err)
	require.Len(t, res.Value, 2)
	assert.Equal(t, "web-fw", res.Value[0].Name)
	assert.Equal(t, "project=101", f.Find("GET", vcsBase+"/firewalls/").Query)
}

func TestGetFirewall(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/firewalls/301/", 200, fixture(t, "firewall_detail"))
	s := newService(t, f)
	res, err := s.GetFirewall(context.Background(), 301)
	require.NoError(t, err)
	assert.Equal(t, int64(301), res.Value.Id)
	assert.Equal(t, "web-fw", res.Value.Name)
	require.Len(t, res.Value.Rules, 2)
	assert.Equal(t, "allow-ssh", res.Value.Rules[0].Name)
	require.Len(t, res.Value.AssociateNetworks, 1)
	assert.Equal(t, "default_network", res.Value.AssociateNetworks[0].Name)
}

// TestGetFirewallAcceptsArrayResponse 比照 GetFirewallRule 的 A33 疑慮：GetFirewall 改用
// 底層 raw client + DecodeObjectOrFirst，即使伺服器回陣列也要能解析成功並取第一筆。
func TestGetFirewallAcceptsArrayResponse(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/firewalls/301/", 200, []byte(`[{"id": 301, "name": "web-fw"}]`))
	s := newService(t, f)
	res, err := s.GetFirewall(context.Background(), 301)
	require.NoError(t, err)
	assert.Equal(t, int64(301), res.Value.Id)
	assert.JSONEq(t, string(res.Raw), `{"id": 301, "name": "web-fw"}`, "Raw 為陣列第一筆的原始 bytes")
}

func TestCreateFirewallBody(t *testing.T) {
	f := newFakeAPI(t)
	f.on("POST", vcsBase+"/firewalls/", 200, []byte(`{"id": 303, "name": "fw"}`))
	s := newService(t, f)
	_, err := s.CreateFirewall(context.Background(), CreateFirewallInput{ProjectID: 101, Name: "fw", Desc: "d", RuleIDs: []int64{501, 502}, NetworkIDs: []int64{328291}})
	require.NoError(t, err)
	req := f.Find("POST", vcsBase+"/firewalls/")
	require.NotNil(t, req)
	assert.JSONEq(t, `{"name":"fw","project":101,"desc":"d","rules":[501,502],"associate_networks":[328291]}`, string(req.Body))
}

// TestCreateFirewallAcceptsArrayResponse 比照 TestCreateFirewallRuleAcceptsArrayResponse：
// CreateFirewall 改用底層 raw client + DecodeObjectOrFirst，伺服器回陣列也要能解析成功。
func TestCreateFirewallAcceptsArrayResponse(t *testing.T) {
	f := newFakeAPI(t)
	f.on("POST", vcsBase+"/firewalls/", 200, []byte(`[{"id": 303, "name": "fw"}]`))
	s := newService(t, f)
	res, err := s.CreateFirewall(context.Background(), CreateFirewallInput{ProjectID: 101, Name: "fw"})
	require.NoError(t, err)
	assert.Equal(t, int64(303), res.Value.Id)
	assert.JSONEq(t, string(res.Raw), `{"id": 303, "name": "fw"}`, "Raw 為陣列第一筆的原始 bytes")
}

func TestUpdateFirewallNilVersusEmptyList(t *testing.T) {
	f := newFakeAPI(t)
	f.on("PATCH", vcsBase+"/firewalls/301/", 200, []byte(`{"id": 301, "name": "fw", "rules": [], "associate_networks": []}`))
	s := newService(t, f)
	empty := []int64{}
	_, err := s.UpdateFirewall(context.Background(), 301, UpdateFirewallInput{RuleIDs: &empty})
	require.NoError(t, err)
	req := f.Find("PATCH", vcsBase+"/firewalls/301/")
	require.NotNil(t, req)
	assert.JSONEq(t, `{"rules":[]}`, string(req.Body), "nil 不送、空 slice 送 []")
}

// TestUpdateFirewallAcceptsArrayResponse 比照 CreateFirewall／GetFirewall：UpdateFirewall
// 改用底層 raw client + DecodeObjectOrFirst，伺服器回陣列也要能解析成功。
func TestUpdateFirewallAcceptsArrayResponse(t *testing.T) {
	f := newFakeAPI(t)
	f.on("PATCH", vcsBase+"/firewalls/301/", 200, []byte(`[{"id": 301, "name": "fw"}]`))
	s := newService(t, f)
	desc := "d"
	res, err := s.UpdateFirewall(context.Background(), 301, UpdateFirewallInput{Desc: &desc})
	require.NoError(t, err)
	assert.Equal(t, int64(301), res.Value.Id)
	assert.JSONEq(t, string(res.Raw), `{"id": 301, "name": "fw"}`, "Raw 為陣列第一筆的原始 bytes")
}

func TestDeleteFirewall(t *testing.T) {
	f := newFakeAPI(t)
	f.onText("DELETE", vcsBase+"/firewalls/301/", 204, "")
	s := newService(t, f)
	err := s.DeleteFirewall(context.Background(), 301)
	require.NoError(t, err)
	assert.NotNil(t, f.Find("DELETE", vcsBase+"/firewalls/301/"))
}

func TestResolveFirewallID(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/firewalls/", 200, fixture(t, "firewalls"))
	s := newService(t, f)
	id, err := s.ResolveFirewallID(context.Background(), 101, "db-fw")
	require.NoError(t, err)
	assert.Equal(t, int64(302), id)
	id, err = s.ResolveFirewallID(context.Background(), 101, "999")
	require.NoError(t, err)
	assert.Equal(t, int64(999), id)
	_, err = s.ResolveFirewallID(context.Background(), 101, "nope")
	require.Error(t, err)
}
