package balancer

import (
	"sync/atomic"
)

type RoundRobin struct {
	next atomic.Uint64
}

func (rr *RoundRobin) Select(pool *Pool) *Backend {
	nodes := pool.HealthyBackends()
	if len(nodes) == 0 {
		return nil
	}

	next := rr.next.Add(1)
	idx := (next - 1) % uint64(len(nodes))
	return nodes[idx]
}
