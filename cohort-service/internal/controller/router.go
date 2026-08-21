package controller

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// NewRouter wires all endpoints. Middleware chain: request id, recover, logger.
func NewRouter(c *CohortController) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Logger)

	r.Get("/healthz", healthz)

	r.Route("/cohorts", func(r chi.Router) {
		r.Post("/", c.Create)
		r.Get("/", c.List)
		r.Get("/{id}", c.Get)
		r.Delete("/{id}", c.Delete)
		r.Post("/{id}/refresh", c.Refresh)
		r.Get("/{id}/members/{userId}", c.IsMember)
	})

	r.Get("/users/{userId}/cohorts", c.UserCohorts)

	return r
}

// NewHealthOnlyRouter is a minimal router used before the full service is wired.
// Kept exported so early-boot smoke tests still work.
func NewHealthOnlyRouter() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Get("/healthz", healthz)
	return r
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
