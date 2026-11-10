package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/Nikola4667636/template/internal/http/domainerror"
	"github.com/Nikola4667636/template/internal/http/requeststate"
	"github.com/Nikola4667636/template/internal/http/response"
)

func Recover(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			state := requeststate.Begin(w, r)
			defer func() {
				var abortResponse bool
				if recovered := recover(); recovered != nil {
					abortResponse = state.Writer.Status() != 0
					if !state.Panicked {
						state.Panicked = true
						err := fmt.Errorf("panic: %v\n%s", recovered, debug.Stack())
						requeststate.RecordError(state.Request.Context(), "recover request", err)
					}
					if !abortResponse {
						_, body := domainerror.Problem(state.Err)
						response.Problem(state.Writer, state.Request, body)
					}
				}
				state.Complete()
				if !state.LogsManaged && state.Err != nil && (state.Panicked || state.Status() >= http.StatusInternalServerError) {
					logger.ErrorContext(state.Request.Context(), "request failed", "op", state.Operation, "error", state.Err)
				}
				if abortResponse {
					// net/http closes the connection without writing a second panic log.
					panic(http.ErrAbortHandler)
				}
			}()
			next.ServeHTTP(state.Writer, state.Request)
		})
	}
}
