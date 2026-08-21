package controller

import (
	"time"

	"github.com/google/uuid"

	"github.com/zomato/cohort-service/internal/domain"
)

// createCohortRequest is the wire form of POST /cohorts.
type createCohortRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Type        string   `json:"type"`
	SQLQuery    string   `json:"sql_query,omitempty"`
	StaticUsers []string `json:"static_users,omitempty"`
}

// cohortResponse is the wire form of a cohort record.
type cohortResponse struct {
	ID              uuid.UUID  `json:"id"`
	Name            string     `json:"name"`
	Description     string     `json:"description,omitempty"`
	Type            string     `json:"type"`
	SQLQuery        string     `json:"sql_query,omitempty"`
	StaticUsers     []string   `json:"static_users,omitempty"`
	Size            int        `json:"size"`
	LastRefreshedAt *time.Time `json:"last_refreshed_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func toCohortResponse(c domain.Cohort) cohortResponse {
	return cohortResponse{
		ID: c.ID, Name: c.Name, Description: c.Description,
		Type: string(c.Type), SQLQuery: c.SQLQuery, StaticUsers: c.StaticUsers,
		Size: c.Size, LastRefreshedAt: c.LastRefreshedAt,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}

type refreshRunResponse struct {
	ID           uuid.UUID  `json:"id"`
	CohortID     uuid.UUID  `json:"cohort_id"`
	Status       string     `json:"status"`
	AddedCount   int        `json:"added_count"`
	RemovedCount int        `json:"removed_count"`
	Error        string     `json:"error,omitempty"`
	StartedAt    time.Time  `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
}

func toRefreshRunResponse(r domain.RefreshRun) refreshRunResponse {
	return refreshRunResponse{
		ID: r.ID, CohortID: r.CohortID, Status: string(r.Status),
		AddedCount: r.AddedCount, RemovedCount: r.RemovedCount, Error: r.Error,
		StartedAt: r.StartedAt, FinishedAt: r.FinishedAt,
	}
}

type isMemberResponse struct {
	Member bool `json:"member"`
}

type cohortsForUserResponse struct {
	CohortIDs []uuid.UUID `json:"cohort_ids"`
}

type errorResponse struct {
	Error string `json:"error"`
}
