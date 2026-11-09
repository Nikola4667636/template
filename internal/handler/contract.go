//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package handler

import (
	"context"

	"github.com/google/uuid"

	domain "github.com/Nikola4667636/template/internal/domain/trip"
)

type Pinger interface {
	Ping(ctx context.Context) error
}

type createTripUseCase interface {
	Execute(ctx context.Context, params domain.CreateTripParams) (*domain.Trip, error)
}

type getTripUseCase interface {
	Execute(ctx context.Context, id uuid.UUID) (*domain.Trip, error)
}

type finishTripUseCase interface {
	Execute(ctx context.Context, id uuid.UUID) (*domain.Trip, error)
}
