package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// CohortType is how a cohort's members are resolved.
type CohortType string

const (
	CohortTypeStatic CohortType = "static"
	CohortTypeSQL    CohortType = "sql"
)

// ParseCohortType validates and returns a CohortType.
func ParseCohortType(s string) (CohortType, error) {
	switch CohortType(s) {
	case CohortTypeStatic, CohortTypeSQL:
		return CohortType(s), nil
	default:
		return "", fmt.Errorf("%w: cohort type %q", ErrInvalidDefinition, s)
	}
}

// RefreshStatus is the terminal or in-flight state of a refresh run.
type RefreshStatus string

const (
	RefreshStatusPending   RefreshStatus = "pending"
	RefreshStatusRunning   RefreshStatus = "running"
	RefreshStatusSucceeded RefreshStatus = "succeeded"
	RefreshStatusFailed    RefreshStatus = "failed"
)

// ParseRefreshStatus validates and returns a RefreshStatus.
func ParseRefreshStatus(s string) (RefreshStatus, error) {
	switch RefreshStatus(s) {
	case RefreshStatusPending, RefreshStatusRunning, RefreshStatusSucceeded, RefreshStatusFailed:
		return RefreshStatus(s), nil
	default:
		return "", fmt.Errorf("%w: refresh status %q", ErrInvalidDefinition, s)
	}
}

// Cohort is the durable definition of a user segment.
type Cohort struct {
	ID              uuid.UUID
	Name            string
	Description     string
	Type            CohortType
	SQLQuery        string
	StaticUsers     []string
	Size            int
	LastRefreshedAt *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// RefreshRun is one execution attempt of a cohort refresh.
type RefreshRun struct {
	ID           uuid.UUID
	CohortID     uuid.UUID
	Status       RefreshStatus
	AddedCount   int
	RemovedCount int
	Error        string
	StartedAt    time.Time
	FinishedAt   *time.Time
}
