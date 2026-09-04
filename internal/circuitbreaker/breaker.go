package circuitbreaker

import (
	"errors"
	"sync"
	"time"
)

var (
	ErrCircuitOpen      = errors.New("circuit breaker is open")
	ErrTooManyRequests  = errors.New("too many requests in half-open state")
)

// State represents the circuit breaker state
type State int

const (
	StateClosed State = iota
	StateHalfOpen
	StateOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateHalfOpen:
		return "half-open"
	case StateOpen:
		return "open"
	default:
		return "unknown"
	}
}

// Config holds circuit breaker configuration
type Config struct {
	MaxRequests     uint32        // Maximum requests in half-open state
	Interval        time.Duration // Interval for counting errors
	Timeout         time.Duration // Time to wait before transitioning from open to half-open
	ReadyToTrip     func(counts Counts) bool
	OnStateChange   func(name string, from State, to State)
}

// DefaultConfig returns a default circuit breaker configuration
func DefaultConfig() Config {
	return Config{
		MaxRequests: 3,
		Interval:    60 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts Counts) bool {
			return counts.Requests > 10 && counts.ConsecutiveFailures > 5
		},
	}
}

// Counts holds the metrics for the circuit breaker
type Counts struct {
	Requests             uint32
	TotalSuccesses       uint32
	TotalFailures        uint32
	ConsecutiveSuccesses uint32
	ConsecutiveFailures  uint32
}

// Breaker implements the circuit breaker pattern
type Breaker struct {
	name          string
	config        Config
	mu            sync.Mutex
	state         State
	generation    uint64
	counts        Counts
	expiry        time.Time
}

// NewBreaker creates a new circuit breaker
func NewBreaker(name string, config Config) *Breaker {
	return &Breaker{
		name:   name,
		config: config,
		state:  StateClosed,
	}
}

// Name returns the name of the circuit breaker
func (cb *Breaker) Name() string {
	return cb.name
}

// State returns the current state of the circuit breaker
func (cb *Breaker) State() State {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	now := time.Now()
	state := cb.currentState(now)
	if state != cb.state {
		cb.setState(state, now)
	}
	return state
}

// Allow checks if the request is allowed to proceed
func (cb *Breaker) Allow() (func(success bool), error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	now := time.Now()
	state := cb.currentState(now)
	
	if state == StateOpen {
		return nil, ErrCircuitOpen
	}
	
	if state == StateHalfOpen && cb.counts.Requests >= cb.config.MaxRequests {
		return nil, ErrTooManyRequests
	}
	
	cb.counts.Requests++
	
	return func(success bool) {
		cb.onResult(success, now)
	}, nil
}

// currentState returns the current state without side effects
func (cb *Breaker) currentState(now time.Time) State {
	switch cb.state {
	case StateClosed:
		if !cb.expiry.IsZero() && cb.expiry.Before(now) {
			cb.resetCounts()
		}
	case StateOpen:
		if cb.expiry.Before(now) {
			return StateHalfOpen
		}
	}
	return cb.state
}

// setState transitions the circuit breaker to a new state
func (cb *Breaker) setState(newState State, now time.Time) {
	if cb.config.OnStateChange != nil {
		cb.config.OnStateChange(cb.name, cb.state, newState)
	}
	
	cb.state = newState
	cb.generation++
	
	switch newState {
	case StateClosed:
		cb.resetCounts()
	case StateOpen:
		cb.expiry = now.Add(cb.config.Timeout)
	case StateHalfOpen:
		cb.resetCounts()
	}
}

// resetCounts resets the counts
func (cb *Breaker) resetCounts() {
	cb.counts = Counts{}
	cb.expiry = time.Time{}
}

// onResult records the result of a request
func (cb *Breaker) onResult(success bool, now time.Time) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	if success {
		cb.onSuccess(now)
	} else {
		cb.onFailure(now)
	}
}

// onSuccess handles a successful request
func (cb *Breaker) onSuccess(now time.Time) {
	cb.counts.TotalSuccesses++
	cb.counts.ConsecutiveSuccesses++
	cb.counts.ConsecutiveFailures = 0
	
	if cb.state == StateHalfOpen && cb.counts.ConsecutiveSuccesses >= cb.config.MaxRequests {
		cb.setState(StateClosed, now)
	}
}

// onFailure handles a failed request
func (cb *Breaker) onFailure(now time.Time) {
	cb.counts.TotalFailures++
	cb.counts.ConsecutiveFailures++
	cb.counts.ConsecutiveSuccesses = 0
	
	if cb.config.ReadyToTrip(cb.counts) {
		cb.setState(StateOpen, now)
	}
}

// Counts returns the current counts
func (cb *Breaker) Counts() Counts {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.counts
}
