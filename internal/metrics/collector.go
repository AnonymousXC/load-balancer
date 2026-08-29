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

	activeConns atomic.Int64
}

func NewCollector() *Collector {
	c := &Collector{
		requestTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "proxy_requests_total",
				Help: "Total Requests",
			},
			[]string{"backend", "status"},
		),
		requestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "proxy_request_duration_seconds",
				Help:    "Request Latency",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"backend"},
		),
		activeGuage: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "proxy_active_connections",
				Help: "Active Conns",
			},
		),
	}
	prometheus.MustRegister(c.requestTotal, c.requestDuration, c.activeGuage)
	return c
}

func (c *Collector) RecordRequest(backend string, status int, dur float64) {
	st := http.StatusText(status)
	if st == "" {
		st = "unknown"
	}
	c.requestTotal.WithLabelValues(backend, st).Inc()
	if dur > 0 {
		c.requestDuration.WithLabelValues(backend).Observe(dur)
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

func (c *Collector) Handler() http.Handler {
	return promhttp.Handler()
}
