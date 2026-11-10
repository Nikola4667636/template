package telemetry_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/mock/gomock"

	"github.com/Nikola4667636/template/internal/domain/trip"
	"github.com/Nikola4667636/template/internal/infra/telemetry"
)

func TestTripObservers(t *testing.T) {
	for _, action := range []string{"create", "finish"} {
		for _, fail := range []bool{false, true} {
			name := action
			if fail {
				name += " failure"
			}
			t.Run(name, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				counter := NewMockactiveTripCounter(ctrl)
				counter.EXPECT().CountActive(gomock.Any()).Return(int64(7), nil)
				reader := sdkmetric.NewManualReader()
				provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
				t.Cleanup(func() { _ = provider.Shutdown(t.Context()) })
				var logs bytes.Buffer
				logger, err := telemetry.NewLogger(&logs, "info", "trip-service")
				if err != nil {
					t.Fatal(err)
				}
				metrics, err := telemetry.NewTripMetrics(provider.Meter("test"), logger, counter)
				if err != nil {
					t.Fatal(err)
				}
				result := &trip.Trip{ID: uuid.New()}
				var wantErr error
				if fail {
					result, wantErr = nil, errors.New("transaction commit failed")
				}
				var got *trip.Trip
				var executeErr error
				var metricName, event string
				if action == "create" {
					next := NewMocktripCreator(ctrl)
					params := trip.CreateTripParams{UserID: uuid.New()}
					next.EXPECT().Execute(t.Context(), params).Return(result, wantErr)
					got, executeErr = telemetry.ObserveCreate(next, metrics).Execute(t.Context(), params)
					metricName, event = "tripgo.trips.created", "trip created"
				} else {
					next := NewMocktripFinisher(ctrl)
					id := uuid.New()
					next.EXPECT().Execute(t.Context(), id).Return(result, wantErr)
					got, executeErr = telemetry.ObserveFinish(next, metrics).Execute(t.Context(), id)
					metricName, event = "tripgo.trips.completed", "trip completed"
				}
				if got != result || !errors.Is(executeErr, wantErr) {
					t.Fatalf("observer changed the result: %v, %v", got, executeErr)
				}
				var data metricdata.ResourceMetrics
				if err := reader.Collect(t.Context(), &data); err != nil {
					t.Fatal(err)
				}
				var count, active int64
				for _, scope := range data.ScopeMetrics {
					for _, metric := range scope.Metrics {
						if metric.Name == metricName {
							for _, point := range metric.Data.(metricdata.Sum[int64]).DataPoints {
								count += point.Value
								if point.Attributes.Len() != 0 {
									t.Error("business counter has unexpected labels")
								}
							}
						}
						if metric.Name == "tripgo.trips.active" {
							active = metric.Data.(metricdata.Gauge[int64]).DataPoints[0].Value
						}
					}
				}
				if active != 7 {
					t.Errorf("active gauge = %d, want database value 7", active)
				}
				if fail {
					if count != 0 || logs.Len() != 0 {
						t.Error("failed transaction emitted a business event")
					}
				} else if count != 1 || !strings.Contains(logs.String(), event) || !strings.Contains(logs.String(), result.ID.String()) {
					t.Errorf("missing success metric or log: count=%d, logs=%s", count, logs.String())
				}
			})
		}
	}
}

func TestActiveGaugeCollectionError(t *testing.T) {
	ctrl := gomock.NewController(t)
	counter := NewMockactiveTripCounter(ctrl)
	wantErr := errors.New("database is unavailable")
	counter.EXPECT().CountActive(gomock.Any()).Return(int64(0), wantErr)
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(t.Context()) })
	var logs bytes.Buffer
	logger, err := telemetry.NewLogger(&logs, "info", "trip-service")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := telemetry.NewTripMetrics(provider.Meter("test"), logger, counter); err != nil {
		t.Fatal(err)
	}
	var data metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &data); !errors.Is(err, wantErr) {
		t.Errorf("gauge collection error = %v, want database error", err)
	}
}
