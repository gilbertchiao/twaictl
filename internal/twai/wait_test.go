package twai

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWaitForReturnsWhenCheckDone(t *testing.T) {
	calls := 0
	err := WaitFor(context.Background(), time.Millisecond, func(context.Context) (bool, error) {
		calls++
		return calls == 3, nil
	})
	require.NoError(t, err)
	assert.Equal(t, 3, calls)
}

func TestWaitForPropagatesCheckError(t *testing.T) {
	boom := errors.New("boom")
	err := WaitFor(context.Background(), time.Millisecond, func(context.Context) (bool, error) { return false, boom })
	assert.ErrorIs(t, err, boom)
}

func TestWaitForTimeoutIsErrWaitTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := WaitFor(ctx, time.Millisecond, func(context.Context) (bool, error) { return false, nil })
	assert.ErrorIs(t, err, ErrWaitTimeout)
}

func TestWaitForCancelIsContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := WaitFor(ctx, time.Millisecond, func(context.Context) (bool, error) { return false, nil })
	assert.ErrorIs(t, err, context.Canceled)
}

func TestWaitForDeadlineDuringCheckIsErrWaitTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	err := WaitFor(ctx, time.Millisecond, func(ctx context.Context) (bool, error) {
		<-ctx.Done() // 模擬 HTTP 呼叫因同一個 ctx 逾時而失敗
		return false, fmt.Errorf("Get \"https://gw\": %w", ctx.Err())
	})
	assert.ErrorIs(t, err, ErrWaitTimeout)
	assert.NotErrorIs(t, err, context.DeadlineExceeded, "對外只暴露 ErrWaitTimeout")
}

func TestWaitForCancelDuringCheckIsContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	err := WaitFor(ctx, time.Millisecond, func(ctx context.Context) (bool, error) {
		cancel()
		return false, fmt.Errorf("Get \"https://gw\": %w", ctx.Err())
	})
	assert.ErrorIs(t, err, context.Canceled)
}

func TestWaitForKeepsRealErrorWhenDeadlineRaces(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	boom := errors.New("API 回應 500")
	err := WaitFor(ctx, time.Millisecond, func(context.Context) (bool, error) {
		cancel() // 模擬 check 回傳真正錯誤的同時 ctx 也結束
		return false, boom
	})
	assert.ErrorIs(t, err, boom, "真正的錯誤不可被 ctx 狀態遮蔽")
	assert.False(t, errors.Is(err, context.Canceled))
}
