package get_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/Nikola4667636/template/internal/domain/trip"
	"github.com/Nikola4667636/template/internal/usecase/trip/get"
)

func TestUseCaseExecute(t *testing.T) {
	id := uuid.UUID{1}
	found := &trip.Trip{
		ID:         id,
		UserID:     uuid.UUID{2},
		DriverID:   uuid.UUID{3},
		Status:     trip.TripStatusActive,
		StartPoint: trip.Coordinates{Latitude: 59.9398, Longitude: 30.3146},
		EndPoint:   trip.Coordinates{Latitude: 59.929, Longitude: 30.3626},
		Price:      1450,
		StartedAt:  time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC),
	}
	repositoryErr := errors.New("repository failed")

	tests := []struct {
		name     string
		repoTrip *trip.Trip
		repoErr  error
		wantTrip *trip.Trip
		wantErr  error
	}{
		{
			name:     "trip found",
			repoTrip: found,
			wantTrip: found,
		},
		{
			name:    "trip not found",
			repoErr: trip.ErrTripNotFound,
			wantErr: trip.ErrTripNotFound,
		},
		{
			name:    "wrapped trip not found",
			repoErr: fmt.Errorf("fetch trip: %w", trip.ErrTripNotFound),
			wantErr: trip.ErrTripNotFound,
		},
		{
			name:    "unexpected repository error",
			repoErr: repositoryErr,
			wantErr: repositoryErr,
		},
		{
			name:     "result is discarded on error",
			repoTrip: found,
			repoErr:  repositoryErr,
			wantErr:  repositoryErr,
		},
		{
			name:    "repository reports cancellation",
			repoErr: context.Canceled,
			wantErr: context.Canceled,
		},
		{
			name:    "repository reports deadline exceeded",
			repoErr: context.DeadlineExceeded,
			wantErr: context.DeadlineExceeded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := NewMocktripRepository(ctrl)
			ctx := t.Context()
			repo.EXPECT().GetByID(ctx, id).Return(tt.repoTrip, tt.repoErr)

			uc := get.New(repo)
			got, err := uc.Execute(ctx, id)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Execute() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil && err == tt.repoErr {
				t.Error("Execute() did not wrap the repository error with context")
			}
			if !reflect.DeepEqual(got, tt.wantTrip) {
				t.Errorf("Execute() trip = %+v, want %+v", got, tt.wantTrip)
			}
		})
	}
}
