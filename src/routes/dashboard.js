const express = require('express');
const crypto = require('crypto');
const { pool } = require('../db');
const PostgresQueue = require('../queue/postgresQueue');
const jwtAuth = require('../middleware/jwtAuth');
const adminOnly = require('../middleware/adminOnly');
const logger = require('../logger');

const router = express.Router();
const queue = new PostgresQueue(pool);

router.use(jwtAuth);

// ── GET /api/stats ────────────────────────────────────────────────────────────
// Returns per-tenant stats from system_stats cache + recent 50 deliveries.
router.get('/stats', async (req, res) => {
  const userId = req.userId;
  try {
    // Stats from system_stats (no COUNT(*) over full table)
    const statsResult = await pool.query(
      `SELECT key, value FROM system_stats WHERE user_id = $1`,
      [userId],
    );
    const statsMap = Object.fromEntries(statsResult.rows.map((r) => [r.key, parseInt(r.value)]));

    const deliveries = {
      pending: statsMap['delivery_pending'] || 0,
      in_flight: statsMap['delivery_in_flight'] || 0,
      delivered: statsMap['delivery_delivered'] || 0,
      failed: statsMap['delivery_failed'] || 0,
      dead: statsMap['delivery_dead'] || 0,
    };

    const queueDepth = await queue.getQueueDepth(userId);

    const { rows: subRows } = await pool.query(
      'SELECT COUNT(*) FROM subscriptions WHERE is_active = true AND user_id = $1',
      [userId],
    );

    // Recent 50 deliveries for the live feed
    const { rows: recent } = await pool.query(
      `SELECT dj.id, e.type AS event_type, s.endpoint_url, dj.status,
              dj.attempt_count, dj.last_status_code, dj.last_error,
              dj.created_at, dj.delivered_at
       FROM delivery_jobs dj
       JOIN events e ON e.id = dj.event_id
       JOIN subscriptions s ON s.id = dj.subscription_id
       WHERE dj.user_id = $1
       ORDER BY dj.created_at DESC
       LIMIT 50`,
      [userId],
    );

    return res.json({
      total_events: statsMap['total_events'] || 0,
      deliveries,
      queue_depth: queueDepth,
      active_subscriptions: parseInt(subRows[0].count),
      recent_deliveries: recent,
    });
  } catch (err) {
    logger.error({ err }, 'Stats error');
    return res.status(500).json({ error: 'internal server error' });
  }
});

// ── GET /api/keys ─────────────────────────────────────────────────────────────
router.get('/keys', async (req, res) => {
  try {
    const { rows } = await pool.query(
      'SELECT id, key, created_at FROM api_keys WHERE user_id = $1 ORDER BY created_at DESC',
      [req.userId],
    );
    return res.json({ keys: rows });
  } catch (err) {
    logger.error({ err }, 'ListAPIKeys error');
    return res.status(500).json({ error: 'internal server error' });
  }
});

// ── POST /api/keys ────────────────────────────────────────────────────────────
router.post('/keys', async (req, res) => {
  try {
    const key = 'sk_live_' + crypto.randomBytes(24).toString('hex');
    const { rows } = await pool.query(
      'INSERT INTO api_keys (key, user_id) VALUES ($1, $2) RETURNING id, key, created_at',
      [key, req.userId],
    );
    return res.status(201).json(rows[0]);
  } catch (err) {
    logger.error({ err }, 'CreateAPIKey error');
    return res.status(500).json({ error: 'internal server error' });
  }
});

// ── DELETE /api/keys/:id ──────────────────────────────────────────────────────
router.delete('/keys/:id', async (req, res) => {
  try {
    const result = await pool.query(
      'DELETE FROM api_keys WHERE id = $1 AND user_id = $2',
      [req.params.id, req.userId],
    );
    if (result.rowCount === 0) return res.status(404).json({ error: 'key not found' });
    return res.json({ status: 'deleted' });
  } catch (err) {
    logger.error({ err }, 'DeleteAPIKey error');
    return res.status(500).json({ error: 'internal server error' });
  }
});

// ── GET /api/users ────────────────────────────────────────────────────────────
// Admin only — returns all users.
router.get('/users', adminOnly, async (_req, res) => {
  try {
    const { rows } = await pool.query(
      'SELECT id, email, role, created_at FROM users ORDER BY created_at DESC',
    );
    return res.json({ users: rows });
  } catch (err) {
    logger.error({ err }, 'ListUsers error');
    return res.status(500).json({ error: 'internal server error' });
  }
});

module.exports = router;
