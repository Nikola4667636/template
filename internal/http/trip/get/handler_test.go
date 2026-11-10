package get_test

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
	"github.com/Nikola4667636/template/internal/http/trip/get"
)

const (
	testTripNotFoundCode   = "trip_not_found"
	testTripNotFoundDetail = "trip not found"
)

func TestGetTrip(t *testing.T) {
	id := uuid.UUID{1}
	startedAt := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(time.Hour)
	active := &domain.Trip{
		ID:         id,
		UserID:     uuid.UUID{2},
		DriverID:   uuid.UUID{3},
		Status:     domain.TripStatusActive,
		StartPoint: domain.Coordinates{Latitude: 59.9398, Longitude: 30.3146},
		EndPoint:   domain.Coordinates{Latitude: 59.929, Longitude: 30.3626},
		Price:      1450,
		StartedAt:  startedAt,
	}
	wantActive := &api.Trip{
		Id:         id,
		UserId:     uuid.UUID{2},
		DriverId:   uuid.UUID{3},
		Status:     "active",
		StartPoint: api.Coordinates{Latitude: 59.9398, Longitude: 30.3146},
		EndPoint:   api.Coordinates{Latitude: 59.929, Longitude: 30.3626},
		Price:      1450,
		StartedAt:  startedAt,
	}
	completed := *active
	completed.Status = domain.TripStatusCompleted
	completed.FinishedAt = &finishedAt
	wantCompleted := *wantActive
	wantCompleted.Status = "completed"
	wantCompleted.FinishedAt = &finishedAt
	internalErr := errors.New("database query failed: SELECT trips")

	tests := []struct {
		name       string
		trip       *domain.Trip
		err        error
		wantStatus int
		wantTrip   *api.Trip
		wantCode   string
		wantDetail string
	}{
		{
			name:       "active trip",
			trip:       active,
			wantStatus: http.StatusOK,
			wantTrip:   wantActive,
		},
		{
			name:       "completed trip",
			trip:       &completed,
			wantStatus: http.StatusOK,
			wantTrip:   &wantCompleted,
		},
		{
			name:       "trip not found",
			err:        domain.ErrTripNotFound,
			wantStatus: http.StatusNotFound,
			wantCode:   testTripNotFoundCode,
			wantDetail: testTripNotFoundDetail,
		},
		{
			name:       "wrapped trip not found",
			err:        fmt.Errorf("get trip by id: %w", domain.ErrTripNotFound),
			wantStatus: http.StatusNotFound,
			wantCode:   testTripNotFoundCode,
			wantDetail: testTripNotFoundDetail,
		},
		{
			name:       "internal details are hidden",
			err:        fmt.Errorf("get trip by id: %w", internalErr),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "internal_error",
			wantDetail: "internal error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			getTrip := NewMockuseCase(ctrl)
			logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
			h := get.New(getTrip, logger)
			path := "/api/v1/trips/" + id.String()
			req := httptest.NewRequest(http.MethodGet, path, nil)
			recorder := httptest.NewRecorder()
			getTrip.EXPECT().Execute(req.Context(), id).Return(tt.trip, tt.err)

			h.Handle(recorder, req, id)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
			if tt.wantTrip != nil {
				if got := recorder.Header().Get("Content-Type"); got != "application/json" {
					t.Errorf("Content-Type = %q, want application/json", got)
				}
				var body api.Trip
				if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
					t.Fatalf("decode trip response: %v", err)
				}
				if !reflect.DeepEqual(&body, tt.wantTrip) {
					t.Errorf("trip response = %+v, want %+v", body, *tt.wantTrip)
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
				t.Fatalf("decode problem response: %v", err)
			}
			if int(body.Status) != tt.wantStatus || body.Code != tt.wantCode {
				t.Errorf("problem = %+v, want status %d and code %q", body, tt.wantStatus, tt.wantCode)
			}
			if body.Type == "" || body.Title == "" {
				t.Errorf("problem has an empty type or title: %+v", body)
			}
			if body.Instance == nil || *body.Instance != path {
				t.Errorf("problem instance = %v, want %q", body.Instance, path)
			}
			if body.Detail == nil || *body.Detail == "" {
				t.Fatal("problem detail is empty")
			}
			if tt.wantDetail != "" && *body.Detail != tt.wantDetail {
				t.Errorf("problem detail = %q, want %q", *body.Detail, tt.wantDetail)
			}
		})
	}
}
