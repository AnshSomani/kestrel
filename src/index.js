/**
 * Kestrel — High-Throughput Webhook Delivery Engine
 * Node.js / Express entry point.
 * Replaces cmd/server/main.go.
 */

require('dotenv').config();

const express = require('express');
const cors = require('cors');
const helmet = require('helmet');
const cookieParser = require('cookie-parser');
const path = require('path');
const { Worker } = require('worker_threads');
const bcrypt = require('bcryptjs');
const crypto = require('crypto');

const logger = require('./logger');
const config = require('./config');
const { pool, runMigrations } = require('./db');
const redis = require('./redis');
const aggregator = require('./metrics/aggregator');
const cleanup = require('./worker/cleanup');

const authRoutes = require('./routes/auth');
const eventsRoutes = require('./routes/events');
const subscriptionsRoutes = require('./routes/subscriptions');
const dashboardRoutes = require('./routes/dashboard');
const healthRoutes = require('./routes/health');

async function bootstrap() {
  // ── 1. Migrations ───────────────────────────────────────────────────────────
  logger.info('Running database migrations...');
  await runMigrations();
  logger.info('Migrations complete');

  // ── 2. Redis ────────────────────────────────────────────────────────────────
  await redis.connect();

  // ── 3. Metrics aggregator ───────────────────────────────────────────────────
  aggregator.init(pool);

  // ── 4. Seed admin user (wipe old plain-text accounts, insert with bcrypt) ──
  await pool.query('DELETE FROM users WHERE role = $1', ['admin']);
  const adminHash = await bcrypt.hash(config.adminPassword, 12);
  const { rows: adminRows } = await pool.query(
    `INSERT INTO users (email, password_hash, role)
     VALUES ($1, $2, 'admin')
     ON CONFLICT (email) DO UPDATE SET password_hash = EXCLUDED.password_hash
     RETURNING id`,
    [config.adminEmail, adminHash],
  );
  const adminId = adminRows[0].id;

  // Seed the legacy dev API key for local scripts / docker-compose
  await pool.query(
    `INSERT INTO api_keys (key, user_id)
     VALUES ('kestrel-dev-key', $1)
     ON CONFLICT DO NOTHING`,
    [adminId],
  );
  logger.info({ email: config.adminEmail }, 'Admin user seeded');

  // ── 5. Express app ──────────────────────────────────────────────────────────
  const app = express();

  app.use(helmet({ contentSecurityPolicy: false }));

  app.use(cors({
    origin: (origin, cb) => cb(null, origin || '*'),
    allowedHeaders: ['Origin', 'Content-Type', 'Accept', 'Authorization', 'X-API-Key'],
    methods: ['GET', 'POST', 'PUT', 'DELETE', 'OPTIONS'],
    credentials: true,
  }));

  app.options('*', cors());
  app.use(express.json({ limit: '1mb' }));
  app.use(cookieParser());

  // Request logger (skip /health and /metrics)
  app.use((req, res, next) => {
    if (req.path === '/health' || req.path === '/metrics') return next();
    const start = Date.now();
    res.on('finish', () => {
      const latency = Date.now() - start;
      const log = res.statusCode >= 500 ? 'error'
        : res.statusCode >= 400 ? 'warn' : 'info';
      logger[log]({ method: req.method, path: req.path, status: res.statusCode, latency, ip: req.ip });
    });
    next();
  });

  // ── Routes ──────────────────────────────────────────────────────────────────
  app.use('/health', healthRoutes);
  app.use('/metrics', async (_req, res) => {
    res.set('Content-Type', aggregator.register.contentType);
    res.send(await aggregator.register.metrics());
  });

  app.use('/api/auth', authRoutes);
  app.use('/api/events', eventsRoutes);
  app.use('/api/subscriptions', subscriptionsRoutes);
  app.use('/api', dashboardRoutes);

  // 404 catch-all
  app.use((_req, res) => res.status(404).json({ error: 'not found' }));

  // ── 6. Cleanup worker ───────────────────────────────────────────────────────
  cleanup.start();

  // ── 7. Worker thread pool ───────────────────────────────────────────────────
  const workers = [];
  const pollerPath = path.join(__dirname, 'worker', 'poller.js');

  for (let i = 0; i < config.numPollers; i++) {
    const worker = new Worker(pollerPath, {
      workerData: {
        threadIndex: i,
        config: {
          databaseUrl: config.databaseUrl,
          redisUrl: config.redisUrl,
          numPollers: config.numPollers,
          pollIntervalMs: config.pollIntervalMs,
          dequeueBatch: config.dequeueBatch,
          maxConcurrent: config.maxConcurrent,
          deliveryTimeoutMs: config.deliveryTimeoutMs,
          dryRun: config.dryRun,
          retryMaxAttempts: config.retryMaxAttempts,
          retryBaseDelayMs: config.retryBaseDelayMs,
          retryMaxDelayMs: config.retryMaxDelayMs,
          cbFailThreshold: config.cbFailThreshold,
          cbWindowMs: config.cbWindowMs,
          cbResetTimeoutMs: config.cbResetTimeoutMs,
          rateLimitRate: config.rateLimitRate,
          rateLimitWindowMs: config.rateLimitWindowMs,
        },
      },
    });

    // Receive stat messages from worker threads → forward to main-thread aggregator
    worker.on('message', (msg) => {
      if (msg?.type === 'stat') {
        aggregator.track(msg.userId, msg.key, msg.delta);
      }
    });

    worker.on('error', (err) => {
      logger.error({ err, threadIndex: i }, 'Worker thread error');
    });

    worker.on('exit', (code) => {
      if (code !== 0) {
        logger.error({ code, threadIndex: i }, 'Worker thread exited unexpectedly');
      }
    });

    workers.push(worker);
  }

  logger.info({ count: config.numPollers }, 'Worker threads started');

  // ── 8. HTTP server ──────────────────────────────────────────────────────────
  const server = app.listen(config.port, () => {
    logger.info({ port: config.port }, 'Kestrel HTTP server started');
  });

  // ── 9. Graceful shutdown ────────────────────────────────────────────────────
  async function shutdown(signal) {
    logger.info({ signal }, 'Shutting down...');
    for (const w of workers) w.postMessage('shutdown');
    server.close(() => {
      pool.end();
      redis.quit();
      logger.info('Shutdown complete');
      process.exit(0);
    });
    // Force-exit after 30s
    setTimeout(() => process.exit(1), 30000).unref();
  }

  process.on('SIGTERM', () => shutdown('SIGTERM'));
  process.on('SIGINT', () => shutdown('SIGINT'));
}

bootstrap().catch((err) => {
  logger.error({ err }, 'Fatal startup error');
  process.exit(1);
});
