package main

import (
	"context"
	"encoding/json"
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

	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

func runFasthttp(cfg *config.Config, pool *balancer.Pool, strategy balancer.Strategy, collector *metrics.Collector, logger *zap.Logger, cancel context.CancelFunc) {
	proxy := server.NewFastProxy(pool, strategy, logger, collector)

	var handler fasthttp.RequestHandler = proxy.Handler

	handler = tracing.FastRequestID(handler)

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
	adminMux.Handle("/metrics", collector.Handler())
	adminMux.HandleFunc("/debug/pprof/", http.DefaultServeMux.ServeHTTP)

	dashboardHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeFile(w, r, "./html/dashboard.html")
	}
	adminMux.HandleFunc("/dashboard", dashboardHandler)

	adminMux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
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

	adminMux.HandleFunc("/api/backends", func(w http.ResponseWriter, r *http.Request) {
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

	adminMux.HandleFunc("/api/chaos/trip", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		host := r.URL.Query().Get("host")
		cb := proxy.CBManager().GetBreaker(host)
		cb.TripManually()
		collector.SetCircuitBreakerState(host, 2)
		collector.RecordCircuitBreakerTrip(host)
		json.NewEncoder(w).Encode(map[string]any{"status": "tripped", "host": host, "state": "open"})
	})

	adminMux.HandleFunc("/api/chaos/reset", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		host := r.URL.Query().Get("host")
		cb := proxy.CBManager().GetBreaker(host)
		cb.ResetManually()
		collector.SetCircuitBreakerState(host, 0)
		json.NewEncoder(w).Encode(map[string]any{"status": "reset", "host": host, "state": "closed"})
	})

	adminServer := &http.Server{
		Addr:    cfg.Server.AdminListen,
		Handler: adminMux,
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		logger.Info("shutting down gracefully...")

		logger.Info("draining connections...")
		shutdownCtx, done := context.WithTimeout(context.Background(), 30*time.Second)
		defer done()

		if err := fastServer.Shutdown(); err != nil {
			logger.Error("fasthttp shutdown error", zap.Error(err))
		}
		if err := adminServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("admin shutdown error", zap.Error(err))
		}

		cancel()
		logger.Info("shutdown complete")
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
