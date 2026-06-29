// Package ratelimit provides a distributed sliding window rate limiter
// backed by Redis. It uses an embedded Lua script for atomic check-and-increment
// operations, ensuring correctness under high concurrency across multiple
// Kestrel worker instances.
//
// The sliding window algorithm (vs. fixed window) prevents burst spikes at
// window boundaries, providing smoother rate limiting for webhook endpoints.
package ratelimit

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"kestrel/internal/metrics"
)

// slidingWindowScript is the Lua script loaded at compile time via go:embed.
// Using EVALSHA (which redis.Script provides automatically) avoids sending
// the full script text on every call — Redis caches it by SHA1 hash.
//
//go:embed lua/sliding_window.lua
var slidingWindowScript string

// SlidingWindowLimiter implements per-key rate limiting using a Redis-backed
// sliding window. Each key (typically a subscription ID) gets its own
// independent rate limit counter.
type SlidingWindowLimiter struct {
	client  *redis.Client
	script  *redis.Script
	metrics *metrics.Metrics
}

// NewSlidingWindowLimiter creates a new limiter. The Lua script is registered
// with Redis lazily on first use (via EVALSHA → EVAL fallback).
func NewSlidingWindowLimiter(client *redis.Client, m *metrics.Metrics) *SlidingWindowLimiter {
	return &SlidingWindowLimiter{
		client:  client,
		script:  redis.NewScript(slidingWindowScript),
		metrics: m,
	}
}

// Allow checks whether a request identified by key is within the rate limit.
//
// Parameters:
//   - key: the rate limit key (e.g., subscription UUID). Prefixed with "rl:" internally.
//   - limit: maximum number of requests allowed within the window.
//   - window: the sliding window duration.
//
// Returns true if the request is allowed, false if rate limited.
// On Redis errors, it fails open (returns true) to avoid blocking deliveries
// when Redis is temporarily unavailable — availability over correctness.
func (l *SlidingWindowLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	nowMs := time.Now().UnixMilli()
	windowMs := window.Milliseconds()

	result, err := l.script.Run(ctx, l.client, []string{"rl:" + key}, limit, windowMs, nowMs).Int()
	if err != nil {
		// Fail open: if Redis is down, allow the request through rather than
		// blocking all webhook deliveries. The circuit breaker provides a
		// secondary safety net for unhealthy endpoints.
		return true, fmt.Errorf("ratelimit: script execution failed: %w", err)
	}

	allowed := result == 1
	if allowed {
		l.metrics.RateLimitTotal.WithLabelValues("allowed").Inc()
	} else {
		l.metrics.RateLimitTotal.WithLabelValues("rejected").Inc()
	}

	return allowed, nil
}
