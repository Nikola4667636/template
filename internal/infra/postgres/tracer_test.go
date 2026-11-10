package postgres_test

import (
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/Nikola4667636/template/internal/infra/postgres"
)

func TestQueryInstrumentation(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	traces := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	reader := sdkmetric.NewManualReader()
	meters := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = traces.Shutdown(t.Context()); _ = meters.Shutdown(t.Context()) })
	tracer, err := postgres.NewTracer(traces, meters)
	if err != nil {
		t.Fatal(err)
	}
	ctx, parent := traces.Tracer("test").Start(t.Context(), "request")
	queryCtx := tracer.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{
		SQL: "INSERT INTO trip_status_history (trip_id, to_status) VALUES ($1, $2)", Args: []any{"private-id", "invalid"},
	})
	tracer.TraceQueryEnd(queryCtx, nil, pgx.TraceQueryEndData{Err: &pgconn.PgError{Code: "23514", Message: "check constraint"}})
	parent.End()
	spans := exporter.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("span count = %d, want parent and query", len(spans))
	}
	query := spans[0]
	if query.Parent.SpanID() != parent.SpanContext().SpanID() {
		t.Error("DB span is not a child of the request")
	}
	attrs := attribute.NewSet(query.Attributes...)
	for key, want := range map[string]string{"db.operation.name": "INSERT", "db.collection.name": "trip_status_history", "error.type": "23514"} {
		got, _ := attrs.Value(attribute.Key(key))
		if got.AsString() != want {
			t.Errorf("DB span %s = %s, want %s", key, got.AsString(), want)
		}
	}
	if attrs.HasValue("db.query.text") || attrs.HasValue("pgx.query.parameters") {
		t.Error("SQL or query arguments leaked into telemetry")
	}
	var data metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &data); err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != "db.client.operation.duration" {
				continue
			}
			found = true
			points := metric.Data.(metricdata.Histogram[float64]).DataPoints
			if len(points) != 1 || points[0].Count != 1 || metric.Unit != "s" {
				t.Fatalf("unexpected DB histogram: %+v", metric)
			}
			for key, want := range map[string]string{"db.operation.name": "INSERT", "db.collection.name": "trip_status_history", "error.type": "23514"} {
				value, _ := points[0].Attributes.Value(attribute.Key(key))
				if value.AsString() != want {
					t.Errorf("DB metric %s = %s, want %s", key, value.AsString(), want)
				}
			}
		}
	}
	if !found {
		t.Error("DB histogram was not recorded")
	}
}
