package circuitbreaker

import (
	"sync"
)

// Manager manages multiple circuit breakers
type Manager struct {
	mu    sync.RWMutex
	breakers map[string]*Breaker
	config Config
}

// NewManager creates a new circuit breaker manager
func NewManager(config Config) *Manager {
	return &Manager{
		breakers: make(map[string]*Breaker),
		config:   config,
	}
}

// GetBreaker returns a circuit breaker for the given name
func (m *Manager) GetBreaker(name string) *Breaker {
	m.mu.RLock()
	if cb, ok := m.breakers[name]; ok {
		m.mu.RUnlock()
		return cb
	}
	m.mu.RUnlock()
	
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Double-check after acquiring write lock
	if cb, ok := m.breakers[name]; ok {
		return cb
	}
	
	cb := NewBreaker(name, m.config)
	m.breakers[name] = cb
	return cb
}

// RemoveBreaker removes a circuit breaker
func (m *Manager) RemoveBreaker(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.breakers, name)
}

// GetAllBreakers returns all circuit breakers
func (m *Manager) GetAllBreakers() map[string]*Breaker {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	breakers := make(map[string]*Breaker, len(m.breakers))
	for k, v := range m.breakers {
		breakers[k] = v
	}
	return breakers
}
