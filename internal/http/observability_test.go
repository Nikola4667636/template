package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/mock/gomock"

	"github.com/Nikola4667636/template/internal/domain/trip"
	api "github.com/Nikola4667636/template/internal/generated"
	transport "github.com/Nikola4667636/template/internal/http"
	"github.com/Nikola4667636/template/internal/http/middleware"
	"github.com/Nikola4667636/template/internal/http/response"
	"github.com/Nikola4667636/template/internal/infra/telemetry"
)

const (
	getRoute          = "/api/v1/trips/{tripId}"
	httpDuration      = "http.server.request.duration"
	httpActive        = "http.server.active_requests"
	privateError      = "private database error"
	errorLogLevel     = "ERROR"
	parentTraceID     = "4bf92f3577b34da6a3ce929d0e0e4736"
	parentSpanID      = "00f067aa0ba902b7"
	parentTraceHeader = "00-" + parentTraceID + "-" + parentSpanID + "-01"
)

func TestRequestObservability(t *testing.T) {
	tests := []struct {
		name   string
		status int
		err    error
		panic  bool
	}{
		{name: "success", status: http.StatusOK},
		{name: "domain error", status: http.StatusNotFound, err: trip.ErrTripNotFound},
		{name: "internal error", status: http.StatusInternalServerError, err: errors.New(privateError)},
		{name: "recovered panic", status: http.StatusInternalServerError, panic: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			exporter := tracetest.NewInMemoryExporter()
			traces := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
			reader := sdkmetric.NewManualReader()
			meters := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
			t.Cleanup(func() { _ = traces.Shutdown(t.Context()); _ = meters.Shutdown(t.Context()) })
			var logs bytes.Buffer
			logger, err := telemetry.NewLogger(&logs, "info", "trip-service")
			if err != nil {
				t.Fatal(err)
			}
			metrics, err := middleware.NewHTTPMetrics(meters.Meter("test"))
			if err != nil {
				t.Fatal(err)
			}
			getter := NewMocktripGetter(ctrl)
			id := uuid.UUID{1}
			getter.EXPECT().Handle(gomock.Any(), gomock.Any(), id).Do(func(w http.ResponseWriter, r *http.Request, _ api.TripId) {
				if active := activeRequests(t, reader); active != 1 {
					t.Errorf("active requests inside handler = %d, want 1", active)
				}
				if tt.panic {
					panic(privateError)
				}
				if tt.err != nil {
					response.Error(w, r, logger, "test request", tt.err)
					return
				}
				w.WriteHeader(tt.status)
			})
			router := transport.NewRouter(NewMocktripCreator(ctrl), getter, NewMocktripFinisher(ctrl),
				NewMockprobe(ctrl), NewMockprobe(ctrl), logger, traces, metrics)
			req := httptest.NewRequest(http.MethodGet, tripPath+"/"+id.String(), strings.NewReader("private body"))
			req.Header.Set("traceparent", parentTraceHeader)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)
			if recorder.Code != tt.status {
				t.Fatalf("response status = %d, want %d", recorder.Code, tt.status)
			}
			if strings.Count(logs.String(), "\n") != 1 {
				t.Fatalf("want exactly one request log, got %s", logs.String())
			}
			var entry map[string]any
			if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
				t.Fatal(err)
			}
			wantLevel := "INFO"
			if tt.status >= http.StatusInternalServerError {
				wantLevel = errorLogLevel
				if !strings.Contains(entry["error"].(string), privateError) {
					t.Error("internal failure details are missing from the log")
				}
			}
			if entry["msg"] != "request handled" || entry["route"] != getRoute || entry["level"] != wantLevel ||
				entry["status"] != float64(tt.status) || entry["trace_id"] != parentTraceID || entry["duration_ms"] == nil {
				t.Errorf("incorrect request log: %+v", entry)
			}
			if strings.Contains(logs.String(), "private body") || strings.Contains(recorder.Body.String(), privateError) {
				t.Error("request body was logged or internal details leaked to the client")
			}
			if tt.panic && !strings.Contains(entry["error"].(string), "goroutine") {
				t.Error("panic log has no stack trace")
			}
			assertHTTPSpan(t, exporter.GetSpans(), entry, tt.status)
			assertHTTPMetrics(t, reader, tt.status)
		})
	}
}

func TestPanicAfterResponseStarted(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "headers already sent"},
		{name: "body already started", body: "partial response"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			exporter := tracetest.NewInMemoryExporter()
			traces := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
			reader := sdkmetric.NewManualReader()
			meters := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
			t.Cleanup(func() { _ = traces.Shutdown(t.Context()); _ = meters.Shutdown(t.Context()) })
			var logs bytes.Buffer
			logger, err := telemetry.NewLogger(&logs, "info", "trip-service")
			if err != nil {
				t.Fatal(err)
			}
			metrics, err := middleware.NewHTTPMetrics(meters.Meter("test"))
			if err != nil {
				t.Fatal(err)
			}
			getter := NewMocktripGetter(ctrl)
			id := uuid.UUID{1}
			getter.EXPECT().Handle(gomock.Any(), gomock.Any(), id).Do(func(w http.ResponseWriter, _ *http.Request, _ api.TripId) {
				w.WriteHeader(http.StatusOK)
				if tt.body != "" {
					_, _ = w.Write([]byte(tt.body))
				}
				panic(privateError)
			})
			router := transport.NewRouter(NewMocktripCreator(ctrl), getter, NewMocktripFinisher(ctrl),
				NewMockprobe(ctrl), NewMockprobe(ctrl), logger, traces, metrics)
			req := httptest.NewRequest(http.MethodGet, tripPath+"/"+id.String(), nil)
			req.Header.Set("traceparent", parentTraceHeader)
			recorder := httptest.NewRecorder()
			var recovered any
			func() {
				defer func() { recovered = recover() }()
				router.ServeHTTP(recorder, req)
			}()
			abort, ok := recovered.(error)
			if !ok || !errors.Is(abort, http.ErrAbortHandler) {
				t.Errorf("panic = %v, want http.ErrAbortHandler to abort the incomplete response", recovered)
			}
			if recorder.Code != http.StatusOK || recorder.Body.String() != tt.body {
				t.Errorf("started response was changed: status=%d body=%q, want status200 and body%q", recorder.Code, recorder.Body.String(), tt.body)
			}
			if strings.Count(logs.String(), "\n") != 1 {
				t.Fatalf("want exactly one panic log, got %s", logs.String())
			}
			var entry map[string]any
			if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
				t.Fatal(err)
			}
			details, _ := entry["error"].(string)
			if entry["level"] != errorLogLevel || entry["status"] != float64(http.StatusOK) ||
				!strings.Contains(details, privateError) || !strings.Contains(details, "goroutine") {
				t.Errorf("panic log lost its level, actual status, error or stack: %+v", entry)
			}
			spans := exporter.GetSpans()
			assertHTTPSpan(t, spans, entry, http.StatusOK)
			attrs := attribute.NewSet(spans[0].Attributes...)
			errorType, _ := attrs.Value("error.type")
			if spans[0].Status.Code != codes.Error || errorType.AsString() != "panic" {
				t.Errorf("panic span was not marked as an error: status=%+v attrs=%+v", spans[0].Status, attrs)
			}
			assertHTTPMetrics(t, reader, http.StatusOK)
		})
	}
}

func TestPanicAfterFlushedResponseAbortsConnection(t *testing.T) {
	ctrl := gomock.NewController(t)
	exporter := tracetest.NewInMemoryExporter()
	traces := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	reader := sdkmetric.NewManualReader()
	meters := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = traces.Shutdown(t.Context()); _ = meters.Shutdown(t.Context()) })
	var logs bytes.Buffer
	logger, err := telemetry.NewLogger(&logs, "info", "trip-service")
	if err != nil {
		t.Fatal(err)
	}
	metrics, err := middleware.NewHTTPMetrics(meters.Meter("test"))
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.UUID{1}
	path := tripPath + "/" + id.String()
	const partialBody = `{"trip":`
	getter := NewMocktripGetter(ctrl)
	getter.EXPECT().Handle(gomock.Any(), gomock.Any(), id).Do(func(w http.ResponseWriter, _ *http.Request, _ api.TripId) {
		w.Header().Set("Content-Length", "1024")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(partialBody))
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Errorf("flush partial response: %v", err)
		}
		panic(privateError)
	})
	health := NewMockprobe(ctrl)
	health.EXPECT().ServeHTTP(gomock.Any(), gomock.Any()).Do(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	router := transport.NewRouter(NewMocktripCreator(ctrl), getter, NewMocktripFinisher(ctrl),
		health, NewMockprobe(ctrl), logger, traces, metrics)
	done := make(chan struct{})
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == path {
			defer close(done)
		}
		router.ServeHTTP(w, r)
	}))
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.Start()
	t.Cleanup(server.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("traceparent", parentTraceHeader)
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("receive flushed response headers: %v", err)
	}
	body, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !errors.Is(readErr, io.ErrUnexpectedEOF) || string(body) != partialBody {
		t.Errorf("response was not aborted cleanly: status=%d read error=%v body=%q", resp.StatusCode, readErr, body)
	}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatalf("panic handler did not complete: %v", ctx.Err())
	}
	if strings.Count(logs.String(), "\n") != 1 {
		t.Fatalf("want one panic log before the health request, got %s", logs.String())
	}
	var entry map[string]any
	if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	details, _ := entry["error"].(string)
	if entry["level"] != errorLogLevel || !strings.Contains(details, privateError) || !strings.Contains(details, "goroutine") {
		t.Errorf("aborted response has no single ERROR log with panic details and stack: %+v", entry)
	}
	assertHTTPSpan(t, exporter.GetSpans(), entry, http.StatusOK)
	assertHTTPMetrics(t, reader, http.StatusOK)
	healthReq, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+healthPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	healthResp, err := server.Client().Do(healthReq)
	if err != nil {
		t.Fatalf("service stopped serving requests after the panic: %v", err)
	}
	_ = healthResp.Body.Close()
	if healthResp.StatusCode != http.StatusOK {
		t.Errorf("health status after panic=%d, want200", healthResp.StatusCode)
	}
}

func assertHTTPSpan(t *testing.T, spans tracetest.SpanStubs, entry map[string]any, status int) {
	t.Helper()
	if len(spans) != 1 {
		t.Fatalf("HTTP span count = %d, want 1", len(spans))
	}
	span := spans[0]
	if span.Name != http.MethodGet+" "+getRoute || span.Parent.SpanID().String() != parentSpanID ||
		span.SpanContext.TraceID().String() != parentTraceID || entry["span_id"] != span.SpanContext.SpanID().String() {
		t.Errorf("incorrect span identity or name: %+v", span)
	}
	spanAttrs := attribute.NewSet(span.Attributes...)
	code, _ := spanAttrs.Value("http.response.status_code")
	if code.AsInt64() != int64(status) {
		t.Errorf("HTTP span status = %d, want %d", code.AsInt64(), status)
	}
	if status >= http.StatusInternalServerError && (span.Status.Code != codes.Error || !spanAttrs.HasValue("error.type")) {
		t.Error("HTTP failure did not mark the span with error.type and codes.Error")
	}
}

func activeRequests(t *testing.T, reader *sdkmetric.ManualReader) int64 {
	t.Helper()
	var data metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &data); err != nil {
		t.Fatal(err)
	}
	var active int64
	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name == httpActive {
				for _, point := range metric.Data.(metricdata.Sum[int64]).DataPoints {
					active += point.Value
				}
			}
		}
	}
	return active
}

func assertHTTPMetrics(t *testing.T, reader *sdkmetric.ManualReader, status int) {
	t.Helper()
	if active := activeRequests(t, reader); active != 0 {
		t.Errorf("active requests after response = %d, want zero", active)
	}
	var data metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &data); err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != httpDuration {
				continue
			}
			found = true
			points := metric.Data.(metricdata.Histogram[float64]).DataPoints
			if metric.Unit != "s" || len(points) != 1 || points[0].Count != 1 {
				t.Fatalf("incorrect HTTP histogram: %+v", metric)
			}
			route, _ := points[0].Attributes.Value("http.route")
			method, _ := points[0].Attributes.Value("http.request.method")
			code, _ := points[0].Attributes.Value("http.response.status_code")
			if route.AsString() != getRoute || method.AsString() != http.MethodGet || code.AsInt64() != int64(status) {
				t.Errorf("incorrect HTTP metric labels: %+v", points[0].Attributes)
			}
		}
	}
	if !found {
		t.Error("HTTP duration was not recorded")
	}
}
