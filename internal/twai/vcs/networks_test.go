package vcs

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListNetworksRequiresProject(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/networks/", 200, fixture(t, "networks"))
	s := newService(t, f)

	res, err := s.ListNetworks(context.Background(), 101)
	require.NoError(t, err)
	require.Len(t, res.Value, 2)
	assert.Equal(t, "project=101", f.Requests()[0].Query)
}

// TestGetNetworkDecodesDespite204Schema 驗證 GetNetwork 不受生成碼把成功回應標成
// JSON204 影響：假 API 回 200 + body 一樣能正確解析（見 networks.go 的說明）。
func TestGetNetworkDecodesDespite204Schema(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/networks/328291/", 200, fixture(t, "network_detail"))
	s := newService(t, f)

	res, err := s.GetNetwork(context.Background(), 328291)
	require.NoError(t, err)
	assert.Equal(t, "default_network", res.Value.Name)
	require.NotNil(t, res.Value.Firewall)
	assert.Equal(t, "fw-default", res.Value.Firewall.Name)
}

func TestCreateNetworkOmitsEmptyFields(t *testing.T) {
	f := newFakeAPI(t)
	f.on("POST", vcsBase+"/networks/", 201, fixture(t, "network_detail"))
	s := newService(t, f)

	_, err := s.CreateNetwork(context.Background(), CreateNetworkInput{ProjectID: 101, Name: "n1", CIDR: "10.0.0.0/24"})
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, json.Unmarshal(f.Requests()[0].Body, &body))
	assert.Equal(t, map[string]any{"name": "n1", "project": float64(101), "cidr": "10.0.0.0/24"}, body, "未指定 gateway/router/dns_domain 時不送")

	_, err = s.CreateNetwork(context.Background(), CreateNetworkInput{
		ProjectID: 101, Name: "n1", CIDR: "10.0.0.0/24", Gateway: "10.0.0.254", DNSDomain: "example.com", WithRouter: true,
	})
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(f.Requests()[1].Body, &body))
	assert.Equal(t, map[string]any{
		"name": "n1", "project": float64(101), "cidr": "10.0.0.0/24",
		"gateway": "10.0.0.254", "dns_domain": "example.com", "with_router": true,
	}, body)
}

func TestDeleteNetworkChecksStatus(t *testing.T) {
	f := newFakeAPI(t)
	f.on("DELETE", vcsBase+"/networks/328291/", 202, nil)
	f.on("DELETE", vcsBase+"/networks/328292/", 404, []byte(`{"message":"Network not found"}`))
	s := newService(t, f)

	require.NoError(t, s.DeleteNetwork(context.Background(), 328291))
	err := s.DeleteNetwork(context.Background(), 328292)
	assert.ErrorContains(t, err, "Network not found")
}
