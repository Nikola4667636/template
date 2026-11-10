package telemetry_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"go.opentelemetry.io/otel/trace"

	"github.com/Nikola4667636/template/internal/infra/telemetry"
)

func TestLoggerCorrelationAndLevel(t *testing.T) {
	var out bytes.Buffer
	logger, err := telemetry.NewLogger(&out, "info", "trip-service")
	if err != nil {
		t.Fatal(err)
	}
	span := trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}})
	ctx := trace.ContextWithSpanContext(t.Context(), span)
	logger.DebugContext(ctx, "hidden")
	logger.With("component", "test").InfoContext(ctx, "visible")
	var entry map[string]any
	if err := json.Unmarshal(out.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry["trace_id"] != span.TraceID().String() || entry["span_id"] != span.SpanID().String() ||
		entry["service"] != "trip-service" || entry["component"] != "test" || entry["level"] != "INFO" || entry["time"] == nil {
		t.Errorf("incomplete correlated log: %+v", entry)
	}
	out.Reset()
	logger.InfoContext(t.Context(), "outside a span")
	if err := json.Unmarshal(out.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry["trace_id"] != "" || entry["span_id"] != "" {
		t.Errorf("unexpected span IDs outside a span: %+v", entry)
	}
	if _, err := telemetry.NewLogger(&out, "invalid", "trip-service"); err == nil {
		t.Error("invalid level was accepted")
	}
}

func TestLoggerGroups(t *testing.T) {
	var out bytes.Buffer
	logger, err := telemetry.NewLogger(&out, "debug", "trip-service")
	if err != nil {
		t.Fatal(err)
	}
	logger.WithGroup("details").DebugContext(t.Context(), "grouped", "key", "value")
	var entry map[string]any
	if err := json.Unmarshal(out.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	group, ok := entry["details"].(map[string]any)
	if !ok || group["key"] != "value" {
		t.Errorf("grouped attributes were lost: %+v", entry)
	}
	if _, ok := entry["trace_id"]; !ok {
		t.Error("trace_id must stay at the root of a grouped log record")
	}
}
