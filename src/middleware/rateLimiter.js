const Redis = require('ioredis');
const config = require('../config');

// Atomic Lua sliding-window script — identical algorithm to Go's sliding_window.lua.
// Returns 1 = allowed, 0 = rejected.
const SLIDING_WINDOW_LUA = `
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local windowMs = tonumber(ARGV[2])
local nowMs = tonumber(ARGV[3])
redis.call('ZREMRANGEBYSCORE', key, 0, nowMs - windowMs)
local count = redis.call('ZCARD', key)
if count < limit then
  redis.call('ZADD', key, nowMs, nowMs .. ':' .. math.random())
  redis.call('PEXPIRE', key, windowMs)
  return 1
end
return 0
`;

class RateLimiter {
  /**
   * @param {Redis} redis - ioredis client (owned by caller)
   * @param {{ rateLimitRate: number, rateLimitWindowMs: number }} cfg
   */
  constructor(redis, cfg) {
    this._redis = redis;
    this._limit = cfg.rateLimitRate;
    this._windowMs = cfg.rateLimitWindowMs;
  }

  /**
   * Returns true if the request is within the rate limit for this subscription.
   * Fail-open: if Redis is unreachable, the delivery is allowed through.
   */
  async allow(subscriptionId) {
    try {
      const key = `rl:${subscriptionId}`;
      const nowMs = Date.now();
      const result = await this._redis.eval(
        SLIDING_WINDOW_LUA,
        1,         // number of KEYS
        key,       // KEYS[1]
        this._limit,
        this._windowMs,
        nowMs,
      );
      return result === 1;
    } catch {
      return true; // fail-open
    }
  }
}

/**
 * Express middleware factory. Rate-limits by API key (req.userId).
 * This is separate from the per-subscription rate limiter used inside workers.
 */
function makeApiRateLimiter() {
  const redisUrl = config.redisUrl.startsWith('redis')
    ? config.redisUrl
    : `redis://${config.redisUrl}`;
  const redis = new Redis(redisUrl, { maxRetriesPerRequest: 1, lazyConnect: true });
  const rl = new RateLimiter(redis, config);

  return async function rateLimiterMiddleware(req, res, next) {
    if (!req.userId) return next();
    const allowed = await rl.allow(req.userId);
    if (!allowed) {
      return res.status(429).json({ error: 'rate limit exceeded' });
    }
    return next();
  };
}

module.exports = { RateLimiter, makeApiRateLimiter };
