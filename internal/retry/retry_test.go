package retry

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetrySuccessOnSecondAttempt(t *testing.T) {
	var attempts int32
	cfg := Config{
		MaxRetries:      3,
		InitialInterval: 10 * time.Millisecond,
		MaxInterval:     50 * time.Millisecond,
		Multiplier:      2.0,
		Jitter:          0,
		Retryable:       DefaultRetryable,
	}

	res, err := RetryWithData(context.Background(), func() (string, error) {
		att := atomic.AddInt32(&attempts, 1)
		if att < 2 {
			return "", errors.New("temporary error")
		}
		return "success", nil
	}, cfg)

	if err != nil {
		t.Fatalf("expected retry to succeed, got: %v", err)
	}
	if res != "success" {
		t.Fatalf("expected 'success', got: %s", res)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got: %d", attempts)
	}
}

func TestRetryExhausted(t *testing.T) {
	var attempts int32
	cfg := Config{
		MaxRetries:      2,
		InitialInterval: 5 * time.Millisecond,
		MaxInterval:     20 * time.Millisecond,
		Multiplier:      1.5,
		Jitter:          0,
		Retryable:       DefaultRetryable,
	}

	err := Retry(context.Background(), func() error {
		atomic.AddInt32(&attempts, 1)
		return errors.New("permanent failure")
	}, cfg)

	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
	if attempts != 3 {
		t.Fatalf("expected 3 total attempts, got: %d", attempts)
	}
}
