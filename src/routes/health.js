const express = require('express');
const { pool } = require('../db');
const aggregator = require('../metrics/aggregator');

const router = express.Router();

// GET /health
router.get('/', async (_req, res) => {
  let postgresStatus = 'up';
  try {
    await pool.query('SELECT 1');
  } catch {
    postgresStatus = 'down';
  }

  // Read total pending from system_stats (avoid COUNT(*) over full table)
  let queueDepth = 0;
  try {
    const { rows } = await pool.query(
      `SELECT COALESCE(SUM(value), 0) AS total
       FROM system_stats WHERE key = 'delivery_pending'`,
    );
    queueDepth = parseInt(rows[0]?.total ?? '0');
  } catch {
    queueDepth = -1;
  }

  const status = postgresStatus === 'up' ? 'ok' : 'degraded';
  return res.status(postgresStatus === 'up' ? 200 : 503).json({
    status,
    postgres: postgresStatus,
    queue_depth: queueDepth,
  });
});

// GET /metrics — Prometheus text format
router.get('/metrics-endpoint', async (_req, res) => {
  res.set('Content-Type', aggregator.register.contentType);
  res.send(await aggregator.register.metrics());
});

module.exports = router;
