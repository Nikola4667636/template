package get

import (
	"log/slog"
	"net/http"

	api "github.com/Nikola4667636/template/internal/generated"
	"github.com/Nikola4667636/template/internal/http/response"
	"github.com/Nikola4667636/template/internal/http/trip"
)

type Handler struct {
	uc     useCase
	logger *slog.Logger
}

func New(uc useCase, logger *slog.Logger) *Handler {
	return &Handler{uc: uc, logger: logger}
}

func (h *Handler) Handle(w http.ResponseWriter, r *http.Request, id api.TripId) {
	result, err := h.uc.Execute(r.Context(), id)
	if err != nil {
		response.Error(w, r, h.logger, "get trip", err)
		return
	}
	response.JSON(w, http.StatusOK, trip.ToAPI(result))
}
