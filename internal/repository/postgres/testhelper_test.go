package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultTestDSN = "postgres://cohort:cohort@127.0.0.1:5432/cohort?sslmode=disable"

// setupTestDB returns a fresh pool and applies migrations. It skips the test
// when `-short` is set (unit-test-only mode) or when Postgres is unreachable.
func setupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test skipped in -short mode")
	}
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		dsn = defaultTestDSN
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := NewPool(ctx, dsn)
	if err != nil {
		t.Skipf("postgres unreachable at %s: %v", dsn, err)
	}
	t.Cleanup(func() { pool.Close() })

	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Truncate to isolate tests. cohort_refresh_runs cascades from cohorts;
	// truncate both to be explicit.
	if _, err := pool.Exec(ctx, `TRUNCATE cohorts, cohort_refresh_runs RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return pool
}
