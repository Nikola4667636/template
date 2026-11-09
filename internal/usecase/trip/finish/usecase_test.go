package finish_test

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
	"github.com/Nikola4667636/template/internal/usecase/trip/finish"
)

func TestUseCaseExecute(t *testing.T) {
	id := uuid.UUID{1}
	startedAt := time.Date(2026, 11, 10, 11, 0, 0, 0, time.UTC)
	now := time.Date(2026, 11, 10, 15, 0, 0, 123456789, time.FixedZone("UTC+3", 3*60*60))
	finishedEarlier := startedAt.Add(30 * time.Minute)
	loadErr := errors.New("load failed")
	saveErr := errors.New("update failed")
	historyErr := errors.New("history insert failed")
	beginErr := errors.New("begin failed")
	commitErr := errors.New("commit failed")

	tests := []struct {
		name       string
		completed  bool
		loadErr    error
		saveErr    error
		historyErr error
		beginErr   error
		commitErr  error
		wantErr    error
	}{
		{name: "active trip finishes"},
		{
			name:    "trip not found",
			loadErr: trip.ErrTripNotFound,
			wantErr: trip.ErrTripNotFound,
		},
		{
			name:    "wrapped trip not found",
			loadErr: fmt.Errorf("fetch trip: %w", trip.ErrTripNotFound),
			wantErr: trip.ErrTripNotFound,
		},
		{
			name:    "trip lookup fails",
			loadErr: loadErr,
			wantErr: loadErr,
		},
		{
			name:      "completed trip keeps its finish time",
			completed: true,
			wantErr:   trip.ErrTripCompleted,
		},
		{
			name:    "state update fails",
			saveErr: saveErr,
			wantErr: saveErr,
		},
		{
			name:       "history insert fails",
			historyErr: historyErr,
			wantErr:    historyErr,
		},
		{
			name:     "transaction cannot start",
			beginErr: beginErr,
			wantErr:  beginErr,
		},
		{
			name:      "transaction cannot commit",
			commitErr: commitErr,
			wantErr:   commitErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := NewMocktripRepository(ctrl)
			tx := NewMocktxManager(ctrl)
			ctx := t.Context()
			txCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			current := &trip.Trip{
				ID:         id,
				UserID:     uuid.UUID{2},
				DriverID:   uuid.UUID{3},
				Status:     trip.TripStatusActive,
				StartPoint: trip.Coordinates{Latitude: 59.9398, Longitude: 30.3146},
				EndPoint:   trip.Coordinates{Latitude: 59.929, Longitude: 30.3626},
				Price:      1450,
				StartedAt:  startedAt,
			}
			if tt.completed {
				current.Status = trip.TripStatusCompleted
				current.FinishedAt = &finishedEarlier
			}
			before := *current

			if tt.beginErr == nil {
				loaded := current
				if tt.loadErr != nil {
					loaded = nil
				}
				load := repo.EXPECT().GetByIDForUpdate(txCtx, id).Return(loaded, tt.loadErr)
				if tt.loadErr == nil && !tt.completed {
					save := repo.EXPECT().UpdateState(txCtx, current).DoAndReturn(
						func(_ context.Context, saved *trip.Trip) error {
							assertFinishedTrip(t, saved, before, now)
							return tt.saveErr
						},
					).After(load)
					if tt.saveErr == nil {
						repo.EXPECT().AddStatusHistory(txCtx, id, gomock.Any(), trip.TripStatusCompleted).DoAndReturn(
							func(_ context.Context, _ uuid.UUID, from *trip.TripStatus, _ trip.TripStatus) error {
								if from == nil || *from != trip.TripStatusActive {
									t.Errorf("history from status = %v, want active", from)
								}
								return tt.historyErr
							},
						).After(save)
					}
				}
			}

			tx.EXPECT().Do(ctx, gomock.Any()).DoAndReturn(
				func(_ context.Context, fn func(context.Context) error) error {
					if tt.beginErr != nil {
						return tt.beginErr
					}
					if err := fn(txCtx); err != nil {
						return err
					}
					return tt.commitErr
				},
			)

			uc := finish.New(repo, tx, func() time.Time { return now })
			got, err := uc.Execute(ctx, id)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Execute() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				if got != nil {
					t.Errorf("Execute() trip = %+v, want nil on error", got)
				}
				if err == tt.wantErr {
					t.Error("Execute() did not wrap the error with context")
				}
				if tt.completed && !reflect.DeepEqual(*current, before) {
					t.Errorf("completed trip changed: got %+v, want %+v", *current, before)
				}
				return
			}
			assertFinishedTrip(t, got, before, now)
		})
	}
}

func assertFinishedTrip(t *testing.T, got *trip.Trip, before trip.Trip, now time.Time) {
	t.Helper()
	if got == nil {
		t.Fatal("finished trip is nil")
	}
	finishedAt := now.UTC().Truncate(time.Microsecond)
	want := before
	want.Status = trip.TripStatusCompleted
	want.FinishedAt = &finishedAt
	if !reflect.DeepEqual(*got, want) {
		t.Errorf("finished trip = %+v, want %+v", *got, want)
	}
}
