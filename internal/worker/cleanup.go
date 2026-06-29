package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// CleanupWorker periodically removes old, completed data to prevent the database from growing indefinitely.
type CleanupWorker struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
	stop   chan struct{}
}

func NewCleanupWorker(pool *pgxpool.Pool, logger *slog.Logger) *CleanupWorker {
	return &CleanupWorker{
		pool:   pool,
		logger: logger,
		stop:   make(chan struct{}),
	}
}

// Start runs the cleanup loop in the background. It executes every hour.
func (w *CleanupWorker) Start() {
	go func() {
		// Run once on startup
		w.cleanup(context.Background())

		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-w.stop:
				return
			case <-ticker.C:
				w.cleanup(context.Background())
			}
		}
	}()
}

// Stop halts the cleanup loop.
func (w *CleanupWorker) Stop() {
	close(w.stop)
}

func (w *CleanupWorker) cleanup(ctx context.Context) {
	// Add timeout context for cleanup
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// 1. Delete jobs in terminal state (delivered, dead) older than 7 days
	const deleteJobsQuery = `
		DELETE FROM delivery_jobs
		WHERE status IN ('delivered', 'dead')
		AND created_at < NOW() - INTERVAL '7 days'
	`
	res, err := w.pool.Exec(ctx, deleteJobsQuery)
	if err != nil {
		w.logger.Error("cleanup: failed to delete old delivery jobs", "error", err)
	} else if count := res.RowsAffected(); count > 0 {
		w.logger.Info("cleanup: deleted old delivery jobs", "count", count)
	}

	// 2. Delete events older than 7 days that have no remaining jobs
	const deleteEventsQuery = `
		DELETE FROM events e
		WHERE e.created_at < NOW() - INTERVAL '7 days'
		AND NOT EXISTS (
			SELECT 1 FROM delivery_jobs dj WHERE dj.event_id = e.id
		)
	`
	res2, err := w.pool.Exec(ctx, deleteEventsQuery)
	if err != nil {
		w.logger.Error("cleanup: failed to delete old events", "error", err)
	} else if count := res2.RowsAffected(); count > 0 {
		w.logger.Info("cleanup: deleted old events", "count", count)
	}
}
