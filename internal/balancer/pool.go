package balancer

import (
	"sync"
)

type Strategy interface {
	Select(pool *Pool) *Backend
}

type Pool struct {
	mu       sync.RWMutex
	backends []*Backend
}

func NewPool() *Pool {
	return &Pool{}
}

func (p *Pool) AddBackend(b *Backend) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.backends = append(p.backends, b)
}

func (p *Pool) SetBackends(backends []*Backend) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.backends = make([]*Backend, len(backends))
	copy(p.backends, backends)
}

func (p *Pool) Backends() []*Backend {
	p.mu.RLock()
	defer p.mu.RUnlock()

	out := make([]*Backend, len(p.backends))
	copy(out, p.backends)

	return out
}

func (p *Pool) HealthyBackends() []*Backend {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var res []*Backend
	for _, b := range p.backends {
		if b.IsAlive() {
			res = append(res, b)
		}
	}

	return res
}

func (p *Pool) GetBackendByHost(host string) *Backend {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for _, b := range p.backends {
		if b.URL.Host == host || b.URL.String() == host {
			return b
		}
	}
	return nil
}
