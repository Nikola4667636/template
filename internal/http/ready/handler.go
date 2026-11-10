package ready

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	api "github.com/Nikola4667636/template/internal/generated"
	"github.com/Nikola4667636/template/internal/http/requeststate"
	"github.com/Nikola4667636/template/internal/http/response"
)

type Handler struct {
	db      pinger
	timeout time.Duration
}

func New(db pinger, timeout time.Duration) *Handler {
	return &Handler{db: db, timeout: timeout}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	var err error
	if h.db == nil {
		err = errors.New("database is not configured")
	} else {
		err = h.db.Ping(ctx)
	}
	if err != nil {
		requeststate.RecordError(r.Context(), "check database readiness", fmt.Errorf("ping database: %w", err))
		response.JSON(w, http.StatusServiceUnavailable, api.HealthResponse{Status: api.Unavailable})
		return
	}
	response.JSON(w, http.StatusOK, api.HealthResponse{Status: api.Ok})
}
