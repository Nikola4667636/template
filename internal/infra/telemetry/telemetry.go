package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"runtime/debug"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/Nikola4667636/template/internal/config"
)

type Runtime struct {
	Logger *slog.Logger
	Traces *sdktrace.TracerProvider
	Meters *sdkmetric.MeterProvider
}

func New(ctx context.Context, cfg *config.Config) (*Runtime, error) {
	logger, err := NewLogger(os.Stdout, cfg.LOG_LEVEL, cfg.OTEL_SERVICE_NAME)
	if err != nil {
		return nil, err
	}
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		logger.Warn("telemetry export failed", "error", err)
	}))
	res, err := resource.New(ctx, resource.WithTelemetrySDK(),
		resource.WithAttributes(attribute.String("service.version", buildVersion())),
		resource.WithFromEnv(), resource.WithAttributes(attribute.String("service.name", cfg.OTEL_SERVICE_NAME)))
	if err != nil {
		return nil, fmt.Errorf("create telemetry resource: %w", err)
	}
	spanExporter, err := newSpanExporter(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create OTLP trace exporter: %w", err)
	}
	metricExporter, err := newMetricExporter(ctx, cfg)
	if err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.SHUTDOWN_TIMEOUT)
		defer cancel()
		return nil, errors.Join(fmt.Errorf("create OTLP metric exporter: %w", err), spanExporter.Shutdown(cleanupCtx))
	}
	return &Runtime{
		Logger: logger,
		Traces: sdktrace.NewTracerProvider(sdktrace.WithResource(res), sdktrace.WithBatcher(spanExporter)),
		Meters: sdkmetric.NewMeterProvider(sdkmetric.WithResource(res),
			sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter))),
	}, nil
}

func newSpanExporter(ctx context.Context, cfg *config.Config) (sdktrace.SpanExporter, error) {
	if cfg.OTEL_EXPORTER_OTLP_PROTOCOL == "grpc" {
		return otlptracegrpc.New(ctx, otlptracegrpc.WithEndpointURL(cfg.OTEL_EXPORTER_OTLP_ENDPOINT))
	}
	endpoint, err := url.JoinPath(cfg.OTEL_EXPORTER_OTLP_ENDPOINT, "v1/traces")
	if err != nil {
		return nil, fmt.Errorf("create trace endpoint: %w", err)
	}
	return otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint))
}

func newMetricExporter(ctx context.Context, cfg *config.Config) (sdkmetric.Exporter, error) {
	if cfg.OTEL_EXPORTER_OTLP_PROTOCOL == "grpc" {
		return otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithEndpointURL(cfg.OTEL_EXPORTER_OTLP_ENDPOINT))
	}
	endpoint, err := url.JoinPath(cfg.OTEL_EXPORTER_OTLP_ENDPOINT, "v1/metrics")
	if err != nil {
		return nil, fmt.Errorf("create metric endpoint: %w", err)
	}
	return otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpointURL(endpoint))
}

func (r *Runtime) Shutdown(ctx context.Context) error {
	// The final gauge collection still needs PostgreSQL, so the pool is closed later.
	metricErr := r.Meters.Shutdown(ctx)
	traceErr := r.Traces.Shutdown(ctx)
	return errors.Join(metricErr, traceErr)
}

func buildVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}
