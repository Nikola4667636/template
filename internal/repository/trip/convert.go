package trip

import (
	"github.com/jackc/pgx/v5"

	domain "github.com/Nikola4667636/template/internal/domain/trip"
)

func scanTrip(row pgx.Row) (*domain.Trip, error) {
	var (
		t      domain.Trip
		status string
	)

	err := row.Scan(
		&t.ID, &t.UserID, &t.DriverID, &status,
		&t.StartPoint.Latitude, &t.StartPoint.Longitude,
		&t.EndPoint.Latitude, &t.EndPoint.Longitude,
		&t.Price, &t.StartedAt, &t.FinishedAt,
	)
	if err != nil {
		return nil, err
	}

	t.Status = domain.TripStatus(status)
	t.StartedAt = t.StartedAt.UTC()
	if t.FinishedAt != nil {
		v := t.FinishedAt.UTC()
		t.FinishedAt = &v
	}

	return &t, nil
}
