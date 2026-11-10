package middleware_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	api "github.com/Nikola4667636/template/internal/generated"
	"github.com/Nikola4667636/template/internal/http/middleware"
)

func TestRecover(t *testing.T) {
	const panicDetails = "private panic details"
	tests := []struct {
		name       string
		panic      bool
		wantStatus int
	}{
		{name: "normal response", wantStatus: http.StatusAccepted},
		{name: "panic becomes a safe error", panic: true, wantStatus: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, nil))
			type contextKey struct{}
			ctx := context.WithValue(t.Context(), contextKey{}, true)
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Context().Value(contextKey{}) != true {
					t.Error("request context values were lost")
				}
				if tt.panic {
					panic(panicDetails)
				}
				w.WriteHeader(http.StatusAccepted)
				_, _ = w.Write([]byte("ok"))
			})
			h := middleware.Recover(logger)(next)
			req := httptest.NewRequest(http.MethodGet, "/test", nil).WithContext(ctx)
			recorder := httptest.NewRecorder()

			h.ServeHTTP(recorder, req)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.wantStatus)
			}
			if !tt.panic {
				if recorder.Body.String() != "ok" || logs.Len() != 0 {
					t.Errorf("unexpected body or log: body=%q log=%q", recorder.Body.String(), logs.String())
				}
				return
			}
			if got := recorder.Header().Get("Content-Type"); got != "application/problem+json" {
				t.Errorf("Content-Type = %q, want application/problem+json", got)
			}
			if strings.Contains(recorder.Body.String(), panicDetails) {
				t.Error("panic details leaked into the response")
			}
			var body api.Problem
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Status != http.StatusInternalServerError || body.Code != "internal_error" || body.Detail == nil || *body.Detail != "internal error" {
				t.Errorf("unexpected problem response: %+v", body)
			}
			if body.Instance == nil || *body.Instance != req.URL.Path {
				t.Errorf("instance = %v, want request path", body.Instance)
			}
			if strings.Count(logs.String(), "\n") != 1 {
				t.Fatalf("want one panic log entry, got %q", logs.String())
			}
			var entry map[string]any
			if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
				t.Fatal(err)
			}
			details, ok := entry["error"].(string)
			if !ok || !strings.Contains(details, panicDetails) || !strings.Contains(details, "goroutine") || entry["level"] != "ERROR" {
				t.Errorf("panic log lacks details or stack trace: %+v", entry)
			}
		})
	}
}
