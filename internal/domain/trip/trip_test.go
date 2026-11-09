package trip

import (
	"errors"
	"testing"
	"time"
)

func TestTripFinish(t *testing.T) {
	at := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	finishedEarlier := at.Add(-time.Hour)

	tests := []struct {
		name           string
		initial        Trip
		wantErr        error
		wantStatus     TripStatus
		wantFinishedAt time.Time
	}{
		{
			name: "active trip finishes successfully",
			initial: Trip{
				Status: TripStatusActive,
			},
			wantErr:        nil,
			wantStatus:     TripStatusCompleted,
			wantFinishedAt: at,
		},
		{
			name: "completed trip keeps its finish time",
			initial: Trip{
				Status:     TripStatusCompleted,
				FinishedAt: &finishedEarlier,
			},
			wantErr:        ErrTripCompleted,
			wantStatus:     TripStatusCompleted,
			wantFinishedAt: finishedEarlier,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trip := tt.initial

			err := trip.Finish(at)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Finish() error = %v, want %v", err, tt.wantErr)
			}

			if trip.Status != tt.wantStatus {
				t.Errorf("Status = %q, want %q", trip.Status, tt.wantStatus)
			}

			if trip.FinishedAt == nil {
				t.Fatal("FinishedAt is nil")
			}

			if !trip.FinishedAt.Equal(tt.wantFinishedAt) {
				t.Errorf(
					"FinishedAt = %v, want %v",
					*trip.FinishedAt,
					tt.wantFinishedAt,
				)
			}
		})
	}
}
