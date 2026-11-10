package domainerror_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/Nikola4667636/template/internal/domain/trip"
	"github.com/Nikola4667636/template/internal/http/domainerror"
)

func TestProblem(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		wantTitle  string
		wantType   string
		wantDetail string
	}{
		{
			name:       "validation details",
			err:        &trip.ValidationError{Field: "price", Reason: "must not be negative"},
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_request",
			wantTitle:  "Invalid request",
			wantType:   "invalid-request",
			wantDetail: "price: must not be negative",
		},
		{
			name:       "invalid arguments without details",
			err:        trip.ErrInvalidArguments,
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_request",
			wantTitle:  "Invalid request",
			wantType:   "invalid-request",
			wantDetail: "invalid input arguments",
		},
		{
			name:       "trip not found",
			err:        trip.ErrTripNotFound,
			wantStatus: http.StatusNotFound,
			wantCode:   "trip_not_found",
			wantTitle:  "Trip not found",
			wantType:   "trip-not-found",
			wantDetail: "trip not found",
		},
		{
			name:       "trip completed",
			err:        trip.ErrTripCompleted,
			wantStatus: http.StatusConflict,
			wantCode:   "trip_completed",
			wantTitle:  "Trip completed",
			wantType:   "trip-completed",
			wantDetail: "trip is already completed",
		},
		{
			name:       "driver busy",
			err:        trip.ErrDriverBusy,
			wantStatus: http.StatusConflict,
			wantCode:   "driver_busy",
			wantTitle:  "Driver busy",
			wantType:   "driver-busy",
			wantDetail: "driver already has an active trip",
		},
		{
			name:       "internal error details are hidden",
			err:        errors.New("database query failed: SELECT trips"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "internal_error",
			wantTitle:  "Internal error",
			wantType:   "internal-error",
			wantDetail: "internal error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, wrapped := range []bool{false, true} {
				t.Run(fmt.Sprintf("wrapped=%t", wrapped), func(t *testing.T) {
					err := tt.err
					if wrapped {
						err = fmt.Errorf("internal operation context: %w", err)
					}
					status, body := domainerror.Problem(err)
					if status != tt.wantStatus || int(body.Status) != status {
						t.Errorf("status = %d, body status = %d, want %d", status, body.Status, tt.wantStatus)
					}
					if body.Code != tt.wantCode || body.Title != tt.wantTitle {
						t.Errorf("problem = %+v, want code %q and title %q", body, tt.wantCode, tt.wantTitle)
					}
					if want := "https://tripgo.example/problems/" + tt.wantType; body.Type != want {
						t.Errorf("type = %q, want %q", body.Type, want)
					}
					if body.Detail == nil || *body.Detail != tt.wantDetail {
						t.Errorf("detail = %v, want %q", body.Detail, tt.wantDetail)
					}
					if body.Instance != nil {
						t.Error("pure error mapping must not set a request-specific instance")
					}
				})
			}
		})
	}
}

func TestNewWithoutDetail(t *testing.T) {
	body := domainerror.New(http.StatusNotFound, "not_found", "Not found", "")
	if body.Detail != nil {
		t.Errorf("detail = %v, want nil", body.Detail)
	}
}
