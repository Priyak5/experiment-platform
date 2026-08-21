package controller

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/zomato/cohort-service/internal/domain"
)

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(code)
	if body != nil {
		_ = json.NewEncoder(w).Encode(body)
	}
}

// writeError maps sentinel domain errors to HTTP status codes.
func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
	case errors.Is(err, domain.ErrConflict):
		writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
	case errors.Is(err, domain.ErrInvalidDefinition):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
	}
}
