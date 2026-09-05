package main

import (
	"context"
	"encoding/json"
	"loadbalancer/internal/balancer"
	"loadbalancer/internal/config"
	"loadbalancer/internal/metrics"
	"loadbalancer/internal/middleware"
	"loadbalancer/internal/server"
	tlsPkg "loadbalancer/internal/tls"
	"loadbalancer/internal/tracing"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

type BackendStatusResponse struct {
	URL                 string `json:"url"`
	Host                string `json:"host"`
	Weight              int    `json:"weight"`
	Alive               bool   `json:"alive"`
	ActiveConnections   int64  `json:"active_connections"`
	TotalRequests       int64  `json:"total_requests"`
	FailedRequests      int64  `json:"failed_requests"`
	ConsecutiveFailures int64  `json:"consecutive_failures"`
	CircuitBreakerState string `json:"circuit_breaker_state"`
}

func setupAdminRoutes(mux *http.ServeMux, proxy *server.Proxy, collector *metrics.Collector, cfg *config.Config) {
	mux.Handle("/metrics", collector.Handler())
	mux.HandleFunc("/debug/pprof/", http.DefaultServeMux.ServeHTTP)

	dashboardHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeFile(w, r, "./html/dashboard.html")
	}
	mux.HandleFunc("/dashboard", dashboardHandler)

	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		snap := collector.GetSnapshot()
		json.NewEncoder(w).Encode(map[string]any{
			"strategy":           cfg.Strategy,
			"fasthttp":           cfg.Fasthttp,
			"uptime_seconds":     snap.UptimeSeconds,
			"active_connections": snap.ActiveConnections,
			"status_codes":       snap.StatusCodes,
			"backend_requests":   snap.BackendRequests,
		})
	})

	mux.HandleFunc("/api/backends", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var statuses []BackendStatusResponse
		for _, b := range proxy.Pool().Backends() {
			cb := proxy.CBManager().GetBreaker(b.URL.Host)
			statuses = append(statuses, BackendStatusResponse{
				URL:                 b.URL.String(),
				Host:                b.URL.Host,
				Weight:              b.Weight,
				Alive:               b.IsAlive(),
				ActiveConnections:   b.ActiveConnections(),
				TotalRequests:       b.TotalRequests(),
				FailedRequests:      b.FailedRequests(),
				ConsecutiveFailures: b.ConsecutiveFailures(),
				CircuitBreakerState: cb.State().String(),
			})
		}
		json.NewEncoder(w).Encode(statuses)
	})

	mux.HandleFunc("/api/chaos/trip", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		host := r.URL.Query().Get("host")
		if host == "" {
			http.Error(w, `{"error":"missing host parameter"}`, http.StatusBadRequest)
			return
		}
		cb := proxy.CBManager().GetBreaker(host)
		cb.TripManually()
		collector.SetCircuitBreakerState(host, 2)
		collector.RecordCircuitBreakerTrip(host)
		json.NewEncoder(w).Encode(map[string]any{"status": "tripped", "host": host, "state": "open"})
	})

	mux.HandleFunc("/api/chaos/reset", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		host := r.URL.Query().Get("host")
		if host == "" {
			http.Error(w, `{"error":"missing host parameter"}`, http.StatusBadRequest)
			return
		}
		cb := proxy.CBManager().GetBreaker(host)
		cb.ResetManually()
		collector.SetCircuitBreakerState(host, 0)
		json.NewEncoder(w).Encode(map[string]any{"status": "reset", "host": host, "state": "closed"})
	})

	mux.HandleFunc("/api/chaos/backend", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		host := r.URL.Query().Get("host")
		b := proxy.Pool().GetBackendByHost(host)
		if b == nil {
			http.Error(w, `{"error":"backend not found"}`, http.StatusNotFound)
			return
		}
		newAlive := !b.IsAlive()
		b.SetAlive(newAlive)
		collector.SetBackendHealth(b.URL.Host, newAlive)
		json.NewEncoder(w).Encode(map[string]any{"host": host, "alive": newAlive})
	})
}

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
	setupAdminRoutes(mux, proxy, collector, cfg)
	mux.Handle("/", handler)

	srv := &http.Server{
		Addr:         cfg.Server.Listen,
		Handler:      mux,
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(cfg.Server.IdleTimeout) * time.Second,
	}

	if cfg.TLS.Enabled && cfg.TLS.CertFile != "" && cfg.TLS.KeyFile != "" {
		tlsConfig, err := tlsPkg.LoadTLSConfig(tlsPkg.Config{
			CertFile:   cfg.TLS.CertFile,
			KeyFile:    cfg.TLS.KeyFile,
			CAFile:     cfg.TLS.CAFile,
			MinVersion: cfg.TLS.MinVersion,
			MaxVersion: cfg.TLS.MaxVersion,
		})
		if err != nil {
			logger.Fatal("failed to load TLS config", zap.Error(err))
		}
		srv.TLSConfig = tlsConfig
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
	if cfg.TLS.Enabled && cfg.TLS.CertFile != "" && cfg.TLS.KeyFile != "" {
		if err := srv.ListenAndServeTLS(cfg.TLS.CertFile, cfg.TLS.KeyFile); err != http.ErrServerClosed {
			logger.Fatal("tls server failed", zap.Error(err))
		}
	} else {
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			logger.Fatal("server failed", zap.Error(err))
		}
	}
}
