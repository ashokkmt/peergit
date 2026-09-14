package httpserver

import (
	"net/http"
	"peergit/internal/health"
	"github.com/go-chi/chi/v5"
)

func NewRouter(healthHandler *health.Handler) http.Handler {
	r := chi.NewRouter()

	r.Get("/healthz", healthHandler.Healthz)
	return r
}
