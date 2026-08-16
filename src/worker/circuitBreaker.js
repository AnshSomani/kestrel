const crypto = require('crypto');

/**
 * Redis-backed per-endpoint circuit breaker.
 * Mirrors the Go implementation in internal/circuitbreaker/breaker.go exactly.
 *
 * States: closed (normal) → open (failures exceeded) → half_open (probe)
 *
 * Redis key layout (keyed by first 16 hex chars of SHA-256(endpointUrl)):
 *   cb:state:{hash}    → Hash  { state, changed_at }
 *   cb:failures:{hash} → Sorted Set (score=epochMs, member=uniqueMs) sliding window
 *   cb:probe:{hash}    → String with TTL (distributed probe lock for half-open)
 */
class CircuitBreaker {
  /**
   * @param {import('ioredis')} redis
   * @param {{ cbFailThreshold: number, cbWindowMs: number, cbResetTimeoutMs: number }} config
   */
  constructor(redis, config) {
    this._redis = redis;
    this._threshold = config.cbFailThreshold;
    this._windowMs = config.cbWindowMs;
    this._resetTimeout = config.cbResetTimeoutMs;
  }

  _hash(endpointUrl) {
    return crypto.createHash('sha256').update(endpointUrl).digest('hex').slice(0, 16);
  }

  _keys(h) {
    return {
      state: `cb:state:${h}`,
      failures: `cb:failures:${h}`,
      probe: `cb:probe:${h}`,
    };
  }

  /**
   * Returns true if the request should be allowed through.
   * Fail-open: if Redis is unreachable, returns true.
   */
  async allow(endpointUrl) {
    try {
      const h = this._hash(endpointUrl);
      const k = this._keys(h);
      const [state, changedAt] = await this._redis.hmget(k.state, 'state', 'changed_at');

      if (!state || state === 'closed') return true;

      if (state === 'open') {
        const elapsed = Date.now() - parseInt(changedAt || '0');
        if (elapsed < this._resetTimeout) return false;
        // Try to acquire the probe lock (only one worker gets to probe)
        const acquired = await this._redis.set(
          k.probe, '1', 'EX', Math.ceil(this._resetTimeout / 1000), 'NX',
        );
        if (!acquired) return false;
        // Transition to half_open
        await this._redis.hset(k.state, 'state', 'half_open', 'changed_at', Date.now());
        return true;
      }

      if (state === 'half_open') {
        const acquired = await this._redis.set(
          k.probe, '1', 'EX', Math.ceil(this._resetTimeout / 1000), 'NX',
        );
        return !!acquired;
      }

      return true;
    } catch {
      return true; // fail-open
    }
  }

  async recordSuccess(endpointUrl) {
    try {
      const h = this._hash(endpointUrl);
      const k = this._keys(h);
      const [state] = await this._redis.hmget(k.state, 'state');
      if (state === 'half_open') {
        const pipeline = this._redis.pipeline();
        pipeline.hset(k.state, 'state', 'closed', 'changed_at', Date.now());
        pipeline.del(k.failures);
        pipeline.del(k.probe);
        await pipeline.exec();
      }
    } catch {
      // ignore
    }
  }

  /**
   * Records a delivery failure.
   * Returns true if this failure caused the circuit to trip open.
   */
  async recordFailure(endpointUrl) {
    try {
      const h = this._hash(endpointUrl);
      const k = this._keys(h);
      const nowMs = Date.now();

      const pipeline = this._redis.pipeline();
      pipeline.zadd(k.failures, nowMs, `${nowMs}:${Math.random()}`);
      pipeline.zremrangebyscore(k.failures, 0, nowMs - this._windowMs);
      pipeline.expire(k.failures, Math.ceil(this._windowMs * 2 / 1000));
      pipeline.zcard(k.failures);
      const results = await pipeline.exec();

      const count = results[3][1]; // zcard result
      const [state] = await this._redis.hmget(k.state, 'state');

      if (state === 'half_open') {
        await this._redis.pipeline()
          .hset(k.state, 'state', 'open', 'changed_at', nowMs)
          .del(k.probe)
          .exec();
        return true;
      }

      if (count >= this._threshold) {
        await this._redis.hset(k.state, 'state', 'open', 'changed_at', nowMs);
        return true;
      }

      return false;
    } catch {
      return false;
    }
  }
}

module.exports = CircuitBreaker;
