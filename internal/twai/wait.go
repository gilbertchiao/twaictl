package twai

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// WaitFor 每隔 interval 呼叫 check，直到 check 回傳 done=true、發生錯誤或 ctx 結束。
// ctx 逾時回傳 ErrWaitTimeout（exit 5）；ctx 被取消則回傳 context.Canceled。
func WaitFor(ctx context.Context, interval time.Duration, check func(ctx context.Context) (bool, error)) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return classifyWaitError(err)
		}
		done, err := check(ctx)
		if err != nil {
			// check 內部的 HTTP 呼叫共用同一個 ctx，若 ctx 在呼叫期間逾時或被取消，
			// check 通常會回傳包著 ctx.Err() 的錯誤（例如 *url.Error）。這種情況要
			// 優先歸類為等待逾時／取消，而不是把底層的網路錯誤原樣往外拋（cli 會
			// 誤判成 ExitNetwork 而非 ExitWaitTimeout）。
			//
			// 只重新分類由同一個 ctx 造成的錯誤（errors.Is(err, ctxErr) 為 true）：
			// 若 check 回傳的是真正的業務錯誤（例如 API 500），而 ctx 恰好在
			// check 回傳「之後」、這個判斷「之前」才逾時／取消，不可以把真正的
			// 錯誤誤判為等待逾時／取消而遮蔽掉。
			if ctxErr := ctx.Err(); ctxErr != nil && errors.Is(err, ctxErr) {
				return classifyWaitError(ctxErr)
			}
			return fmt.Errorf("輪詢狀態失敗: %w", err)
		}
		if done {
			return nil
		}
		select {
		case <-ctx.Done():
			return classifyWaitError(ctx.Err())
		case <-ticker.C:
		}
	}
}

func classifyWaitError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrWaitTimeout
	}
	return err
}
