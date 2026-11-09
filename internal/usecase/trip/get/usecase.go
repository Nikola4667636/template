package get

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/Nikola4667636/template/internal/domain/trip"
)

type UseCase struct {
	repo tripRepository
}

func New(repo tripRepository) *UseCase {
	return &UseCase{repo: repo}
}

func (uc *UseCase) Execute(ctx context.Context, id uuid.UUID) (*trip.Trip, error) {
	result, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get trip by id: %w", err)
	}
	return result, nil
}
