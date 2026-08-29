package metrics

import (
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
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
