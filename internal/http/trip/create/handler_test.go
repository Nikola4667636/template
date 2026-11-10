package create_test

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
	"github.com/Nikola4667636/template/internal/http/trip/create"
)

const (
	createPath             = "/api/v1/trips"
	testInvalidRequestCode = "invalid_request"
	testPriceField         = "price"
)

func TestHandleSuccess(t *testing.T) {
	for _, zeroValues := range []bool{false, true} {
		t.Run(fmt.Sprintf("zero_values=%t", zeroValues), func(t *testing.T) {
			ctrl := gomock.NewController(t)
			uc := NewMockuseCase(ctrl)
			params := validParams()
			if zeroValues {
				params.Price = 0
				params.StartPoint = domain.Coordinates{}
				params.EndPoint = domain.Coordinates{}
			}
			created := &domain.Trip{
				ID:         uuid.UUID{3},
				UserID:     params.UserID,
				DriverID:   params.DriverID,
				Status:     domain.TripStatusActive,
				StartPoint: params.StartPoint,
				EndPoint:   params.EndPoint,
				Price:      params.Price,
				StartedAt:  time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC),
			}
			body := bodyFor(params)
			req := httptest.NewRequest(http.MethodPost, createPath, strings.NewReader(encodeBody(t, body))).WithContext(t.Context())
			uc.EXPECT().Execute(req.Context(), params).Return(created, nil)
			h := create.New(uc, testLogger())
			recorder := httptest.NewRecorder()

			h.Handle(recorder, req, api.CreateTripParams{})

			if recorder.Code != http.StatusCreated {
				t.Fatalf("status = %d, want 201; body = %s", recorder.Code, recorder.Body.String())
			}
			if got := recorder.Header().Get("Location"); got != createPath+"/"+created.ID.String() {
				t.Errorf("Location = %q, want the created trip URL", got)
			}
			if got := recorder.Header().Get("Content-Type"); got != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", got)
			}
			var got api.Trip
			if err := json.NewDecoder(recorder.Body).Decode(&got); err != nil {
				t.Fatal(err)
			}
			want := api.Trip{
				Id: created.ID, UserId: params.UserID, DriverId: params.DriverID, Status: api.Active,
				StartPoint: api.Coordinates{Latitude: params.StartPoint.Latitude, Longitude: params.StartPoint.Longitude},
				EndPoint:   api.Coordinates{Latitude: params.EndPoint.Latitude, Longitude: params.EndPoint.Longitude},
				Price:      params.Price, StartedAt: created.StartedAt,
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("trip response = %+v, want %+v", got, want)
			}
		})
	}
}

func TestHandleUseCaseError(t *testing.T) {
	internalErr := errors.New("database query failed: INSERT trips")
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		wantDetail string
	}{
		{
			name: "driver busy", err: fmt.Errorf("create transaction: %w", domain.ErrDriverBusy),
			wantStatus: http.StatusConflict, wantCode: "driver_busy", wantDetail: "driver already has an active trip",
		},
		{
			name: "validation details survive wrapping",
			err: fmt.Errorf("validate create trip: %w", &domain.ValidationError{
				Field: testPriceField, Reason: "must not be negative",
			}),
			wantStatus: http.StatusBadRequest, wantCode: testInvalidRequestCode, wantDetail: "price: must not be negative",
		},
		{
			name: "internal details are hidden", err: fmt.Errorf("create transaction: %w", internalErr),
			wantStatus: http.StatusInternalServerError, wantCode: "internal_error", wantDetail: "internal error",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			uc := NewMockuseCase(ctrl)
			req := httptest.NewRequest(http.MethodPost, createPath, strings.NewReader(encodeBody(t, validBody()))).WithContext(t.Context())
			uc.EXPECT().Execute(req.Context(), validParams()).Return(nil, tt.err)
			recorder := httptest.NewRecorder()

			create.New(uc, testLogger()).Handle(recorder, req, api.CreateTripParams{})

			assertProblem(t, recorder, tt.wantStatus, tt.wantCode, tt.wantDetail)
			if strings.Contains(recorder.Body.String(), internalErr.Error()) {
				t.Error("response exposes internal error details")
			}
			if got := recorder.Header().Get("Location"); got != "" {
				t.Errorf("Location = %q, want empty on error", got)
			}
		})
	}
}

func TestHandleRequiredFields(t *testing.T) {
	fields := []string{
		"user_id", "driver_id", testPriceField, "start_point", "end_point",
		"start_point.latitude", "start_point.longitude", "end_point.latitude", "end_point.longitude",
	}
	for _, field := range fields {
		for _, null := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/null=%t", field, null), func(t *testing.T) {
				ctrl := gomock.NewController(t)
				uc := NewMockuseCase(ctrl)
				body := validBody()
				parts := strings.Split(field, ".")
				target := body
				if len(parts) == 2 {
					target = body[parts[0]].(map[string]any)
				}
				key := parts[len(parts)-1]
				if null {
					target[key] = nil
				} else {
					delete(target, key)
				}
				req := httptest.NewRequest(http.MethodPost, createPath, strings.NewReader(encodeBody(t, body)))
				recorder := httptest.NewRecorder()

				create.New(uc, testLogger()).Handle(recorder, req, api.CreateTripParams{})

				assertProblem(t, recorder, http.StatusBadRequest, testInvalidRequestCode, field+": is required")
			})
		}
	}
}

func TestHandleInvalidBody(t *testing.T) {
	valid := encodeBody(t, validBody())
	tests := []struct {
		name string
		body string
	}{
		{name: "empty body"},
		{name: "invalid JSON", body: "{"},
		{name: "null object", body: "null"},
		{name: "array", body: "[]"},
		{name: "two objects", body: valid + valid},
		{name: "trailing garbage", body: valid + " garbage"},
		{name: "unknown field", body: strings.Replace(valid, "{", `{"unknown":true,`, 1)},
		{name: "unknown nested field", body: strings.Replace(valid, `"start_point":{`, `"start_point":{"unknown":true,`, 1)},
		{name: "invalid UUID", body: strings.Replace(valid, validParams().UserID.String(), "not-a-uuid", 1)},
		{name: "invalid price type", body: strings.Replace(valid, `"price":1450`, `"price":"wrong"`, 1)},
		{name: "price overflow", body: strings.Replace(valid, `"price":1450`, `"price":9223372036854775808`, 1)},
		{name: "body exceeds limit", body: strings.Repeat(" ", (1<<20)+1) + valid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			uc := NewMockuseCase(ctrl)
			req := httptest.NewRequest(http.MethodPost, createPath, strings.NewReader(tt.body))
			recorder := httptest.NewRecorder()

			create.New(uc, testLogger()).Handle(recorder, req, api.CreateTripParams{})

			assertProblem(t, recorder, http.StatusBadRequest, testInvalidRequestCode, "")
		})
	}
}

func validParams() domain.CreateTripParams {
	return domain.CreateTripParams{
		UserID: uuid.UUID{1}, DriverID: uuid.UUID{2}, Price: 1450,
		StartPoint: domain.Coordinates{Latitude: 59.9398, Longitude: 30.3146},
		EndPoint:   domain.Coordinates{Latitude: 59.929, Longitude: 30.3626},
	}
}

func validBody() map[string]any {
	return bodyFor(validParams())
}

func bodyFor(params domain.CreateTripParams) map[string]any {
	return map[string]any{
		"user_id": params.UserID, "driver_id": params.DriverID, testPriceField: params.Price,
		"start_point": map[string]any{"latitude": params.StartPoint.Latitude, "longitude": params.StartPoint.Longitude},
		"end_point":   map[string]any{"latitude": params.EndPoint.Latitude, "longitude": params.EndPoint.Longitude},
	}
}

func encodeBody(t *testing.T, body map[string]any) string {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

func assertProblem(t *testing.T, recorder *httptest.ResponseRecorder, status int, code, detail string) {
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
	if body.Code != code || int(body.Status) != status || body.Type == "" || body.Title == "" {
		t.Errorf("invalid problem response: %+v", body)
	}
	if body.Instance == nil || *body.Instance != createPath {
		t.Errorf("instance = %v, want %q", body.Instance, createPath)
	}
	if body.Detail == nil || *body.Detail == "" {
		t.Fatal("problem detail is empty")
	}
	if detail != "" && *body.Detail != detail {
		t.Errorf("detail = %q, want %q", *body.Detail, detail)
	}
}
