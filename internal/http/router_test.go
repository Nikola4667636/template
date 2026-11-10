package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	api "github.com/Nikola4667636/template/internal/generated"
	transport "github.com/Nikola4667636/template/internal/http"
)

type routeContextKey struct{}

type routeAction int

const (
	actionCreate routeAction = iota
	actionGet
	actionFinish
	actionHealth
	actionReady
)

const (
	tripPath           = "/api/v1/trips"
	healthPath         = "/health"
	invalidRequestCode = "invalid_request"
)

type handlers struct {
	create *MocktripCreator
	get    *MocktripGetter
	finish *MocktripFinisher
	health *Mockprobe
	ready  *Mockprobe
	logs   bytes.Buffer
}

func newRouter(t *testing.T) (http.Handler, *handlers) {
	t.Helper()
	ctrl := gomock.NewController(t)
	h := &handlers{
		create: NewMocktripCreator(ctrl), get: NewMocktripGetter(ctrl), finish: NewMocktripFinisher(ctrl),
		health: NewMockprobe(ctrl), ready: NewMockprobe(ctrl),
	}
	logger := slog.New(slog.NewJSONHandler(&h.logs, nil))
	return transport.NewRouter(h.create, h.get, h.finish, h.health, h.ready, logger), h
}

func TestRoutes(t *testing.T) {
	id := uuid.UUID{1}
	idempotencyKey := uuid.UUID{2}
	tests := []struct {
		name   string
		method string
		path   string
		action routeAction
		key    bool
	}{
		{name: "create", method: http.MethodPost, path: tripPath, action: actionCreate},
		{name: "create with optional header", method: http.MethodPost, path: tripPath, action: actionCreate, key: true},
		{name: "get", method: http.MethodGet, path: tripPath + "/" + id.String(), action: actionGet},
		{name: "finish", method: http.MethodPost, path: tripPath + "/" + id.String() + "/finish", action: actionFinish},
		{name: "health", method: http.MethodGet, path: healthPath, action: actionHealth},
		{name: "ready", method: http.MethodGet, path: "/ready", action: actionReady},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, h := newRouter(t)
			respond := func(w http.ResponseWriter, r *http.Request) {
				if r.Context().Value(routeContextKey{}) != tt.name {
					t.Error("request context was not forwarded")
				}
				w.WriteHeader(http.StatusNoContent)
			}
			switch tt.action {
			case actionCreate:
				h.create.EXPECT().Handle(gomock.Any(), gomock.Any(), gomock.Any()).Do(
					func(w http.ResponseWriter, r *http.Request, params api.CreateTripParams) {
						if tt.key {
							if params.IdempotencyKey == nil || *params.IdempotencyKey != idempotencyKey {
								t.Errorf("Idempotency-Key = %v, want %s", params.IdempotencyKey, idempotencyKey)
							}
						} else if params.IdempotencyKey != nil {
							t.Error("unexpected Idempotency-Key")
						}
						respond(w, r)
					},
				)
			case actionGet:
				h.get.EXPECT().Handle(gomock.Any(), gomock.Any(), id).Do(
					func(w http.ResponseWriter, r *http.Request, _ api.TripId) { respond(w, r) },
				)
			case actionFinish:
				h.finish.EXPECT().Handle(gomock.Any(), gomock.Any(), id).Do(
					func(w http.ResponseWriter, r *http.Request, _ api.TripId) { respond(w, r) },
				)
			case actionHealth:
				h.health.EXPECT().ServeHTTP(gomock.Any(), gomock.Any()).Do(respond)
			case actionReady:
				h.ready.EXPECT().ServeHTTP(gomock.Any(), gomock.Any()).Do(respond)
			}
			ctx := context.WithValue(t.Context(), routeContextKey{}, tt.name)
			req := httptest.NewRequest(tt.method, tt.path, nil).WithContext(ctx)
			if tt.key {
				req.Header.Set("Idempotency-Key", idempotencyKey.String())
			}
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, req)

			if recorder.Code != http.StatusNoContent {
				t.Errorf("status = %d, want 204; body = %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestInvalidRoutes(t *testing.T) {
	key := uuid.UUID{2}.String()
	tests := []struct {
		name       string
		method     string
		path       string
		headers    []string
		wantStatus int
		wantCode   string
		wantDetail string
	}{
		{
			name: "invalid get UUID", method: http.MethodGet, path: tripPath + "/not-a-uuid",
			wantStatus: http.StatusBadRequest, wantCode: invalidRequestCode, wantDetail: "tripId: must be a valid UUID",
		},
		{
			name: "invalid finish UUID", method: http.MethodPost, path: tripPath + "/not-a-uuid/finish",
			wantStatus: http.StatusBadRequest, wantCode: invalidRequestCode, wantDetail: "tripId: must be a valid UUID",
		},
		{
			name: "invalid optional header", method: http.MethodPost, path: tripPath, headers: []string{"not-a-uuid"},
			wantStatus: http.StatusBadRequest, wantCode: invalidRequestCode, wantDetail: "Idempotency-Key: must be a valid UUID",
		},
		{
			name: "duplicate optional header", method: http.MethodPost, path: tripPath, headers: []string{key, key},
			wantStatus: http.StatusBadRequest, wantCode: invalidRequestCode, wantDetail: "Idempotency-Key: must appear once",
		},
		{
			name: "unknown route", method: http.MethodGet, path: "/missing",
			wantStatus: http.StatusNotFound, wantCode: "not_found", wantDetail: "route not found",
		},
		{
			name: "unsupported method", method: http.MethodPost, path: healthPath,
			wantStatus: http.StatusMethodNotAllowed, wantCode: "method_not_allowed", wantDetail: "method is not allowed for this route",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, _ := newRouter(t)
			req := httptest.NewRequest(tt.method, tt.path, nil)
			for _, header := range tt.headers {
				req.Header.Add("Idempotency-Key", header)
			}
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, req)

			assertProblem(t, recorder, tt.path, tt.wantStatus, tt.wantCode, tt.wantDetail)
		})
	}
}

func TestRecover(t *testing.T) {
	router, h := newRouter(t)
	id := uuid.UUID{1}
	const panicDetail = "private panic details"
	h.get.EXPECT().Handle(gomock.Any(), gomock.Any(), id).Do(
		func(http.ResponseWriter, *http.Request, api.TripId) { panic(panicDetail) },
	)
	path := tripPath + "/" + id.String()
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))

	assertProblem(t, recorder, path, http.StatusInternalServerError, "internal_error", "internal error")
	if strings.Contains(recorder.Body.String(), panicDetail) || strings.Contains(recorder.Body.String(), "goroutine") {
		t.Error("panic details or stack trace leaked into the response")
	}
	if strings.Count(h.logs.String(), "\n") != 1 {
		t.Fatalf("want one panic log entry, got %q", h.logs.String())
	}
	var entry map[string]any
	if err := json.Unmarshal(h.logs.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	details, ok := entry["error"].(string)
	if !ok || !strings.Contains(details, panicDetail) || !strings.Contains(details, "goroutine") || entry["level"] != "ERROR" {
		t.Errorf("panic log lacks error details or stack trace: %+v", entry)
	}
	h.health.EXPECT().ServeHTTP(gomock.Any(), gomock.Any()).Do(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	next := httptest.NewRecorder()
	router.ServeHTTP(next, httptest.NewRequest(http.MethodGet, healthPath, nil))
	if next.Code != http.StatusOK {
		t.Errorf("request after panic returned %d, want 200", next.Code)
	}
}

func assertProblem(t *testing.T, recorder *httptest.ResponseRecorder, path string, status int, code, detail string) {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, status, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Errorf("Content-Type = %q, want application/problem+json", got)
	}
	var body api.Problem
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if int(body.Status) != status || body.Code != code || body.Detail == nil || *body.Detail != detail || body.Type == "" || body.Title == "" {
		t.Errorf("invalid problem response: %+v", body)
	}
	if body.Instance == nil || *body.Instance != path {
		t.Errorf("instance = %v, want %q", body.Instance, path)
	}
}
