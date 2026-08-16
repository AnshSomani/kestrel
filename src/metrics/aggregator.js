const client = require('prom-client');

// ─── Prometheus metrics ───────────────────────────────────────────────────────
const register = new client.Registry();
client.collectDefaultMetrics({ register });

const eventsIngested = new client.Counter({
  name: 'kestrel_events_ingested_total',
  help: 'Total events accepted by the API',
  registers: [register],
});

const deliveriesTotal = new client.Counter({
  name: 'kestrel_deliveries_total',
  help: 'Webhook delivery outcomes',
  labelNames: ['status'],
  registers: [register],
});

const deliveryDuration = new client.Histogram({
  name: 'kestrel_delivery_duration_seconds',
  help: 'Webhook POST latency in seconds',
  buckets: [0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10],
  registers: [register],
});

const activeDeliveries = new client.Gauge({
  name: 'kestrel_active_deliveries',
  help: 'Number of in-flight webhook deliveries',
  registers: [register],
});

const queueDepth = new client.Gauge({
  name: 'kestrel_queue_depth',
  help: 'Approximate number of pending delivery jobs',
  registers: [register],
});

const circuitBreaksTotal = new client.Counter({
  name: 'kestrel_circuit_breaks_total',
  help: 'Number of circuit breaker trips',
  registers: [register],
});

// ─── In-memory stat aggregator ────────────────────────────────────────────────
// Collects (tenantId, key, delta) updates from API handlers and worker threads,
// then batch-flushes to system_stats in PostgreSQL every second.
// This replaces the Go channel-based DBStatsFlusher and eliminates the Postgres
// trigger bottleneck removed in migration 006.

let _pool = null;
const _buffer = []; // { tenantId, key, delta }

/**
 * Initialise the aggregator with a pg.Pool. Call once at startup.
 */
function init(pool) {
  _pool = pool;
  setInterval(_flush, 1000).unref();
}

/**
 * Track a stat delta. Thread-safe for the main thread.
 * Worker threads call this via parentPort messages handled in index.js.
 */
function track(tenantId, key, delta) {
  _buffer.push({ tenantId, key, delta });
}

async function _flush() {
  if (!_pool || _buffer.length === 0) return;
  const batch = _buffer.splice(0, _buffer.length);

  // Group by (tenantId, key) and sum deltas
  const acc = new Map();
  for (const { tenantId, key, delta } of batch) {
    const mapKey = `${tenantId}::${key}`;
    acc.set(mapKey, { tenantId, key, total: (acc.get(mapKey)?.total ?? 0) + delta });
  }

  const client = await _pool.connect();
  try {
    await client.query('BEGIN');
    for (const { tenantId, key, total } of acc.values()) {
      await client.query(
        `INSERT INTO system_stats (user_id, key, value)
         VALUES ($1, $2, $3)
         ON CONFLICT (user_id, key)
         DO UPDATE SET value = system_stats.value + EXCLUDED.value`,
        [tenantId, key, total],
      );
    }
    await client.query('COMMIT');
  } catch (err) {
    await client.query('ROLLBACK');
  } finally {
    client.release();
  }
}

module.exports = {
  init,
  track,
  register,
  metrics: {
    eventsIngested,
    deliveriesTotal,
    deliveryDuration,
    activeDeliveries,
    queueDepth,
    circuitBreaksTotal,
  },
};
