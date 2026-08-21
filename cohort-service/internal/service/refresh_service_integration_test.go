package service_test

import (
	"context"
	"os"
	"testing"
	"time"

	pgrepo "github.com/zomato/cohort-service/internal/repository/postgres"
	rdsrepo "github.com/zomato/cohort-service/internal/repository/redis"
	"github.com/zomato/cohort-service/internal/domain"
	"github.com/zomato/cohort-service/internal/service"
)

func TestRefresh_EndToEnd_SQLCohort(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in -short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	pgDSN := os.Getenv("POSTGRES_DSN")
	if pgDSN == "" {
		pgDSN = "postgres://cohort:cohort@127.0.0.1:5432/cohort?sslmode=disable"
	}
	rdsAddr := os.Getenv("REDIS_ADDR")
	if rdsAddr == "" {
		rdsAddr = "127.0.0.1:6379"
	}

	pool, err := pgrepo.NewPool(ctx, pgDSN)
	if err != nil {
		t.Skipf("postgres unreachable: %v", err)
	}
	defer pool.Close()
	if err := pgrepo.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Wipe cohorts state; keep users seeded from migration 0004.
	if _, err := pool.Exec(ctx, `TRUNCATE cohorts, cohort_refresh_runs RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	rds, err := rdsrepo.NewClient(ctx, rdsAddr)
	if err != nil {
		t.Skipf("redis unreachable: %v", err)
	}
	defer rds.Close()
	if err := rds.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("flushdb: %v", err)
	}

	cohortRepo := pgrepo.NewCohortRepo(pool)
	userRepo := pgrepo.NewUserRepo(pool, 30*time.Second) // use the main pool for POC
	memRepo := rdsrepo.NewMembershipRepo(rds)

	refreshSvc := service.NewRefreshService(cohortRepo, userRepo, memRepo)

	// Seed data comes from migration 0004: 10k users, Delhi is city index 1 -> (gs % 6) == 1
	// LTV > 1_000_000 filters to a subset. Just count via SQL first to establish an expectation.
	var expected int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM users WHERE city='Delhi' AND ltv_cents > 1000000`,
	).Scan(&expected)
	if err != nil {
		t.Fatalf("expected count query: %v", err)
	}
	if expected == 0 {
		t.Fatalf("expected > 0 users to match; seed data missing?")
	}

	created, err := cohortRepo.Create(ctx, domain.Cohort{
		Name: "delhi-high-ltv",
		Type: domain.CohortTypeSQL,
		SQLQuery: `SELECT id FROM users WHERE city='Delhi' AND ltv_cents > 1000000`,
	})
	if err != nil {
		t.Fatalf("create cohort: %v", err)
	}
	run, err := cohortRepo.CreateRefreshRun(ctx, created.ID)
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := refreshSvc.Run(ctx, created.ID, run.ID); err != nil {
		t.Fatalf("refresh run: %v", err)
	}

	size, err := memRepo.Size(ctx, created.ID)
	if err != nil {
		t.Fatalf("size: %v", err)
	}
	if size != expected {
		t.Fatalf("membership size = %d, want %d", size, expected)
	}

	// Sanity: pick one member and confirm the reverse index sees the cohort.
	var sampleID string
	err = pool.QueryRow(ctx,
		`SELECT id FROM users WHERE city='Delhi' AND ltv_cents > 1000000 LIMIT 1`,
	).Scan(&sampleID)
	if err != nil {
		t.Fatalf("sample query: %v", err)
	}
	ok, err := memRepo.IsMember(ctx, created.ID, sampleID)
	if err != nil || !ok {
		t.Fatalf("IsMember(%s) = %v, err=%v", sampleID, ok, err)
	}
	ids, err := memRepo.CohortsForUser(ctx, sampleID)
	if err != nil {
		t.Fatalf("cohortsForUser: %v", err)
	}
	if len(ids) != 1 || ids[0] != created.ID {
		t.Fatalf("reverse index for %s = %v, want [%s]", sampleID, ids, created.ID)
	}

	// Latest run row records the counts.
	latest, err := cohortRepo.LatestRefreshRun(ctx, created.ID)
	if err != nil {
		t.Fatalf("latest run: %v", err)
	}
	if latest.Status != domain.RefreshStatusSucceeded {
		t.Fatalf("status = %s", latest.Status)
	}
	if latest.AddedCount != expected {
		t.Fatalf("added = %d, want %d", latest.AddedCount, expected)
	}
}
