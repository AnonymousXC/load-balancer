package middleware

import (
	"time"

	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

type FastMiddleware func(fasthttp.RequestHandler) fasthttp.RequestHandler

func FastChain(h fasthttp.RequestHandler, mws ...FastMiddleware) fasthttp.RequestHandler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

func FastLogging(logger *zap.Logger) FastMiddleware {
	return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		return func(ctx *fasthttp.RequestCtx) {
			start := time.Now()
			next(ctx)
			logger.Info("request",
				zap.String("method", string(ctx.Method())),
				zap.String("path", string(ctx.Path())),
				zap.String("remote", ctx.RemoteIP().String()),
				zap.Int("status", ctx.Response.StatusCode()),
				zap.Int("bytes", len(ctx.Response.Body())),
				zap.Duration("duration", time.Since(start)),
			)
		}
	}
}

func FastRecovery(logger *zap.Logger) FastMiddleware {
	return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		return func(ctx *fasthttp.RequestCtx) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error("panic recovered", zap.Any("error", rec))
					ctx.Error("Internal Server Error", fasthttp.StatusInternalServerError)
				}
			}()
			next(ctx)
		}
	}
}

func FastRateLimit(rl *RateLimiter) FastMiddleware {
	return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		return func(ctx *fasthttp.RequestCtx) {
			ip := ctx.RemoteIP().String()
			if !rl.getLimiter(ip).Allow() {
				ctx.Error("Too Many Requests", fasthttp.StatusTooManyRequests)
				return
			}
			next(ctx)
		}
	}
}
