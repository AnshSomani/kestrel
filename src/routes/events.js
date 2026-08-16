const express = require('express');
const { v4: uuidv4 } = require('uuid');
const { pool } = require('../db');
const PostgresQueue = require('../queue/postgresQueue');
const aggregator = require('../metrics/aggregator');
const apiKeyAuth = require('../middleware/apiKeyAuth');
const jwtAuth = require('../middleware/jwtAuth');
const logger = require('../logger');

const router = express.Router();
const queue = new PostgresQueue(pool);

// ── POST /api/events ─────────────────────────────────────────────────────────
// Create an event, match subscriptions, and fan-out delivery jobs.
// Protected by API key auth (server-to-server).
router.post('/', apiKeyAuth, async (req, res) => {
  const { type, payload, idempotency_key } = req.body;

  if (!type || payload === undefined) {
    return res.status(400).json({ error: 'type and payload are required' });
  }
  if (typeof payload !== 'object' || payload === null) {
    return res.status(400).json({ error: 'payload must be a valid JSON object' });
  }

  const userId = req.userId;
  const idempotencyKey = idempotency_key || uuidv4();

  try {
    // Idempotent insert — ON CONFLICT DO NOTHING
    const insertResult = await pool.query(
      `INSERT INTO events (type, payload, idempotency_key, user_id)
       VALUES ($1, $2, $3, $4)
       ON CONFLICT (idempotency_key) DO NOTHING
       RETURNING id, type, payload, idempotency_key, created_at`,
      [type, payload, idempotencyKey, userId],
    );

    let event;
    let isNew = false;

    if (insertResult.rows.length > 0) {
      event = insertResult.rows[0];
      isNew = true;
    } else {
      // Duplicate idempotency key — return existing event
      const existing = await pool.query(
        `SELECT id, type, payload, idempotency_key, created_at
         FROM events WHERE idempotency_key = $1 AND user_id = $2`,
        [idempotencyKey, userId],
      );
      event = existing.rows[0];
    }

    if (!isNew) {
      return res.status(200).json({
        id: event.id,
        type: event.type,
        payload: event.payload,
        idempotency_key: event.idempotency_key,
        created_at: event.created_at,
        deliveries_created: 0,
      });
    }

    // Match active subscriptions for this tenant + event type
    const subResult = await pool.query(
      `SELECT id FROM subscriptions
       WHERE is_active = true AND user_id = $1 AND $2 = ANY(event_types)`,
      [userId, type],
    );
    const subscriptionIds = subResult.rows.map((r) => r.id);

    let deliveriesCreated = 0;
    if (subscriptionIds.length > 0) {
      await queue.enqueueBatch(event.id, subscriptionIds, userId);
      deliveriesCreated = subscriptionIds.length;
      aggregator.track(userId, 'delivery_pending', deliveriesCreated);
    }

    aggregator.metrics.eventsIngested.inc();
    aggregator.track(userId, 'total_events', 1);

    return res.status(201).json({
      id: event.id,
      type: event.type,
      payload: event.payload,
      idempotency_key: event.idempotency_key,
      deliveries_created: deliveriesCreated,
      created_at: event.created_at,
    });
  } catch (err) {
    logger.error({ err }, 'CreateEvent error');
    return res.status(500).json({ error: 'internal server error' });
  }
});

// ── GET /api/events ──────────────────────────────────────────────────────────
// Paginated list of events for the authenticated tenant.
// Protected by JWT.
router.get('/', jwtAuth, async (req, res) => {
  const userId = req.userId;
  const limit = Math.min(parseInt(req.query.limit) || 20, 100);
  const page = Math.max(parseInt(req.query.page) || 1, 1);
  const offset = (page - 1) * limit;
  const typeFilter = req.query.type || null;

  try {
    const countParams = [userId];
    let countSql = 'SELECT COUNT(*) FROM events WHERE user_id = $1';
    if (typeFilter) {
      countSql += ' AND type = $2';
      countParams.push(typeFilter);
    }
    const countResult = await pool.query(countSql, countParams);
    const totalCount = parseInt(countResult.rows[0].count);
    const totalPages = Math.ceil(totalCount / limit) || 1;

    const params = [userId];
    let sql = 'SELECT id, type, payload, idempotency_key, created_at FROM events WHERE user_id = $1';
    if (typeFilter) {
      sql += ' AND type = $2';
      params.push(typeFilter);
    }
    sql += ` ORDER BY created_at DESC, id DESC LIMIT $${params.length + 1} OFFSET $${params.length + 2}`;
    params.push(limit, offset);

    const { rows } = await pool.query(sql, params);
    return res.json({ events: rows, total_pages: totalPages, current_page: page, total_count: totalCount });
  } catch (err) {
    logger.error({ err }, 'ListEvents error');
    return res.status(500).json({ error: 'internal server error' });
  }
});

// ── GET /api/events/:id ──────────────────────────────────────────────────────
router.get('/:id', jwtAuth, async (req, res) => {
  const { id } = req.params;
  const userId = req.userId;
  try {
    const { rows } = await pool.query(
      'SELECT id, type, payload, idempotency_key, created_at FROM events WHERE id = $1 AND user_id = $2',
      [id, userId],
    );
    if (rows.length === 0) return res.status(404).json({ error: 'event not found' });
    return res.json(rows[0]);
  } catch (err) {
    logger.error({ err }, 'GetEvent error');
    return res.status(500).json({ error: 'internal server error' });
  }
});

// ── GET /api/events/:id/deliveries ───────────────────────────────────────────
// Returns all delivery jobs for an event. Protected by API key.
router.get('/:id/deliveries', apiKeyAuth, async (req, res) => {
  const { id } = req.params;
  const userId = req.userId;
  try {
    const jobs = await queue.getJobsByEvent(id, userId);
    return res.json({ deliveries: jobs });
  } catch (err) {
    logger.error({ err }, 'GetDeliveryJobs error');
    return res.status(500).json({ error: 'internal server error' });
  }
});

module.exports = router;
