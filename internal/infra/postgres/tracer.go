package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
)

type queryKey struct{}

type queryObservation struct {
	started    time.Time
	operation  string
	collection string
}

// Tracer adds the course's collection/error metric labels around otelpgx.
type Tracer struct {
	*otelpgx.Tracer
	duration metric.Float64Histogram
}

func NewTracer(traces trace.TracerProvider, meters metric.MeterProvider) (*Tracer, error) {
	duration, err := meters.Meter("trip-service/postgres").Float64Histogram("db.client.operation.duration",
		metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 10))
	if err != nil {
		return nil, fmt.Errorf("create DB operation histogram: %w", err)
	}
	return &Tracer{
		Tracer: otelpgx.NewTracer(otelpgx.WithTracerProvider(traces),
			// Record one histogram with all required labels instead of duplicating otelpgx's histogram.
			otelpgx.WithMeterProvider(noop.NewMeterProvider()),
			otelpgx.WithDisableAcquireTracer(), otelpgx.WithDisableConnectionDetailsInAttributes(),
			otelpgx.WithDisableSQLStatementInAttributes()),
		duration: duration,
	}, nil
}

func (t *Tracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	ctx = t.Tracer.TraceQueryStart(ctx, conn, data)
	return t.start(ctx, data.SQL)
}

func (t *Tracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	t.end(ctx, data.Err)
	t.Tracer.TraceQueryEnd(ctx, conn, data)
}

func (t *Tracer) TracePrepareStart(ctx context.Context, conn *pgx.Conn, data pgx.TracePrepareStartData) context.Context {
	ctx = t.Tracer.TracePrepareStart(ctx, conn, data)
	return t.start(ctx, data.SQL)
}

func (t *Tracer) TracePrepareEnd(ctx context.Context, conn *pgx.Conn, data pgx.TracePrepareEndData) {
	t.end(ctx, data.Err)
	t.Tracer.TracePrepareEnd(ctx, conn, data)
}

func (t *Tracer) start(ctx context.Context, sql string) context.Context {
	observation := queryObservation{started: time.Now(), operation: otelpgx.SQLOperationName(ctx, sql), collection: collection(sql)}
	trace.SpanFromContext(ctx).SetAttributes(attribute.String("db.operation.name", observation.operation),
		attribute.String("db.collection.name", observation.collection))
	return context.WithValue(ctx, queryKey{}, observation)
}

func (t *Tracer) end(ctx context.Context, err error) {
	observation, ok := ctx.Value(queryKey{}).(queryObservation)
	if !ok {
		return
	}
	var errorType string
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			errorType = pgErr.Code
		} else {
			errorType = fmt.Sprintf("%T", err)
		}
		trace.SpanFromContext(ctx).SetAttributes(attribute.String("error.type", errorType))
	}
	t.duration.Record(ctx, time.Since(observation.started).Seconds(), metric.WithAttributes(
		attribute.String("db.operation.name", observation.operation),
		attribute.String("db.collection.name", observation.collection), attribute.String("error.type", errorType),
	))
}

func collection(sql string) string {
	words := strings.Fields(sql)
	for i, word := range words {
		if (strings.EqualFold(word, "FROM") || strings.EqualFold(word, "INTO") || strings.EqualFold(word, "UPDATE")) && i+1 < len(words) {
			table := strings.Trim(words[i+1], "\"")
			// Only schema tables become metric labels; arbitrary SQL never increases cardinality.
			if table == "trips" || table == "trip_status_history" {
				return table
			}
		}
	}
	return ""
}
