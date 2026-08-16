const express = require('express');
const { pool } = require('../db');
const multiAuth = require('../middleware/multiAuth');
const logger = require('../logger');

const router = express.Router();

// All subscription routes accept either API key or JWT (server-to-server or dashboard).
router.use(multiAuth);

// ── POST /api/subscriptions ──────────────────────────────────────────────────
router.post('/', async (req, res) => {
  const { endpoint_url, secret, event_types } = req.body;
  if (!endpoint_url || !secret || !event_types || event_types.length === 0) {
    return res.status(400).json({ error: 'endpoint_url, secret, and event_types are required' });
  }
  try {
    const { rows } = await pool.query(
      `INSERT INTO subscriptions (endpoint_url, secret, event_types, is_active, user_id)
       VALUES ($1, $2, $3, true, $4)
       RETURNING id, endpoint_url, secret, event_types, is_active, created_at`,
      [endpoint_url, secret, event_types, req.userId],
    );
    return res.status(201).json(rows[0]);
  } catch (err) {
    logger.error({ err }, 'CreateSubscription error');
    return res.status(500).json({ error: 'internal server error' });
  }
});

// ── PUT /api/subscriptions/:id ───────────────────────────────────────────────
router.put('/:id', async (req, res) => {
  const { id } = req.params;
  const { endpoint_url, secret, event_types } = req.body;
  if (!endpoint_url || !secret || !event_types || event_types.length === 0) {
    return res.status(400).json({ error: 'endpoint_url, secret, and event_types are required' });
  }
  try {
    const { rows } = await pool.query(
      `UPDATE subscriptions
       SET endpoint_url = $1, secret = $2, event_types = $3
       WHERE id = $4 AND user_id = $5
       RETURNING id, endpoint_url, secret, event_types, is_active, created_at`,
      [endpoint_url, secret, event_types, id, req.userId],
    );
    if (rows.length === 0) return res.status(404).json({ error: 'subscription not found' });
    return res.json(rows[0]);
  } catch (err) {
    logger.error({ err }, 'UpdateSubscription error');
    return res.status(500).json({ error: 'internal server error' });
  }
});

// ── GET /api/subscriptions ───────────────────────────────────────────────────
router.get('/', async (req, res) => {
  try {
    const { rows } = await pool.query(
      `SELECT id, endpoint_url, secret, event_types, is_active, created_at
       FROM subscriptions
       WHERE is_active = true AND user_id = $1
       ORDER BY created_at DESC`,
      [req.userId],
    );
    return res.json({ subscriptions: rows });
  } catch (err) {
    logger.error({ err }, 'ListSubscriptions error');
    return res.status(500).json({ error: 'internal server error' });
  }
});

module.exports = router;
