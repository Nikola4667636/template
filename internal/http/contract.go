//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package http

import (
	"net/http"

	api "github.com/Nikola4667636/template/internal/generated"
)

type tripCreator interface {
	Handle(w http.ResponseWriter, r *http.Request, params api.CreateTripParams)
}

type tripGetter interface {
	Handle(w http.ResponseWriter, r *http.Request, id api.TripId)
}

type tripFinisher interface {
	Handle(w http.ResponseWriter, r *http.Request, id api.TripId)
}

type probe interface {
	ServeHTTP(w http.ResponseWriter, r *http.Request)
}
