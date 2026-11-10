//go:build integration

package telemetry_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	metricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	"github.com/Nikola4667636/template/internal/config"
	"github.com/Nikola4667636/template/internal/infra/telemetry"
)

func TestOTLPHTTPExport(t *testing.T) {
	var traces, metrics atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		switch r.URL.Path {
		case "/v1/traces":
			var request tracev1.ExportTraceServiceRequest
			if err := proto.Unmarshal(body, &request); err != nil || len(request.ResourceSpans) == 0 {
				t.Errorf("invalid OTLP trace payload: %v", err)
			}
			traces.Add(1)
		case "/v1/metrics":
			var request metricsv1.ExportMetricsServiceRequest
			if err := proto.Unmarshal(body, &request); err != nil || len(request.ResourceMetrics) == 0 {
				t.Errorf("invalid OTLP metric payload: %v", err)
			}
			metrics.Add(1)
		default:
			t.Errorf("OTLP export used the wrong path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	runtime := exportRuntime(t, "http/protobuf", server.URL)
	flushSignals(t, runtime)
	if traces.Load() == 0 || metrics.Load() == 0 {
		t.Errorf("missing HTTP OTLP signals: traces=%d metrics=%d", traces.Load(), metrics.Load())
	}
}

type traceCollector struct {
	tracev1.UnimplementedTraceServiceServer
	count *atomic.Int64
}

func (c *traceCollector) Export(_ context.Context, _ *tracev1.ExportTraceServiceRequest) (*tracev1.ExportTraceServiceResponse, error) {
	c.count.Add(1)
	return &tracev1.ExportTraceServiceResponse{}, nil
}

type metricCollector struct {
	metricsv1.UnimplementedMetricsServiceServer
	count *atomic.Int64
}

func (c *metricCollector) Export(_ context.Context, _ *metricsv1.ExportMetricsServiceRequest) (*metricsv1.ExportMetricsServiceResponse, error) {
	c.count.Add(1)
	return &metricsv1.ExportMetricsServiceResponse{}, nil
}

func TestOTLPGRPCExport(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	var traces, metrics atomic.Int64
	tracev1.RegisterTraceServiceServer(server, &traceCollector{count: &traces})
	metricsv1.RegisterMetricsServiceServer(server, &metricCollector{count: &metrics})
	stopped := make(chan error, 1)
	go func() { stopped <- server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		if err := <-stopped; err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Error(err)
		}
	})
	runtime := exportRuntime(t, "grpc", "http://"+listener.Addr().String())
	flushSignals(t, runtime)
	if traces.Load() == 0 || metrics.Load() == 0 {
		t.Errorf("missing gRPC OTLP signals: traces=%d metrics=%d", traces.Load(), metrics.Load())
	}
}

func exportRuntime(t *testing.T, protocol, endpoint string) *telemetry.Runtime {
	t.Helper()
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "service.namespace=tripgo,deployment.environment=local")
	t.Setenv("OTEL_TRACES_SAMPLER", "always_on")
	t.Setenv("OTEL_BSP_SCHEDULE_DELAY", "3600000")
	t.Setenv("OTEL_METRIC_EXPORT_INTERVAL", "3600000")
	runtime, err := telemetry.New(t.Context(), &config.Config{
		LOG_LEVEL: "error", SHUTDOWN_TIMEOUT: 5 * time.Second, OTEL_SERVICE_NAME: "trip-service",
		OTEL_EXPORTER_OTLP_PROTOCOL: protocol, OTEL_EXPORTER_OTLP_ENDPOINT: endpoint,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 5*time.Second)
		defer cancel()
		if err := runtime.Shutdown(ctx); err != nil {
			t.Errorf("shutdown test telemetry: %v", err)
		}
	})
	return runtime
}

func flushSignals(t *testing.T, runtime *telemetry.Runtime) {
	t.Helper()
	_, span := runtime.Traces.Tracer("test").Start(t.Context(), "export check")
	span.End()
	counter, err := runtime.Meters.Meter("test").Int64Counter("test.events")
	if err != nil {
		t.Fatal(err)
	}
	counter.Add(t.Context(), 1)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := runtime.Traces.ForceFlush(ctx); err != nil {
		t.Fatalf("flush traces: %v", err)
	}
	if err := runtime.Meters.ForceFlush(ctx); err != nil {
		t.Fatalf("flush metrics: %v", err)
	}
}
