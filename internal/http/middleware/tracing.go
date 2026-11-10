package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/Nikola4667636/template/internal/http/requeststate"
)

func Tracing(provider trace.TracerProvider, routes chi.Routes) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		guard := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			state := requeststate.FromContext(r.Context())
			state.Request = r
			span := trace.SpanFromContext(r.Context())
			span.SetAttributes(attribute.String("http.route", state.Route))
			defer func() {
				if recovered := recover(); recovered != nil {
					state.Panicked = true
					err := fmt.Errorf("panic: %v\n%s", recovered, debug.Stack())
					requeststate.RecordError(r.Context(), "recover request", err)
					status := state.Writer.Status()
					if status == 0 {
						status = http.StatusInternalServerError
					}
					span.SetAttributes(attribute.Int("http.response.status_code", status),
						attribute.String("error.type", "panic"))
					panic(recovered)
				}
			}()
			next.ServeHTTP(w, r)
		})
		handler := otelhttp.NewHandler(guard, "http request",
			otelhttp.WithTracerProvider(provider),
			// Metrics are recorded separately after recovery, including panic responses.
			otelhttp.WithMeterProvider(noop.NewMeterProvider()),
			otelhttp.WithPropagators(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})),
			otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
				return r.Method + " " + requeststate.FromContext(r.Context()).Route
			}))
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			state := requeststate.FromContext(r.Context())
			state.Route = routePattern(routes, r)
			handler.ServeHTTP(w, r)
		})
	}
}

func routePattern(routes chi.Routes, r *http.Request) string {
	ctx := chi.NewRouteContext()
	if routes.Match(ctx, r.Method, r.URL.Path) {
		return ctx.RoutePattern()
	}
	// Preserve the route template for a known path with an unsupported method.
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		ctx.Reset()
		if routes.Match(ctx, method, r.URL.Path) {
			return ctx.RoutePattern()
		}
	}
	return "/unmatched"
}
