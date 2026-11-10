//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package telemetry

import (
	"context"

	"github.com/google/uuid"

	"github.com/Nikola4667636/template/internal/domain/trip"
)

type activeTripCounter interface {
	CountActive(ctx context.Context) (int64, error)
}

type tripCreator interface {
	Execute(ctx context.Context, params trip.CreateTripParams) (*trip.Trip, error)
}

type tripFinisher interface {
	Execute(ctx context.Context, id uuid.UUID) (*trip.Trip, error)
}
