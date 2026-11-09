//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package finish

import (
	"context"

	"github.com/google/uuid"

	"github.com/Nikola4667636/template/internal/domain/trip"
)

type tripRepository interface {
	GetByIDForUpdate(ctx context.Context, id uuid.UUID) (*trip.Trip, error)
	UpdateState(ctx context.Context, trip *trip.Trip) error
	AddStatusHistory(ctx context.Context, tripID uuid.UUID, from *trip.TripStatus, to trip.TripStatus) error
}

type txManager interface {
	Do(ctx context.Context, fn func(context.Context) error) error
}
