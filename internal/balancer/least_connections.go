package balancer

type LeastConnections struct{}

func (s *LeastConnections) Select(pool *Pool) *Backend {
	nodes := pool.HealthyBackends()
	if len(nodes) == 0 {
		return nil
	}

	var best *Backend
	var min int64 = -1
	for _, b := range nodes {
		load := b.ActiveConnections()
		if min == -1 || load < min {
			best = b
			min = load
		}
	}
	return best
}
