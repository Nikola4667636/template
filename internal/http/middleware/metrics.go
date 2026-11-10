package middleware

import (
	"fmt"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/Nikola4667636/template/internal/http/requeststate"
)

type HTTPMetrics struct {
	duration metric.Float64Histogram
	active   metric.Int64UpDownCounter
}

func NewHTTPMetrics(meter metric.Meter) (*HTTPMetrics, error) {
	duration, err := meter.Float64Histogram("http.server.request.duration", metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10))
	if err != nil {
		return nil, fmt.Errorf("create HTTP duration histogram: %w", err)
	}
	active, err := meter.Int64UpDownCounter("http.server.active_requests", metric.WithUnit("{request}"))
	if err != nil {
		return nil, fmt.Errorf("create active HTTP requests counter: %w", err)
	}
	return &HTTPMetrics{duration: duration, active: active}, nil
}

func (m *HTTPMetrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state := requeststate.FromContext(r.Context())
		attrs := []attribute.KeyValue{attribute.String("http.request.method", r.Method), attribute.String("http.route", state.Route)}
		options := metric.WithAttributes(attrs...)
		started := time.Now()
		m.active.Add(r.Context(), 1, options)
		state.OnComplete(func() {
			m.active.Add(r.Context(), -1, options)
			m.duration.Record(r.Context(), time.Since(started).Seconds(),
				metric.WithAttributes(append(attrs, attribute.Int("http.response.status_code", state.Status()))...))
		})
		next.ServeHTTP(w, r)
	})
}
