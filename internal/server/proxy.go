package server

import (
	"context"
	"io"
	"loadbalancer/internal/balancer"
	"loadbalancer/internal/circuitbreaker"
	"loadbalancer/internal/errors"
	"loadbalancer/internal/metrics"
	"loadbalancer/internal/retry"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"go.uber.org/zap"
)

type ctxKey int

const (
	selectedBackendKey ctxKey = iota
	cbDoneFuncKey
)

type Proxy struct {
	pool      *balancer.Pool
	proxy     *httputil.ReverseProxy
	logger    *zap.Logger
	metrics   *metrics.Collector
	strategy  balancer.Strategy
	cbManager *circuitbreaker.Manager
	retryCfg  retry.Config
}

func NewProxy(pool *balancer.Pool, logger *zap.Logger, strategy balancer.Strategy, m *metrics.Collector) *Proxy {
	cbConfig := circuitbreaker.DefaultConfig()
	cbConfig.OnStateChange = func(name string, from, to circuitbreaker.State) {
		logger.Warn("circuit breaker state changed",
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

	p := &Proxy{
		pool:      pool,
		logger:    logger,
		metrics:   m,
		strategy:  strategy,
		cbManager: cbManager,
		retryCfg:  retry.DefaultConfig(),
	}

	p.retryCfg.OnRetry = func(attempt int, err error) {
		logger.Warn("retrying backend request",
			zap.Int("attempt", attempt),
			zap.Error(err),
		)
	}

	p.proxy = &httputil.ReverseProxy{
		Director:       p.director,
		Transport:      &countingTransport{base: p.buildTransport(), metrics: m, cbManager: cbManager},
		ErrorHandler:   p.errorHandler,
		ModifyResponse: p.modifyResponse,
	}
	return p
}

func (p *Proxy) CBManager() *circuitbreaker.Manager {
	return p.cbManager
}

func (p *Proxy) Pool() *balancer.Pool {
	return p.pool
}

func (p *Proxy) Strategy() balancer.Strategy {
	return p.strategy
}

func (p *Proxy) director(req *http.Request) {
	be, ok := req.Context().Value(selectedBackendKey).(*balancer.Backend)
	if !ok || be == nil {
		return
	}
	req.URL.Scheme = be.URL.Scheme
	req.URL.Host = be.URL.Host
	req.Host = be.URL.Host

	clientIP := req.RemoteAddr
	if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
		req.Header.Set("X-Forwarded-For", xff+", "+clientIP)
	} else {
		req.Header.Set("X-Forwarded-For", clientIP)
	}
	req.Header.Set("X-Real-Ip", clientIP)

	for _, h := range []string{"Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "Te", "Trailers", "Transfer-Encoding", "Upgrade"} {
		req.Header.Del(h)
	}
}

func (p *Proxy) selectAvailableBackend(r *http.Request) (*balancer.Backend, func(bool)) {
	// 1. If strategy supports key hashing, extract client key
	var be *balancer.Backend
	if keyed, ok := p.strategy.(balancer.KeyedStrategy); ok {
		key := r.Header.Get("X-Forwarded-For")
		if key == "" {
			key = r.RemoteAddr
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
	p.logger.Warn("backend circuit open, seeking healthy fallback", zap.String("backend", be.URL.Host))

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

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	be, done := p.selectAvailableBackend(r)
	if be == nil {
		p.metrics.RecordRequest("none", http.StatusServiceUnavailable, 0, r.Method, int(r.ContentLength), 0)
		p.metrics.RecordError("none", "all_backends_unavailable")
		http.Error(w, "Service Unavailable: No healthy backends available", http.StatusServiceUnavailable)
		return
	}

	ctx := context.WithValue(r.Context(), selectedBackendKey, be)
	if done != nil {
		ctx = context.WithValue(ctx, cbDoneFuncKey, done)
	}

	p.proxy.ServeHTTP(w, r.WithContext(ctx))
}

func (p *Proxy) errorHandler(w http.ResponseWriter, r *http.Request, err error) {
	p.logger.Error("proxy error", zap.Error(err), zap.String("path", r.URL.Path))

	be, _ := r.Context().Value(selectedBackendKey).(*balancer.Backend)
	if done, ok := r.Context().Value(cbDoneFuncKey).(func(bool)); ok && done != nil {
		done(false)
	}

	host := "unknown"
	if be != nil {
		host = be.URL.Host
		be.RecordFailure()
	}

	proxyErr, ok := err.(*errors.ProxyError)
	if ok {
		p.metrics.RecordRequest(host, proxyErr.StatusCode, 0, r.Method, int(r.ContentLength), 0)
		p.metrics.RecordError(host, string(proxyErr.Type))
		http.Error(w, proxyErr.Message, proxyErr.StatusCode)
		return
	}

	p.metrics.RecordRequest(host, http.StatusBadGateway, 0, r.Method, int(r.ContentLength), 0)
	p.metrics.RecordError(host, "bad_gateway")
	http.Error(w, "Bad Gateway: Upstream server unreachable", http.StatusBadGateway)
}

func (p *Proxy) modifyResponse(resp *http.Response) error {
	be, _ := resp.Request.Context().Value(selectedBackendKey).(*balancer.Backend)
	done, hasDone := resp.Request.Context().Value(cbDoneFuncKey).(func(bool))

	if resp.StatusCode >= 500 {
		if be != nil {
			be.RecordFailure()
		}
		if hasDone && done != nil {
			done(false)
		}
	} else {
		if be != nil {
			be.RecordSuccess()
		}
		if hasDone && done != nil {
			done(true)
		}
	}
	return nil
}

func (p *Proxy) buildTransport() *http.Transport {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
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
	base      http.RoundTripper
	metrics   *metrics.Collector
	cbManager *circuitbreaker.Manager
}

func (t *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	be, ok := req.Context().Value(selectedBackendKey).(*balancer.Backend)
	if !ok || be == nil {
		return t.base.RoundTrip(req)
	}

	be.IncrementConn()
	t.metrics.SetBackendActive(be.URL.Host, be.ActiveConnections())
	active := t.metrics.SetActive(t.metrics.GetActiveConnections() + 1)
	t.metrics.SetActive(active)

	start := time.Now()

	isIdempotent := req.Method == http.MethodGet || req.Method == http.MethodHead || req.Method == http.MethodOptions
	var resp *http.Response
	var err error

	if isIdempotent {
		retryCfg := retry.Config{
			MaxRetries:      2,
			InitialInterval: 50 * time.Millisecond,
			MaxInterval:     500 * time.Millisecond,
			Multiplier:      2.0,
			Jitter:          0.1,
			Retryable: func(err error) bool {
				if err == nil {
					return false
				}
				return true
			},
			OnRetry: func(attempt int, err error) {
				t.metrics.RecordRetry(be.URL.Host, attempt)
			},
		}

		resp, err = retry.RetryWithData(req.Context(), func() (*http.Response, error) {
			r, e := t.base.RoundTrip(req)
			if e != nil {
				return nil, e
			}
			if r.StatusCode >= 502 && r.StatusCode <= 504 {
				return r, &net.OpError{Op: "dial", Net: "tcp", Err: io.EOF}
			}
			return r, nil
		}, retryCfg)
	} else {
		resp, err = t.base.RoundTrip(req)
	}

	be.DecrementConn()
	t.metrics.SetBackendActive(be.URL.Host, be.ActiveConnections())
	t.metrics.SetActive(t.metrics.GetActiveConnections() - 1)

	dur := time.Since(start).Seconds()
	status := http.StatusBadGateway
	requestSize := int(req.ContentLength)
	if requestSize < 0 {
		requestSize = 0
	}
	responseSize := 0
	if resp != nil {
		status = resp.StatusCode
		if resp.ContentLength > 0 {
			responseSize = int(resp.ContentLength)
		}
	}

	t.metrics.RecordRequest(be.URL.Host, status, dur, req.Method, requestSize, responseSize)

	if err != nil {
		if strings.Contains(err.Error(), "timeout") {
			t.metrics.RecordError(be.URL.Host, "timeout")
		} else {
			t.metrics.RecordError(be.URL.Host, "transport_error")
		}
	}

	return resp, err
}
