package middleware

import (
	"net/http"

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
