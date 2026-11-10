package response

import (
	"encoding/json"
	"log/slog"
	"net/http"

	api "github.com/Nikola4667636/template/internal/generated"
	"github.com/Nikola4667636/template/internal/http/domainerror"
)

func JSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func Problem(w http.ResponseWriter, r *http.Request, body api.Problem) {
	body.Instance = &r.URL.Path
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(int(body.Status))
	_ = json.NewEncoder(w).Encode(body)
}

func Error(w http.ResponseWriter, r *http.Request, logger *slog.Logger, op string, err error) {
	status, body := domainerror.Problem(err)
	if status >= http.StatusInternalServerError {
		logger.ErrorContext(r.Context(), "request failed", "op", op, "error", err)
	}
	Problem(w, r, body)
}
