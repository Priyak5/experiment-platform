package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/zomato/cohort-service/internal/domain"
)

func newTestCohortSvc(t *testing.T) (*CohortService, *fakeCohortRepo, *fakeMembershipRepo, *fakeProducer) {
	t.Helper()
	cr := newFakeCohortRepo()
	mr := newFakeMembershipRepo()
	pr := &fakeProducer{}
	return NewCohortService(cr, mr, pr), cr, mr, pr
}

func TestCohortService_Create_Static(t *testing.T) {
	svc, _, _, _ := newTestCohortSvc(t)
	c, err := svc.Create(context.Background(), CreateInput{
		Name: "vip", Type: domain.CohortTypeStatic, StaticUsers: []string{"u1", "u2"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if c.ID == uuid.Nil || c.Name != "vip" {
		t.Fatalf("bad cohort: %+v", c)
	}
}

func TestCohortService_Create_ValidationErrors(t *testing.T) {
	tests := []struct {
		name string
		in   CreateInput
	}{
		{"empty name", CreateInput{Type: domain.CohortTypeStatic, StaticUsers: []string{"u"}}},
		{"static no users", CreateInput{Name: "n", Type: domain.CohortTypeStatic}},
		{"static with sql", CreateInput{Name: "n", Type: domain.CohortTypeStatic, StaticUsers: []string{"u"}, SQLQuery: "select"}},
		{"sql no query", CreateInput{Name: "n", Type: domain.CohortTypeSQL}},
		{"sql with users", CreateInput{Name: "n", Type: domain.CohortTypeSQL, SQLQuery: "select", StaticUsers: []string{"u"}}},
		{"bad type", CreateInput{Name: "n", Type: domain.CohortType("weird"), StaticUsers: []string{"u"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, _, _ := newTestCohortSvc(t)
			_, err := svc.Create(context.Background(), tc.in)
			if !errors.Is(err, domain.ErrInvalidDefinition) {
				t.Fatalf("err = %v, want ErrInvalidDefinition", err)
			}
		})
	}
}

func TestCohortService_Delete_PurgesMembership(t *testing.T) {
	svc, _, mr, _ := newTestCohortSvc(t)
	c, _ := svc.Create(context.Background(), CreateInput{
		Name: "d", Type: domain.CohortTypeStatic, StaticUsers: []string{"u1"},
	})
	// prime membership so purge has something to do
	_ = mr.Apply(context.Background(), c.ID, []string{"u1"}, nil)

	if err := svc.Delete(context.Background(), c.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if mr.purgeCalls != 1 {
		t.Fatalf("purge called %d times, want 1", mr.purgeCalls)
	}
	_, err := svc.Get(context.Background(), c.ID)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("post-delete get = %v, want ErrNotFound", err)
	}
}

func TestCohortService_Delete_MissingCohort(t *testing.T) {
	svc, _, mr, _ := newTestCohortSvc(t)
	err := svc.Delete(context.Background(), uuid.New())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if mr.purgeCalls != 0 {
		t.Fatalf("purge called on missing cohort")
	}
}

func TestCohortService_EnqueueRefresh_HappyPath(t *testing.T) {
	svc, _, _, pr := newTestCohortSvc(t)
	c, _ := svc.Create(context.Background(), CreateInput{
		Name: "e", Type: domain.CohortTypeStatic, StaticUsers: []string{"u1"},
	})
	run, err := svc.EnqueueRefresh(context.Background(), c.ID)
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if run.Status != domain.RefreshStatusPending {
		t.Fatalf("run.Status = %s", run.Status)
	}
	if len(pr.jobs) != 1 {
		t.Fatalf("producer got %d jobs, want 1", len(pr.jobs))
	}
	if pr.jobs[0].CohortID != c.ID || pr.jobs[0].RunID != run.ID {
		t.Fatalf("job payload wrong: %+v", pr.jobs[0])
	}
}

func TestCohortService_EnqueueRefresh_MarksRunFailedOnPublishError(t *testing.T) {
	svc, cr, _, pr := newTestCohortSvc(t)
	c, _ := svc.Create(context.Background(), CreateInput{
		Name: "e2", Type: domain.CohortTypeStatic, StaticUsers: []string{"u1"},
	})
	pr.failNext = errors.New("boom")
	_, err := svc.EnqueueRefresh(context.Background(), c.ID)
	if err == nil {
		t.Fatal("want error")
	}
	// Latest run should be terminal-failed.
	latest, err := cr.LatestRefreshRun(context.Background(), c.ID)
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if latest.Status != domain.RefreshStatusFailed {
		t.Fatalf("status = %s, want failed", latest.Status)
	}
}

func TestCohortService_EnqueueRefresh_MissingCohort(t *testing.T) {
	svc, _, _, _ := newTestCohortSvc(t)
	_, err := svc.EnqueueRefresh(context.Background(), uuid.New())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
