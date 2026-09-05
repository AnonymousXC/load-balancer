package server

import (
	"loadbalancer/internal/balancer"
	"loadbalancer/internal/metrics"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"
)

func TestProxyRoutingAndFailover(t *testing.T) {
	var s1Hits, s2Hits int32

	s1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&s1Hits, 1)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("from server 1"))
	}))
	defer s1.Close()

	s2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&s2Hits, 1)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer s2.Close()

	pool := balancer.NewPool()
	b1, err := balancer.NewBackend(s1.URL, 1)
	if err != nil {
		t.Fatal(err)
	}
	b2, err := balancer.NewBackend(s2.URL, 1)
	if err != nil {
		t.Fatal(err)
	}
	pool.AddBackend(b1)
	pool.AddBackend(b2)

	logger := zap.NewNop()
	collector := metrics.NewCollector()
	strategy := &balancer.RoundRobin{}

	proxy := NewProxy(pool, logger, strategy, collector)

	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()
		proxy.ServeHTTP(rec, req)
	}

	if atomic.LoadInt32(&s1Hits) == 0 {
		t.Fatal("expected hits on server 1")
	}

	snap := collector.GetSnapshot()
	if snap.StatusCodes["200"] == 0 {
		t.Fatalf("expected recorded 200 OK statuses in metrics")
	}
}

func TestProxyNoBackends(t *testing.T) {
	pool := balancer.NewPool()
	logger := zap.NewNop()
	collector := metrics.NewCollector()
	strategy := &balancer.RoundRobin{}

	proxy := NewProxy(pool, logger, strategy, collector)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 Service Unavailable, got: %d", rec.Code)
	}
}
