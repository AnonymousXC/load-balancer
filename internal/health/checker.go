package health

import (
	"context"
	"loadbalancer/internal/balancer"
	"loadbalancer/internal/metrics"
	"net/http"
	"time"

	"go.uber.org/zap"
)

type Checker struct {
	interval time.Duration
	timeout  time.Duration
	path     string
	client   *http.Client
	logger   *zap.Logger
	metrics  *metrics.Collector
}

func NewChecker(interval, timeout time.Duration, path string, logger *zap.Logger, metrics *metrics.Collector) *Checker {
	return &Checker{
		interval: interval,
		timeout:  timeout,
		path:     path,
		client:   &http.Client{Timeout: timeout},
		logger:   logger,
		metrics:  metrics,
	}
}

func (c *Checker) Start(ctx context.Context, pool *balancer.Pool) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	c.checkAll(pool)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.checkAll(pool)
		}
	}
}

func (c *Checker) checkAll(pool *balancer.Pool) {
	for _, b := range pool.Backends() {
		go c.probe(b)
	}
}

func (c *Checker) probe(b *balancer.Backend) {
	url := b.URL.String() + c.path
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		c.setAlive(b, false, time.Since(start).Seconds())
		return
	}

	resp, err := c.client.Do(req)
	dur := time.Since(start).Seconds()

	if err != nil {
		c.setAlive(b, false, dur)
		return
	}
	defer resp.Body.Close()

	alive := resp.StatusCode >= 200 && resp.StatusCode < 300
	c.setAlive(b, alive, dur)
}

func (c *Checker) setAlive(b *balancer.Backend, alive bool, probeDuration float64) {
	prev := b.IsAlive()
	b.SetAlive(alive)

	if !alive {
		b.RecordFailure()
	} else {
		b.RecordSuccess()
	}

	if c.metrics != nil {
		c.metrics.SetBackendHealth(b.URL.Host, alive)
		c.metrics.RecordHealthCheck(b.URL.Host, probeDuration, int(b.ConsecutiveFailures()))
	}

	if prev != alive {
		c.logger.Info("backend health status changed",
			zap.String("url", b.URL.String()),
			zap.Bool("alive", alive),
			zap.Float64("probe_latency_sec", probeDuration),
		)
	}
}
