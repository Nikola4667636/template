package telemetry

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"slices"

	"go.opentelemetry.io/otel/trace"
)

type traceHandler struct {
	slog.Handler
	scoped func(slog.Handler) slog.Handler
}

func NewLogger(out io.Writer, levelName, service string) (*slog.Logger, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(levelName)); err != nil {
		return nil, fmt.Errorf("parse log level: %w", err)
	}
	handler := slog.NewJSONHandler(out, &slog.HandlerOptions{Level: level})
	return slog.New(traceHandler{Handler: handler}).With("service", service), nil
}

func (h traceHandler) Handle(ctx context.Context, record slog.Record) error {
	span := trace.SpanContextFromContext(ctx)
	var traceID, spanID string
	if span.IsValid() {
		traceID, spanID = span.TraceID().String(), span.SpanID().String()
	}
	handler := h.Handler.WithAttrs([]slog.Attr{slog.String("trace_id", traceID), slog.String("span_id", spanID)})
	if h.scoped != nil {
		handler = h.scoped(handler)
	}
	return handler.Handle(ctx, record)
}

func (h traceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	attrs = slices.Clone(attrs)
	return traceHandler{Handler: h.Handler, scoped: func(handler slog.Handler) slog.Handler {
		if h.scoped != nil {
			handler = h.scoped(handler)
		}
		return handler.WithAttrs(attrs)
	}}
}

func (h traceHandler) WithGroup(name string) slog.Handler {
	return traceHandler{Handler: h.Handler, scoped: func(handler slog.Handler) slog.Handler {
		if h.scoped != nil {
			handler = h.scoped(handler)
		}
		return handler.WithGroup(name)
	}}
}
