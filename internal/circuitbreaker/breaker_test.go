package circuitbreaker

import (
	"sync"
	"testing"
	"time"
)

func TestCircuitBreakerTransitions(t *testing.T) {
	cfg := Config{
		MaxRequests: 2,
		Interval:    1 * time.Second,
		Timeout:     50 * time.Millisecond,
		ReadyToTrip: func(counts Counts) bool {
			return counts.ConsecutiveFailures >= 2
		},
	}

	cb := NewBreaker("test-breaker", cfg)

	if cb.State() != StateClosed {
		t.Fatalf("expected state Closed, got %v", cb.State())
	}

	// 1st Failure
	done, err := cb.Allow()
	if err != nil {
		t.Fatalf("expected allowed, got %v", err)
	}
	done(false)
	if cb.State() != StateClosed {
		t.Fatalf("expected still Closed after 1 failure, got %v", cb.State())
	}

	// 2nd Failure -> should trip to Open
	done, err = cb.Allow()
	if err != nil {
		t.Fatalf("expected allowed, got %v", err)
	}
	done(false)

	if cb.State() != StateOpen {
		t.Fatalf("expected state Open after 2 failures, got %v", cb.State())
	}

	// Requests should now be rejected
	_, err = cb.Allow()
	if err != ErrCircuitOpen {
		t.Fatalf("expected ErrCircuitOpen, got %v", err)
	}

	// Wait for Timeout -> transitions to Half-Open
	time.Sleep(70 * time.Millisecond)
	if cb.State() != StateHalfOpen {
		t.Fatalf("expected state HalfOpen, got %v", cb.State())
	}

	// 2 consecutive successes in Half-Open -> should transition back to Closed
	done, err = cb.Allow()
	if err != nil {
		t.Fatalf("expected allowed in half-open, got %v", err)
	}
	done(true)

	done, err = cb.Allow()
	if err != nil {
		t.Fatalf("expected allowed in half-open, got %v", err)
	}
	done(true)

	if cb.State() != StateClosed {
		t.Fatalf("expected state Closed after recovery, got %v", cb.State())
	}
}

func TestManualTripAndReset(t *testing.T) {
	cb := NewBreaker("manual-test", DefaultConfig())
	if cb.State() != StateClosed {
		t.Fatalf("expected Closed")
	}

	cb.TripManually()
	if cb.State() != StateOpen {
		t.Fatalf("expected Open after TripManually")
	}

	cb.ResetManually()
	if cb.State() != StateClosed {
		t.Fatalf("expected Closed after ResetManually")
	}
}

func TestConcurrentBreakerAccess(t *testing.T) {
	cb := NewBreaker("concurrent-test", DefaultConfig())
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				done, err := cb.Allow()
				if err == nil {
					done(true)
				}
			}
		}()
	}
	wg.Wait()

	counts := cb.Counts()
	if counts.TotalSuccesses == 0 {
		t.Fatalf("expected recorded successes")
	}
}
