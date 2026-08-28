package balancer

import (
	"net/url"
	"sync"
	"sync/atomic"
)

type Backend struct {
	URL    *url.URL
	Weight int

	mu          sync.RWMutex
	alive       bool
	connections int64
}

func NewBackend(rawURL string, weight int) (*Backend, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	if weight < 0 {
		weight = 1
	}
	return &Backend{URL: u, Weight: weight, alive: true}, nil
}

func (b *Backend) SetAlive(v bool) {
	b.mu.Lock()
	b.alive = v
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
}

func (b *Backend) DecrementConn() {
	atomic.AddInt64(&b.connections, -1)
}
