package create

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	domain "github.com/Nikola4667636/template/internal/domain/trip"
	api "github.com/Nikola4667636/template/internal/generated"
	"github.com/Nikola4667636/template/internal/http/response"
	"github.com/Nikola4667636/template/internal/http/trip"
)

const (
	maxBodyBytes    = 1 << 20
	decodeOperation = "decode create trip"
)

type Handler struct {
	uc     useCase
	logger *slog.Logger
}

func New(uc useCase, logger *slog.Logger) *Handler {
	return &Handler{uc: uc, logger: logger}
}

func (h *Handler) Handle(w http.ResponseWriter, r *http.Request, _ api.CreateTripParams) {
	var raw request
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		response.Error(w, r, h.logger, decodeOperation, &domain.ValidationError{
			Field: "body", Reason: "has invalid JSON or field values",
		})
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		response.Error(w, r, h.logger, decodeOperation, &domain.ValidationError{
			Field: "body", Reason: "must contain a single JSON object",
		})
		return
	}
	params, err := toParams(raw)
	if err != nil {
		response.Error(w, r, h.logger, decodeOperation, err)
		return
	}
	created, err := h.uc.Execute(r.Context(), params)
	if err != nil {
		response.Error(w, r, h.logger, "create trip", err)
		return
	}
	w.Header().Set("Location", "/api/v1/trips/"+created.ID.String())
	response.JSON(w, http.StatusCreated, trip.ToAPI(created))
}
