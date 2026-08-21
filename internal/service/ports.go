package service

import (
	"context"

	"github.com/google/uuid"

	streamrepo "github.com/zomato/cohort-service/internal/repository/redis"

	"github.com/zomato/cohort-service/internal/domain"
)

// CohortRepository persists cohort metadata and refresh_run audit rows.
type CohortRepository interface {
	Create(ctx context.Context, c domain.Cohort) (domain.Cohort, error)
	Get(ctx context.Context, id uuid.UUID) (domain.Cohort, error)
	List(ctx context.Context) ([]domain.Cohort, error)
	Delete(ctx context.Context, id uuid.UUID) error
	MarkRefreshed(ctx context.Context, id uuid.UUID, size int) error
	CreateRefreshRun(ctx context.Context, cohortID uuid.UUID) (domain.RefreshRun, error)
	FinishRefreshRun(ctx context.Context, runID uuid.UUID, status domain.RefreshStatus, added, removed int, errMsg string) error
	LatestRefreshRun(ctx context.Context, cohortID uuid.UUID) (domain.RefreshRun, error)
}

// MembershipRepository owns forward + reverse membership indexes.
type MembershipRepository interface {
	Apply(ctx context.Context, cohortID uuid.UUID, adds, removes []string) error
	CurrentMembers(ctx context.Context, cohortID uuid.UUID) ([]string, error)
	Purge(ctx context.Context, cohortID uuid.UUID) error
	IsMember(ctx context.Context, cohortID uuid.UUID, userID string) (bool, error)
	CohortsForUser(ctx context.Context, userID string) ([]uuid.UUID, error)
	Size(ctx context.Context, cohortID uuid.UUID) (int, error)
}

// StreamProducer enqueues refresh jobs.
type StreamProducer interface {
	Publish(ctx context.Context, job streamrepo.RefreshJob) (string, error)
}

// UserRepository executes SQL cohort definitions against the read-only role.
type UserRepository interface {
	ResolveSQL(ctx context.Context, query string, emit func(userID string) error) error
}
