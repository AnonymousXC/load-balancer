package metrics

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Collector struct {
	reg *prometheus.Registry

	requestTotal    *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
	activeConnsAgg  prometheus.Gauge
	backendConns    *prometheus.GaugeVec
	bytesInTotal    *prometheus.CounterVec
	bytesOutTotal   *prometheus.CounterVec
	requestSize     *prometheus.HistogramVec
	responseSize    *prometheus.HistogramVec
	errorRate       *prometheus.CounterVec

	backendHealth       *prometheus.GaugeVec
	healthCheckDuration *prometheus.HistogramVec
	consecutiveFailures *prometheus.GaugeVec

	circuitState      *prometheus.GaugeVec
	circuitTrips      *prometheus.CounterVec
	circuitRejections *prometheus.CounterVec
	retriesTotal      *prometheus.CounterVec
	retriesExhausted  *prometheus.CounterVec

	rateLimitedTotal *prometheus.CounterVec
	blockedRequests  *prometheus.CounterVec

	activeConns atomic.Int64
	startTime   time.Time

	mu          sync.RWMutex
	statusCodes map[string]int64
	backendReqs map[string]int64
}

func NewCollector() *Collector {
	reg := prometheus.NewRegistry()

	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	c := &Collector{
		reg:         reg,
		startTime:   time.Now(),
		statusCodes: make(map[string]int64),
		backendReqs: make(map[string]int64),

		requestTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "proxy_requests_total",
				Help: "Total number of HTTP requests proxied by backend, HTTP status code, and HTTP method",
			},
			[]string{"backend", "status", "method", "code"},
		),

		requestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "proxy_request_duration_seconds",
				Help:    "End-to-end request latency distribution in seconds",
				Buckets: []float64{0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0},
			},
			[]string{"backend", "status"},
		),

		activeConnsAgg: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "proxy_active_connections_aggregate",
				Help: "Total number of active in-flight connections across all backends",
			},
		),

		backendConns: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "proxy_backend_active_connections",
				Help: "Current active in-flight connections per backend",
			},
			[]string{"backend"},
		),

		bytesInTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "proxy_bytes_in_total",
				Help: "Total inbound request payload bytes received by backend",
			},
			[]string{"backend"},
		),

		bytesOutTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "proxy_bytes_out_total",
				Help: "Total outbound response payload bytes forwarded to clients by backend",
			},
			[]string{"backend"},
		),

		requestSize: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "proxy_request_size_bytes",
				Help:    "Inbound request payload size distribution in bytes",
				Buckets: prometheus.ExponentialBuckets(128, 4, 8), // 128B to ~2MB
			},
			[]string{"backend"},
		),

		responseSize: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "proxy_response_size_bytes",
				Help:    "Outbound response payload size distribution in bytes",
				Buckets: prometheus.ExponentialBuckets(128, 4, 8), // 128B to ~2MB
			},
			[]string{"backend"},
		),

		errorRate: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "proxy_errors_total",
				Help: "Total count of proxy errors categorized by backend and error category",
			},
			[]string{"backend", "error_type"},
		),

		backendHealth: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "proxy_backend_health",
				Help: "Current health status of upstream backend (1 = healthy, 0 = unhealthy)",
			},
			[]string{"backend"},
		),

		healthCheckDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "proxy_backend_health_check_duration_seconds",
				Help:    "Latency of health check probe requests in seconds",
				Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.0},
			},
			[]string{"backend"},
		),

		consecutiveFailures: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "proxy_backend_consecutive_failures",
				Help: "Current consecutive health probe failure count for backend",
			},
			[]string{"backend"},
		),

		circuitState: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "proxy_circuit_breaker_state",
				Help: "Circuit breaker status per backend (0 = Closed/Healthy, 1 = Half-Open, 2 = Open/Tripped)",
			},
			[]string{"backend"},
		),

		circuitTrips: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "proxy_circuit_breaker_trips_total",
				Help: "Cumulative number of times the circuit breaker tripped to Open for a backend",
			},
			[]string{"backend"},
		),

		circuitRejections: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "proxy_circuit_breaker_rejections_total",
				Help: "Total requests rejected immediately due to an Open circuit breaker",
			},
			[]string{"backend"},
		),

		retriesTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "proxy_retries_total",
				Help: "Total request retry attempts executed by backend and attempt index",
			},
			[]string{"backend", "attempt"},
		),

		retriesExhausted: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "proxy_retries_exhausted_total",
				Help: "Total requests where all retry attempts failed and were exhausted",
			},
			[]string{"backend"},
		),

		rateLimitedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "proxy_rate_limited_total",
				Help: "Total requests throttled and rejected with HTTP 429 Too Many Requests",
			},
			[]string{"client_ip"},
		),

		blockedRequests: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "proxy_blocked_requests_total",
				Help: "Total requests blocked by security middleware (e.g. IP blacklist, size limit)",
			},
			[]string{"reason"},
		),
	}

	reg.MustRegister(
		c.requestTotal,
		c.requestDuration,
		c.activeConnsAgg,
		c.backendConns,
		c.bytesInTotal,
		c.bytesOutTotal,
		c.requestSize,
		c.responseSize,
		c.errorRate,
		c.backendHealth,
		c.healthCheckDuration,
		c.consecutiveFailures,
		c.circuitState,
		c.circuitTrips,
		c.circuitRejections,
		c.retriesTotal,
		c.retriesExhausted,
		c.rateLimitedTotal,
		c.blockedRequests,
	)

	return c
}

func (c *Collector) RecordRequest(backend string, status int, dur float64, method string, requestSize, responseSize int) {
	st := http.StatusText(status)
	if st == "" {
		st = "unknown"
	}
	codeStr := fmt.Sprintf("%d", status)

	c.requestTotal.WithLabelValues(backend, st, method, codeStr).Inc()

	if dur > 0 {
		c.requestDuration.WithLabelValues(backend, st).Observe(dur)
	}
	if requestSize > 0 {
		c.requestSize.WithLabelValues(backend).Observe(float64(requestSize))
		c.bytesInTotal.WithLabelValues(backend).Add(float64(requestSize))
	}
	if responseSize > 0 {
		c.responseSize.WithLabelValues(backend).Observe(float64(responseSize))
		c.bytesOutTotal.WithLabelValues(backend).Add(float64(responseSize))
	}

	if status >= 500 {
		c.errorRate.WithLabelValues(backend, "5xx").Inc()
	} else if status >= 400 {
		c.errorRate.WithLabelValues(backend, "4xx").Inc()
	}

	c.mu.Lock()
	c.statusCodes[codeStr]++
	c.backendReqs[backend]++
	c.mu.Unlock()
}

func (c *Collector) SetActive(n int64) int64 {
	c.activeConnsAgg.Set(float64(n))
	c.activeConns.Store(n)
	return n
}

func (c *Collector) SetBackendActive(backend string, n int64) {
	c.backendConns.WithLabelValues(backend).Set(float64(n))
}

func (c *Collector) GetActiveConnections() int64 {
	return c.activeConns.Load()
}

func (c *Collector) SetBackendHealth(backend string, healthy bool) {
	val := 0.0
	if healthy {
		val = 1.0
	}
	c.backendHealth.WithLabelValues(backend).Set(val)
}

func (c *Collector) RecordHealthCheck(backend string, durationSeconds float64, consecutiveFails int) {
	if durationSeconds > 0 {
		c.healthCheckDuration.WithLabelValues(backend).Observe(durationSeconds)
	}
	c.consecutiveFailures.WithLabelValues(backend).Set(float64(consecutiveFails))
}

func (c *Collector) RecordError(backend string, errorType string) {
	c.errorRate.WithLabelValues(backend, errorType).Inc()
}

func (c *Collector) SetCircuitBreakerState(backend string, state int) {
	c.circuitState.WithLabelValues(backend).Set(float64(state))
}

func (c *Collector) RecordCircuitBreakerTrip(backend string) {
	c.circuitTrips.WithLabelValues(backend).Inc()
}

func (c *Collector) RecordCircuitBreakerRejection(backend string) {
	c.circuitRejections.WithLabelValues(backend).Inc()
}

func (c *Collector) RecordRetry(backend string, attempt int) {
	c.retriesTotal.WithLabelValues(backend, fmt.Sprintf("%d", attempt)).Inc()
}

func (c *Collector) RecordRetriesExhausted(backend string) {
	c.retriesExhausted.WithLabelValues(backend).Inc()
}

func (c *Collector) RecordRateLimited(clientIP string) {
	c.rateLimitedTotal.WithLabelValues(clientIP).Inc()
}

func (c *Collector) RecordBlockedRequest(reason string) {
	c.blockedRequests.WithLabelValues(reason).Inc()
}

func (c *Collector) Handler() http.Handler {
	return promhttp.HandlerFor(c.reg, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
}

func (c *Collector) Registry() *prometheus.Registry {
	return c.reg
}

type TelemetrySnapshot struct {
	UptimeSeconds     float64          `json:"uptime_seconds"`
	ActiveConnections int64            `json:"active_connections"`
	StatusCodes       map[string]int64 `json:"status_codes"`
	BackendRequests   map[string]int64 `json:"backend_requests"`
}

func (c *Collector) GetSnapshot() TelemetrySnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()

	scCopy := make(map[string]int64, len(c.statusCodes))
	for k, v := range c.statusCodes {
		scCopy[k] = v
	}

	beCopy := make(map[string]int64, len(c.backendReqs))
	for k, v := range c.backendReqs {
		beCopy[k] = v
	}

	return TelemetrySnapshot{
		UptimeSeconds:     time.Since(c.startTime).Seconds(),
		ActiveConnections: c.activeConns.Load(),
		StatusCodes:       scCopy,
		BackendRequests:   beCopy,
	}
}
