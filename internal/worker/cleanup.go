package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"kestrel/internal/metrics"
)

// CleanupWorker periodically removes old, completed data to prevent the database from growing indefinitely.
type CleanupWorker struct {
	pool    *pgxpool.Pool
	logger  *slog.Logger
	metrics *metrics.Metrics
	stop    chan struct{}
}

func NewCleanupWorker(pool *pgxpool.Pool, logger *slog.Logger, m *metrics.Metrics) *CleanupWorker {
	return &CleanupWorker{
		pool:    pool,
		logger:  logger,
		metrics: m,
		stop:    make(chan struct{}),
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

	// Because Dequeue doesn't update next_attempt_at, it holds the time the job was claimed.
	// If a job is in_flight and its next_attempt_at is older than 5 minutes, the worker died.
	const reapStalledQuery = `
		UPDATE delivery_jobs
		SET status = 'pending',
		    next_attempt_at = NOW(),
		    last_error = 'worker stalled/crashed during delivery'
		WHERE status = 'in_flight'
		AND next_attempt_at < NOW() - INTERVAL '5 minutes'
		RETURNING user_id
	`
	rows, err := w.pool.Query(ctx, reapStalledQuery)
	if err != nil {
		w.logger.Error("cleanup: failed to reap stalled jobs", "error", err)
	} else {
		var count int
		for rows.Next() {
			var userID [16]byte
			if err := rows.Scan(&userID); err == nil {
				w.metrics.DBStats.TrackUpdate(userID, "delivery_in_flight", -1)
				w.metrics.DBStats.TrackUpdate(userID, "delivery_pending", 1)
				count++
			}
		}
		rows.Close()
		if count > 0 {
			w.logger.Info("cleanup: reaped stalled jobs", "count", count)
		}
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
