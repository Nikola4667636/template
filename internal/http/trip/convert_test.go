package trip_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	domain "github.com/Nikola4667636/template/internal/domain/trip"
	api "github.com/Nikola4667636/template/internal/generated"
	"github.com/Nikola4667636/template/internal/http/trip"
)

func TestToAPI(t *testing.T) {
	startedAt := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(time.Hour)
	for _, completed := range []bool{false, true} {
		name := "active"
		if completed {
			name = "completed"
		}
		t.Run(name, func(t *testing.T) {
			value := &domain.Trip{
				ID: uuid.UUID{1}, UserID: uuid.UUID{2}, DriverID: uuid.UUID{3}, Status: domain.TripStatusActive,
				StartPoint: domain.Coordinates{Latitude: 59.9398, Longitude: 30.3146},
				EndPoint:   domain.Coordinates{Latitude: 59.929, Longitude: 30.3626},
				Price:      1450, StartedAt: startedAt,
			}
			want := api.Trip{
				Id: uuid.UUID{1}, UserId: uuid.UUID{2}, DriverId: uuid.UUID{3}, Status: api.Active,
				StartPoint: api.Coordinates{Latitude: 59.9398, Longitude: 30.3146},
				EndPoint:   api.Coordinates{Latitude: 59.929, Longitude: 30.3626},
				Price:      1450, StartedAt: startedAt,
			}
			if completed {
				value.Status = domain.TripStatusCompleted
				value.FinishedAt = &finishedAt
				want.Status = api.Completed
				want.FinishedAt = &finishedAt
			}
			before := *value

			got := trip.ToAPI(value)

			if !reflect.DeepEqual(got, want) {
				t.Errorf("API trip = %+v, want %+v", got, want)
			}
			if !reflect.DeepEqual(*value, before) {
				t.Error("conversion changed the domain trip")
			}
		})
	}
}
