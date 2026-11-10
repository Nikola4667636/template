package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/Nikola4667636/template/internal/http/response"
)

func Recover(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					err := fmt.Errorf("panic: %v\n%s", recovered, debug.Stack())
					response.Error(w, r, logger, "recover request", err)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
