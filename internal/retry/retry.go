package retry

import (
	"context"
	"math"
	"math/rand"
	"time"
)

type Config struct {
	MaxRetries      int
	InitialInterval time.Duration
	MaxInterval     time.Duration
	Multiplier      float64
	Jitter          float64
	Retryable       func(error) bool
	OnRetry         func(attempt int, err error)
}

func DefaultConfig() Config {
	return Config{
		MaxRetries:      3,
		InitialInterval: 100 * time.Millisecond,
		MaxInterval:     10 * time.Second,
		Multiplier:      2.0,
		Jitter:          0.1,
		Retryable:       DefaultRetryable,
	}
}

func DefaultRetryable(err error) bool {
	if err == nil {
		return false
	}
	return true
}

func Retry(ctx context.Context, fn func() error, config Config) error {
	var lastErr error

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(calculateBackoff(attempt, config)):
			}

			if config.OnRetry != nil {
				config.OnRetry(attempt, lastErr)
			}
		}

		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err
		if !config.Retryable(err) {
			return err
		}
	}

	return lastErr
}

func calculateBackoff(attempt int, config Config) time.Duration {
	if attempt == 0 {
		return 0
	}

	backoff := float64(config.InitialInterval) * math.Pow(config.Multiplier, float64(attempt-1))

	if config.Jitter > 0 {
		jitterRange := backoff * config.Jitter
		jitter := (rand.Float64()*2 - 1) * jitterRange
		backoff += jitter
	}

	if backoff > float64(config.MaxInterval) {
		backoff = float64(config.MaxInterval)
	}

	return time.Duration(backoff)
}

func RetryWithData[T any](ctx context.Context, fn func() (T, error), config Config) (T, error) {
	var result T
	var lastErr error

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return result, ctx.Err()
			case <-time.After(calculateBackoff(attempt, config)):
			}

			if config.OnRetry != nil {
				config.OnRetry(attempt, lastErr)
			}
		}

		data, err := fn()
		if err == nil {
			return data, nil
		}

		lastErr = err
		if !config.Retryable(err) {
			return result, err
		}
	}

	return result, lastErr
}
