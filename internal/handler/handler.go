package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"template/api"
)

type Pinger interface {
	Ping(ctx context.Context) error
}

type APIHandler struct {
	db Pinger
}

func NewAPIHandler(db Pinger) *APIHandler {
	return &APIHandler{db: db}
}

// GET /health
func (h *APIHandler) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(api.HealthResponse{Status: api.Ok})
}

// GET /ready
func (h *APIHandler) Ready(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if h.db == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(api.HealthResponse{Status: api.Unavailable})
		return
	}

	if err := h.db.Ping(r.Context()); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(api.HealthResponse{Status: api.Unavailable})
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(api.HealthResponse{Status: api.Ok})
}

// POST /api/v1/trips
func (h *APIHandler) CreateTrip(w http.ResponseWriter, r *http.Request, params api.CreateTripParams) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

// GET /api/v1/trips/{tripId}
func (h *APIHandler) GetTrip(w http.ResponseWriter, r *http.Request, tripId api.TripId) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

// POST /api/v1/trips/{tripId}/finish
func (h *APIHandler) FinishTrip(w http.ResponseWriter, r *http.Request, tripId api.TripId) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

// POST /api/v1/trips/{tripId}/positions
func (h *APIHandler) CreateTripPosition(w http.ResponseWriter, r *http.Request, tripId api.TripId) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

// GET /api/v1/trips/{tripId}/positions
func (h *APIHandler) ListTripPositions(w http.ResponseWriter, r *http.Request, tripId api.TripId) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}
