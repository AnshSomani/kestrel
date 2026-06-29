// Package retry implements the decorrelated jitter backoff algorithm
// recommended by AWS for distributed retry scenarios.
//
// Unlike standard exponential backoff, decorrelated jitter uses the previous
// delay (not the attempt number) to compute the next window, then picks a
// random value inside that window. This breaks temporal correlation between
// retries originating from different workers, preventing thundering-herd
// effects when a downstream service recovers.
//
// Reference: https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/
package retry

import (
	"math/rand/v2"
	"time"
)

// NextDelay computes the next retry delay using decorrelated jitter.
//
// Algorithm:
//
//	sleep = random_between(base, min(cap, prev * 3))
//
// On the first attempt (attempt <= 1), prevDelay is ignored and baseDelay is
// used as the starting point. This ensures the first retry always waits at
// least baseDelay.
//
// Parameters:
//   - attempt:    the 1-based attempt number (1 = first retry).
//   - prevDelay:  the delay used for the preceding retry (ignored when attempt <= 1).
//   - baseDelay:  the minimum possible delay.
//   - maxDelay:   the absolute ceiling on delay (the "cap").
//
// Returns a duration in [baseDelay, min(maxDelay, prevDelay*3)).
func NextDelay(attempt int, prevDelay, baseDelay, maxDelay time.Duration) time.Duration {
	if attempt <= 1 {
		prevDelay = baseDelay
	}

	// Upper bound of the jitter window.
	maxNext := min(maxDelay, prevDelay*3)
	if maxNext <= baseDelay {
		return baseDelay
	}

	// Pick a random duration in [baseDelay, maxNext).
	jitter := time.Duration(rand.Int64N(int64(maxNext - baseDelay)))
	return baseDelay + jitter
}

// ShouldRetry reports whether the job should be retried given the current
// attempt count and the configured maximum. attemptCount is the number of
// attempts already made (including the one that just failed).
func ShouldRetry(attemptCount, maxAttempts int) bool {
	return attemptCount < maxAttempts
}
