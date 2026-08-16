const { Pool } = require('pg');

/**
 * PostgreSQL-backed job queue using SELECT … FOR UPDATE SKIP LOCKED.
 * Multiple workers can dequeue concurrently without row-level contention.
 *
 * Accepts either a pg.Pool (main thread) or a connection string (worker threads
 * that need their own isolated pool).
 */
class PostgresQueue {
  constructor(poolOrUrl) {
    if (typeof poolOrUrl === 'string') {
      this._pool = new Pool({
        connectionString: poolOrUrl,
        max: 5,
        min: 1,
        idleTimeoutMillis: 30000,
      });
      this._pool.on('error', () => {}); // silence pool errors in worker thread
    } else {
      this._pool = poolOrUrl;
    }
  }

  /** Insert one delivery job for a single (event, subscription) pair. */
  async enqueue(eventId, subscriptionId, userId) {
    await this._pool.query(
      'INSERT INTO delivery_jobs (event_id, subscription_id, user_id) VALUES ($1, $2, $3)',
      [eventId, subscriptionId, userId],
    );
  }

  /**
   * Atomically insert delivery jobs for one event across multiple subscriptions.
   * All inserts happen in one transaction — either all succeed or none do.
   */
  async enqueueBatch(eventId, subscriptionIds, userId) {
    if (!subscriptionIds || subscriptionIds.length === 0) return;
    const client = await this._pool.connect();
    try {
      await client.query('BEGIN');
      for (const subId of subscriptionIds) {
        await client.query(
          'INSERT INTO delivery_jobs (event_id, subscription_id, user_id) VALUES ($1, $2, $3)',
          [eventId, subId, userId],
        );
      }
      await client.query('COMMIT');
    } catch (err) {
      await client.query('ROLLBACK');
      throw err;
    } finally {
      client.release();
    }
  }

  /**
   * Atomically claim up to `batchSize` pending jobs.
   * Uses SKIP LOCKED so multiple pollers don't block each other.
   * Returns fully-hydrated job objects including event payload and endpoint URL.
   */
  async dequeue(batchSize) {
    const { rows } = await this._pool.query(
      `WITH claimed AS (
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
         u.last_error, u.last_status_code, u.user_id,
         e.type AS event_type, e.payload,
         s.endpoint_url, s.secret
       FROM updated u
       JOIN events e ON e.id = u.event_id
       JOIN subscriptions s ON s.id = u.subscription_id`,
      [batchSize],
    );
    return rows;
  }

  /** Terminal success state. */
  async markDelivered(jobId) {
    await this._pool.query(
      `UPDATE delivery_jobs
       SET status = 'delivered', delivered_at = NOW()
       WHERE id = $1`,
      [jobId],
    );
  }

  /**
   * Return job to pending with a scheduled next retry time.
   * This counts as a real delivery attempt.
   */
  async markFailed(jobId, errMsg, statusCode, nextAttemptAt) {
    await this._pool.query(
      `UPDATE delivery_jobs
       SET status = 'pending', last_error = $2, last_status_code = $3, next_attempt_at = $4
       WHERE id = $1`,
      [jobId, errMsg, statusCode, nextAttemptAt],
    );
  }

  /** Terminal failure state — Dead Letter Queue. Never retried automatically. */
  async markDead(jobId, errMsg) {
    await this._pool.query(
      `UPDATE delivery_jobs
       SET status = 'dead', last_error = $2
       WHERE id = $1`,
      [jobId, errMsg],
    );
  }

  /**
   * Return a job to pending WITHOUT consuming a retry attempt.
   * Used for circuit-breaker and rate-limiter rejections — these are not real
   * delivery failures, so attempt_count is decremented back.
   */
  async requeue(jobId, reason, nextAttemptAt) {
    await this._pool.query(
      `UPDATE delivery_jobs
       SET status = 'pending',
           last_error = $2,
           next_attempt_at = $3,
           attempt_count = GREATEST(attempt_count - 1, 0)
       WHERE id = $1`,
      [jobId, reason, nextAttemptAt],
    );
  }

  /** Reads queue depth from system_stats (no COUNT(*) over full table). */
  async getQueueDepth(tenantId) {
    const { rows } = await this._pool.query(
      `SELECT value FROM system_stats WHERE key = 'delivery_pending' AND user_id = $1`,
      [tenantId],
    );
    return rows.length > 0 ? parseInt(rows[0].value) : 0;
  }

  /** Returns all delivery jobs for a given event (hydrated with event + sub data). */
  async getJobsByEvent(eventId, tenantId) {
    const { rows } = await this._pool.query(
      `SELECT
         dj.id, dj.event_id, dj.subscription_id, dj.status,
         dj.attempt_count, dj.max_attempts, dj.next_attempt_at,
         dj.last_error, dj.last_status_code, dj.user_id,
         e.type AS event_type, e.payload,
         s.endpoint_url, s.secret
       FROM delivery_jobs dj
       JOIN events e ON e.id = dj.event_id
       JOIN subscriptions s ON s.id = dj.subscription_id
       WHERE dj.event_id = $1 AND dj.user_id = $2
       ORDER BY dj.created_at`,
      [eventId, tenantId],
    );
    return rows;
  }
}

module.exports = PostgresQueue;
