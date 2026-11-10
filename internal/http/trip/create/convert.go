package create

import (
	"github.com/google/uuid"

	"github.com/Nikola4667636/template/internal/domain/trip"
)

type coordinates struct {
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
}

type request struct {
	UserID     *uuid.UUID   `json:"user_id"`
	DriverID   *uuid.UUID   `json:"driver_id"`
	StartPoint *coordinates `json:"start_point"`
	EndPoint   *coordinates `json:"end_point"`
	Price      *int64       `json:"price"`
}

func toParams(raw request) (trip.CreateTripParams, error) {
	var missing string
	switch {
	case raw.UserID == nil:
		missing = "user_id"
	case raw.DriverID == nil:
		missing = "driver_id"
	case raw.StartPoint == nil:
		missing = "start_point"
	case raw.StartPoint.Latitude == nil:
		missing = "start_point.latitude"
	case raw.StartPoint.Longitude == nil:
		missing = "start_point.longitude"
	case raw.EndPoint == nil:
		missing = "end_point"
	case raw.EndPoint.Latitude == nil:
		missing = "end_point.latitude"
	case raw.EndPoint.Longitude == nil:
		missing = "end_point.longitude"
	case raw.Price == nil:
		missing = "price"
	}
	if missing != "" {
		return trip.CreateTripParams{}, &trip.ValidationError{Field: missing, Reason: "is required"}
	}
	return trip.CreateTripParams{
		UserID:   *raw.UserID,
		DriverID: *raw.DriverID,
		StartPoint: trip.Coordinates{
			Latitude:  *raw.StartPoint.Latitude,
			Longitude: *raw.StartPoint.Longitude,
		},
		EndPoint: trip.Coordinates{
			Latitude:  *raw.EndPoint.Latitude,
			Longitude: *raw.EndPoint.Longitude,
		},
		Price: *raw.Price,
	}, nil
}
