const { pool } = require('../db');
const logger = require('../logger');

/**
 * Periodic cleanup worker — mirrors internal/worker/cleanup.go.
 * Runs once at startup then every hour.
 *
 *  1. Reaps stalled 'in_flight' jobs (handles crashed worker threads)
 *  2. Deletes old delivered/dead jobs (7-day retention)
 *  3. Deletes orphaned events (no remaining delivery_jobs, older than 7 days)
 */
async function runCleanup() {
  try {
    // 1. Reap stalled in_flight jobs — worker crashed before marking them
    const reapResult = await pool.query(`
      UPDATE delivery_jobs
      SET status = 'pending'
      WHERE status = 'in_flight'
        AND next_attempt_at < NOW() - INTERVAL '5 minutes'
    `);
    if (reapResult.rowCount > 0) {
      logger.info({ count: reapResult.rowCount }, 'Reaped stalled in_flight jobs');
    }

    // 2. Delete old terminal jobs
    await pool.query(`
      DELETE FROM delivery_jobs
      WHERE status IN ('delivered', 'dead')
        AND created_at < NOW() - INTERVAL '7 days'
    `);

    // 3. Delete orphaned events
    await pool.query(`
      DELETE FROM events
      WHERE created_at < NOW() - INTERVAL '7 days'
        AND id NOT IN (SELECT DISTINCT event_id FROM delivery_jobs)
    `);

    logger.info('Cleanup cycle complete');
  } catch (err) {
    logger.error({ err }, 'Cleanup worker error');
  }
}

function start() {
  // Run immediately on boot, then every hour
  runCleanup();
  setInterval(runCleanup, 60 * 60 * 1000).unref();
}

module.exports = { start };
