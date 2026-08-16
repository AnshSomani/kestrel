const Redis = require('ioredis');
const logger = require('./logger');
const config = require('./config');

// Normalise the Redis URL — docker-compose passes "redis:6379" without scheme
function normaliseRedisUrl(url) {
  if (!url) return 'redis://localhost:6379';
  if (url.startsWith('redis://') || url.startsWith('rediss://')) return url;
  return `redis://${url}`;
}

const redis = new Redis(normaliseRedisUrl(config.redisUrl), {
  maxRetriesPerRequest: 3,
  enableReadyCheck: true,
  lazyConnect: true,
});

redis.on('error', (err) => {
  logger.error({ err }, 'Redis connection error');
});

redis.on('connect', () => {
  logger.info('Redis connected');
});

module.exports = redis;
