package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/zomato/cohort-service/internal/domain"
)

func TestCohortRepo_CreateGetDelete(t *testing.T) {
	pool := setupTestDB(t)
	repo := NewCohortRepo(pool)
	ctx := context.Background()

	created, err := repo.Create(ctx, domain.Cohort{
		Name: "vip-1", Description: "hand picked", Type: domain.CohortTypeStatic,
		StaticUsers: []string{"u1", "u2", "u3"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == uuid.Nil {
		t.Fatal("id not set")
	}
	if created.CreatedAt.IsZero() {
		t.Fatal("created_at not set")
	}

	got, err := repo.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "vip-1" || got.Type != domain.CohortTypeStatic || len(got.StaticUsers) != 3 {
		t.Fatalf("unexpected cohort: %+v", got)
	}

	if err := repo.Delete(ctx, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = repo.Get(ctx, created.ID)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("post-delete get err = %v, want ErrNotFound", err)
	}
}

func TestCohortRepo_UniqueNameConflict(t *testing.T) {
	pool := setupTestDB(t)
	repo := NewCohortRepo(pool)
	ctx := context.Background()

	_, err := repo.Create(ctx, domain.Cohort{
		Name: "dup", Type: domain.CohortTypeStatic, StaticUsers: []string{"u1"},
	})
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err = repo.Create(ctx, domain.Cohort{
		Name: "dup", Type: domain.CohortTypeStatic, StaticUsers: []string{"u2"},
	})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("second create err = %v, want ErrConflict", err)
	}
}

func TestCohortRepo_GetNotFound(t *testing.T) {
	pool := setupTestDB(t)
	repo := NewCohortRepo(pool)
	_, err := repo.Get(context.Background(), uuid.New())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestCohortRepo_DeleteNotFound(t *testing.T) {
	pool := setupTestDB(t)
	repo := NewCohortRepo(pool)
	err := repo.Delete(context.Background(), uuid.New())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestCohortRepo_List(t *testing.T) {
	pool := setupTestDB(t)
	repo := NewCohortRepo(pool)
	ctx := context.Background()

	for _, n := range []string{"a", "b", "c"} {
		if _, err := repo.Create(ctx, domain.Cohort{
			Name: n, Type: domain.CohortTypeStatic, StaticUsers: []string{"u"},
		}); err != nil {
			t.Fatalf("create %s: %v", n, err)
		}
	}
	got, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0].Name != "a" || got[2].Name != "c" {
		t.Fatalf("list order wrong: %+v", got)
	}
}

func TestCohortRepo_SQLCohortRoundTrip(t *testing.T) {
	pool := setupTestDB(t)
	repo := NewCohortRepo(pool)
	ctx := context.Background()

	created, err := repo.Create(ctx, domain.Cohort{
		Name: "sql-c", Type: domain.CohortTypeSQL,
		SQLQuery: "SELECT id FROM users WHERE city = 'Delhi'",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := repo.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Type != domain.CohortTypeSQL || got.SQLQuery == "" {
		t.Fatalf("bad round trip: %+v", got)
	}
}

func TestCohortRepo_MarkRefreshed(t *testing.T) {
	pool := setupTestDB(t)
	repo := NewCohortRepo(pool)
	ctx := context.Background()

	c, err := repo.Create(ctx, domain.Cohort{
		Name: "mark", Type: domain.CohortTypeStatic, StaticUsers: []string{"u1"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if c.LastRefreshedAt != nil {
		t.Fatalf("new cohort should have nil LastRefreshedAt")
	}
	if err := repo.MarkRefreshed(ctx, c.ID, 42); err != nil {
		t.Fatalf("mark: %v", err)
	}
	got, err := repo.Get(ctx, c.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Size != 42 || got.LastRefreshedAt == nil {
		t.Fatalf("mark refreshed did not persist: %+v", got)
	}
}

func TestCohortRepo_RefreshRunLifecycle(t *testing.T) {
	pool := setupTestDB(t)
	repo := NewCohortRepo(pool)
	ctx := context.Background()

	c, err := repo.Create(ctx, domain.Cohort{
		Name: "run-lc", Type: domain.CohortTypeStatic, StaticUsers: []string{"u1"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	run, err := repo.CreateRefreshRun(ctx, c.ID)
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if run.Status != domain.RefreshStatusPending {
		t.Fatalf("initial status = %s", run.Status)
	}
	if err := repo.FinishRefreshRun(ctx, run.ID, domain.RefreshStatusSucceeded, 5, 1, ""); err != nil {
		t.Fatalf("finish: %v", err)
	}
	latest, err := repo.LatestRefreshRun(ctx, c.ID)
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if latest.Status != domain.RefreshStatusSucceeded {
		t.Fatalf("latest status = %s", latest.Status)
	}
	if latest.AddedCount != 5 || latest.RemovedCount != 1 {
		t.Fatalf("counts wrong: added=%d removed=%d", latest.AddedCount, latest.RemovedCount)
	}
	if latest.FinishedAt == nil {
		t.Fatal("finished_at should be set")
	}
}

func TestCohortRepo_LatestRefreshRun_NotFound(t *testing.T) {
	pool := setupTestDB(t)
	repo := NewCohortRepo(pool)
	ctx := context.Background()

	c, err := repo.Create(ctx, domain.Cohort{
		Name: "no-runs", Type: domain.CohortTypeStatic, StaticUsers: []string{"u"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err = repo.LatestRefreshRun(ctx, c.ID)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
