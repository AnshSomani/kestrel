/**
 * Worker thread poller — runs inside each worker_threads Worker.
 *
 * Receives workerData: { threadIndex, config }
 * Creates its own pg.Pool and ioredis connection (threads don't share V8 heap).
 * Staggered startup prevents all pollers hitting the DB simultaneously.
 * Uses a simple in-thread semaphore to cap concurrent in-flight deliveries.
 */

const { workerData, parentPort } = require('worker_threads');
const Redis = require('ioredis');

const PostgresQueue = require('../queue/postgresQueue');
const CircuitBreaker = require('./circuitBreaker');
const { RateLimiter } = require('../middleware/rateLimiter');
const { deliverJob } = require('./delivery');

const { threadIndex, config } = workerData;

// ── Normalise Redis URL (docker-compose passes "redis:6379" without scheme) ──
function normaliseRedisUrl(url) {
  if (!url) return 'redis://localhost:6379';
  if (url.startsWith('redis://') || url.startsWith('rediss://')) return url;
  return `redis://${url}`;
}

// ── Per-thread resources ──────────────────────────────────────────────────────
const queue = new PostgresQueue(config.databaseUrl);

const redis = new Redis(normaliseRedisUrl(config.redisUrl), {
  maxRetriesPerRequest: 3,
  lazyConnect: true,
});
redis.on('error', () => {}); // silence in worker thread

const cb = new CircuitBreaker(redis, config);
const rl = new RateLimiter(redis, config);

// Each thread owns a fair share of the global concurrency budget
const perThreadConcurrency = Math.max(
  1,
  Math.floor(config.maxConcurrent / config.numPollers),
);

// ── Simple semaphore ──────────────────────────────────────────────────────────
let activeCount = 0;
const waiters = [];

function semAcquire() {
  if (activeCount < perThreadConcurrency) {
    activeCount++;
    return Promise.resolve();
  }
  return new Promise((resolve) => waiters.push(resolve));
}

function semRelease() {
  if (waiters.length > 0) {
    const next = waiters.shift();
    next(); // transfer slot directly
  } else {
    activeCount--;
  }
}

// ── Dispatch one job with semaphore ──────────────────────────────────────────
async function dispatch(job) {
  await semAcquire();
  try {
    await deliverJob(job, { cb, rl, queue, config });
  } catch {
    // deliverJob has internal error handling; swallow any unexpected throw
  } finally {
    semRelease();
  }
}

// ── Poll loop ─────────────────────────────────────────────────────────────────
let running = true;

async function poll() {
  while (running) {
    try {
      const jobs = await queue.dequeue(config.dequeueBatch);

      if (jobs.length === 0) {
        await sleep(config.pollIntervalMs);
        continue;
      }

      // Notify main thread of in_flight stat changes
      const byTenant = new Map();
      for (const j of jobs) {
        byTenant.set(j.user_id, (byTenant.get(j.user_id) || 0) + 1);
      }
      for (const [userId, count] of byTenant) {
        parentPort.postMessage({ type: 'stat', userId, key: 'delivery_in_flight', delta: count });
        parentPort.postMessage({ type: 'stat', userId, key: 'delivery_pending', delta: -count });
      }

      // Dispatch all jobs concurrently (bounded by semaphore)
      await Promise.all(jobs.map(dispatch));

    } catch {
      // Brief pause on unexpected poll errors to avoid tight-loop CPU burn
      await sleep(config.pollIntervalMs);
    }
  }
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

// ── Graceful shutdown ─────────────────────────────────────────────────────────
parentPort.on('message', (msg) => {
  if (msg === 'shutdown') running = false;
});

// ── Staggered startup ─────────────────────────────────────────────────────────
// Poller i waits i * (interval / (numPollers + 1)) ms before first poll.
// This prevents all 32 pollers stampeding the DB at the exact same tick.
const startDelay = threadIndex * Math.floor(config.pollIntervalMs / (config.numPollers + 1));
sleep(startDelay).then(poll);
