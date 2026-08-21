package postgres

import (
	"context"
	"embed"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate applies every embedded *.up.sql file in filename order,
// tracking applied versions in a schema_migrations table. Safe to run repeatedly.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, `
        CREATE TABLE IF NOT EXISTS schema_migrations (
            version TEXT PRIMARY KEY,
            applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
        )`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read embed: %w", err)
	}
	var upFiles []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".up.sql") {
			upFiles = append(upFiles, e.Name())
		}
	}
	sort.Strings(upFiles)

	for _, name := range upFiles {
		version := strings.TrimSuffix(name, ".up.sql")

		var exists bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)`, version,
		).Scan(&exists); err != nil {
			return fmt.Errorf("check %s: %w", version, err)
		}
		if exists {
			continue
		}

		body, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if _, err := pool.Exec(ctx, string(body)); err != nil {
			return fmt.Errorf("apply %s: %w", name, err)
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO schema_migrations (version) VALUES ($1)`, version,
		); err != nil {
			return fmt.Errorf("record %s: %w", version, err)
		}
	}
	return nil
}
