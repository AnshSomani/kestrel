const crypto = require('crypto');
const axios = require('axios');
const { parentPort } = require('worker_threads');

/**
 * AWS decorrelated jitter backoff — identical algorithm to internal/retry/backoff.go.
 * sleep = random_between(baseDelay, min(maxDelay, prevDelay * 3))
 */
function nextDelay(base, max, prev) {
  const floor = base;
  const ceil = Math.min(max, prev * 3);
  return Math.floor(Math.random() * (ceil - floor) + floor);
}

/**
 * Build the HMAC-SHA256 signature for an outbound webhook.
 * Formula: sha256=HMAC-SHA256(secret, "<timestamp>.<payloadString>")
 * Identical to Go's Deliver() in internal/worker/delivery.go.
 */
function sign(secret, timestamp, payload) {
  const message = `${timestamp}.${JSON.stringify(payload)}`;
  const mac = crypto.createHmac('sha256', secret).update(message).digest('hex');
  return `sha256=${mac}`;
}

/**
 * Process one delivery job.
 * Steps (mirrors Go worker pool process() exactly):
 *   1. Circuit breaker check → requeue if open
 *   2. Rate limiter check → requeue if rejected
 *   3. HTTP POST delivery with HMAC signing
 *   4. Success → mark delivered
 *   5. Failure → retry or dead-letter
 *
 * @param {object} job        - Hydrated job row from dequeue()
 * @param {object} deps
 * @param {import('../worker/circuitBreaker')} deps.cb
 * @param {import('../middleware/rateLimiter').RateLimiter} deps.rl
 * @param {import('../queue/postgresQueue')} deps.queue
 * @param {object} deps.config
 */
async function deliverJob(job, { cb, rl, queue, config }) {
  const { id: jobId, endpoint_url: endpointUrl, subscription_id: subId,
          event_type: eventType, payload, secret, user_id: userId,
          attempt_count: attemptCount, max_attempts: maxAttempts } = job;

  // ── 1. Circuit breaker ──────────────────────────────────────────────────────
  const cbAllowed = await cb.allow(endpointUrl);
  if (!cbAllowed) {
    const nextAt = new Date(Date.now() + config.retryBaseDelayMs);
    await queue.requeue(jobId, 'circuit breaker open', nextAt);
    _stat(userId, 'circuit_open');
    return;
  }

  // ── 2. Rate limiter ─────────────────────────────────────────────────────────
  const rlAllowed = await rl.allow(subId);
  if (!rlAllowed) {
    const nextAt = new Date(Date.now() + 5000);
    await queue.requeue(jobId, 'rate limited', nextAt);
    _stat(userId, 'rate_limited');
    return;
  }

  // ── 3. HTTP delivery ─────────────────────────────────────────────────────────
  if (config.dryRun) {
    await queue.markDelivered(jobId);
    await cb.recordSuccess(endpointUrl);
    _stat(userId, 'delivery_delivered', 1);
    _stat(userId, 'delivery_pending', -1);
    return;
  }

  const timestamp = Math.floor(Date.now() / 1000).toString();
  const signature = sign(secret, timestamp, payload);
  const startMs = Date.now();

  try {
    const response = await axios.post(endpointUrl, payload, {
      timeout: config.deliveryTimeoutMs,
      maxRedirects: 0,
      headers: {
        'Content-Type': 'application/json',
        'User-Agent': 'Kestrel-Webhook/1.0',
        'X-Kestrel-Signature': signature,
        'X-Kestrel-Timestamp': timestamp,
        'X-Kestrel-Event-Type': eventType,
        'X-Kestrel-Delivery-ID': jobId,
      },
      validateStatus: (s) => s >= 200 && s < 300, // throw on non-2xx
    });

    // ── Success ───────────────────────────────────────────────────────────────
    const _ = response; // body already consumed
    await queue.markDelivered(jobId);
    await cb.recordSuccess(endpointUrl);
    _stat(userId, 'delivery_delivered', 1);
    _stat(userId, 'delivery_pending', -1);

  } catch (err) {
    // ── Failure ───────────────────────────────────────────────────────────────
    const tripped = await cb.recordFailure(endpointUrl);
    if (tripped) _stat(userId, 'circuit_break');

    const statusCode = err.response?.status ?? null;
    const errMsg = err.response
      ? `HTTP ${statusCode}: ${String(err.response.data || '').slice(0, 256)}`
      : err.message;

    if (attemptCount < maxAttempts) {
      const prevDelay = config.retryBaseDelayMs;
      const delay = nextDelay(config.retryBaseDelayMs, config.retryMaxDelayMs, prevDelay);
      const nextAt = new Date(Date.now() + delay);
      await queue.markFailed(jobId, errMsg, statusCode, nextAt);
      _stat(userId, 'delivery_failed');
    } else {
      await queue.markDead(jobId, errMsg);
      _stat(userId, 'delivery_dead', 1);
      _stat(userId, 'delivery_pending', -1);
    }
  }
}

/**
 * Send a stat delta to the main thread's aggregator via parentPort.
 * (parentPort is null when delivery.js is used outside a worker thread.)
 */
function _stat(userId, key, delta = 1) {
  if (parentPort) {
    parentPort.postMessage({ type: 'stat', userId, key, delta });
  }
}

module.exports = { deliverJob };
