package server

import (
	"loadbalancer/internal/balancer"
	"loadbalancer/internal/circuitbreaker"
	"loadbalancer/internal/errors"
	"loadbalancer/internal/metrics"
	"net/http"
	"time"

	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

type FastProxy struct {
	pool      *balancer.Pool
	strategy  balancer.Strategy
	logger    *zap.Logger
	metrics   *metrics.Collector
	cbManager *circuitbreaker.Manager
	clients   map[string]*fasthttp.HostClient
}

func NewFastProxy(pool *balancer.Pool, strategy balancer.Strategy, logger *zap.Logger, m *metrics.Collector) *FastProxy {
	cbConfig := circuitbreaker.DefaultConfig()
	cbConfig.OnStateChange = func(name string, from, to circuitbreaker.State) {
		logger.Warn("fastproxy: circuit breaker state changed",
			zap.String("backend", name),
			zap.String("from", from.String()),
			zap.String("to", to.String()),
		)
		m.SetCircuitBreakerState(name, int(to))
		if to == circuitbreaker.StateOpen {
			m.RecordCircuitBreakerTrip(name)
		}
	}
	cbManager := circuitbreaker.NewManager(cbConfig)

	return &FastProxy{
		pool:      pool,
		strategy:  strategy,
		logger:    logger,
		metrics:   m,
		cbManager: cbManager,
		clients:   make(map[string]*fasthttp.HostClient),
	}
}

func (p *FastProxy) CBManager() *circuitbreaker.Manager {
	return p.cbManager
}

func (p *FastProxy) Pool() *balancer.Pool {
	return p.pool
}

func (p *FastProxy) getClient(addr string) *fasthttp.HostClient {
	if c, ok := p.clients[addr]; ok {
		return c
	}
	c := &fasthttp.HostClient{
		Addr:                          addr,
		MaxConns:                      10000,
		MaxIdleConnDuration:           90 * time.Second,
		ReadTimeout:                   10 * time.Second,
		WriteTimeout:                  10 * time.Second,
		DisableHeaderNamesNormalizing: true,
		DisablePathNormalizing:        true,
		MaxConnDuration:               300 * time.Second,
		Dial:                          fasthttp.DialDualStack,
	}
	p.clients[addr] = c
	return c
}

func (p *FastProxy) selectBackend(ctx *fasthttp.RequestCtx) (*balancer.Backend, func(bool)) {
	var be *balancer.Backend
	if keyed, ok := p.strategy.(balancer.KeyedStrategy); ok {
		key := string(ctx.Request.Header.Peek("X-Forwarded-For"))
		if key == "" {
			key = ctx.RemoteIP().String()
		}
		be = keyed.SelectKey(p.pool, key)
	} else {
		be = p.strategy.Select(p.pool)
	}

	if be == nil {
		return nil, nil
	}

	cb := p.cbManager.GetBreaker(be.URL.Host)
	done, err := cb.Allow()
	if err == nil {
		return be, done
	}

	p.metrics.RecordCircuitBreakerRejection(be.URL.Host)

	for _, alt := range p.pool.HealthyBackends() {
		if alt.URL.Host == be.URL.Host {
			continue
		}
		altCB := p.cbManager.GetBreaker(alt.URL.Host)
		if altDone, altErr := altCB.Allow(); altErr == nil {
			return alt, altDone
		}
	}

	return nil, nil
}

func (p *FastProxy) Handler(ctx *fasthttp.RequestCtx) {
	path := string(ctx.Path())
	if path == "/test/dashboard" || path == "/dashboard" {
		fasthttp.ServeFile(ctx, "./html/dashboard.html")
		return
	}

	start := time.Now()

	be, cbDone := p.selectBackend(ctx)
	if be == nil {
		p.metrics.RecordRequest("none", http.StatusServiceUnavailable, 0, string(ctx.Method()), len(ctx.Request.Body()), 0)
		p.metrics.RecordError("none", "all_backends_unavailable")
		ctx.Error("Service Unavailable: No healthy backends available", fasthttp.StatusServiceUnavailable)
		return
	}

	be.IncrementConn()
	p.metrics.SetBackendActive(be.URL.Host, be.ActiveConnections())
	p.metrics.SetActive(p.metrics.GetActiveConnections() + 1)

	defer func() {
		be.DecrementConn()
		p.metrics.SetBackendActive(be.URL.Host, be.ActiveConnections())
		p.metrics.SetActive(p.metrics.GetActiveConnections() - 1)
	}()

	client := p.getClient(be.URL.Host)

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	req.Header.SetMethodBytes(ctx.Method())
	req.SetRequestURI(be.URL.Scheme + "://" + be.URL.Host + string(ctx.Path()))
	ctx.Request.Header.CopyTo(&req.Header)
	req.SetBody(ctx.Request.Body())

	req.Header.Set("X-Forwarded-For", ctx.RemoteIP().String())
	req.Header.Set("X-Real-Ip", ctx.RemoteIP().String())

	if err := client.Do(req, resp); err != nil {
		if cbDone != nil {
			cbDone(false)
		}
		be.RecordFailure()

		p.logger.Error("fastproxy error", zap.Error(err), zap.String("backend", be.URL.Host))
		p.metrics.RecordRequest(be.URL.Host, http.StatusBadGateway, 0, string(ctx.Method()), len(ctx.Request.Body()), 0)
		p.metrics.RecordError(be.URL.Host, "proxy_error")

		proxyErr := errors.NewBackendError(err, be.URL.Host)
		ctx.Error(proxyErr.Message, fasthttp.StatusBadGateway)
		return
	}

	if resp.StatusCode() >= 500 {
		if cbDone != nil {
			cbDone(false)
		}
		be.RecordFailure()
	} else {
		if cbDone != nil {
			cbDone(true)
		}
		be.RecordSuccess()
	}

	resp.Header.CopyTo(&ctx.Response.Header)
	ctx.SetStatusCode(resp.StatusCode())
	ctx.SetBody(resp.Body())

	p.metrics.RecordRequest(be.URL.Host, resp.StatusCode(), time.Since(start).Seconds(), string(ctx.Method()), len(ctx.Request.Body()), len(resp.Body()))
}
