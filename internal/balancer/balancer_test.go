package balancer

import (
	"testing"
)

func TestRoundRobin(t *testing.T) {
	pool := NewPool()
	b1, _ := NewBackend("http://127.0.0.1:8081", 1)
	b2, _ := NewBackend("http://127.0.0.1:8082", 1)
	b3, _ := NewBackend("http://127.0.0.1:8083", 1)
	pool.AddBackend(b1)
	pool.AddBackend(b2)
	pool.AddBackend(b3)

	rr := &RoundRobin{}

	seq := []string{
		rr.Select(pool).URL.String(),
		rr.Select(pool).URL.String(),
		rr.Select(pool).URL.String(),
		rr.Select(pool).URL.String(),
	}

	if seq[0] != "http://127.0.0.1:8081" || seq[1] != "http://127.0.0.1:8082" || seq[2] != "http://127.0.0.1:8083" || seq[3] != "http://127.0.0.1:8081" {
		t.Fatalf("unexpected round robin sequence: %v", seq)
	}

	b2.SetAlive(false)
	next := rr.Select(pool).URL.String()
	if next == "http://127.0.0.1:8082" {
		t.Fatalf("selected unhealthy backend: %s", next)
	}
}

func TestWeightedRoundRobin(t *testing.T) {
	pool := NewPool()
	b1, _ := NewBackend("http://127.0.0.1:8081", 1)
	b2, _ := NewBackend("http://127.0.0.1:8082", 3)
	pool.AddBackend(b1)
	pool.AddBackend(b2)

	wrr := NewWeightedRoundRobin()
	counts := make(map[string]int)

	for i := 0; i < 400; i++ {
		b := wrr.Select(pool)
		counts[b.URL.String()]++
	}

	if counts["http://127.0.0.1:8082"] < counts["http://127.0.0.1:8081"]*2 {
		t.Fatalf("weighted distribution mismatch: %v", counts)
	}
}

func TestLeastConnections(t *testing.T) {
	pool := NewPool()
	b1, _ := NewBackend("http://127.0.0.1:8081", 1)
	b2, _ := NewBackend("http://127.0.0.1:8082", 1)
	pool.AddBackend(b1)
	pool.AddBackend(b2)

	b1.IncrementConn()
	b1.IncrementConn()
	b2.IncrementConn()

	lc := &LeastConnections{}
	selected := lc.Select(pool)
	if selected.URL.String() != "http://127.0.0.1:8082" {
		t.Fatalf("expected backend with least conn (b2), got: %s", selected.URL.String())
	}
}

func TestConsistentHash(t *testing.T) {
	pool := NewPool()
	b1, _ := NewBackend("http://127.0.0.1:8081", 1)
	b2, _ := NewBackend("http://127.0.0.1:8082", 1)
	b3, _ := NewBackend("http://127.0.0.1:8083", 1)
	pool.AddBackend(b1)
	pool.AddBackend(b2)
	pool.AddBackend(b3)

	ch := NewConsistentHash(100)

	key := "user-session-12345"
	target1 := ch.SelectKey(pool, key)
	target2 := ch.SelectKey(pool, key)

	if target1.URL.String() != target2.URL.String() {
		t.Fatalf("consistent hash key affinity violated: %s vs %s", target1.URL.String(), target2.URL.String())
	}

	n := ch.Select(pool)
	if n == nil {
		t.Fatal("expected valid backend from Select")
	}
}
