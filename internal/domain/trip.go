package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrTripNotFound     = errors.New("trip not found")
	ErrDriverBusy       = errors.New("driver already has active trip")
	ErrTripCompleted    = errors.New("trip is already completed")
	ErrInvalidArguments = errors.New("invalid input arguments")
)

type TripStatus string

const (
	TripStatusActive    TripStatus = "active"
	TripStatusCompleted TripStatus = "completed"
	TripStatusCanceled  TripStatus = "canceled"
)

type Coordinates struct {
	Latitude  float64
	Longitude float64
}

type Trip struct {
	ID             uuid.UUID
	UserID         uuid.UUID
	DriverID       uuid.UUID
	Status         TripStatus
	StartPoint     Coordinates
	EndPoint       Coordinates
	Price          int64
	StartedAt      time.Time
	FinishedAt     *time.Time
	LastPositionAt *time.Time
}

type CreateTripParams struct {
	UserID     uuid.UUID
	DriverID   uuid.UUID
	StartPoint Coordinates
	EndPoint   Coordinates
	Price      int64
}

func (p CreateTripParams) Validate() error {
	if p.UserID == uuid.Nil || p.DriverID == uuid.Nil {
		return ErrInvalidArguments
	}
	if p.Price < 0 {
		return ErrInvalidArguments
	}
	if p.StartPoint.Latitude < -90 || p.StartPoint.Latitude > 90 ||
		p.StartPoint.Longitude < -180 || p.StartPoint.Longitude > 180 ||
		p.EndPoint.Latitude < -90 || p.EndPoint.Latitude > 90 ||
		p.EndPoint.Longitude < -180 || p.EndPoint.Longitude > 180 {
		return ErrInvalidArguments
	}
	return nil
}

type TripPosition struct {
	ID        uuid.UUID
	TripID    uuid.UUID
	Point     Coordinates
	CreatedAt time.Time
}

type AddPositionParams struct {
	TripID uuid.UUID
	Point  Coordinates
}

func (p AddPositionParams) Validate() error {
	if p.TripID == uuid.Nil {
		return ErrInvalidArguments
	}
	if p.Point.Latitude < -90 || p.Point.Latitude > 90 ||
		p.Point.Longitude < -180 || p.Point.Longitude > 180 {
		return ErrInvalidArguments
	}
	return nil
}
