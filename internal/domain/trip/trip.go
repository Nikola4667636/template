package trip

import (
	"time"

	"github.com/google/uuid"
)

type TripStatus string

const (
	TripStatusActive    TripStatus = "active"
	TripStatusCompleted TripStatus = "completed"
)

type Coordinates struct {
	Latitude  float64
	Longitude float64
}

type Trip struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	DriverID   uuid.UUID
	Status     TripStatus
	StartPoint Coordinates
	EndPoint   Coordinates
	Price      int64
	StartedAt  time.Time
	FinishedAt *time.Time
}

func (t *Trip) Finish(at time.Time) error {
	if t.Status != TripStatusActive {
		return ErrTripCompleted
	}
	t.Status = TripStatusCompleted
	t.FinishedAt = &at
	return nil
}
