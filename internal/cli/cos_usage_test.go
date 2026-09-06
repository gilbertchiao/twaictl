package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilbertchiao/twaictl/internal/testutil"
)

// TestCosUsageGolden 驗證 `cos usage` 直式輸出總用量（golden），-o json 為 API 原始回應。
func TestCosUsageGolden(t *testing.T) {
	api := newFakeAPI(t)
	api.onFixture("GET", cephBase+"/projects/60178/buckets_util/", 200, "ceph/buckets_util")

	code, stdout, stderr := runCLI(t, api.env(t), "", "cos", "usage")
	require.Equal(t, ExitOK, code, stderr)
	testutil.AssertGolden(t, "cos_usage.txt", []byte(stdout))

	code, stdout, stderr = runCLI(t, api.env(t), "", "cos", "usage", "-o", "json")
	require.Equal(t, ExitOK, code, stderr)
	assert.JSONEq(t, fixture(t, "ceph/buckets_util"), stdout)
}

// TestCosUsagePerBucketGolden 驗證 `cos usage --per-bucket` 列出每個 bucket 的用量（golden），
// --key-name 會帶 key_name 查詢參數。
func TestCosUsagePerBucketGolden(t *testing.T) {
	api := newFakeAPI(t)
	api.onFixture("GET", cephBase+"/projects/60178/buckets/", 200, "ceph/buckets")

	code, stdout, stderr := runCLI(t, api.env(t), "", "cos", "usage", "--per-bucket", "--key-name", "backup")
	require.Equal(t, ExitOK, code, stderr)
	testutil.AssertGolden(t, "cos_usage_per_bucket.txt", []byte(stdout))
	req := api.find("GET", cephBase+"/projects/60178/buckets/")
	require.NotNil(t, req)
	assert.Equal(t, "key_name=backup", req.Query)
}

// TestCosUsageDryRunPrintsCurl 驗證 --dry-run 印出 curl（含 buckets_util 路徑）且不含 API key。
func TestCosUsageDryRunPrintsCurl(t *testing.T) {
	api := newFakeAPI(t)
	code, stdout, stderr := runCLI(t, api.env(t), "", "--dry-run", "--project", "60178", "cos", "usage")
	require.Equal(t, ExitOK, code, stderr)
	assert.Contains(t, stdout, "curl")
	assert.Contains(t, stdout, "/buckets_util/")
	assert.NotContains(t, stdout, "test-key")
}
