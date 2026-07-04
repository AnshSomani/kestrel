// Package store provides persistent storage adapters for the Kestrel webhook
// delivery engine. PostgresStore wraps a pgxpool connection pool and handles
// schema migrations.
package store

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"

	"kestrel/migrations"
)

// PostgresStore manages a connection pool to PostgreSQL and provides
// helpers for migration execution.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// pool. MaxConns is set to 40 and MinConns to 5 to balance throughput with
// resource usage, ensuring enough connections for both the 32 background pollers
// and concurrent HTTP ingestion requests without hitting Render's 50-conn limit.
func NewPostgresStore(ctx context.Context, databaseURL string) (*PostgresStore, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing database URL: %w", err)
	}

	cfg.MaxConns = 15
	cfg.MinConns = 2

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("creating connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	slog.Info("connected to PostgreSQL", "max_conns", cfg.MaxConns, "min_conns", cfg.MinConns)

	return &PostgresStore{pool: pool}, nil
}

// Close releases all pooled connections.
func (s *PostgresStore) Close() {
	s.pool.Close()
}

// Pool returns the underlying pgxpool.Pool for direct access by other
// packages (e.g. the queue).
func (s *PostgresStore) Pool() *pgxpool.Pool {
	return s.pool
}

// RunMigrations reads embedded SQL migration files from the migrations
// package and executes them in lexicographic order. Each file is applied as a
// single statement block. This is intentionally simple — for production you
// would add a migrations tracking table, but for the current scope
// idempotent CREATE IF NOT EXISTS statements are sufficient.
func (s *PostgresStore) RunMigrations(ctx context.Context) error {
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("reading migration directory: %w", err)
	}

	// Collect and sort .sql files to guarantee deterministic ordering.
	var sqlFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if matched, _ := fs.Glob(migrations.FS, entry.Name()); len(matched) > 0 {
			// Only include .sql files.
			name := entry.Name()
			if len(name) > 4 && name[len(name)-4:] == ".sql" {
				sqlFiles = append(sqlFiles, name)
			}
		}
	}
	sort.Strings(sqlFiles)

	for _, name := range sqlFiles {
		slog.Info("applying migration", "file", name)

		content, err := fs.ReadFile(migrations.FS, name)
		if err != nil {
			return fmt.Errorf("reading migration %s: %w", name, err)
		}

		if _, err := s.pool.Exec(ctx, string(content)); err != nil {
			return fmt.Errorf("executing migration %s: %w", name, err)
		}

		slog.Info("migration applied", "file", name)
	}

	return nil
}
