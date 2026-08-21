package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/zomato/cohort-service/internal/domain"
)

// RefreshService executes a single refresh job end-to-end:
// load cohort → resolve intended membership → diff against current →
// apply to Redis → mark cohort refreshed → write audit row.
type RefreshService struct {
	cohorts CohortRepository
	users   UserRepository
	members MembershipRepository
}

// NewRefreshService constructs a service.
func NewRefreshService(cohorts CohortRepository, users UserRepository, members MembershipRepository) *RefreshService {
	return &RefreshService{cohorts: cohorts, users: users, members: members}
}

// Run executes the refresh job identified by cohortID + runID.
// Regardless of success or failure, the run row is closed with a terminal status.
func (s *RefreshService) Run(ctx context.Context, cohortID, runID uuid.UUID) error {
	cohort, err := s.cohorts.Get(ctx, cohortID)
	if err != nil {
		_ = s.cohorts.FinishRefreshRun(ctx, runID, domain.RefreshStatusFailed, 0, 0, "load cohort: "+err.Error())
		return fmt.Errorf("load cohort: %w", err)
	}

	desired, err := s.resolveDesired(ctx, cohort)
	if err != nil {
		_ = s.cohorts.FinishRefreshRun(ctx, runID, domain.RefreshStatusFailed, 0, 0, "resolve: "+err.Error())
		return fmt.Errorf("resolve desired: %w", err)
	}

	current, err := s.members.CurrentMembers(ctx, cohortID)
	if err != nil {
		_ = s.cohorts.FinishRefreshRun(ctx, runID, domain.RefreshStatusFailed, 0, 0, "current: "+err.Error())
		return fmt.Errorf("current members: %w", err)
	}

	adds, removes := diff(current, desired)

	if err := s.members.Apply(ctx, cohortID, adds, removes); err != nil {
		_ = s.cohorts.FinishRefreshRun(ctx, runID, domain.RefreshStatusFailed, 0, 0, "apply: "+err.Error())
		return fmt.Errorf("apply: %w", err)
	}

	if err := s.cohorts.MarkRefreshed(ctx, cohortID, len(desired)); err != nil {
		_ = s.cohorts.FinishRefreshRun(ctx, runID, domain.RefreshStatusFailed, len(adds), len(removes), "mark: "+err.Error())
		return fmt.Errorf("mark refreshed: %w", err)
	}

	if err := s.cohorts.FinishRefreshRun(ctx, runID, domain.RefreshStatusSucceeded, len(adds), len(removes), ""); err != nil {
		return fmt.Errorf("finish run: %w", err)
	}
	return nil
}

func (s *RefreshService) resolveDesired(ctx context.Context, c domain.Cohort) ([]string, error) {
	switch c.Type {
	case domain.CohortTypeStatic:
		return c.StaticUsers, nil
	case domain.CohortTypeSQL:
		var out []string
		err := s.users.ResolveSQL(ctx, c.SQLQuery, func(id string) error {
			out = append(out, id)
			return nil
		})
		if err != nil {
			return nil, err
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unknown cohort type %q", c.Type)
	}
}

// diff returns (adds, removes) needed to move `current` to `desired`.
// Nil-safe: either side may be empty.
func diff(current, desired []string) (adds, removes []string) {
	curSet := make(map[string]struct{}, len(current))
	for _, u := range current {
		curSet[u] = struct{}{}
	}
	desSet := make(map[string]struct{}, len(desired))
	for _, u := range desired {
		desSet[u] = struct{}{}
	}
	for u := range desSet {
		if _, ok := curSet[u]; !ok {
			adds = append(adds, u)
		}
	}
	for u := range curSet {
		if _, ok := desSet[u]; !ok {
			removes = append(removes, u)
		}
	}
	return adds, removes
}
