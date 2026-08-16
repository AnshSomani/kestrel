require('dotenv').config();

module.exports = {
  port: parseInt(process.env.PORT || process.env.SERVER_PORT || '8080'),
  databaseUrl: process.env.DATABASE_URL,
  redisUrl: process.env.REDIS_URL || 'redis://localhost:6379',
  jwtSecret: process.env.JWT_SECRET || 'dev-jwt-secret-change-in-production',

  // Worker pool
  numPollers: parseInt(process.env.NUM_POLLERS || '1'),
  pollIntervalMs: parseInt(process.env.POLL_INTERVAL_MS || '500'),
  dequeueBatch: parseInt(process.env.DEQUEUE_BATCH || '50'),
  maxConcurrent: parseInt(process.env.MAX_CONCURRENT || process.env.WORKER_POOL_SIZE || '100'),

  // Delivery
  deliveryTimeoutMs: parseInt(process.env.DELIVERY_TIMEOUT_MS || '10000'),
  dryRun: process.env.DRY_RUN === 'true',

  // Retry — decorrelated jitter backoff
  retryMaxAttempts: parseInt(process.env.RETRY_MAX_ATTEMPTS || '5'),
  retryBaseDelayMs: parseInt(process.env.RETRY_BASE_DELAY_MS || '1000'),
  retryMaxDelayMs: parseInt(process.env.RETRY_MAX_DELAY_MS || '300000'),

  // Circuit breaker
  cbFailThreshold: parseInt(process.env.CB_FAIL_THRESHOLD || '5'),
  cbWindowMs: parseInt(process.env.CB_WINDOW_MS || '60000'),
  cbResetTimeoutMs: parseInt(process.env.CB_RESET_TIMEOUT_MS || '30000'),

  // Rate limiter (sliding window)
  rateLimitRate: parseInt(process.env.RATE_LIMIT_RATE || '100'),
  rateLimitWindowMs: parseInt(process.env.RATE_LIMIT_WINDOW_MS || '60000'),

  // Admin seed credentials
  adminEmail: process.env.ADMIN_EMAIL || 'admin@kestrel.local',
  adminPassword: process.env.ADMIN_PASSWORD || 'password',
};
