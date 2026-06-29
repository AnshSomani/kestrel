// Package queue implements a durable, distributed job queue backed by
// PostgreSQL. It uses SELECT ... FOR UPDATE SKIP LOCKED for safe, concurrent
// job claiming without row-level contention between workers.
package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Job represents a hydrated delivery job with all the data a worker needs to
// execute the delivery: the job metadata, the event payload, and the target
// subscription details.
type Job struct {
	ID             uuid.UUID       `json:"id"`
	EventID        uuid.UUID       `json:"event_id"`
	SubscriptionID uuid.UUID       `json:"subscription_id"`
	Status         string          `json:"status"`
	AttemptCount   int             `json:"attempt_count"`
	MaxAttempts    int             `json:"max_attempts"`
	NextAttemptAt  time.Time       `json:"next_attempt_at"`
	LastError      *string         `json:"last_error,omitempty"`
	LastStatusCode *int            `json:"last_status_code,omitempty"`
	EventType      string          `json:"event_type"`
	Payload        json.RawMessage `json:"payload"`
	EndpointURL    string          `json:"endpoint_url"`
	Secret         string          `json:"-"` // never serialise secrets
	UserID         uuid.UUID       `json:"user_id"`
}

// PostgresQueue is a durable job queue backed by PostgreSQL. It leverages
// SKIP LOCKED to allow multiple pollers to claim disjoint batches of jobs
// without blocking each other.
type PostgresQueue struct {
	pool *pgxpool.Pool
}

// NewPostgresQueue returns a queue that operates against the given connection
// pool.
func NewPostgresQueue(pool *pgxpool.Pool) *PostgresQueue {
	return &PostgresQueue{pool: pool}
}

// Enqueue creates a single pending delivery job for the given event and
// subscription pair.
func (q *PostgresQueue) Enqueue(ctx context.Context, eventID, subscriptionID, userID uuid.UUID) error {
	const query = `INSERT INTO delivery_jobs (event_id, subscription_id, user_id) VALUES ($1, $2, $3)`
	if _, err := q.pool.Exec(ctx, query, eventID, subscriptionID, userID); err != nil {
		return fmt.Errorf("enqueue job: %w", err)
	}
	return nil
}

// EnqueueBatch creates delivery jobs for one event across multiple
// subscriptions inside a single transaction for atomicity and efficiency.
func (q *PostgresQueue) EnqueueBatch(ctx context.Context, eventID uuid.UUID, subscriptionIDs []uuid.UUID, userID uuid.UUID) error {
	if len(subscriptionIDs) == 0 {
		return nil
	}

	tx, err := q.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback on committed tx is a no-op

	batch := &pgx.Batch{}
	const query = `INSERT INTO delivery_jobs (event_id, subscription_id, user_id) VALUES ($1, $2, $3)`
	for _, subID := range subscriptionIDs {
		batch.Queue(query, eventID, subID, userID)
	}

	br := tx.SendBatch(ctx, batch)
	for range subscriptionIDs {
		if _, err := br.Exec(); err != nil {
			_ = br.Close()
			return fmt.Errorf("batch insert: %w", err)
		}
	}
	if err := br.Close(); err != nil {
		return fmt.Errorf("closing batch: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// Dequeue atomically claims up to batchSize pending jobs and returns them
// fully hydrated with event and subscription data.
//
// The query uses a CTE with FOR UPDATE SKIP LOCKED so multiple workers can
// poll concurrently without contention. Claimed jobs are moved to
// "in_flight" status and their attempt_count is incremented.
func (q *PostgresQueue) Dequeue(ctx context.Context, batchSize int) ([]*Job, error) {
	const query = `
		WITH claimed AS (
			SELECT id FROM delivery_jobs
			WHERE status = 'pending' AND next_attempt_at <= NOW()
			ORDER BY next_attempt_at ASC
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		),
		updated AS (
			UPDATE delivery_jobs
			SET status = 'in_flight', attempt_count = attempt_count + 1
			WHERE id IN (SELECT id FROM claimed)
			RETURNING *
		)
		SELECT
			u.id, u.event_id, u.subscription_id, u.status,
			u.attempt_count, u.max_attempts, u.next_attempt_at,
			u.last_error, u.last_status_code,
			e.type AS event_type, e.payload,
			s.endpoint_url, s.secret, u.user_id
		FROM updated u
		JOIN events e ON e.id = u.event_id
		JOIN subscriptions s ON s.id = u.subscription_id`

	rows, err := q.pool.Query(ctx, query, batchSize)
	if err != nil {
		return nil, fmt.Errorf("dequeue query: %w", err)
	}
	defer rows.Close()

	var jobs []*Job
	for rows.Next() {
		j := &Job{}
		if err := rows.Scan(
			&j.ID, &j.EventID, &j.SubscriptionID, &j.Status,
			&j.AttemptCount, &j.MaxAttempts, &j.NextAttemptAt,
			&j.LastError, &j.LastStatusCode,
			&j.EventType, &j.Payload,
			&j.EndpointURL, &j.Secret, &j.UserID,
		); err != nil {
			return nil, fmt.Errorf("scanning job row: %w", err)
		}
		jobs = append(jobs, j)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating job rows: %w", err)
	}

	return jobs, nil
}

// MarkDelivered transitions a job to the "delivered" terminal state and
// records the delivery timestamp.
func (q *PostgresQueue) MarkDelivered(ctx context.Context, jobID uuid.UUID) error {
	const query = `UPDATE delivery_jobs SET status = 'delivered', delivered_at = NOW() WHERE id = $1`
	if _, err := q.pool.Exec(ctx, query, jobID); err != nil {
		return fmt.Errorf("mark delivered: %w", err)
	}
	return nil
}

// MarkFailed returns a job to "pending" status with updated error details and
// a scheduled next retry time.
func (q *PostgresQueue) MarkFailed(ctx context.Context, jobID uuid.UUID, errMsg string, statusCode *int, nextAttempt time.Time) error {
	const query = `
		UPDATE delivery_jobs
		SET status = 'pending', last_error = $2, last_status_code = $3, next_attempt_at = $4
		WHERE id = $1`
	if _, err := q.pool.Exec(ctx, query, jobID, errMsg, statusCode, nextAttempt); err != nil {
		return fmt.Errorf("mark failed: %w", err)
	}
	return nil
}

// MarkDead transitions a job to the "dead" terminal state after exhausting
// all retry attempts. Dead jobs are never requeued automatically.
func (q *PostgresQueue) MarkDead(ctx context.Context, jobID uuid.UUID, errMsg string) error {
	const query = `UPDATE delivery_jobs SET status = 'dead', last_error = $2 WHERE id = $1`
	if _, err := q.pool.Exec(ctx, query, jobID, errMsg); err != nil {
		return fmt.Errorf("mark dead: %w", err)
	}
	return nil
}

// Requeue returns a job to "pending" status and decrements the attempt count
// (since Dequeue incremented it). Use this for rate limiting and circuit
// breaker rejections — these should NOT consume a retry attempt.
func (q *PostgresQueue) Requeue(ctx context.Context, jobID uuid.UUID, reason string, nextAttempt time.Time) error {
	const query = `
		UPDATE delivery_jobs
		SET status = 'pending', last_error = $2, next_attempt_at = $3,
		    attempt_count = GREATEST(attempt_count - 1, 0)
		WHERE id = $1`
	if _, err := q.pool.Exec(ctx, query, jobID, reason, nextAttempt); err != nil {
		return fmt.Errorf("requeue: %w", err)
	}
	return nil
}

// GetQueueDepth returns the current number of pending delivery jobs for a user.
func (q *PostgresQueue) GetQueueDepth(ctx context.Context, tenantID uuid.UUID) (int64, error) {
	var count int64
	const query = `SELECT value FROM system_stats WHERE key = 'delivery_pending' AND user_id = $1`
	if err := q.pool.QueryRow(ctx, query, tenantID).Scan(&count); err != nil {
		return 0, fmt.Errorf("get queue depth: %w", err)
	}
	return count, nil
}

// GetJobsByEvent returns all delivery jobs for a given event, ordered by
// creation time. Each job is hydrated with event and subscription data.
func (q *PostgresQueue) GetJobsByEvent(ctx context.Context, eventID, tenantID uuid.UUID) ([]*Job, error) {
	const query = `
		SELECT
			dj.id, dj.event_id, dj.subscription_id, dj.status,
			dj.attempt_count, dj.max_attempts, dj.next_attempt_at,
			dj.last_error, dj.last_status_code,
			e.type AS event_type, e.payload,
			s.endpoint_url, s.secret, dj.user_id
		FROM delivery_jobs dj
		JOIN events e ON e.id = dj.event_id
		JOIN subscriptions s ON s.id = dj.subscription_id
		WHERE dj.event_id = $1 AND dj.user_id = $2
		ORDER BY dj.created_at`

	rows, err := q.pool.Query(ctx, query, eventID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("get jobs by event: %w", err)
	}
	defer rows.Close()

	var jobs []*Job
	for rows.Next() {
		j := &Job{}
		if err := rows.Scan(
			&j.ID, &j.EventID, &j.SubscriptionID, &j.Status,
			&j.AttemptCount, &j.MaxAttempts, &j.NextAttemptAt,
			&j.LastError, &j.LastStatusCode,
			&j.EventType, &j.Payload,
			&j.EndpointURL, &j.Secret, &j.UserID,
		); err != nil {
			return nil, fmt.Errorf("scanning job row: %w", err)
		}
		jobs = append(jobs, j)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating job rows: %w", err)
	}

	return jobs, nil
}
