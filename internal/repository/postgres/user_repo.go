package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UserRepo executes user-supplied cohort SQL against the read-only role.
// Phase 5 wires up the real streaming implementation; this exposes the API surface.
type UserRepo struct {
	readerPool *pgxpool.Pool
	timeout    time.Duration
}

// NewUserRepo returns a repo bound to the given read-only pool.
func NewUserRepo(readerPool *pgxpool.Pool, timeout time.Duration) *UserRepo {
	return &UserRepo{readerPool: readerPool, timeout: timeout}
}

// ResolveSQL executes the given SELECT and streams userIds into the supplied callback.
// The query runs inside a read-only transaction with a statement timeout,
// so writes and runaway queries are rejected by the database itself.
func (r *UserRepo) ResolveSQL(ctx context.Context, query string, emit func(userID string) error) error {
	q := strings.TrimSpace(query)
	if q == "" {
		return fmt.Errorf("empty query")
	}

	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	tx, err := r.readerPool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return fmt.Errorf("begin ro tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL statement_timeout = %d", r.timeout.Milliseconds())); err != nil {
		return fmt.Errorf("set statement_timeout: %w", err)
	}

	rows, err := tx.Query(ctx, q)
	if err != nil {
		return fmt.Errorf("exec cohort sql: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("scan user id: %w", err)
		}
		if err := emit(id); err != nil {
			return err
		}
	}
	return rows.Err()
}
