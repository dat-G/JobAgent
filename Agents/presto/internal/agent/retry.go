package agent

import (
	"context"
	"math/rand"
	"time"
)

type RetryPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	Jitter      bool
}

func FastRetry() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: 2,
		BaseDelay:   80 * time.Millisecond,
		MaxDelay:    800 * time.Millisecond,
		Jitter:      true,
	}
}

func RunRetry() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: 3,
		BaseDelay:   120 * time.Millisecond,
		MaxDelay:    time.Second,
		Jitter:      true,
	}
}

func NoRetry() RetryPolicy {
	return RetryPolicy{MaxAttempts: 1}
}

func (p RetryPolicy) attempts() int {
	if p.MaxAttempts <= 0 {
		return 1
	}
	return p.MaxAttempts
}

func (p RetryPolicy) delay(attempt int) time.Duration {
	if attempt <= 0 || p.BaseDelay <= 0 {
		return 0
	}
	delay := p.BaseDelay << (attempt - 1)
	if p.MaxDelay > 0 && delay > p.MaxDelay {
		delay = p.MaxDelay
	}
	if p.Jitter && delay > 0 {
		spread := int64(delay / 3)
		if spread > 0 {
			delay += time.Duration(rand.Int63n(spread))
		}
	}
	return delay
}

func withRetry[T any](ctx context.Context, policy RetryPolicy, fn func(context.Context) (T, error)) (T, error) {
	var zero T
	var lastErr error

	for attempt := 1; attempt <= policy.attempts(); attempt++ {
		value, err := fn(ctx)
		if err == nil {
			return value, nil
		}
		lastErr = err
		if attempt == policy.attempts() {
			break
		}
		delay := policy.delay(attempt)
		if delay <= 0 {
			continue
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return zero, ctx.Err()
		case <-timer.C:
		}
	}

	return zero, lastErr
}
