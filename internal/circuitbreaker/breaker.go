// Package circuitbreaker implements a per-endpoint circuit breaker pattern
// with state stored in Redis. This allows the circuit breaker to be shared
// across multiple Kestrel worker instances, preventing thundering herd
// problems when an endpoint goes down.
//
// State machine: Closed → Open → HalfOpen → Closed (on success) or Open (on failure)
//
// Redis key design:
//   - cb:state:{hash}    → Hash with fields "state" and "changed_at" (unix timestamp)
//   - cb:failures:{hash} → Sorted Set for sliding window failure counting
//   - cb:probe:{hash}    → String with TTL used as a distributed lock for half-open probing
package circuitbreaker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"kestrel/internal/metrics"
)

// State represents the circuit breaker state.
type State string

const (
	// Closed means the circuit is healthy — requests flow through normally.
	Closed State = "closed"
	// Open means the circuit has tripped — requests are blocked to allow recovery.
	Open State = "open"
	// HalfOpen means the circuit is testing recovery — a single probe request is allowed.
	HalfOpen State = "half_open"
)

// Redis key prefixes for circuit breaker data.
const (
	stateKeyPrefix   = "cb:state:"
	failureKeyPrefix = "cb:failures:"
	probeKeyPrefix   = "cb:probe:"
)

// CircuitBreaker implements a per-endpoint circuit breaker backed by Redis.
// It uses a sliding window of failures (stored as a sorted set) to decide
// when to trip, and a Redis hash to persist the current state across instances.
type CircuitBreaker struct {
	client       *redis.Client
	threshold    int           // number of failures within the window before the circuit opens
	window       time.Duration // sliding window duration for counting failures
	resetTimeout time.Duration // how long to stay open before transitioning to half-open
	metrics      *metrics.Metrics
}

// New creates a new CircuitBreaker.
//
// Parameters:
//   - threshold: number of failures within the window that triggers the circuit to open
//   - window: the sliding window duration for counting failures
//   - resetTimeout: duration the circuit stays open before allowing a half-open probe
func New(client *redis.Client, threshold int, window, resetTimeout time.Duration, m *metrics.Metrics) *CircuitBreaker {
	return &CircuitBreaker{
		client:       client,
		threshold:    threshold,
		window:       window,
		resetTimeout: resetTimeout,
		metrics:      m,
	}
}

// hashEndpoint produces a deterministic, short hash of the endpoint URL.
// We use the first 16 hex characters of the SHA-256 digest to keep Redis
// keys compact while still providing collision resistance for practical use.
func hashEndpoint(endpoint string) string {
	h := sha256.Sum256([]byte(endpoint))
	return hex.EncodeToString(h[:])[:16]
}

// stateKey returns the Redis key for the circuit breaker state hash.
func stateKey(hash string) string { return stateKeyPrefix + hash }

// failureKey returns the Redis key for the sliding window failure sorted set.
func failureKey(hash string) string { return failureKeyPrefix + hash }

// probeKey returns the Redis key for the half-open probe lock.
func probeKey(hash string) string { return probeKeyPrefix + hash }

// Allow checks whether a request to the given endpoint should be permitted.
//
// Decision logic:
//  1. Closed (or no state) → always allow
//  2. Open → check if resetTimeout has elapsed; if so, transition to HalfOpen
//     and attempt to acquire a probe lock so only one request tests the endpoint
//  3. HalfOpen → only allow if this caller acquires the probe lock
func (cb *CircuitBreaker) Allow(ctx context.Context, endpoint string) (bool, error) {
	hash := hashEndpoint(endpoint)
	sk := stateKey(hash)

	// Fetch current state and changed_at in a single pipeline round-trip.
	pipe := cb.client.Pipeline()
	stateCmd := pipe.HGet(ctx, sk, "state")
	changedAtCmd := pipe.HGet(ctx, sk, "changed_at")
	_, err := pipe.Exec(ctx)

	// If the key doesn't exist, the circuit is implicitly Closed.
	if err != nil && err != redis.Nil {
		// On individual field misses HGet returns redis.Nil but pipeline Exec
		// may return it too — we treat missing keys as Closed below.
		if !isAllNil(err) {
			return true, fmt.Errorf("circuitbreaker: allow pipeline: %w", err)
		}
	}

	currentState := State(stateCmd.Val())
	if currentState == "" {
		// No state stored — treat as Closed.
		return true, nil
	}

	switch currentState {
	case Closed:
		return true, nil

	case Open:
		// Check whether enough time has passed to transition to HalfOpen.
		changedAt, parseErr := strconv.ParseInt(changedAtCmd.Val(), 10, 64)
		if parseErr != nil {
			// If we can't parse the timestamp, fail open to avoid blocking traffic.
			return true, fmt.Errorf("circuitbreaker: parse changed_at: %w", parseErr)
		}
		elapsed := time.Since(time.Unix(changedAt, 0))
		if elapsed < cb.resetTimeout {
			// Still within the cooldown period — block the request.
			return false, nil
		}

		// Transition to HalfOpen and try to become the single probe request.
		if err := cb.transitionState(ctx, hash, HalfOpen); err != nil {
			return true, err
		}
		return cb.acquireProbe(ctx, hash)

	case HalfOpen:
		// Only one probe at a time — try to acquire the lock.
		return cb.acquireProbe(ctx, hash)

	default:
		// Unknown state — fail open.
		return true, nil
	}
}

// RecordSuccess records a successful delivery to the endpoint.
// In HalfOpen state this transitions the circuit back to Closed and clears
// all failure history, signaling that the endpoint has recovered.
func (cb *CircuitBreaker) RecordSuccess(ctx context.Context, endpoint string) error {
	hash := hashEndpoint(endpoint)

	state, err := cb.GetState(ctx, endpoint)
	if err != nil {
		return err
	}

	if state == HalfOpen {
		// The probe succeeded — endpoint is healthy again. Use a pipeline to
		// atomically reset everything in a single round-trip.
		pipe := cb.client.Pipeline()
		pipe.HSet(ctx, stateKey(hash), "state", string(Closed), "changed_at", time.Now().Unix())
		pipe.Del(ctx, failureKey(hash))
		pipe.Del(ctx, probeKey(hash))
		_, err = pipe.Exec(ctx)
		if err != nil {
			return fmt.Errorf("circuitbreaker: record success pipeline: %w", err)
		}
	}
	// In Closed state a success is a no-op — we don't need to track successes.
	return nil
}

// RecordFailure records a failed delivery attempt to the endpoint.
//
// It maintains a sliding window of failure timestamps in a Redis sorted set,
// and transitions the circuit to Open when failures exceed the threshold.
// In HalfOpen state, any failure immediately re-opens the circuit.
func (cb *CircuitBreaker) RecordFailure(ctx context.Context, endpoint string) error {
	hash := hashEndpoint(endpoint)
	nowMs := time.Now().UnixMilli()
	windowStartMs := nowMs - cb.window.Milliseconds()

	// Pipeline: add failure, prune old entries, count current failures.
	// This minimizes round-trips while keeping the sliding window accurate.
	pipe := cb.client.Pipeline()

	// Add the current failure timestamp as both score and member.
	fk := failureKey(hash)
	pipe.ZAdd(ctx, fk, redis.Z{Score: float64(nowMs), Member: nowMs})

	// Remove failures outside the sliding window.
	pipe.ZRemRangeByScore(ctx, fk, "0", strconv.FormatInt(windowStartMs, 10))

	// Set TTL on the failures key so it auto-expires if the endpoint goes quiet.
	// We use 2× the window to give a comfortable margin.
	pipe.Expire(ctx, fk, cb.window*2)

	// Count remaining failures in the window.
	countCmd := pipe.ZCard(ctx, fk)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("circuitbreaker: record failure pipeline: %w", err)
	}

	failureCount := countCmd.Val()

	// Check current state to decide the transition.
	state, err := cb.GetState(ctx, endpoint)
	if err != nil {
		return err
	}

	switch {
	case state == HalfOpen:
		// The probe request failed — immediately re-open the circuit.
		if err := cb.transitionState(ctx, hash, Open); err != nil {
			return err
		}
		// Clean up the probe lock since we're going back to Open.
		cb.client.Del(ctx, probeKey(hash))

	case int(failureCount) >= cb.threshold:
		// Failure threshold exceeded in the sliding window — trip the circuit.
		if err := cb.transitionState(ctx, hash, Open); err != nil {
			return err
		}
		cb.metrics.CircuitBreaks.Inc()
	}

	return nil
}

// GetState reads the current circuit breaker state from Redis.
// Returns Closed if the key doesn't exist (i.e., no state has been recorded yet).
func (cb *CircuitBreaker) GetState(ctx context.Context, endpoint string) (State, error) {
	hash := hashEndpoint(endpoint)
	val, err := cb.client.HGet(ctx, stateKey(hash), "state").Result()
	if err == redis.Nil {
		return Closed, nil
	}
	if err != nil {
		return Closed, fmt.Errorf("circuitbreaker: get state: %w", err)
	}
	return State(val), nil
}

// transitionState atomically sets the circuit state and records the transition
// timestamp. Both fields are set in a single HSET call to avoid inconsistency.
func (cb *CircuitBreaker) transitionState(ctx context.Context, hash string, newState State) error {
	err := cb.client.HSet(ctx, stateKey(hash),
		"state", string(newState),
		"changed_at", time.Now().Unix(),
	).Err()
	if err != nil {
		return fmt.Errorf("circuitbreaker: transition to %s: %w", newState, err)
	}
	return nil
}

// acquireProbe attempts to acquire the half-open probe lock using SETNX.
// Only one worker across all instances should probe at a time to avoid
// overwhelming a recovering endpoint.
func (cb *CircuitBreaker) acquireProbe(ctx context.Context, hash string) (bool, error) {
	ok, err := cb.client.SetNX(ctx, probeKey(hash), "1", cb.resetTimeout).Result()
	if err != nil {
		return true, fmt.Errorf("circuitbreaker: acquire probe: %w", err)
	}
	return ok, nil
}

// isAllNil checks if a pipeline error is just redis.Nil responses.
// Pipeline Exec returns the first non-nil error, and redis.Nil is returned
// when a key or field doesn't exist — which is expected for new endpoints.
func isAllNil(err error) bool {
	return err == redis.Nil
}
