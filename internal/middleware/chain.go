package middleware

import (
	"net/http"

	"github.com/valyala/fasthttp"
)

type Middleware func(http.Handler) http.Handler
type FastMiddleware func(fasthttp.RequestHandler) fasthttp.RequestHandler

// net/tls
func Chain(h http.Handler, mws ...Middleware) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}

	return h
}

// fasthttp
func FastChain(h fasthttp.RequestHandler, mws ...FastMiddleware) fasthttp.RequestHandler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}
