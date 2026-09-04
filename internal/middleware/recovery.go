package middleware

import (
	"net/http"

	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

func Recovery(logger *zap.Logger) Middleware {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error("panic recovered", zap.Any("error", rec))
					http.Error(w, "Internal server error", http.StatusInternalServerError)
				}
			}()
			h.ServeHTTP(w, r)
		})
	}
}

// fasthttp
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
