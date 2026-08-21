package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/zomato/cohort-service/internal/domain"
	"github.com/zomato/cohort-service/internal/service"
)

// CohortServiceAPI is the subset of *service.CohortService the controller needs.
// Local interface = trivial testability with a fake.
type CohortServiceAPI interface {
	Create(ctx context.Context, in service.CreateInput) (domain.Cohort, error)
	Get(ctx context.Context, id uuid.UUID) (domain.Cohort, error)
	List(ctx context.Context) ([]domain.Cohort, error)
	Delete(ctx context.Context, id uuid.UUID) error
	EnqueueRefresh(ctx context.Context, cohortID uuid.UUID) (domain.RefreshRun, error)
}

// LookupServiceAPI is the subset of *service.LookupService the controller needs.
type LookupServiceAPI interface {
	IsMember(ctx context.Context, cohortID uuid.UUID, userID string) (bool, error)
	CohortsForUser(ctx context.Context, userID string) ([]uuid.UUID, error)
}

// CohortController wires HTTP handlers to service methods.
type CohortController struct {
	cohorts CohortServiceAPI
	lookups LookupServiceAPI
}

// New constructs a controller.
func New(cohorts CohortServiceAPI, lookups LookupServiceAPI) *CohortController {
	return &CohortController{cohorts: cohorts, lookups: lookups}
}

// Create handles POST /cohorts.
func (c *CohortController) Create(w http.ResponseWriter, r *http.Request) {
	var req createCohortRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: fmt.Sprintf("decode: %v", err)})
		return
	}
	t, err := domain.ParseCohortType(req.Type)
	if err != nil {
		writeError(w, err)
		return
	}
	created, err := c.cohorts.Create(r.Context(), service.CreateInput{
		Name: req.Name, Description: req.Description, Type: t,
		SQLQuery: req.SQLQuery, StaticUsers: req.StaticUsers,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toCohortResponse(created))
}

// List handles GET /cohorts.
func (c *CohortController) List(w http.ResponseWriter, r *http.Request) {
	cohorts, err := c.cohorts.List(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]cohortResponse, len(cohorts))
	for i, c := range cohorts {
		out[i] = toCohortResponse(c)
	}
	writeJSON(w, http.StatusOK, out)
}

// Get handles GET /cohorts/{id}.
func (c *CohortController) Get(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	got, err := c.cohorts.Get(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toCohortResponse(got))
}

// Delete handles DELETE /cohorts/{id}.
func (c *CohortController) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	if err := c.cohorts.Delete(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Refresh handles POST /cohorts/{id}/refresh.
func (c *CohortController) Refresh(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	run, err := c.cohorts.EnqueueRefresh(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, toRefreshRunResponse(run))
}

// IsMember handles GET /cohorts/{id}/members/{userId}.
func (c *CohortController) IsMember(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	userID := chi.URLParam(r, "userId")
	if userID == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "userId required"})
		return
	}
	ok, err := c.lookups.IsMember(r.Context(), id, userID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, isMemberResponse{Member: ok})
}

func parseUUIDParam(r *http.Request, name string) (uuid.UUID, error) {
	raw := chi.URLParam(r, name)
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, errors.New("invalid uuid in path")
	}
	return id, nil
}
