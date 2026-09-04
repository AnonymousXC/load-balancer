package metrics

import (
	"net/http"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Collector struct {
	requestTotal    *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
	activeGuage     prometheus.Gauge
	backendHealth   *prometheus.GaugeVec
	requestSize     *prometheus.HistogramVec
	responseSize    *prometheus.HistogramVec
	errorRate       *prometheus.CounterVec

	activeConns atomic.Int64
}

func NewCollector() *Collector {
	c := &Collector{
		requestTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "proxy_requests_total",
				Help: "Total number of requests proxied",
			},
			[]string{"backend", "status", "method"},
		),
		requestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "proxy_request_duration_seconds",
				Help:    "Request latency in seconds",
				Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
			},
			[]string{"backend", "status"},
		),
		activeGuage: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "proxy_active_connections",
				Help: "Current number of active connections",
			},
		),
		backendHealth: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "proxy_backend_health",
				Help: "Backend health status (1 = healthy, 0 = unhealthy)",
			},
			[]string{"backend"},
		),
		requestSize: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "proxy_request_size_bytes",
				Help:    "Request size in bytes",
				Buckets: prometheus.ExponentialBuckets(100, 10, 7), // 100B to 10MB
			},
			[]string{"backend"},
		),
		responseSize: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "proxy_response_size_bytes",
				Help:    "Response size in bytes",
				Buckets: prometheus.ExponentialBuckets(100, 10, 7), // 100B to 10MB
			},
			[]string{"backend"},
		),
		errorRate: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "proxy_errors_total",
				Help: "Total number of errors",
			},
			[]string{"backend", "error_type"},
		),
	}
	prometheus.MustRegister(c.requestTotal, c.requestDuration, c.activeGuage, c.backendHealth, c.requestSize, c.responseSize, c.errorRate)
	return c
}

func (c *Collector) RecordRequest(backend string, status int, dur float64, method string, requestSize, responseSize int) {
	st := http.StatusText(status)
	if st == "" {
		st = "unknown"
	}
	c.requestTotal.WithLabelValues(backend, st, method).Inc()
	if dur > 0 {
		c.requestDuration.WithLabelValues(backend, st).Observe(dur)
	}
	if requestSize > 0 {
		c.requestSize.WithLabelValues(backend).Observe(float64(requestSize))
	}
	if responseSize > 0 {
		c.responseSize.WithLabelValues(backend).Observe(float64(responseSize))
	}

	if status >= 500 {
		c.errorRate.WithLabelValues(backend, "5xx").Inc()
	} else if status >= 400 {
		c.errorRate.WithLabelValues(backend, "4xx").Inc()
	}
}

func (c *Collector) SetActive(n int64) int64 {
	c.activeGuage.Set(float64(n))
	c.activeConns.Store(n)
	return n
}

func (c *Collector) GetActiveConnections() int64 {
	return c.activeConns.Load()
}

func (c *Collector) SetBackendHealth(backend string, healthy bool) {
	value := 0.0
	if healthy {
		value = 1.0
	}
	c.backendHealth.WithLabelValues(backend).Set(value)
}

func (c *Collector) RecordError(backend string, errorType string) {
	c.errorRate.WithLabelValues(backend, errorType).Inc()
}

func (c *Collector) Handler() http.Handler {
	return promhttp.Handler()
}
