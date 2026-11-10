package ready_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/codes"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/mock/gomock"

	api "github.com/Nikola4667636/template/internal/generated"
	"github.com/Nikola4667636/template/internal/http/middleware"
	"github.com/Nikola4667636/template/internal/http/ready"
	"github.com/Nikola4667636/template/internal/infra/telemetry"
)

type requestKey struct{}

func TestServeHTTP(t *testing.T) {
	tests := []struct {
		name       string
		pingErr    error
		noDB       bool
		cancelled  bool
		timeout    bool
		wantStatus int
		wantBody   api.HealthResponseStatus
	}{
		{name: "database ready", wantStatus: http.StatusOK, wantBody: api.Ok},
		{name: "database unavailable", pingErr: errors.New("database unavailable"), wantStatus: http.StatusServiceUnavailable, wantBody: api.Unavailable},
		{name: "database missing", noDB: true, wantStatus: http.StatusServiceUnavailable, wantBody: api.Unavailable},
		{name: "request cancelled", cancelled: true, wantStatus: http.StatusServiceUnavailable, wantBody: api.Unavailable},
		{name: "ping times out", timeout: true, wantStatus: http.StatusServiceUnavailable, wantBody: api.Unavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			db := NewMockpinger(ctrl)
			ctx, cancel := context.WithCancel(context.WithValue(t.Context(), requestKey{}, tt.name))
			defer cancel()
			if tt.cancelled {
				cancel()
			}
			timeout := time.Second
			if tt.timeout {
				timeout = 0
			}
			var pingCtx context.Context
			if !tt.noDB {
				db.EXPECT().Ping(gomock.Any()).DoAndReturn(func(ctx context.Context) error {
					pingCtx = ctx
					if ctx.Value(requestKey{}) != tt.name {
						t.Error("request context value was not forwarded")
					}
					if _, ok := ctx.Deadline(); !ok {
						t.Error("ping context has no deadline")
					}
					if tt.timeout || tt.cancelled {
						<-ctx.Done()
						return ctx.Err()
					}
					return tt.pingErr
				})
			}
			h := ready.New(db, timeout)
			if tt.noDB {
				h = ready.New(nil, timeout)
			}
			recorder := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/ready", nil).WithContext(ctx)

			h.ServeHTTP(recorder, req)

			if recorder.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", recorder.Code, tt.wantStatus)
			}
			if got := recorder.Header().Get("Content-Type"); got != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", got)
			}
			var body api.HealthResponse
			if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.Status != tt.wantBody {
				t.Errorf("body status = %q, want %q", body.Status, tt.wantBody)
			}
			if pingCtx != nil && pingCtx.Err() == nil {
				t.Error("ping context was not cancelled after the handler returned")
			}
		})
	}
}

func TestReadinessFailureIsLoggedWithCause(t *testing.T) {
	ctrl := gomock.NewController(t)
	db := NewMockpinger(ctrl)
	const privateDetails = "private PostgreSQL connection failure"
	db.EXPECT().Ping(gomock.Any()).Return(errors.New(privateDetails))
	var logs bytes.Buffer
	logger, err := telemetry.NewLogger(&logs, "info", "trip-service")
	if err != nil {
		t.Fatal(err)
	}
	exporter := tracetest.NewInMemoryExporter()
	traces := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() { _ = traces.Shutdown(t.Context()) })
	metrics, err := middleware.NewHTTPMetrics(metricnoop.NewMeterProvider().Meter("test"))
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	router.Get("/ready", ready.New(db, time.Second).ServeHTTP)
	handler := middleware.Recover(logger)(middleware.Tracing(traces, router)(middleware.Logging(logger)(metrics.Middleware(router))))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if recorder.Code != http.StatusServiceUnavailable || recorder.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("unexpected readiness response: status=%d headers=%v", recorder.Code, recorder.Header())
	}
	var body api.HealthResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != api.Unavailable || strings.Contains(recorder.Body.String(), privateDetails) {
		t.Errorf("readiness response leaked the failure or changed its contract: %s", recorder.Body.String())
	}
	if strings.Count(logs.String(), "\n") != 1 {
		t.Fatalf("want exactly one failure log, got %s", logs.String())
	}
	var entry map[string]any
	if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	details, _ := entry["error"].(string)
	if entry["msg"] != "request handled" || entry["level"] != "ERROR" ||
		entry["status"] != float64(http.StatusServiceUnavailable) || !strings.Contains(details, privateDetails) {
		t.Errorf("readiness failure was not logged once with its cause: %+v", entry)
	}
	spans := exporter.GetSpans()
	if len(spans) != 1 || spans[0].Status.Code != codes.Error {
		t.Fatalf("readiness failure span is missing or not marked as an error: %+v", spans)
	}
	var foundCause bool
	for _, event := range spans[0].Events {
		for _, attr := range event.Attributes {
			if attr.Key == "exception.message" && strings.Contains(attr.Value.AsString(), privateDetails) {
				foundCause = true
			}
		}
	}
	if !foundCause {
		t.Error("readiness span lost the underlying Ping error")
	}
}
