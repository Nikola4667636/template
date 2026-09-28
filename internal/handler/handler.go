package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"template/api"
	"template/internal/domain"
)

const maxBodyBytes = 1 << 20

type Pinger interface {
	Ping(ctx context.Context) error
}

type TripService interface {
	CreateTrip(ctx context.Context, params domain.CreateTripParams) (*domain.Trip, error)
	GetTrip(ctx context.Context, id uuid.UUID) (*domain.Trip, error)
	FinishTrip(ctx context.Context, id uuid.UUID) (*domain.Trip, error)
	AddTripPosition(ctx context.Context, params domain.AddPositionParams) (*domain.TripPosition, error)
	ListTripPositions(ctx context.Context, tripId uuid.UUID) ([]domain.TripPosition, error)
}

type APIHandler struct {
	db          Pinger
	tripService TripService
	dbTimeout   time.Duration
}

func NewAPIHandler(db Pinger, tripService TripService, dbTimeout time.Duration) *APIHandler {
	return &APIHandler{
		db:          db,
		tripService: tripService,
		dbTimeout:   dbTimeout,
	}
}

func (h *APIHandler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, api.HealthResponse{Status: api.Ok})
}

func (h *APIHandler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.dbTimeout)
	defer cancel()

	if h.db == nil || h.db.Ping(ctx) != nil {
		writeJSON(w, http.StatusServiceUnavailable, api.HealthResponse{Status: api.Unavailable})
		return
	}

	writeJSON(w, http.StatusOK, api.HealthResponse{Status: api.Ok})
}

func (h *APIHandler) CreateTrip(w http.ResponseWriter, r *http.Request, _ api.CreateTripParams) {
	var body api.TripData

	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		slog.DebugContext(r.Context(), "invalid create trip body", "error", err)
		writeProblem(w, problemInvalidRequest)
		return
	}

	if err := validateTripData(body); err != nil {
		slog.DebugContext(r.Context(), "create trip validation failed", "error", err)
		writeProblem(w, problemInvalidRequest)
		return
	}

	trip, err := h.tripService.CreateTrip(r.Context(), domain.CreateTripParams{
		UserID:   body.UserId,
		DriverID: body.DriverId,
		StartPoint: domain.Coordinates{
			Latitude:  body.StartPoint.Latitude,
			Longitude: body.StartPoint.Longitude,
		},
		EndPoint: domain.Coordinates{
			Latitude:  body.EndPoint.Latitude,
			Longitude: body.EndPoint.Longitude,
		},
		Price: body.Price,
	})
	if err != nil {
		writeError(w, r, "create trip", err)
		return
	}

	w.Header().Set("Location", "/api/v1/trips/"+trip.ID.String())
	writeJSON(w, http.StatusCreated, mapDomainTripToAPI(trip))
}

func (h *APIHandler) GetTrip(w http.ResponseWriter, r *http.Request, tripId api.TripId) {
	trip, err := h.tripService.GetTrip(r.Context(), tripId)
	if err != nil {
		writeError(w, r, "get trip", err)
		return
	}

	writeJSON(w, http.StatusOK, mapDomainTripToAPI(trip))
}

func (h *APIHandler) FinishTrip(w http.ResponseWriter, r *http.Request, tripId api.TripId) {
	trip, err := h.tripService.FinishTrip(r.Context(), tripId)
	if err != nil {
		writeError(w, r, "finish trip", err)
		return
	}

	writeJSON(w, http.StatusOK, mapDomainTripToAPI(trip))
}

func (h *APIHandler) CreateTripPosition(w http.ResponseWriter, r *http.Request, tripId api.TripId) {
	var body api.PositionData

	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		slog.DebugContext(r.Context(), "invalid create position body", "error", err)
		writeProblem(w, problemInvalidRequest)
		return
	}

	if !validCoordinates(body.Coordinates.Latitude, body.Coordinates.Longitude) {
		slog.DebugContext(r.Context(), "invalid coordinates for position")
		writeProblem(w, problemInvalidRequest)
		return
	}

	pos, err := h.tripService.AddTripPosition(r.Context(), domain.AddPositionParams{
		TripID: tripId,
		Point: domain.Coordinates{
			Latitude:  body.Coordinates.Latitude,
			Longitude: body.Coordinates.Longitude,
		},
	})
	if err != nil {
		writeError(w, r, "create trip position", err)
		return
	}

	writeJSON(w, http.StatusCreated, api.TripPosition{
		TripId: pos.TripID,
		Coordinates: api.Coordinates{
			Latitude:  pos.Point.Latitude,
			Longitude: pos.Point.Longitude,
		},
		RecordedAt: pos.CreatedAt,
	})
}

func (h *APIHandler) ListTripPositions(w http.ResponseWriter, r *http.Request, tripId api.TripId) {
	positions, err := h.tripService.ListTripPositions(r.Context(), tripId)
	if err != nil {
		writeError(w, r, "list trip positions", err)
		return
	}

	apiPositions := make([]api.TripPosition, 0, len(positions))
	for _, pos := range positions {
		apiPositions = append(apiPositions, api.TripPosition{
			TripId: pos.TripID,
			Coordinates: api.Coordinates{
				Latitude:  pos.Point.Latitude,
				Longitude: pos.Point.Longitude,
			},
			RecordedAt: pos.CreatedAt,
		})
	}

	writeJSON(w, http.StatusOK, api.TripPositionsResponse{
		Positions: apiPositions,
	})
}
func validateTripData(b api.TripData) error {
	if b.UserId == uuid.Nil {
		return fmt.Errorf("%w: user_id must be a non-empty UUID", domain.ErrInvalidArguments)
	}
	if b.DriverId == uuid.Nil {
		return fmt.Errorf("%w: driver_id must be a non-empty UUID", domain.ErrInvalidArguments)
	}
	if !validCoordinates(b.StartPoint.Latitude, b.StartPoint.Longitude) {
		return fmt.Errorf("%w: start_point is out of range", domain.ErrInvalidArguments)
	}
	if !validCoordinates(b.EndPoint.Latitude, b.EndPoint.Longitude) {
		return fmt.Errorf("%w: end_point is out of range", domain.ErrInvalidArguments)
	}
	if b.Price < 0 {
		return fmt.Errorf("%w: price must not be negative", domain.ErrInvalidArguments)
	}
	return nil
}

func validCoordinates(lat, lng float64) bool {
	return lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180
}

func mapDomainTripToAPI(trip *domain.Trip) api.Trip {
	return api.Trip{
		Id:       trip.ID,
		UserId:   trip.UserID,
		DriverId: trip.DriverID,
		Status:   api.TripStatus(trip.Status),
		StartPoint: api.Coordinates{
			Latitude:  trip.StartPoint.Latitude,
			Longitude: trip.StartPoint.Longitude,
		},
		EndPoint: api.Coordinates{
			Latitude:  trip.EndPoint.Latitude,
			Longitude: trip.EndPoint.Longitude,
		},
		Price:          trip.Price,
		StartedAt:      trip.StartedAt,
		FinishedAt:     trip.FinishedAt,
		LastPositionAt: trip.LastPositionAt,
	}
}

const problemTypeBase = "https://tripgo.example/problems/"

type problem struct {
	status int
	code   string
	title  string
	typ    string
}

var (
	problemInvalidRequest = problem{http.StatusBadRequest, "invalid_request", "Invalid request", problemTypeBase + "invalid-request"}
	problemTripNotFound   = problem{http.StatusNotFound, "trip_not_found", "Trip not found", problemTypeBase + "trip-not-found"}
	problemTripCompleted  = problem{http.StatusConflict, "trip_completed", "Trip completed", problemTypeBase + "trip-completed"}
	problemDriverBusy     = problem{http.StatusConflict, "driver_busy", "Driver busy", problemTypeBase + "driver-busy"}
	problemInternal       = problem{http.StatusInternalServerError, "internal_error", "Internal error", problemTypeBase + "internal-error"}
)

func InvalidRequest(w http.ResponseWriter, _ *http.Request, _ error) {
	writeProblem(w, problemInvalidRequest)
}

func writeError(w http.ResponseWriter, r *http.Request, op string, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidArguments):
		writeProblem(w, problemInvalidRequest)
	case errors.Is(err, domain.ErrTripNotFound):
		writeProblem(w, problemTripNotFound)
	case errors.Is(err, domain.ErrTripCompleted):
		writeProblem(w, problemTripCompleted)
	case errors.Is(err, domain.ErrDriverBusy):
		writeProblem(w, problemDriverBusy)
	default:
		slog.ErrorContext(r.Context(), "request failed", "op", op, "error", err)
		writeProblem(w, problemInternal)
	}
}

func writeProblem(w http.ResponseWriter, p problem) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(p.status)
	_ = json.NewEncoder(w).Encode(api.Problem{
		Status: int32(p.status),
		Code:   p.code,
		Title:  p.title,
		Type:   p.typ,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
