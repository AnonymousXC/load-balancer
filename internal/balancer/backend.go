package balancer

import (
	"net/url"
	"sync"
	"sync/atomic"
)

type Backend struct {
	URL    *url.URL
	Weight int

	mu                  sync.RWMutex
	alive               bool
	connections         int64
	totalRequests       int64
	failedRequests      int64
	consecutiveFailures int64
}

func NewBackend(rawURL string, weight int) (*Backend, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	if weight <= 0 {
		weight = 1
	}
	return &Backend{URL: u, Weight: weight, alive: true}, nil
}

func (b *Backend) SetAlive(v bool) {
	b.mu.Lock()
	b.alive = v
	if v {
		b.consecutiveFailures = 0
	}
	b.mu.Unlock()
}

func (b *Backend) IsAlive() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.alive
}

func (b *Backend) ActiveConnections() int64 {
	return atomic.LoadInt64(&b.connections)
}

func (b *Backend) IncrementConn() {
	atomic.AddInt64(&b.connections, 1)
	atomic.AddInt64(&b.totalRequests, 1)
}

func (b *Backend) DecrementConn() {
	atomic.AddInt64(&b.connections, -1)
}

func (b *Backend) RecordFailure() {
	atomic.AddInt64(&b.failedRequests, 1)
	atomic.AddInt64(&b.consecutiveFailures, 1)
}

func (b *Backend) RecordSuccess() {
	atomic.StoreInt64(&b.consecutiveFailures, 0)
}

func (b *Backend) TotalRequests() int64 {
	return atomic.LoadInt64(&b.totalRequests)
}

func (b *Backend) FailedRequests() int64 {
	return atomic.LoadInt64(&b.failedRequests)
}

func (b *Backend) ConsecutiveFailures() int64 {
	return atomic.LoadInt64(&b.consecutiveFailures)
}
