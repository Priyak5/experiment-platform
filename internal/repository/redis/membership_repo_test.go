package redis

import (
	"context"
	"sort"
	"testing"

	"github.com/google/uuid"
)

func TestMembership_ApplyAdds(t *testing.T) {
	c := setupTestRedis(t)
	repo := NewMembershipRepo(c)
	ctx := context.Background()

	cohortID := uuid.New()
	if err := repo.Apply(ctx, cohortID, []string{"u1", "u2", "u3"}, nil); err != nil {
		t.Fatalf("apply: %v", err)
	}
	got, err := repo.CurrentMembers(ctx, cohortID)
	if err != nil {
		t.Fatalf("current: %v", err)
	}
	sort.Strings(got)
	if !equalStrings(got, []string{"u1", "u2", "u3"}) {
		t.Fatalf("members = %v", got)
	}

	// Reverse index populated for each user.
	for _, u := range []string{"u1", "u2", "u3"} {
		ids, err := repo.CohortsForUser(ctx, u)
		if err != nil {
			t.Fatalf("cohortsForUser(%s): %v", u, err)
		}
		if len(ids) != 1 || ids[0] != cohortID {
			t.Fatalf("reverse index for %s = %v", u, ids)
		}
	}

	sz, err := repo.Size(ctx, cohortID)
	if err != nil {
		t.Fatalf("size: %v", err)
	}
	if sz != 3 {
		t.Fatalf("size = %d, want 3", sz)
	}
}

func TestMembership_AddAndRemove(t *testing.T) {
	c := setupTestRedis(t)
	repo := NewMembershipRepo(c)
	ctx := context.Background()

	cohortID := uuid.New()
	if err := repo.Apply(ctx, cohortID, []string{"u1", "u2", "u3"}, nil); err != nil {
		t.Fatalf("apply1: %v", err)
	}
	if err := repo.Apply(ctx, cohortID, []string{"u4"}, []string{"u2"}); err != nil {
		t.Fatalf("apply2: %v", err)
	}

	got, err := repo.CurrentMembers(ctx, cohortID)
	if err != nil {
		t.Fatalf("current: %v", err)
	}
	sort.Strings(got)
	if !equalStrings(got, []string{"u1", "u3", "u4"}) {
		t.Fatalf("members = %v", got)
	}

	// u2 reverse index cleaned.
	ids, err := repo.CohortsForUser(ctx, "u2")
	if err != nil {
		t.Fatalf("u2: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("u2 still has cohorts: %v", ids)
	}

	// u4 reverse index populated.
	ids, err = repo.CohortsForUser(ctx, "u4")
	if err != nil {
		t.Fatalf("u4: %v", err)
	}
	if len(ids) != 1 || ids[0] != cohortID {
		t.Fatalf("u4 reverse index = %v", ids)
	}
}

func TestMembership_Idempotent(t *testing.T) {
	c := setupTestRedis(t)
	repo := NewMembershipRepo(c)
	ctx := context.Background()

	cohortID := uuid.New()
	adds := []string{"u1", "u2"}
	for i := 0; i < 3; i++ {
		if err := repo.Apply(ctx, cohortID, adds, nil); err != nil {
			t.Fatalf("apply %d: %v", i, err)
		}
	}
	sz, _ := repo.Size(ctx, cohortID)
	if sz != 2 {
		t.Fatalf("size after 3x apply = %d, want 2", sz)
	}
}

func TestMembership_IsMember(t *testing.T) {
	c := setupTestRedis(t)
	repo := NewMembershipRepo(c)
	ctx := context.Background()

	cohortID := uuid.New()
	_ = repo.Apply(ctx, cohortID, []string{"present"}, nil)

	ok, err := repo.IsMember(ctx, cohortID, "present")
	if err != nil || !ok {
		t.Fatalf("present: ok=%v err=%v", ok, err)
	}
	ok, err = repo.IsMember(ctx, cohortID, "absent")
	if err != nil || ok {
		t.Fatalf("absent: ok=%v err=%v", ok, err)
	}
}

func TestMembership_CohortsForUser_MultipleCohorts(t *testing.T) {
	c := setupTestRedis(t)
	repo := NewMembershipRepo(c)
	ctx := context.Background()

	c1, c2 := uuid.New(), uuid.New()
	_ = repo.Apply(ctx, c1, []string{"shared"}, nil)
	_ = repo.Apply(ctx, c2, []string{"shared"}, nil)

	ids, err := repo.CohortsForUser(ctx, "shared")
	if err != nil {
		t.Fatalf("cohortsForUser: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("len = %d, want 2 (%v)", len(ids), ids)
	}
}

func TestMembership_Purge(t *testing.T) {
	c := setupTestRedis(t)
	repo := NewMembershipRepo(c)
	ctx := context.Background()

	cohortID := uuid.New()
	_ = repo.Apply(ctx, cohortID, []string{"a", "b", "c"}, nil)
	if err := repo.Purge(ctx, cohortID); err != nil {
		t.Fatalf("purge: %v", err)
	}
	sz, _ := repo.Size(ctx, cohortID)
	if sz != 0 {
		t.Fatalf("size after purge = %d", sz)
	}
	for _, u := range []string{"a", "b", "c"} {
		ids, _ := repo.CohortsForUser(ctx, u)
		if len(ids) != 0 {
			t.Fatalf("reverse index for %s not cleaned: %v", u, ids)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
