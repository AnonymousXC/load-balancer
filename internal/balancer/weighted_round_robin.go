package balancer

import (
	"sync/atomic"
)

type WeightedRoundRobin struct {
	currentWeight atomic.Int64
}

func NewWeightedRoundRobin() *WeightedRoundRobin {
	return &WeightedRoundRobin{}
}

func (wrr *WeightedRoundRobin) Select(pool *Pool) *Backend {
	nodes := pool.HealthyBackends()
	if len(nodes) == 0 {
		return nil
	}

	totalWeight := 0
	for _, node := range nodes {
		totalWeight += node.Weight
	}

	if totalWeight == 0 {
		current := wrr.currentWeight.Load()
		wrr.currentWeight.Store(current + 1)
		idx := int(current) % len(nodes)
		return nodes[idx]
	}

	current := wrr.currentWeight.Load()
	wrr.currentWeight.Store(current + 1)

	runningWeight := 0
	targetWeight := int(current) % totalWeight

	for _, node := range nodes {
		runningWeight += node.Weight
		if targetWeight < runningWeight {
			return node
		}
	}

	return nodes[len(nodes)-1]
}
