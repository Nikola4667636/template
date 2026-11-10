package http

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/Nikola4667636/template/internal/domain/trip"
	api "github.com/Nikola4667636/template/internal/generated"
	"github.com/Nikola4667636/template/internal/http/domainerror"
	"github.com/Nikola4667636/template/internal/http/middleware"
	"github.com/Nikola4667636/template/internal/http/response"
)

type adapter struct {
	create tripCreator
	get    tripGetter
	finish tripFinisher
	health probe
	ready  probe
}

var _ api.ServerInterface = (*adapter)(nil)

func NewRouter(create tripCreator, get tripGetter, finish tripFinisher, health, ready probe, logger *slog.Logger) http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.Recover(logger))
	router.Use(chimiddleware.RequestID)
	router.NotFound(func(w http.ResponseWriter, r *http.Request) {
		response.Problem(w, r, domainerror.New(http.StatusNotFound, "not_found", "Not found", "route not found"))
	})
	router.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		response.Problem(w, r, domainerror.New(http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "method is not allowed for this route"))
	})
	return api.HandlerWithOptions(&adapter{create: create, get: get, finish: finish, health: health, ready: ready}, api.ChiServerOptions{
		BaseRouter: router,
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			invalid := &trip.ValidationError{Field: "request", Reason: "has invalid parameters"}
			var formatErr *api.InvalidParamFormatError
			var countErr *api.TooManyValuesForParamError
			switch {
			case errors.As(err, &formatErr):
				invalid.Field = formatErr.ParamName
				invalid.Reason = "must be a valid UUID"
			case errors.As(err, &countErr):
				invalid.Field = countErr.ParamName
				invalid.Reason = "must appear once"
			}
			response.Error(w, r, logger, "parse request parameters", invalid)
		},
	})
}

func (a *adapter) CreateTrip(w http.ResponseWriter, r *http.Request, params api.CreateTripParams) {
	a.create.Handle(w, r, params)
}

func (a *adapter) GetTrip(w http.ResponseWriter, r *http.Request, id api.TripId) {
	a.get.Handle(w, r, id)
}

func (a *adapter) FinishTrip(w http.ResponseWriter, r *http.Request, id api.TripId) {
	a.finish.Handle(w, r, id)
}

func (a *adapter) Health(w http.ResponseWriter, r *http.Request) {
	a.health.ServeHTTP(w, r)
}

func (a *adapter) Ready(w http.ResponseWriter, r *http.Request) {
	a.ready.ServeHTTP(w, r)
}
