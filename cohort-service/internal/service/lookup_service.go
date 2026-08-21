package service

import (
	"context"

	"github.com/google/uuid"
)

// LookupService is the hot path for downstream services. It only touches Redis.
type LookupService struct {
	members MembershipRepository
}

// NewLookupService binds to the membership repository.
func NewLookupService(members MembershipRepository) *LookupService {
	return &LookupService{members: members}
}

// IsMember reports whether userID is in cohortID.
func (s *LookupService) IsMember(ctx context.Context, cohortID uuid.UUID, userID string) (bool, error) {
	return s.members.IsMember(ctx, cohortID, userID)
}

// CohortsForUser returns every cohort the user currently belongs to.
func (s *LookupService) CohortsForUser(ctx context.Context, userID string) ([]uuid.UUID, error) {
	return s.members.CohortsForUser(ctx, userID)
}
