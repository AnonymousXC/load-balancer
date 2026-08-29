package main

import (
	"context"
	"loadbalancer/internal/balancer"
	"loadbalancer/internal/config"
	"loadbalancer/internal/health"
	"loadbalancer/internal/metrics"
	"loadbalancer/internal/middleware"
	"loadbalancer/internal/server"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
	"golang.org/x/time/rate"
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

	proxy := server.NewProxy(pool, logger, strategy, collector)

	var handler http.Handler = proxy
	if cfg.RateLimit.RPS > 0 {
		rl := middleware.NewRateLimiter(rate.Limit(cfg.RateLimit.RPS), cfg.RateLimit.Burst)
		handler = rl.Middleware(handler)
	}
	handler = middleware.Chain(handler,
		middleware.Recovery(logger),
		middleware.Logging(logger),
	)

	mux := http.NewServeMux()
	mux.Handle("/metrics", collector.Handler())
	mux.HandleFunc("/debug/pprof/", http.DefaultServeMux.ServeHTTP)
	mux.Handle("/", handler)

	srv := &http.Server{
		Addr:         cfg.Server.Listen,
		Handler:      mux,
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(cfg.Server.IdleTimeout) * time.Second,
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		logger.Info("shutting down gracefully...")
		shutdownCtx, done := context.WithTimeout(context.Background(), 30*time.Second)
		defer done()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("shutdown error", zap.Error(err))
		}
		cancel()
	}()

	logger.Info("proxy listening", zap.String("addr", cfg.Server.Listen))
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		logger.Fatal("server failed", zap.Error(err))
	}
}
