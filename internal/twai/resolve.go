package twai

import (
	"fmt"
	"strconv"
	"strings"
)

// IsNumericRef 判斷使用者輸入是否為純數字 id。
func IsNumericRef(ref string) bool {
	if ref == "" {
		return false
	}
	for _, r := range ref {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ResolveRef 依 DESIGN 6.4 的規則把 <id|name> 解析為 id：
// 純數字直接回傳；否則在 items 中以 name 完全比對，0 筆與多筆都是錯誤。
// kind 是資源種類的顯示名稱（例如 "site"），只用在錯誤訊息。
//
// 重要契約：ref 為純數字時，直接把它解析成 id 回傳，完全不會走訪 items、
// 呼叫 name 或 id 這兩個 callback ——因此呼叫端在只有數字 ref 的情境下
// 可以放心傳入 nil items（例如尚未查詢清單、或不需要名稱解析時）。
func ResolveRef[T any](kind, ref string, items []T, name func(T) string, id func(T) int64) (int64, error) {
	if IsNumericRef(ref) {
		value, err := strconv.ParseInt(ref, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("解析 %s id %q 失敗: %w", kind, ref, err)
		}
		return value, nil
	}
	var matches []int64
	for _, item := range items {
		if name(item) == ref {
			matches = append(matches, id(item))
		}
	}
	switch len(matches) {
	case 0:
		return 0, fmt.Errorf("找不到 %s %q", kind, ref)
	case 1:
		return matches[0], nil
	}
	ids := make([]string, 0, len(matches))
	for _, m := range matches {
		ids = append(ids, strconv.FormatInt(m, 10))
	}
	return 0, fmt.Errorf("%s 名稱 %q 對應多筆資源（id: %s），請改用 id", kind, ref, strings.Join(ids, ", "))
}
