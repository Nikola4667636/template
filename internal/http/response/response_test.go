package response_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Nikola4667636/template/internal/domain/trip"
	api "github.com/Nikola4667636/template/internal/generated"
	"github.com/Nikola4667636/template/internal/http/response"
)

func TestJSON(t *testing.T) {
	recorder := httptest.NewRecorder()

	response.JSON(recorder, http.StatusCreated, map[string]string{"result": "ok"})

	if recorder.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	var body map[string]string
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["result"] != "ok" {
		t.Errorf("body = %+v, want result ok", body)
	}
}

func TestErrorLogging(t *testing.T) {
	internalErr := errors.New("query failed: SELECT trips")
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantLog    bool
	}{
		{name: "client error is not logged as a failure", err: trip.ErrTripNotFound, wantStatus: http.StatusNotFound},
		{name: "internal error is logged once", err: fmt.Errorf("load trip: %w", internalErr), wantStatus: http.StatusInternalServerError, wantLog: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, nil))
			req := httptest.NewRequest(http.MethodGet, "/api/v1/trips/example", nil)
			recorder := httptest.NewRecorder()

			response.Error(recorder, req, logger, "get trip", tt.err)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.wantStatus)
			}
			if got := recorder.Header().Get("Content-Type"); got != "application/problem+json" {
				t.Errorf("Content-Type = %q, want application/problem+json", got)
			}
			if strings.Contains(recorder.Body.String(), internalErr.Error()) {
				t.Error("response exposes the repository error")
			}
			var body api.Problem
			if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.Instance == nil || *body.Instance != req.URL.Path {
				t.Errorf("instance = %v, want request path", body.Instance)
			}
			if !tt.wantLog {
				if logs.Len() != 0 {
					t.Errorf("unexpected error log: %s", logs.String())
				}
				return
			}
			if strings.Count(logs.String(), "\n") != 1 {
				t.Fatalf("want one log entry, got %q", logs.String())
			}
			var entry map[string]any
			if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
				t.Fatal(err)
			}
			if entry["level"] != "ERROR" || entry["op"] != "get trip" || entry["error"] != tt.err.Error() {
				t.Errorf("log entry lacks failure details: %+v", entry)
			}
		})
	}
}
