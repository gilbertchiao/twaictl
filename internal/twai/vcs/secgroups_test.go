package vcs

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListSecurityGroupsSendsProjectAndServer(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/security_groups/", 200, fixture(t, "security_groups"))
	s := newService(t, f)

	res, err := s.ListSecurityGroups(context.Background(), 101, 3658462)
	require.NoError(t, err)
	require.Len(t, res.Value, 1)
	require.Len(t, res.Value[0].SecurityGroupRules, 3)
	assert.Equal(t, "project=101&server=3658462", f.Requests()[0].Query)

	_, err = s.ListSecurityGroups(context.Background(), 101, 0)
	require.NoError(t, err)
	assert.Equal(t, "project=101", f.Requests()[1].Query, "serverID 為 0 時不送 server")
}

func TestAddSecurityGroupRuleBody(t *testing.T) {
	f := newFakeAPI(t)
	f.on("PATCH", vcsBase+"/security_groups/sg-1/", 201, nil)
	s := newService(t, f)

	err := s.AddSecurityGroupRule(context.Background(), "sg-1", SecurityRuleInput{
		ProjectID: 101, Direction: "ingress", Protocol: "tcp",
		RemoteIPPrefix: "0.0.0.0/0", PortMin: 22, PortMax: 22,
	})
	require.NoError(t, err)

	var body map[string]any
	require.NoError(t, json.Unmarshal(f.Requests()[0].Body, &body))
	assert.Equal(t, map[string]any{
		"direction": "ingress", "protocol": "tcp",
		"port_range_min": float64(22), "port_range_max": float64(22),
		"remote_ip_prefix": "0.0.0.0/0", "project": float64(101),
	}, body)
}

// TestAddSecurityGroupRuleOmitsUnsetFields 驗證 PortMin/PortMax 為 0、RemoteIPPrefix 為
// 空字串時不送對應欄位。
func TestAddSecurityGroupRuleOmitsUnsetFields(t *testing.T) {
	f := newFakeAPI(t)
	f.on("PATCH", vcsBase+"/security_groups/sg-1/", 201, nil)
	s := newService(t, f)

	err := s.AddSecurityGroupRule(context.Background(), "sg-1", SecurityRuleInput{
		ProjectID: 101, Direction: "egress", Protocol: "icmp",
	})
	require.NoError(t, err)

	var body map[string]any
	require.NoError(t, json.Unmarshal(f.Requests()[0].Body, &body))
	assert.Equal(t, map[string]any{"direction": "egress", "protocol": "icmp", "project": float64(101)}, body)
}

func TestDeleteSecurityGroupRuleProjectQuery(t *testing.T) {
	f := newFakeAPI(t)
	f.on("DELETE", vcsBase+"/security_group_rules/r-1/", 202, nil)
	s := newService(t, f)

	require.NoError(t, s.DeleteSecurityGroupRule(context.Background(), 101, "r-1"))
	assert.Equal(t, "project=101", f.Requests()[0].Query)
}
