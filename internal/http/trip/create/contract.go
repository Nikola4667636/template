//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package create

import (
	"context"

	"github.com/Nikola4667636/template/internal/domain/trip"
)

type useCase interface {
	Execute(ctx context.Context, params trip.CreateTripParams) (*trip.Trip, error)
}
