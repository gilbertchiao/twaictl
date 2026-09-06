// Package testutil 提供測試共用的小工具。
package testutil

import (
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// update 由 `go test ./... -update` 開啟，會覆寫 golden 檔而不是比對。
var update = flag.Bool("update", false, "重新產生 testdata/golden 下的 golden 檔")

// goldenDir 回傳 repo 根目錄下的 testdata/golden（以本檔位置反推，不依賴 cwd）。
func goldenDir() string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "testdata", "golden")
}

// AssertGolden 把 got 與 testdata/golden/<name> 比對；加 -update 時改為寫入。
func AssertGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join(goldenDir(), name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("建立 golden 目錄失敗: %v", err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("寫入 golden 檔失敗: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("讀取 golden 檔 %s 失敗（可用 -update 產生）: %v", path, err)
	}
	if string(want) != string(got) {
		t.Errorf("輸出與 golden 檔 %s 不符\n--- want ---\n%s\n--- got ---\n%s", name, want, got)
	}
}
