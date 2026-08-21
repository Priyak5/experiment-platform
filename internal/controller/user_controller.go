package controller

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// UserCohorts handles GET /users/{userId}/cohorts — the reverse lookup.
func (c *CohortController) UserCohorts(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userId")
	if userID == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "userId required"})
		return
	}
	ids, err := c.lookups.CohortsForUser(r.Context(), userID)
	if err != nil {
		writeError(w, err)
		return
	}
	if ids == nil {
		ids = []uuid.UUID{}
	}
	writeJSON(w, http.StatusOK, cohortsForUserResponse{CohortIDs: ids})
}
