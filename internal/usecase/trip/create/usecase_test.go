package create_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/Nikola4667636/template/internal/domain/trip"
	"github.com/Nikola4667636/template/internal/usecase/trip/create"
)

func TestUseCaseExecute(t *testing.T) {
	now := time.Date(2026, 11, 10, 15, 0, 0, 123456789, time.FixedZone("UTC+3", 3*60*60))
	insertErr := errors.New("insert failed")
	historyErr := errors.New("history insert failed")
	beginErr := errors.New("begin failed")
	commitErr := errors.New("commit failed")

	tests := []struct {
		name       string
		createErr  error
		historyErr error
		beginErr   error
		commitErr  error
		wantErr    error
	}{
		{name: "trip created"},
		{
			name:      "driver is busy",
			createErr: trip.ErrDriverBusy,
			wantErr:   trip.ErrDriverBusy,
		},
		{
			name:      "trip insert fails",
			createErr: insertErr,
			wantErr:   insertErr,
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
			params := validParams()
			var inserted *trip.Trip

			if tt.beginErr == nil {
				insert := repo.EXPECT().Create(txCtx, gomock.Any()).DoAndReturn(
					func(_ context.Context, created *trip.Trip) error {
						assertCreatedTrip(t, created, params, now)
						inserted = created
						return tt.createErr
					},
				)
				if tt.createErr == nil {
					history := repo.EXPECT().AddStatusHistory(txCtx, gomock.Any(), nil, trip.TripStatusActive).DoAndReturn(
						func(_ context.Context, id uuid.UUID, _ *trip.TripStatus, _ trip.TripStatus) error {
							if inserted == nil || id != inserted.ID {
								t.Errorf("history trip ID = %s, want the inserted trip ID", id)
							}
							return tt.historyErr
						},
					)
					gomock.InOrder(insert, history)
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

			uc := create.New(repo, tx, func() time.Time { return now })
			got, err := uc.Execute(ctx, params)

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
				return
			}
			assertCreatedTrip(t, got, params, now)
			if !reflect.DeepEqual(got, inserted) {
				t.Errorf("Execute() trip = %+v, want inserted trip %+v", got, inserted)
			}
		})
	}
}

func TestUseCaseExecuteInvalidParams(t *testing.T) {
	tests := []struct {
		name      string
		modify    func(*trip.CreateTripParams)
		wantField string
	}{
		{
			name:      "empty user ID",
			modify:    func(p *trip.CreateTripParams) { p.UserID = uuid.Nil },
			wantField: "user_id",
		},
		{
			name:      "empty driver ID",
			modify:    func(p *trip.CreateTripParams) { p.DriverID = uuid.Nil },
			wantField: "driver_id",
		},
		{
			name:      "negative price",
			modify:    func(p *trip.CreateTripParams) { p.Price = -1 },
			wantField: "price",
		},
		{
			name:      "invalid start point",
			modify:    func(p *trip.CreateTripParams) { p.StartPoint.Latitude = 91 },
			wantField: "start_point.latitude",
		},
		{
			name:      "invalid end point",
			modify:    func(p *trip.CreateTripParams) { p.EndPoint.Longitude = 181 },
			wantField: "end_point.longitude",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := NewMocktripRepository(ctrl)
			tx := NewMocktxManager(ctrl)
			params := validParams()
			tt.modify(&params)
			clockCalls := 0
			uc := create.New(repo, tx, func() time.Time {
				clockCalls++
				return time.Time{}
			})

			got, err := uc.Execute(t.Context(), params)

			if got != nil {
				t.Errorf("Execute() trip = %+v, want nil", got)
			}
			if !errors.Is(err, trip.ErrInvalidArguments) {
				t.Fatalf("Execute() error = %v, want ErrInvalidArguments", err)
			}
			var validationErr *trip.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("Execute() error type = %T, want a wrapped *ValidationError", err)
			}
			if validationErr.Field != tt.wantField || validationErr.Reason == "" {
				t.Errorf("validation details = %+v, want field %q and a reason", validationErr, tt.wantField)
			}
			if clockCalls != 0 {
				t.Errorf("clock calls = %d, want 0 for invalid parameters", clockCalls)
			}
		})
	}
}

func validParams() trip.CreateTripParams {
	return trip.CreateTripParams{
		UserID:     uuid.UUID{1},
		DriverID:   uuid.UUID{2},
		StartPoint: trip.Coordinates{Latitude: 59.9398, Longitude: 30.3146},
		EndPoint:   trip.Coordinates{Latitude: 59.929, Longitude: 30.3626},
		Price:      1450,
	}
}

func assertCreatedTrip(t *testing.T, got *trip.Trip, params trip.CreateTripParams, now time.Time) {
	t.Helper()
	if got == nil {
		t.Fatal("created trip is nil")
	}
	if got.ID == uuid.Nil {
		t.Error("created trip has an empty ID")
	}
	want := trip.Trip{
		ID:         got.ID,
		UserID:     params.UserID,
		DriverID:   params.DriverID,
		Status:     trip.TripStatusActive,
		StartPoint: params.StartPoint,
		EndPoint:   params.EndPoint,
		Price:      params.Price,
		StartedAt:  now.UTC().Truncate(time.Microsecond),
	}
	if !reflect.DeepEqual(*got, want) {
		t.Errorf("created trip = %+v, want %+v", *got, want)
	}
}
