package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/Nikola4667636/template/internal/http/requeststate"
)

func Logging(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			state := requeststate.FromContext(r.Context())
			state.LogsManaged = true
			started := time.Now()
			state.OnComplete(func() {
				level := slog.LevelInfo
				if state.Panicked || state.Status() >= http.StatusInternalServerError {
					level = slog.LevelError
				}
				attrs := []any{
					"method", r.Method, "route", state.Route, "status", state.Status(),
					"duration_ms", float64(time.Since(started).Microseconds()) / 1000,
				}
				if state.Err != nil && level == slog.LevelError {
					attrs = append(attrs, "op", state.Operation, "error", state.Err)
				}
				logger.Log(state.Request.Context(), level, "request handled", attrs...)
			})
			next.ServeHTTP(w, r)
		})
	}
}
