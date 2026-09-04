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

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := c.client.Do(req)
	if err != nil {
		c.setAlive(b, false)
		return
	}
	defer resp.Body.Close()

	alive := resp.StatusCode >= 200 && resp.StatusCode < 300
	c.setAlive(b, alive)
}

func (c *Checker) setAlive(b *balancer.Backend, alive bool) {
	if b.IsAlive() == alive {
		return
	}
	b.SetAlive(alive)
	c.logger.Info("backend health changed",
		zap.String("url", b.URL.String()),
		zap.Bool("alive", alive))

	if c.metrics != nil {
		c.metrics.SetBackendHealth(b.URL.Host, alive)
	}
}
