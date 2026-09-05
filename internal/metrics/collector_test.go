package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCollectorMetricsRecording(t *testing.T) {
	c := NewCollector()

	c.RecordRequest("backend-1", http.StatusOK, 0.025, "GET", 512, 1024)
	c.RecordRequest("backend-1", http.StatusInternalServerError, 0.150, "POST", 128, 256)
	c.RecordRequest("backend-2", http.StatusOK, 0.010, "GET", 256, 512)

	c.SetActive(42)
	c.SetBackendActive("backend-1", 10)
	c.SetBackendHealth("backend-1", true)
	c.SetBackendHealth("backend-2", false)
	c.SetCircuitBreakerState("backend-2", 2)
	c.RecordCircuitBreakerTrip("backend-2")
	c.RecordCircuitBreakerRejection("backend-2")
	c.RecordRetry("backend-1", 1)
	c.RecordRateLimited("127.0.0.1")

	snap := c.GetSnapshot()
	if snap.ActiveConnections != 42 {
		t.Fatalf("expected 42 active connections, got: %d", snap.ActiveConnections)
	}

	if snap.StatusCodes["200"] != 2 {
		t.Fatalf("expected 2 status 200s, got: %d", snap.StatusCodes["200"])
	}
	if snap.StatusCodes["500"] != 1 {
		t.Fatalf("expected 1 status 500, got: %d", snap.StatusCodes["500"])
	}

	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	c.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from /metrics, got %d", rec.Code)
	}

	body := rec.Body.String()
	expectedMetrics := []string{
		"proxy_requests_total",
		"proxy_request_duration_seconds",
		"proxy_active_connections_aggregate",
		"proxy_backend_health",
		"proxy_circuit_breaker_state",
		"proxy_circuit_breaker_trips_total",
		"proxy_retries_total",
		"proxy_rate_limited_total",
	}

	for _, metricName := range expectedMetrics {
		if !strings.Contains(body, metricName) {
			t.Fatalf("expected Prometheus output to contain metric '%s'", metricName)
		}
	}
}
