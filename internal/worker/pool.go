package worker

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"kestrel/internal/circuitbreaker"
	"kestrel/internal/metrics"
	"kestrel/internal/queue"
	"kestrel/internal/ratelimit"
	"kestrel/internal/retry"
)

// PoolConfig controls the worker pool's operational parameters.
type PoolConfig struct {
	Size          int           // number of conceptual workers (used for logging/identification)
	MaxConcurrent int           // semaphore size — hard limit on concurrent HTTP deliveries
	PollInterval  time.Duration // how often to poll the queue for new jobs
	BatchSize     int           // max jobs to dequeue per poll cycle
	NumPollers    int           // number of concurrent goroutines fetching from the queue
}

// RetryConfig defines the exponential backoff parameters for failed deliveries.
type RetryConfig struct {
	MaxAttempts int           // total attempts before dead-lettering (including the first)
	BaseDelay   time.Duration // initial retry delay — doubles with each attempt (with jitter)
	MaxDelay    time.Duration // upper bound on retry delay to prevent unbounded waits
}

// RateLimitConfig defines per-subscription rate limiting parameters.
type RateLimitConfig struct {
	Rate   int           // maximum number of deliveries allowed within the window
	Window time.Duration // sliding window duration for rate counting
}

// Pool is the central orchestrator for webhook delivery. It polls the job queue,
// applies rate limiting and circuit breaking, dispatches deliveries concurrently
// (bounded by a semaphore), and handles retry/dead-letter decisions.
//
// Concurrency model:
//   - Multiple poll goroutines dequeue batches of jobs on a ticker
//   - Each job is dispatched to a goroutine, gated by a channel-based semaphore
//   - sync.WaitGroup tracks all in-flight work for graceful shutdown
//
// Panic safety:
//   - Each worker goroutine has a deferred recover() that catches panics,
//     increments the panic metric, and marks the job for retry rather than
//     silently dropping it.
type Pool struct {
	queue        *queue.PostgresQueue
	deliverer    *Deliverer
	cb           *circuitbreaker.CircuitBreaker
	rl           *ratelimit.SlidingWindowLimiter
	sem          chan struct{} // buffered channel used as a counting semaphore
	wg           sync.WaitGroup
	metrics      *metrics.Metrics
	logger       *slog.Logger
	retryCfg     RetryConfig
	rlCfg        RateLimitConfig
	pollInterval time.Duration
	batchSize    int
	numPollers   int
}

// NewPool creates a worker pool. It does not start processing — call Start()
// to begin polling and delivering.
func NewPool(
	cfg PoolConfig,
	q *queue.PostgresQueue,
	d *Deliverer,
	cb *circuitbreaker.CircuitBreaker,
	rl *ratelimit.SlidingWindowLimiter,
	retryCfg RetryConfig,
	rlCfg RateLimitConfig,
	m *metrics.Metrics,
	logger *slog.Logger,
) *Pool {
	return &Pool{
		queue:        q,
		deliverer:    d,
		cb:           cb,
		rl:           rl,
		sem:          make(chan struct{}, cfg.MaxConcurrent),
		metrics:      m,
		logger:       logger,
		retryCfg:     retryCfg,
		rlCfg:        rlCfg,
		pollInterval: cfg.PollInterval,
		batchSize:    cfg.BatchSize,
		numPollers:   cfg.NumPollers,
	}
}

// Start begins the polling loop in background goroutines. It returns
// immediately. Use Shutdown() to stop the pool and drain in-flight deliveries.
func (p *Pool) Start(ctx context.Context) {
	numPollers := p.numPollers
	if numPollers < 1 {
		numPollers = 1
	}

	p.logger.Info("worker pool starting",
		"max_concurrent", cap(p.sem),
		"poll_interval", p.pollInterval,
		"batch_size", p.batchSize,
		"num_pollers", numPollers,
	)

	for i := 0; i < numPollers; i++ {
		p.wg.Add(1)
		go p.poll(ctx, i)
	}
}

// poll is the main loop that periodically dequeues jobs from Postgres and
// dispatches them for delivery. It runs until the context is cancelled.
func (p *Pool) poll(ctx context.Context, id int) {
	defer p.wg.Done()

	// Add slight staggered startup so multiple pollers don't stampede DB initially
	// Jitter up to 1 interval
	time.Sleep(time.Duration(id) * (p.pollInterval / time.Duration(p.numPollers+1)))

	ticker := time.NewTicker(p.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("poller stopping — context cancelled", "poller_id", id)
			return
		case <-ticker.C:
			jobs, err := p.queue.Dequeue(ctx, p.batchSize)
			if err != nil {
				// If the context was cancelled while we were dequeuing, exit
				// cleanly rather than logging a spurious error.
				if ctx.Err() != nil {
					return
				}
				p.logger.Error("dequeue failed", "error", err)
				continue
			}
			for _, job := range jobs {
				p.dispatch(ctx, job)
			}
		}
	}
}

// dispatch sends a job to a worker goroutine, blocking if the concurrency
// semaphore is full. This provides natural backpressure — if all workers are
// busy, the poller waits rather than accumulating unbounded goroutines.
func (p *Pool) dispatch(ctx context.Context, job *queue.Job) {
	select {
	case p.sem <- struct{}{}: // acquire semaphore slot
		p.metrics.ActiveDeliveries.Inc()
		p.wg.Add(1)
		go func() {
			// IMPORTANT: The panic recovery defer MUST come first (i.e., be
			// deferred last so it runs first), before the semaphore release
			// and wg.Done. This ensures that a panic in process() is caught
			// before we signal completion to the WaitGroup.
			defer func() {
				if r := recover(); r != nil {
					p.metrics.PanicsTotal.Inc()
					p.logger.Error("worker panic recovered",
						"job_id", job.ID,
						"panic", r,
					)
					// Mark the job for retry rather than silently losing it.
					// Use context.Background() because the parent ctx may be
					// cancelled (which could be what triggered shutdown).
					_ = p.queue.MarkFailed(
						context.Background(),
						job.ID,
						fmt.Sprintf("panic: %v", r),
						nil,
						time.Now().Add(p.retryCfg.BaseDelay),
					)
				}
				<-p.sem // release semaphore slot
				p.metrics.ActiveDeliveries.Dec()
				p.wg.Done()
			}()
			p.process(ctx, job)
		}()
	case <-ctx.Done():
		// Context cancelled while waiting for a semaphore slot — don't start
		// new work. The job will be picked up on next dequeue since we haven't
		// marked it as in-progress.
		return
	}
}

// process handles the full lifecycle of a single delivery attempt:
//  1. Rate limit check (per-subscription)
//  2. Circuit breaker check (per-endpoint)
//  3. HTTP delivery
//  4. Success → mark delivered, record CB success
//  5. Failure → record CB failure, decide retry vs dead-letter
func (p *Pool) process(ctx context.Context, job *queue.Job) {
	logger := p.logger.With(
		"job_id", job.ID,
		"event_type", job.EventType,
		"endpoint", job.EndpointURL,
		"attempt", job.AttemptCount,
	)

	// --- Step 1: Per-endpoint circuit breaker ---
	// Prevents hammering endpoints that are down, giving them time to recover
	// and protecting Kestrel from wasting resources on doomed deliveries.
	cbAllowed, err := p.cb.Allow(ctx, job.EndpointURL)
	if err != nil {
		logger.Error("circuit breaker check failed", "error", err)
		// On error, fail open — attempt delivery anyway.
	}
	if !cbAllowed {
		logger.Warn("circuit open, re-queuing")
		p.metrics.DeliveriesTotal.WithLabelValues("circuit_open").Inc()
		_ = p.queue.Requeue(ctx, job.ID, "circuit breaker open", time.Now().Add(p.retryCfg.BaseDelay))
		return
	}

	// --- Step 2: Per-subscription rate limiting ---
	// This prevents a single high-volume subscription from monopolizing
	// delivery capacity and starving other subscriptions.
	// Checked AFTER the circuit breaker so that jobs failing the CB do not
	// exhaust the rate limiter tokens.
	allowed, err := p.rl.Allow(ctx, job.SubscriptionID.String(), p.rlCfg.Rate, p.rlCfg.Window)
	if err != nil {
		logger.Error("rate limit check failed", "error", err)
		// On error, the limiter fails open — we proceed with delivery.
	}
	if !allowed {
		logger.Warn("rate limited, re-queuing")
		p.metrics.DeliveriesTotal.WithLabelValues("rate_limited").Inc()
		// Re-queue with a short delay to avoid tight retry loops.
		_ = p.queue.Requeue(ctx, job.ID, "rate limited", time.Now().Add(5*time.Second))
		return
	}

	// --- Step 3: HTTP delivery ---
	result := p.deliverer.Deliver(ctx, job)
	p.metrics.DeliveryDuration.Observe(result.Duration.Seconds())

	if result.Success {
		logger.Info("delivered",
			"status", result.StatusCode,
			"duration", result.Duration,
		)
		// Record success with the circuit breaker — if we were in HalfOpen,
		// this transitions back to Closed.
		_ = p.cb.RecordSuccess(ctx, job.EndpointURL)
		p.metrics.DeliveriesTotal.WithLabelValues("delivered").Inc()
		_ = p.queue.MarkDelivered(ctx, job.ID)
		return
	}

	// --- Step 4: Handle failure ---
	logger.Warn("delivery failed",
		"error", result.Error,
		"status", result.StatusCode,
		"duration", result.Duration,
	)

	// Feed the failure to the circuit breaker regardless of retry decision.
	_ = p.cb.RecordFailure(ctx, job.EndpointURL)

	// Normalize zero status codes (connection errors, timeouts) to nil
	// so they're stored as NULL in Postgres rather than a misleading "0".
	var statusCode *int
	if result.StatusCode != 0 {
		statusCode = &result.StatusCode
	}

	// --- Step 5: Retry or dead-letter ---
	if retry.ShouldRetry(job.AttemptCount, job.MaxAttempts) {
		delay := retry.NextDelay(
			job.AttemptCount,
			p.retryCfg.BaseDelay,
			p.retryCfg.BaseDelay,
			p.retryCfg.MaxDelay,
		)
		p.metrics.DeliveriesTotal.WithLabelValues("failed").Inc()
		p.metrics.RetryTotal.WithLabelValues(strconv.Itoa(job.AttemptCount)).Inc()
		logger.Info("scheduling retry",
			"next_attempt", time.Now().Add(delay),
			"delay", delay,
		)
		_ = p.queue.MarkFailed(ctx, job.ID, result.Error, statusCode, time.Now().Add(delay))
	} else {
		// All retries exhausted — move to the dead letter queue so the job
		// is preserved for manual inspection but no longer retried.
		p.metrics.DeliveriesTotal.WithLabelValues("dead").Inc()
		logger.Error("max retries exhausted, sending to DLQ",
			"attempts", job.AttemptCount,
		)
		_ = p.queue.MarkDead(ctx, job.ID, result.Error)
	}
}

// Shutdown gracefully stops the worker pool. It waits for all in-flight
// deliveries to complete, respecting the provided context's deadline.
//
// Call pattern:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
//	defer cancel()
//	if err := pool.Shutdown(ctx); err != nil { ... }
func (p *Pool) Shutdown(ctx context.Context) error {
	p.logger.Info("worker pool shutting down — draining in-flight deliveries")

	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		p.logger.Info("all workers drained")
		return nil
	case <-ctx.Done():
		p.logger.Error("shutdown deadline exceeded — some deliveries may be lost")
		return ctx.Err()
	}
}
