package service

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/zomato/cohort-service/internal/domain"
)

func TestDiff(t *testing.T) {
	tests := []struct {
		name                   string
		current, desired       []string
		wantAdds, wantRemoves  []string
	}{
		{"empty to non-empty", nil, []string{"a", "b"}, []string{"a", "b"}, nil},
		{"non-empty to empty", []string{"a", "b"}, nil, nil, []string{"a", "b"}},
		{"no change", []string{"a", "b"}, []string{"b", "a"}, nil, nil},
		{"add and remove", []string{"a", "b"}, []string{"b", "c"}, []string{"c"}, []string{"a"}},
		{"identical duplicates ignored", []string{"a"}, []string{"a", "a"}, nil, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			adds, removes := diff(tc.current, tc.desired)
			sort.Strings(adds)
			sort.Strings(removes)
			sort.Strings(tc.wantAdds)
			sort.Strings(tc.wantRemoves)
			if !equalStrings(adds, tc.wantAdds) {
				t.Errorf("adds = %v, want %v", adds, tc.wantAdds)
			}
			if !equalStrings(removes, tc.wantRemoves) {
				t.Errorf("removes = %v, want %v", removes, tc.wantRemoves)
			}
		})
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

func TestRefreshService_Run_StaticHappyPath(t *testing.T) {
	cr := newFakeCohortRepo()
	mr := newFakeMembershipRepo()
	ur := &fakeUserRepo{}
	svc := NewRefreshService(cr, ur, mr)

	c, _ := cr.Create(context.Background(), domain.Cohort{
		Name: "s", Type: domain.CohortTypeStatic, StaticUsers: []string{"u1", "u2", "u3"},
	})
	run, _ := cr.CreateRefreshRun(context.Background(), c.ID)

	if err := svc.Run(context.Background(), c.ID, run.ID); err != nil {
		t.Fatalf("run: %v", err)
	}
	got, _ := mr.CurrentMembers(context.Background(), c.ID)
	sort.Strings(got)
	if !equalStrings(got, []string{"u1", "u2", "u3"}) {
		t.Fatalf("members = %v", got)
	}
	latest, _ := cr.LatestRefreshRun(context.Background(), c.ID)
	if latest.Status != domain.RefreshStatusSucceeded {
		t.Fatalf("status = %s", latest.Status)
	}
	if latest.AddedCount != 3 || latest.RemovedCount != 0 {
		t.Fatalf("counts wrong: added=%d removed=%d", latest.AddedCount, latest.RemovedCount)
	}
}

func TestRefreshService_Run_SQL(t *testing.T) {
	cr := newFakeCohortRepo()
	mr := newFakeMembershipRepo()
	ur := &fakeUserRepo{rows: []string{"a", "b"}}
	svc := NewRefreshService(cr, ur, mr)

	c, _ := cr.Create(context.Background(), domain.Cohort{
		Name: "q", Type: domain.CohortTypeSQL, SQLQuery: "SELECT id FROM users",
	})
	run, _ := cr.CreateRefreshRun(context.Background(), c.ID)
	if err := svc.Run(context.Background(), c.ID, run.ID); err != nil {
		t.Fatalf("run: %v", err)
	}
	sz, _ := mr.Size(context.Background(), c.ID)
	if sz != 2 {
		t.Fatalf("size = %d, want 2", sz)
	}
}

func TestRefreshService_Run_DiffAppliesAddsAndRemoves(t *testing.T) {
	cr := newFakeCohortRepo()
	mr := newFakeMembershipRepo()
	ur := &fakeUserRepo{}
	svc := NewRefreshService(cr, ur, mr)

	c, _ := cr.Create(context.Background(), domain.Cohort{
		Name: "diff", Type: domain.CohortTypeStatic, StaticUsers: []string{"a", "b", "c"},
	})
	run1, _ := cr.CreateRefreshRun(context.Background(), c.ID)
	if err := svc.Run(context.Background(), c.ID, run1.ID); err != nil {
		t.Fatalf("run1: %v", err)
	}

	// Rewrite the cohort's static list to drop `b` and add `d`.
	// Simulate this by mutating the fake directly, since there is no Update API on the service.
	cr.mu.Lock()
	cohort := cr.cohorts[c.ID]
	cohort.StaticUsers = []string{"a", "c", "d"}
	cr.cohorts[c.ID] = cohort
	cr.mu.Unlock()

	run2, _ := cr.CreateRefreshRun(context.Background(), c.ID)
	if err := svc.Run(context.Background(), c.ID, run2.ID); err != nil {
		t.Fatalf("run2: %v", err)
	}
	got, _ := mr.CurrentMembers(context.Background(), c.ID)
	sort.Strings(got)
	if !equalStrings(got, []string{"a", "c", "d"}) {
		t.Fatalf("members = %v", got)
	}
	latest, _ := cr.LatestRefreshRun(context.Background(), c.ID)
	if latest.AddedCount != 1 || latest.RemovedCount != 1 {
		t.Fatalf("counts: added=%d removed=%d", latest.AddedCount, latest.RemovedCount)
	}
	// b's reverse index cleaned.
	ids, _ := mr.CohortsForUser(context.Background(), "b")
	if len(ids) != 0 {
		t.Fatalf("b still linked to cohorts: %v", ids)
	}
}

func TestRefreshService_Run_SQLError(t *testing.T) {
	cr := newFakeCohortRepo()
	mr := newFakeMembershipRepo()
	ur := &fakeUserRepo{err: errors.New("boom")}
	svc := NewRefreshService(cr, ur, mr)

	c, _ := cr.Create(context.Background(), domain.Cohort{
		Name: "err", Type: domain.CohortTypeSQL, SQLQuery: "SELECT id FROM users",
	})
	run, _ := cr.CreateRefreshRun(context.Background(), c.ID)
	err := svc.Run(context.Background(), c.ID, run.ID)
	if err == nil {
		t.Fatal("want error")
	}
	latest, _ := cr.LatestRefreshRun(context.Background(), c.ID)
	if latest.Status != domain.RefreshStatusFailed {
		t.Fatalf("status = %s, want failed", latest.Status)
	}
	if latest.Error == "" {
		t.Fatal("error message empty")
	}
}
