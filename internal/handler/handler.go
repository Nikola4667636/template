package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	domain "github.com/Nikola4667636/template/internal/domain/trip"
	api "github.com/Nikola4667636/template/internal/generated"
)

const maxBodyBytes = 1 << 20

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		return err
	}

	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("body must contain a single JSON object")
	}
	return nil
}

type tripDataRaw struct {
	UserId     *uuid.UUID       `json:"user_id"`
	DriverId   *uuid.UUID       `json:"driver_id"`
	StartPoint *api.Coordinates `json:"start_point"`
	EndPoint   *api.Coordinates `json:"end_point"`
	Price      *int64           `json:"price"`
}

type APIHandler struct {
	db         Pinger
	createTrip createTripUseCase
	getTrip    getTripUseCase
	finishTrip finishTripUseCase
	dbTimeout  time.Duration
}

func NewAPIHandler(db Pinger, createTrip createTripUseCase, getTrip getTripUseCase, finishTrip finishTripUseCase, dbTimeout time.Duration) *APIHandler {
	return &APIHandler{
		db:         db,
		createTrip: createTrip,
		getTrip:    getTrip,
		finishTrip: finishTrip,
		dbTimeout:  dbTimeout,
	}
}

func (h *APIHandler) Health(w http.ResponseWriter, _ *http.Request) {
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
	var raw tripDataRaw

	if err := decodeJSON(w, r, &raw); err != nil {
		slog.DebugContext(r.Context(), "invalid create trip body", "error", err)
		writeProblem(w, r, problemInvalidRequest, "invalid JSON body")
		return
	}

	if raw.UserId == nil || raw.DriverId == nil ||
		raw.StartPoint == nil || raw.EndPoint == nil || raw.Price == nil {
		slog.DebugContext(r.Context(), "create trip: required fields missing")
		writeProblem(w, r, problemInvalidRequest, "required fields are missing")
		return
	}

	body := api.TripData{
		UserId:     *raw.UserId,
		DriverId:   *raw.DriverId,
		StartPoint: *raw.StartPoint,
		EndPoint:   *raw.EndPoint,
		Price:      *raw.Price,
	}

	trip, err := h.createTrip.Execute(r.Context(), domain.CreateTripParams{
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
	trip, err := h.getTrip.Execute(r.Context(), tripId)
	if err != nil {
		writeError(w, r, "get trip", err)
		return
	}

	writeJSON(w, http.StatusOK, mapDomainTripToAPI(trip))
}

func (h *APIHandler) FinishTrip(w http.ResponseWriter, r *http.Request, tripId api.TripId) {
	trip, err := h.finishTrip.Execute(r.Context(), tripId)
	if err != nil {
		writeError(w, r, "finish trip", err)
		return
	}

	writeJSON(w, http.StatusOK, mapDomainTripToAPI(trip))
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
		Price:      trip.Price,
		StartedAt:  trip.StartedAt,
		FinishedAt: trip.FinishedAt,
	}
}

const problemTypeBase = "https://tripgo.example/problems/"

type problem struct {
	status int32
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

func InvalidRequest(w http.ResponseWriter, r *http.Request, err error) {
	writeProblem(w, r, problemInvalidRequest, err.Error())
}

func writeError(w http.ResponseWriter, r *http.Request, op string, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidArguments):
		detail := domain.ErrInvalidArguments.Error()
		var validationErr *domain.ValidationError
		if errors.As(err, &validationErr) {
			detail = validationErr.Error()
		}
		writeProblem(w, r, problemInvalidRequest, detail)
	case errors.Is(err, domain.ErrTripNotFound):
		writeProblem(w, r, problemTripNotFound, "trip not found")
	case errors.Is(err, domain.ErrTripCompleted):
		writeProblem(w, r, problemTripCompleted, "trip is already completed")
	case errors.Is(err, domain.ErrDriverBusy):
		writeProblem(w, r, problemDriverBusy, "driver already has an active trip")
	default:
		slog.ErrorContext(r.Context(), "request failed", "op", op, "error", err)
		writeProblem(w, r, problemInternal, "internal error")
	}
}

func writeProblem(w http.ResponseWriter, r *http.Request, p problem, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(int(p.status))

	prob := api.Problem{
		Status:   p.status,
		Code:     p.code,
		Title:    p.title,
		Type:     p.typ,
		Instance: &r.URL.Path,
	}
	if detail != "" {
		prob.Detail = &detail
	}
	_ = json.NewEncoder(w).Encode(prob)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
