package server

import (
	"loadbalancer/internal/balancer"
	"loadbalancer/internal/errors"
	"loadbalancer/internal/metrics"
	"net/http"
	"time"

	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

type FastProxy struct {
	pool     *balancer.Pool
	strategy balancer.Strategy
	logger   *zap.Logger
	metrics  *metrics.Collector

	clients map[string]*fasthttp.HostClient
}

func NewFastProxy(pool *balancer.Pool, strategy balancer.Strategy, logger *zap.Logger, m *metrics.Collector) *FastProxy {
	return &FastProxy{
		pool:     pool,
		strategy: strategy,
		logger:   logger,
		metrics:  m,
		clients:  make(map[string]*fasthttp.HostClient),
	}
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

func (p *FastProxy) Handler(ctx *fasthttp.RequestCtx) {
	path := string(ctx.Path())
	if path == "/test/dashboard" {
		fasthttp.ServeFile(ctx, "./html/dashboard.html")
		return
	}

	start := time.Now()

	be := p.strategy.Select(p.pool)
	if be == nil {
		p.metrics.RecordRequest("none", http.StatusServiceUnavailable, 0, string(ctx.Method()), len(ctx.Request.Body()), 0)
		ctx.Error("Service Unavailable", fasthttp.StatusServiceUnavailable)
		return
	}

	be.IncrementConn()
	defer be.DecrementConn()

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
		p.logger.Error("proxy error", zap.Error(err), zap.String("backend", be.URL.Host))
		p.metrics.RecordRequest(be.URL.Host, http.StatusBadGateway, 0, string(ctx.Method()), len(ctx.Request.Body()), 0)
		p.metrics.RecordError(be.URL.Host, "proxy_error")

		proxyErr := errors.NewBackendError(err, be.URL.Host)
		ctx.Error(proxyErr.Message, fasthttp.StatusBadGateway)
		return
	}

	resp.Header.CopyTo(&ctx.Response.Header)
	ctx.SetStatusCode(resp.StatusCode())
	ctx.SetBody(resp.Body())

	p.metrics.RecordRequest(be.URL.Host, resp.StatusCode(), time.Since(start).Seconds(), string(ctx.Method()), len(ctx.Request.Body()), len(resp.Body()))
}
