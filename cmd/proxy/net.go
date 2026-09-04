package main

import (
	"context"
	"loadbalancer/internal/balancer"
	"loadbalancer/internal/config"
	"loadbalancer/internal/metrics"
	"loadbalancer/internal/middleware"
	"loadbalancer/internal/server"
	"loadbalancer/internal/tracing"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

func runNetHttp(
	pool *balancer.Pool,
	logger *zap.Logger,
	strategy balancer.Strategy,
	collector *metrics.Collector,
	cfg *config.Config,
	cancel context.CancelFunc,
) {
	proxy := server.NewProxy(pool, logger, strategy, collector)

	var handler http.Handler = proxy

	handler = tracing.RequestIDMiddleware(handler)

	if cfg.RateLimit.RPS > 0 {
		rl := middleware.NewRateLimiter(rate.Limit(cfg.RateLimit.RPS), cfg.RateLimit.Burst)
		handler = rl.Middleware(handler)
	}

	securityConfig := middleware.DefaultSecurityConfig()
	if cfg.Security.EnableCORS {
		securityConfig.EnableCORS = true
		securityConfig.AllowedOrigins = cfg.Security.AllowedOrigins
		securityConfig.AllowedMethods = cfg.Security.AllowedMethods
		securityConfig.AllowedHeaders = cfg.Security.AllowedHeaders
	}
	handler = middleware.SecurityMiddleware(securityConfig)(handler)

	if len(cfg.Security.IPWhitelist) > 0 || len(cfg.Security.IPBlacklist) > 0 {
		ipConfig := middleware.DefaultIPFilterConfig()
		ipConfig.Whitelist = cfg.Security.IPWhitelist
		ipConfig.Blacklist = cfg.Security.IPBlacklist
		handler = middleware.IPFilterMiddleware(ipConfig)(handler)
	}

	if cfg.Security.MaxRequestSize > 0 || cfg.Security.MaxResponseSize > 0 {
		sizeConfig := middleware.DefaultSizeLimitConfig()
		sizeConfig.MaxRequestSize = cfg.Security.MaxRequestSize
		sizeConfig.MaxResponseSize = cfg.Security.MaxResponseSize
		handler = middleware.SizeLimitMiddleware(sizeConfig)(handler)
	}

	handler = middleware.Chain(handler,
		middleware.Recovery(logger),
		middleware.Logging(logger),
	)

	mux := http.NewServeMux()
	mux.Handle("/metrics", collector.Handler())
	mux.HandleFunc("/debug/pprof/", http.DefaultServeMux.ServeHTTP)
	mux.Handle("/", handler)
	mux.HandleFunc("/test/dashboard", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./html/dashboard.html")
	})

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

		logger.Info("draining connections...")
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("shutdown error", zap.Error(err))
		}

		cancel()
		logger.Info("shutdown complete")
	}()

	logger.Info("proxy listening", zap.String("addr", cfg.Server.Listen))
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		logger.Fatal("server failed", zap.Error(err))
	}
}
