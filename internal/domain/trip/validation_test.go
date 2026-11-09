package trip

import (
	"errors"
	"math"
	"testing"

	"github.com/google/uuid"
)

const (
	testUUIDReason         = "must be a non-empty UUID"
	testPriceReason        = "must not be negative"
	testLatitudeReason     = "must be between -90 and 90"
	testLongitudeReason    = "must be between -180 and 180"
	testPriceField         = "price"
	testStartLatitudeField = "start_point.latitude"
	testEndLongitudeField  = "end_point.longitude"
)

func TestCreateTripParamsValidate(t *testing.T) {
	tests := []struct {
		name       string
		modify     func(*CreateTripParams)
		wantField  string
		wantReason string
	}{
		{name: "valid parameters"},
		{
			name:       "empty user ID",
			modify:     func(p *CreateTripParams) { p.UserID = uuid.Nil },
			wantField:  "user_id",
			wantReason: testUUIDReason,
		},
		{
			name:       "empty driver ID",
			modify:     func(p *CreateTripParams) { p.DriverID = uuid.Nil },
			wantField:  "driver_id",
			wantReason: testUUIDReason,
		},
		{
			name:   "zero price is valid",
			modify: func(p *CreateTripParams) { p.Price = 0 },
		},
		{
			name:       "negative price",
			modify:     func(p *CreateTripParams) { p.Price = -1 },
			wantField:  testPriceField,
			wantReason: testPriceReason,
		},
		{
			name: "zero coordinates are valid",
			modify: func(p *CreateTripParams) {
				p.StartPoint = Coordinates{}
				p.EndPoint = Coordinates{}
			},
		},
		{
			name: "minimum coordinates are valid",
			modify: func(p *CreateTripParams) {
				p.StartPoint = Coordinates{Latitude: -90, Longitude: -180}
				p.EndPoint = p.StartPoint
			},
		},
		{
			name: "maximum coordinates are valid",
			modify: func(p *CreateTripParams) {
				p.StartPoint = Coordinates{Latitude: 90, Longitude: 180}
				p.EndPoint = p.StartPoint
			},
		},
		{
			name:       "start latitude below minimum",
			modify:     func(p *CreateTripParams) { p.StartPoint.Latitude = -90.1 },
			wantField:  testStartLatitudeField,
			wantReason: testLatitudeReason,
		},
		{
			name:       "start latitude above maximum",
			modify:     func(p *CreateTripParams) { p.StartPoint.Latitude = 90.1 },
			wantField:  testStartLatitudeField,
			wantReason: testLatitudeReason,
		},
		{
			name:       "start longitude below minimum",
			modify:     func(p *CreateTripParams) { p.StartPoint.Longitude = -180.1 },
			wantField:  "start_point.longitude",
			wantReason: testLongitudeReason,
		},
		{
			name:       "start longitude above maximum",
			modify:     func(p *CreateTripParams) { p.StartPoint.Longitude = 180.1 },
			wantField:  "start_point.longitude",
			wantReason: testLongitudeReason,
		},
		{
			name:       "end latitude below minimum",
			modify:     func(p *CreateTripParams) { p.EndPoint.Latitude = -90.1 },
			wantField:  "end_point.latitude",
			wantReason: testLatitudeReason,
		},
		{
			name:       "end latitude above maximum",
			modify:     func(p *CreateTripParams) { p.EndPoint.Latitude = 90.1 },
			wantField:  "end_point.latitude",
			wantReason: testLatitudeReason,
		},
		{
			name:       "end longitude below minimum",
			modify:     func(p *CreateTripParams) { p.EndPoint.Longitude = -180.1 },
			wantField:  testEndLongitudeField,
			wantReason: testLongitudeReason,
		},
		{
			name:       "end longitude above maximum",
			modify:     func(p *CreateTripParams) { p.EndPoint.Longitude = 180.1 },
			wantField:  testEndLongitudeField,
			wantReason: testLongitudeReason,
		},
		{
			name:       "NaN start latitude",
			modify:     func(p *CreateTripParams) { p.StartPoint.Latitude = math.NaN() },
			wantField:  testStartLatitudeField,
			wantReason: testLatitudeReason,
		},
		{
			name:       "NaN end longitude",
			modify:     func(p *CreateTripParams) { p.EndPoint.Longitude = math.NaN() },
			wantField:  testEndLongitudeField,
			wantReason: testLongitudeReason,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := CreateTripParams{
				UserID:     uuid.UUID{1},
				DriverID:   uuid.UUID{2},
				StartPoint: Coordinates{Latitude: 59.9398, Longitude: 30.3146},
				EndPoint:   Coordinates{Latitude: 59.929, Longitude: 30.3626},
				Price:      1450,
			}
			if tt.modify != nil {
				tt.modify(&params)
			}

			assertValidationResult(t, params.Validate(), tt.wantField, tt.wantReason)
		})
	}
}

func assertValidationResult(t *testing.T, err error, wantField, wantReason string) {
	t.Helper()
	if wantField == "" {
		if err != nil {
			t.Fatalf("Validate() error = %v, want nil", err)
		}
		return
	}

	if !errors.Is(err, ErrInvalidArguments) {
		t.Fatalf("Validate() error = %v, want ErrInvalidArguments", err)
	}

	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("Validate() error type = %T, want *ValidationError", err)
	}
	if validationErr.Field != wantField {
		t.Errorf("Field = %q, want %q", validationErr.Field, wantField)
	}
	if validationErr.Reason != wantReason {
		t.Errorf("Reason = %q, want %q", validationErr.Reason, wantReason)
	}
}
