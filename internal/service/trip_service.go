package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"template/internal/domain"
)

type TxManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

type TripRepository interface {
	Create(ctx context.Context, trip *domain.Trip) error
	AddStatusHistory(ctx context.Context, tripID uuid.UUID, from *domain.TripStatus, to domain.TripStatus) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Trip, error)
	Finish(ctx context.Context, id uuid.UUID) (*domain.Trip, error)
	AddPosition(ctx context.Context, pos *domain.TripPosition) error
	ListPositions(ctx context.Context, tripID uuid.UUID) ([]domain.TripPosition, error)
}

type TripService struct {
	repo      TripRepository
	txManager TxManager
}

func NewTripService(repo TripRepository, txManager TxManager) *TripService {
	return &TripService{
		repo:      repo,
		txManager: txManager,
	}
}

func (s *TripService) CreateTrip(ctx context.Context, params domain.CreateTripParams) (*domain.Trip, error) {
	trip := &domain.Trip{
		ID:         uuid.New(),
		UserID:     params.UserID,
		DriverID:   params.DriverID,
		Status:     domain.TripStatusActive,
		StartPoint: params.StartPoint,
		EndPoint:   params.EndPoint,
		Price:      params.Price,
		StartedAt:  time.Now().UTC().Truncate(time.Microsecond),
	}

	err := s.txManager.Do(ctx, func(txCtx context.Context) error {
		if err := s.repo.Create(txCtx, trip); err != nil {
			return err
		}
		return s.repo.AddStatusHistory(txCtx, trip.ID, nil, domain.TripStatusActive)
	})
	if err != nil {
		return nil, err
	}

	return trip, nil
}

func (s *TripService) GetTrip(ctx context.Context, id uuid.UUID) (*domain.Trip, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *TripService) FinishTrip(ctx context.Context, id uuid.UUID) (*domain.Trip, error) {
	var trip *domain.Trip

	err := s.txManager.Do(ctx, func(txCtx context.Context) error {
		var err error
		trip, err = s.repo.Finish(txCtx, id)
		if err != nil {
			return err
		}

		from := domain.TripStatusActive
		return s.repo.AddStatusHistory(txCtx, trip.ID, &from, domain.TripStatusCompleted)
	})
	if err != nil {
		return nil, err
	}

	return trip, nil
}

func (s *TripService) AddTripPosition(ctx context.Context, params domain.AddPositionParams) (*domain.TripPosition, error) {
	trip, err := s.repo.GetByID(ctx, params.TripID)
	if err != nil {
		return nil, err
	}

	if trip.Status != domain.TripStatusActive {
		return nil, domain.ErrTripCompleted
	}

	pos := &domain.TripPosition{
		ID:        uuid.New(),
		TripID:    params.TripID,
		Point:     params.Point,
		CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
	}

	err = s.txManager.Do(ctx, func(txCtx context.Context) error {
		return s.repo.AddPosition(txCtx, pos)
	})
	if err != nil {
		return nil, err
	}

	return pos, nil
}

func (s *TripService) ListTripPositions(ctx context.Context, tripID uuid.UUID) ([]domain.TripPosition, error) {
	if _, err := s.repo.GetByID(ctx, tripID); err != nil {
		return nil, err
	}

	return s.repo.ListPositions(ctx, tripID)
}
