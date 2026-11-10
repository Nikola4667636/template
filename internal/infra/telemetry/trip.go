package telemetry

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/metric"

	"github.com/Nikola4667636/template/internal/domain/trip"
)

type TripMetrics struct {
	created   metric.Int64Counter
	completed metric.Int64Counter
	logger    *slog.Logger
}

func NewTripMetrics(meter metric.Meter, logger *slog.Logger, repo activeTripCounter) (*TripMetrics, error) {
	created, err := meter.Int64Counter("tripgo.trips.created", metric.WithUnit("{trip}"))
	if err != nil {
		return nil, fmt.Errorf("create trips created counter: %w", err)
	}
	completed, err := meter.Int64Counter("tripgo.trips.completed", metric.WithUnit("{trip}"))
	if err != nil {
		return nil, fmt.Errorf("create trips completed counter: %w", err)
	}
	active, err := meter.Int64ObservableGauge("tripgo.trips.active", metric.WithUnit("{trip}"))
	if err != nil {
		return nil, fmt.Errorf("create active trips gauge: %w", err)
	}
	if _, err := meter.RegisterCallback(func(ctx context.Context, observer metric.Observer) error {
		count, err := repo.CountActive(ctx)
		if err != nil {
			return fmt.Errorf("collect active trips: %w", err)
		}
		observer.ObserveInt64(active, count)
		return nil
	}, active); err != nil {
		return nil, fmt.Errorf("register active trips callback: %w", err)
	}
	return &TripMetrics{created: created, completed: completed, logger: logger}, nil
}

type CreateObserver struct {
	next    tripCreator
	metrics *TripMetrics
}

func ObserveCreate(next tripCreator, metrics *TripMetrics) *CreateObserver {
	return &CreateObserver{next: next, metrics: metrics}
}

func (o *CreateObserver) Execute(ctx context.Context, params trip.CreateTripParams) (*trip.Trip, error) {
	created, err := o.next.Execute(ctx, params)
	if err != nil {
		return nil, err
	}
	o.metrics.created.Add(ctx, 1)
	o.metrics.logger.InfoContext(ctx, "trip created", "trip_id", created.ID)
	return created, nil
}

type FinishObserver struct {
	next    tripFinisher
	metrics *TripMetrics
}

func ObserveFinish(next tripFinisher, metrics *TripMetrics) *FinishObserver {
	return &FinishObserver{next: next, metrics: metrics}
}

func (o *FinishObserver) Execute(ctx context.Context, id uuid.UUID) (*trip.Trip, error) {
	finished, err := o.next.Execute(ctx, id)
	if err != nil {
		return nil, err
	}
	o.metrics.completed.Add(ctx, 1)
	o.metrics.logger.InfoContext(ctx, "trip completed", "trip_id", finished.ID)
	return finished, nil
}
