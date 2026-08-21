package redis

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// MembershipRepo owns forward (cohort→users) and reverse (user→cohorts)
// membership indexes in Redis.
type MembershipRepo struct {
	c *redis.Client
}

// NewMembershipRepo binds to the given client.
func NewMembershipRepo(c *redis.Client) *MembershipRepo {
	return &MembershipRepo{c: c}
}

func cohortMembersKey(id uuid.UUID) string { return "cohort:" + id.String() + ":members" }
func userCohortsKey(userID string) string  { return "user:" + userID + ":cohorts" }

// CurrentMembers returns the current forward-membership set for a cohort.
func (r *MembershipRepo) CurrentMembers(ctx context.Context, cohortID uuid.UUID) ([]string, error) {
	members, err := r.c.SMembers(ctx, cohortMembersKey(cohortID)).Result()
	if err != nil {
		return nil, fmt.Errorf("smembers cohort: %w", err)
	}
	return members, nil
}

// Apply atomically pipelines add + remove operations against both the
// forward (cohort:{id}:members) and reverse (user:{u}:cohorts) indexes.
// Empty adds/removes are no-ops.
func (r *MembershipRepo) Apply(ctx context.Context, cohortID uuid.UUID, adds, removes []string) error {
	if len(adds) == 0 && len(removes) == 0 {
		return nil
	}
	cohortKey := cohortMembersKey(cohortID)
	cohortIDStr := cohortID.String()

	pipe := r.c.Pipeline()
	if len(adds) > 0 {
		asAny := toIfaceSlice(adds)
		pipe.SAdd(ctx, cohortKey, asAny...)
		for _, u := range adds {
			pipe.SAdd(ctx, userCohortsKey(u), cohortIDStr)
		}
	}
	if len(removes) > 0 {
		asAny := toIfaceSlice(removes)
		pipe.SRem(ctx, cohortKey, asAny...)
		for _, u := range removes {
			pipe.SRem(ctx, userCohortsKey(u), cohortIDStr)
		}
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("pipeline exec: %w", err)
	}
	return nil
}

// Purge removes a cohort entirely: deletes forward set and cleans reverse index for every current member.
func (r *MembershipRepo) Purge(ctx context.Context, cohortID uuid.UUID) error {
	members, err := r.CurrentMembers(ctx, cohortID)
	if err != nil {
		return err
	}
	pipe := r.c.Pipeline()
	pipe.Del(ctx, cohortMembersKey(cohortID))
	cohortIDStr := cohortID.String()
	for _, u := range members {
		pipe.SRem(ctx, userCohortsKey(u), cohortIDStr)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("purge pipeline: %w", err)
	}
	return nil
}

// IsMember returns whether userID is in cohortID.
func (r *MembershipRepo) IsMember(ctx context.Context, cohortID uuid.UUID, userID string) (bool, error) {
	ok, err := r.c.SIsMember(ctx, cohortMembersKey(cohortID), userID).Result()
	if err != nil {
		return false, fmt.Errorf("sismember: %w", err)
	}
	return ok, nil
}

// CohortsForUser returns the list of cohort ids the user is currently in.
func (r *MembershipRepo) CohortsForUser(ctx context.Context, userID string) ([]uuid.UUID, error) {
	raw, err := r.c.SMembers(ctx, userCohortsKey(userID)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		return nil, fmt.Errorf("smembers user: %w", err)
	}
	out := make([]uuid.UUID, 0, len(raw))
	for _, s := range raw {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, fmt.Errorf("parse cohort id %q: %w", s, err)
		}
		out = append(out, id)
	}
	return out, nil
}

// Size returns SCARD of the forward set.
func (r *MembershipRepo) Size(ctx context.Context, cohortID uuid.UUID) (int, error) {
	n, err := r.c.SCard(ctx, cohortMembersKey(cohortID)).Result()
	if err != nil {
		return 0, fmt.Errorf("scard: %w", err)
	}
	return int(n), nil
}

func toIfaceSlice(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}
