package server

import (
	"context"
	"loadbalancer/internal/balancer"
	"loadbalancer/internal/errors"
	"loadbalancer/internal/metrics"
	"net"
	"net/http"
	"net/http/httputil"
	"time"

	"go.uber.org/zap"
)

type ctxKey int

const selectedBackendKey ctxKey = iota

type Proxy struct {
	pool     *balancer.Pool
	proxy    *httputil.ReverseProxy
	logger   *zap.Logger
	metrics  *metrics.Collector
	strategy balancer.Strategy
}

func NewProxy(pool *balancer.Pool, logger *zap.Logger, strategy balancer.Strategy, m *metrics.Collector) *Proxy {
	p := &Proxy{pool: pool, logger: logger, metrics: m, strategy: strategy}
	p.proxy = &httputil.ReverseProxy{
		Director:       p.director,
		Transport:      &countingTransport{base: p.buildTransport(), metrics: m},
		ErrorHandler:   p.errorHandler,
		ModifyResponse: p.modifyResponse,
	}
	return p
}

func (p *Proxy) director(req *http.Request) {
	be, ok := req.Context().Value(selectedBackendKey).(*balancer.Backend)
	if !ok || be == nil {
		return
	}
	req.URL.Scheme = be.URL.Scheme
	req.URL.Host = be.URL.Host
	req.Host = be.URL.Host
	req.Header.Set("X-Forwarded-For", req.RemoteAddr)
	req.Header.Set("X-Real-Ip", req.RemoteAddr)

	for _, h := range []string{"Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "Te", "Trailers", "Transfer-Encoding", "Upgrade"} {
		req.Header.Del(h)
	}
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	be := p.strategy.Select(p.pool)
	if be == nil {
		p.metrics.RecordRequest("none", http.StatusServiceUnavailable, 0, r.Method, int(r.ContentLength), 0)
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}
	ctx := context.WithValue(r.Context(), selectedBackendKey, be)
	p.proxy.ServeHTTP(w, r.WithContext(ctx))
}

func (p *Proxy) errorHandler(w http.ResponseWriter, r *http.Request, err error) {
	p.logger.Error("proxy error", zap.Error(err), zap.String("path", r.URL.Path))

	proxyErr, ok := err.(*errors.ProxyError)
	if ok {
		if be, ok := r.Context().Value(selectedBackendKey).(*balancer.Backend); ok && be != nil {
			p.metrics.RecordRequest(be.URL.Host, proxyErr.StatusCode, 0, r.Method, int(r.ContentLength), 0)
			p.metrics.RecordError(be.URL.Host, string(proxyErr.Type))
		}
		http.Error(w, proxyErr.Message, proxyErr.StatusCode)
		return
	}

	if be, ok := r.Context().Value(selectedBackendKey).(*balancer.Backend); ok && be != nil {
		p.metrics.RecordRequest(be.URL.Host, http.StatusBadGateway, 0, r.Method, int(r.ContentLength), 0)
		p.metrics.RecordError(be.URL.Host, "unknown_error")
	}
	http.Error(w, "Bad Gateway", http.StatusBadGateway)
}

func (p *Proxy) modifyResponse(resp *http.Response) error {
	if be, ok := resp.Request.Context().Value(selectedBackendKey).(*balancer.Backend); ok && be != nil {
	}
	return nil
}

func (p *Proxy) buildTransport() *http.Transport {
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	return &http.Transport{
		MaxIdleConns:          10000,
		MaxIdleConnsPerHost:   1000,
		MaxConnsPerHost:       10000,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
		DisableCompression:    false,
		DialContext:           dialer.DialContext,
	}
}

type countingTransport struct {
	base    http.RoundTripper
	metrics *metrics.Collector
}

func (t *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	be, ok := req.Context().Value(selectedBackendKey).(*balancer.Backend)
	if !ok || be == nil {
		return t.base.RoundTrip(req)
	}

	be.IncrementConn()
	active := t.metrics.SetActive(t.metrics.GetActiveConnections() + 1)
	t.metrics.SetActive(active)

	start := time.Now()
	resp, err := t.base.RoundTrip(req)

	be.DecrementConn()
	active = t.metrics.SetActive(t.metrics.GetActiveConnections() - 1)
	t.metrics.SetActive(active)

	dur := time.Since(start).Seconds()
	status := http.StatusBadGateway
	requestSize := int(req.ContentLength)
	responseSize := 0
	if resp != nil {
		status = resp.StatusCode
	}
	t.metrics.RecordRequest(be.URL.Host, status, dur, req.Method, requestSize, responseSize)

	if err != nil {
		t.metrics.RecordError(be.URL.Host, "transport_error")
	}

	return resp, err
}
