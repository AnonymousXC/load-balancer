package main

import (
	"context"
	"loadbalancer/internal/balancer"
	"loadbalancer/internal/config"
	"loadbalancer/internal/metrics"
	"loadbalancer/internal/middleware"
	"loadbalancer/internal/server"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

func runFasthttp(cfg *config.Config, pool *balancer.Pool, strategy balancer.Strategy, collector *metrics.Collector, logger *zap.Logger, cancel context.CancelFunc) {
	proxy := server.NewFastProxy(pool, strategy, logger, collector)

	var handler fasthttp.RequestHandler = proxy.Handler
	if cfg.RateLimit.RPS > 0 {
		rl := middleware.NewRateLimiter(rate.Limit(cfg.RateLimit.RPS), cfg.RateLimit.Burst)
		handler = middleware.FastChain(handler,
			middleware.FastRateLimit(rl),
			middleware.FastRecovery(logger),
			middleware.FastLogging(logger),
		)
	} else {
		handler = middleware.FastChain(handler,
			middleware.FastRecovery(logger),
			middleware.FastLogging(logger),
		)
	}

	fastServer := &fasthttp.Server{
		Handler:                       handler,
		ReadTimeout:                   time.Duration(cfg.Server.ReadTimeout) * time.Second,
		WriteTimeout:                  time.Duration(cfg.Server.WriteTimeout) * time.Second,
		IdleTimeout:                   time.Duration(cfg.Server.IdleTimeout) * time.Second,
		MaxConnsPerIP:                 10000,
		MaxRequestsPerConn:            100000,
		DisableKeepalive:              false,
		TCPKeepalive:                  true,
		ReadBufferSize:                16 * 1024,
		WriteBufferSize:               16 * 1024,
		DisableHeaderNamesNormalizing: true,
		ReduceMemoryUsage:             true,
	}

	adminMux := http.NewServeMux()
	adminMux.Handle("/metrics", promhttp.Handler())
	adminMux.HandleFunc("/debug/pprof/", http.DefaultServeMux.ServeHTTP)
	adminServer := &http.Server{
		Addr:    ":8081",
		Handler: adminMux,
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		logger.Info("shutting down gracefully...")
		cancel()

		shutdownCtx, done := context.WithTimeout(context.Background(), 30*time.Second)
		defer done()

		if err := fastServer.Shutdown(); err != nil {
			logger.Error("fasthttp shutdown error", zap.Error(err))
		}
		if err := adminServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("admin shutdown error", zap.Error(err))
		}
	}()

	go func() {
		logger.Info("admin server listening", zap.String("addr", adminServer.Addr))
		if err := adminServer.ListenAndServe(); err != http.ErrServerClosed {
			logger.Fatal("admin server failed", zap.Error(err))
		}
	}()

	logger.Info("fasthttp proxy listening", zap.String("addr", cfg.Server.Listen))
	if err := fastServer.ListenAndServe(cfg.Server.Listen); err != nil {
		logger.Fatal("proxy server failed", zap.Error(err))
	}
}
