package finish_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	domain "github.com/Nikola4667636/template/internal/domain/trip"
	api "github.com/Nikola4667636/template/internal/generated"
	"github.com/Nikola4667636/template/internal/http/trip/finish"
)

func TestHandle(t *testing.T) {
	id := uuid.UUID{1}
	startedAt := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(time.Hour)
	completed := &domain.Trip{
		ID: id, UserID: uuid.UUID{2}, DriverID: uuid.UUID{3}, Status: domain.TripStatusCompleted,
		StartPoint: domain.Coordinates{Latitude: 59.9398, Longitude: 30.3146},
		EndPoint:   domain.Coordinates{Latitude: 59.929, Longitude: 30.3626},
		Price:      1450, StartedAt: startedAt, FinishedAt: &finishedAt,
	}
	internalErr := errors.New("database update failed: UPDATE trips")
	tests := []struct {
		name       string
		result     *domain.Trip
		err        error
		wantStatus int
		wantCode   string
		wantDetail string
	}{
		{name: "trip finished", result: completed, wantStatus: http.StatusOK},
		{
			name: "already completed", err: fmt.Errorf("finish transaction: %w", domain.ErrTripCompleted),
			wantStatus: http.StatusConflict, wantCode: "trip_completed", wantDetail: "trip is already completed",
		},
		{
			name: "not found", err: fmt.Errorf("lock trip: %w", domain.ErrTripNotFound),
			wantStatus: http.StatusNotFound, wantCode: "trip_not_found", wantDetail: "trip not found",
		},
		{
			name: "internal details are hidden", err: fmt.Errorf("finish transaction: %w", internalErr),
			wantStatus: http.StatusInternalServerError, wantCode: "internal_error", wantDetail: "internal error",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			uc := NewMockuseCase(ctrl)
			logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
			path := "/api/v1/trips/" + id.String() + "/finish"
			req := httptest.NewRequest(http.MethodPost, path, nil).WithContext(t.Context())
			uc.EXPECT().Execute(req.Context(), id).Return(tt.result, tt.err)
			recorder := httptest.NewRecorder()

			finish.New(uc, logger).Handle(recorder, req, id)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
			if tt.err == nil {
				if got := recorder.Header().Get("Content-Type"); got != "application/json" {
					t.Errorf("Content-Type = %q, want application/json", got)
				}
				var body api.Trip
				if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				want := api.Trip{
					Id: id, UserId: completed.UserID, DriverId: completed.DriverID, Status: api.Completed,
					StartPoint: api.Coordinates{Latitude: 59.9398, Longitude: 30.3146},
					EndPoint:   api.Coordinates{Latitude: 59.929, Longitude: 30.3626},
					Price:      1450, StartedAt: startedAt, FinishedAt: &finishedAt,
				}
				if !reflect.DeepEqual(body, want) {
					t.Errorf("trip response = %+v, want %+v", body, want)
				}
				return
			}
			if got := recorder.Header().Get("Content-Type"); got != "application/problem+json" {
				t.Errorf("Content-Type = %q, want application/problem+json", got)
			}
			if strings.Contains(recorder.Body.String(), internalErr.Error()) {
				t.Error("response exposes internal error details")
			}
			var body api.Problem
			if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if int(body.Status) != tt.wantStatus || body.Code != tt.wantCode || body.Type == "" || body.Title == "" {
				t.Errorf("invalid problem response: %+v", body)
			}
			if body.Detail == nil || *body.Detail != tt.wantDetail {
				t.Errorf("detail = %v, want %q", body.Detail, tt.wantDetail)
			}
			if body.Instance == nil || *body.Instance != path {
				t.Errorf("instance = %v, want %q", body.Instance, path)
			}
		})
	}
}
