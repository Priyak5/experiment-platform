package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/zomato/cohort-service/internal/domain"
	streamrepo "github.com/zomato/cohort-service/internal/repository/redis"
)

// CohortService owns cohort lifecycle: create, read, delete, and refresh-enqueue.
type CohortService struct {
	cohorts  CohortRepository
	members  MembershipRepository
	producer StreamProducer
}

// NewCohortService constructs a service with its collaborators.
func NewCohortService(cohorts CohortRepository, members MembershipRepository, producer StreamProducer) *CohortService {
	return &CohortService{cohorts: cohorts, members: members, producer: producer}
}

// CreateInput carries the validated request body for creating a cohort.
type CreateInput struct {
	Name        string
	Description string
	Type        domain.CohortType
	SQLQuery    string
	StaticUsers []string
}

// Create validates the input and inserts a new cohort. It does NOT populate
// membership — callers must trigger a refresh (or the Create flow can do so
// itself; kept explicit for clarity).
func (s *CohortService) Create(ctx context.Context, in CreateInput) (domain.Cohort, error) {
	if strings.TrimSpace(in.Name) == "" {
		return domain.Cohort{}, fmt.Errorf("name required: %w", domain.ErrInvalidDefinition)
	}
	switch in.Type {
	case domain.CohortTypeStatic:
		if len(in.StaticUsers) == 0 {
			return domain.Cohort{}, fmt.Errorf("static cohort needs users: %w", domain.ErrInvalidDefinition)
		}
		if in.SQLQuery != "" {
			return domain.Cohort{}, fmt.Errorf("static cohort must not have sql_query: %w", domain.ErrInvalidDefinition)
		}
	case domain.CohortTypeSQL:
		if strings.TrimSpace(in.SQLQuery) == "" {
			return domain.Cohort{}, fmt.Errorf("sql cohort needs sql_query: %w", domain.ErrInvalidDefinition)
		}
		if len(in.StaticUsers) > 0 {
			return domain.Cohort{}, fmt.Errorf("sql cohort must not have static_users: %w", domain.ErrInvalidDefinition)
		}
	default:
		return domain.Cohort{}, fmt.Errorf("unknown type %q: %w", in.Type, domain.ErrInvalidDefinition)
	}
	return s.cohorts.Create(ctx, domain.Cohort{
		Name: in.Name, Description: in.Description, Type: in.Type,
		SQLQuery: in.SQLQuery, StaticUsers: in.StaticUsers,
	})
}

// Get fetches a cohort by id.
func (s *CohortService) Get(ctx context.Context, id uuid.UUID) (domain.Cohort, error) {
	return s.cohorts.Get(ctx, id)
}

// List returns all cohorts.
func (s *CohortService) List(ctx context.Context) ([]domain.Cohort, error) {
	return s.cohorts.List(ctx)
}

// Delete purges Redis membership then removes the metadata row.
// Order matters: if metadata is gone first, subsequent Purge lookups have no
// cohort to key on; if Redis purge fails, metadata is preserved so we can retry.
func (s *CohortService) Delete(ctx context.Context, id uuid.UUID) error {
	if _, err := s.cohorts.Get(ctx, id); err != nil {
		return err
	}
	if err := s.members.Purge(ctx, id); err != nil {
		return fmt.Errorf("purge membership: %w", err)
	}
	return s.cohorts.Delete(ctx, id)
}

// EnqueueRefresh creates a run row (pending) and publishes a job to the stream.
// The worker picks it up and executes the refresh.
func (s *CohortService) EnqueueRefresh(ctx context.Context, cohortID uuid.UUID) (domain.RefreshRun, error) {
	if _, err := s.cohorts.Get(ctx, cohortID); err != nil {
		return domain.RefreshRun{}, err
	}
	run, err := s.cohorts.CreateRefreshRun(ctx, cohortID)
	if err != nil {
		return domain.RefreshRun{}, err
	}
	if _, err := s.producer.Publish(ctx, streamrepo.RefreshJob{
		CohortID: cohortID, RunID: run.ID,
	}); err != nil {
		// Best-effort: mark the run failed so we don't leave a stale pending row.
		_ = s.cohorts.FinishRefreshRun(ctx, run.ID, domain.RefreshStatusFailed, 0, 0, "enqueue: "+err.Error())
		return domain.RefreshRun{}, fmt.Errorf("publish: %w", err)
	}
	return run, nil
}
