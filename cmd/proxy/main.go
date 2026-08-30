package main

import (
	"context"
	"loadbalancer/internal/balancer"
	"loadbalancer/internal/config"
	"loadbalancer/internal/health"
	"loadbalancer/internal/metrics"
	_ "net/http/pprof"
	"time"

	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	cfgMgr, err := config.NewManager("config.yaml")
	if err != nil {
		logger.Fatal("config load failed", zap.Error(err))
	}
	cfg := cfgMgr.Get()

	collector := metrics.NewCollector()

	var strategy balancer.Strategy
	switch cfg.Strategy {
	case "least_conn":
		strategy = &balancer.LeastConnections{}
	// case "consistent_hash":
	// 	strategy = &balancer.ConsistentHash{}
	// case "weighted_random":
	// 	strategy = &balancer.WeightedRandom{}
	default:
		strategy = &balancer.RoundRobin{}
	}
	pool := balancer.NewPool()
	for _, bc := range cfg.Backends {
		b, err := balancer.NewBackend(bc.URL, bc.Weight)
		if err != nil {
			logger.Fatal("bad backend url", zap.String("url", bc.URL), zap.Error(err))
		}
		pool.AddBackend(b)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	checker := health.NewChecker(
		time.Duration(cfg.Health.Interval)*time.Second,
		time.Duration(cfg.Health.Timeout)*time.Second,
		cfg.Health.Path,
		logger,
	)
	go checker.Start(ctx, pool)

	watcher := config.NewWatcher(cfgMgr, logger)
	go func() {
		if err := watcher.Start(ctx, "config.yaml"); err != nil {
			logger.Error("watcher exited", zap.Error(err))
		}
	}()

	if cfg.Fasthttp == false {
		runNetHttp(pool, logger, strategy, collector, cfg, cancel)
	} else {
		runFasthttp(cfg, pool, strategy, collector, logger, cancel)
	}

}
