package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zomato/cohort-service/internal/domain"
)

// CohortRepo persists cohort metadata and refresh_run audit rows in Postgres.
type CohortRepo struct {
	pool *pgxpool.Pool
}

// NewCohortRepo returns a repo bound to the given pool.
func NewCohortRepo(pool *pgxpool.Pool) *CohortRepo {
	return &CohortRepo{pool: pool}
}

// Create inserts a new cohort. The ID, timestamps, and size are set on the
// returned struct. A unique-name violation is returned as domain.ErrConflict.
func (r *CohortRepo) Create(ctx context.Context, c domain.Cohort) (domain.Cohort, error) {
	const q = `
        INSERT INTO cohorts (name, description, type, sql_query, static_users)
        VALUES ($1, $2, $3, $4, $5)
        RETURNING id, size, created_at, updated_at`

	var sqlQuery *string
	if c.SQLQuery != "" {
		s := c.SQLQuery
		sqlQuery = &s
	}
	var staticUsers []string
	if c.Type == domain.CohortTypeStatic {
		staticUsers = c.StaticUsers
		if staticUsers == nil {
			staticUsers = []string{}
		}
	}

	err := r.pool.QueryRow(ctx, q,
		c.Name, c.Description, string(c.Type), sqlQuery, staticUsers,
	).Scan(&c.ID, &c.Size, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.Cohort{}, fmt.Errorf("cohort name %q: %w", c.Name, domain.ErrConflict)
		}
		return domain.Cohort{}, fmt.Errorf("insert cohort: %w", err)
	}
	return c, nil
}

// Get returns a cohort by id, or domain.ErrNotFound.
func (r *CohortRepo) Get(ctx context.Context, id uuid.UUID) (domain.Cohort, error) {
	const q = `
        SELECT id, name, description, type, COALESCE(sql_query, ''), COALESCE(static_users, '{}'),
               size, last_refreshed_at, created_at, updated_at
        FROM cohorts WHERE id = $1`
	var c domain.Cohort
	var cohortType string
	err := r.pool.QueryRow(ctx, q, id).Scan(
		&c.ID, &c.Name, &c.Description, &cohortType, &c.SQLQuery, &c.StaticUsers,
		&c.Size, &c.LastRefreshedAt, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Cohort{}, fmt.Errorf("cohort %s: %w", id, domain.ErrNotFound)
		}
		return domain.Cohort{}, fmt.Errorf("select cohort: %w", err)
	}
	c.Type = domain.CohortType(cohortType)
	return c, nil
}

// List returns all cohorts in creation order.
func (r *CohortRepo) List(ctx context.Context) ([]domain.Cohort, error) {
	const q = `
        SELECT id, name, description, type, COALESCE(sql_query, ''), COALESCE(static_users, '{}'),
               size, last_refreshed_at, created_at, updated_at
        FROM cohorts ORDER BY created_at ASC`
	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list cohorts: %w", err)
	}
	defer rows.Close()

	var out []domain.Cohort
	for rows.Next() {
		var c domain.Cohort
		var cohortType string
		if err := rows.Scan(
			&c.ID, &c.Name, &c.Description, &cohortType, &c.SQLQuery, &c.StaticUsers,
			&c.Size, &c.LastRefreshedAt, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan cohort: %w", err)
		}
		c.Type = domain.CohortType(cohortType)
		out = append(out, c)
	}
	return out, rows.Err()
}

// Delete removes a cohort by id. Missing rows return domain.ErrNotFound.
func (r *CohortRepo) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM cohorts WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete cohort: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("cohort %s: %w", id, domain.ErrNotFound)
	}
	return nil
}

// MarkRefreshed updates the cached size + last_refreshed_at after a successful refresh.
func (r *CohortRepo) MarkRefreshed(ctx context.Context, id uuid.UUID, size int) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE cohorts SET size = $1, last_refreshed_at = now(), updated_at = now() WHERE id = $2`,
		size, id,
	)
	if err != nil {
		return fmt.Errorf("mark refreshed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("cohort %s: %w", id, domain.ErrNotFound)
	}
	return nil
}

// CreateRefreshRun inserts a new refresh_run row in the pending state.
func (r *CohortRepo) CreateRefreshRun(ctx context.Context, cohortID uuid.UUID) (domain.RefreshRun, error) {
	const q = `
        INSERT INTO cohort_refresh_runs (cohort_id, status)
        VALUES ($1, 'pending')
        RETURNING id, cohort_id, status, added_count, removed_count, error, started_at, finished_at`
	var run domain.RefreshRun
	var status string
	err := r.pool.QueryRow(ctx, q, cohortID).Scan(
		&run.ID, &run.CohortID, &status,
		&run.AddedCount, &run.RemovedCount, &run.Error,
		&run.StartedAt, &run.FinishedAt,
	)
	if err != nil {
		return domain.RefreshRun{}, fmt.Errorf("insert refresh_run: %w", err)
	}
	run.Status = domain.RefreshStatus(status)
	return run, nil
}

// FinishRefreshRun updates a run row terminal fields.
func (r *CohortRepo) FinishRefreshRun(ctx context.Context, runID uuid.UUID, status domain.RefreshStatus, added, removed int, errMsg string) error {
	tag, err := r.pool.Exec(ctx, `
        UPDATE cohort_refresh_runs
        SET status = $1, added_count = $2, removed_count = $3, error = $4, finished_at = now()
        WHERE id = $5`,
		string(status), added, removed, strings.TrimSpace(errMsg), runID,
	)
	if err != nil {
		return fmt.Errorf("update refresh_run: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("refresh_run %s: %w", runID, domain.ErrNotFound)
	}
	return nil
}

// LatestRefreshRun returns the most recent run for a cohort, or ErrNotFound if none exists.
func (r *CohortRepo) LatestRefreshRun(ctx context.Context, cohortID uuid.UUID) (domain.RefreshRun, error) {
	const q = `
        SELECT id, cohort_id, status, added_count, removed_count, error, started_at, finished_at
        FROM cohort_refresh_runs
        WHERE cohort_id = $1
        ORDER BY started_at DESC
        LIMIT 1`
	var run domain.RefreshRun
	var status string
	err := r.pool.QueryRow(ctx, q, cohortID).Scan(
		&run.ID, &run.CohortID, &status,
		&run.AddedCount, &run.RemovedCount, &run.Error,
		&run.StartedAt, &run.FinishedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.RefreshRun{}, fmt.Errorf("no runs for %s: %w", cohortID, domain.ErrNotFound)
		}
		return domain.RefreshRun{}, fmt.Errorf("select latest run: %w", err)
	}
	run.Status = domain.RefreshStatus(status)
	return run, nil
}
