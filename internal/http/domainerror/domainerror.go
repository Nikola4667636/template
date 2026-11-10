package domainerror

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Nikola4667636/template/internal/domain/trip"
	api "github.com/Nikola4667636/template/internal/generated"
)

const typeBase = "https://tripgo.example/problems/"

// New constructs transport error details without exposing an underlying error.
func New(status int32, code, title, detail string) api.Problem {
	body := api.Problem{
		Status: status,
		Code:   code,
		Title:  title,
		Type:   typeBase + strings.ReplaceAll(code, "_", "-"),
	}
	if detail != "" {
		body.Detail = &detail
	}
	return body
}

// Problem maps an error to its public HTTP representation.
func Problem(err error) (int, api.Problem) {
	var body api.Problem
	var validationErr *trip.ValidationError
	switch {
	case errors.As(err, &validationErr):
		body = New(http.StatusBadRequest, "invalid_request", "Invalid request", validationErr.Error())
	case errors.Is(err, trip.ErrInvalidArguments):
		body = New(http.StatusBadRequest, "invalid_request", "Invalid request", trip.ErrInvalidArguments.Error())
	case errors.Is(err, trip.ErrTripNotFound):
		body = New(http.StatusNotFound, "trip_not_found", "Trip not found", "trip not found")
	case errors.Is(err, trip.ErrTripCompleted):
		body = New(http.StatusConflict, "trip_completed", "Trip completed", "trip is already completed")
	case errors.Is(err, trip.ErrDriverBusy):
		body = New(http.StatusConflict, "driver_busy", "Driver busy", "driver already has an active trip")
	default:
		body = New(http.StatusInternalServerError, "internal_error", "Internal error", "internal error")
	}
	return int(body.Status), body
}
