// Package config provides centralized configuration for the Kestrel webhook
// delivery engine. All values are loaded from environment variables with
// sensible development defaults.
package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds all runtime configuration for the Kestrel service.
// Each field maps to an environment variable; if the variable is unset
// the documented default is used.
type Config struct {
	// ServerPort is the TCP port the HTTP server listens on.
	// Env: SERVER_PORT — Default: "8080"
	ServerPort string

	// DatabaseURL is the PostgreSQL connection string.
	// Env: DATABASE_URL — Default: "postgres://kestrel:kestrel@localhost:5432/kestrel?sslmode=disable"
	DatabaseURL string

	// RedisURL is the address of the Redis instance (host:port).
	// Env: REDIS_URL — Default: "localhost:6379"
	RedisURL string

	// APIKey is a shared secret used to authenticate inbound API requests.
	// Env: API_KEY — Default: "kestrel-dev-key"
	APIKey string

	// JWTSecret is the symmetric key used for signing dashboard authentication tokens.
	// Env: JWT_SECRET — Default: "kestrel-jwt-secret-dev"
	JWTSecret string

	// WorkerPoolSize is the number of worker goroutines that process deliveries.
	// Env: WORKER_POOL_SIZE — Default: 8
	WorkerPoolSize int

	// MaxConcurrent caps the total number of in-flight HTTP deliveries.
	// Env: MAX_CONCURRENT — Default: 100
	MaxConcurrent int

	// PollInterval is how often the poller queries for new pending jobs.
	// Env: POLL_INTERVAL_MS (milliseconds) — Default: 500ms
	PollInterval time.Duration

	// DequeueBatch is the max number of jobs claimed per dequeue call.
	// Env: DEQUEUE_BATCH — Default: 50
	DequeueBatch int

	// CBFailThreshold is the number of consecutive failures before the
	// circuit breaker trips for an endpoint.
	// Env: CB_FAIL_THRESHOLD — Default: 5
	CBFailThreshold int

	// CBWindowSize is the rolling time window the circuit breaker observes.
	// Env: CB_WINDOW_MS (milliseconds) — Default: 60s
	CBWindowSize time.Duration

	// CBResetTimeout is how long a tripped breaker stays open before
	// transitioning to half-open.
	// Env: CB_RESET_TIMEOUT_MS (milliseconds) — Default: 30s
	CBResetTimeout time.Duration

	// RetryMaxAttempts is the ceiling on delivery attempts per job.
	// Env: RETRY_MAX_ATTEMPTS — Default: 5
	RetryMaxAttempts int

	// RetryBaseDelay is the minimum delay between retries.
	// Env: RETRY_BASE_DELAY_MS (milliseconds) — Default: 1s
	RetryBaseDelay time.Duration

	// RetryMaxDelay caps the maximum delay between retries.
	// Env: RETRY_MAX_DELAY_MS (milliseconds) — Default: 300s (5 min)
	RetryMaxDelay time.Duration

	// RateLimitRate is the number of allowed requests per window per endpoint.
	// Env: RATE_LIMIT_RATE — Default: 100
	RateLimitRate int

	// RateLimitWindow is the sliding window for rate limiting.
	// Env: RATE_LIMIT_WINDOW_MS (milliseconds) — Default: 60s
	RateLimitWindow time.Duration

	// DeliveryTimeout is the per-request HTTP timeout when posting to a
	// subscriber endpoint.
	// Env: DELIVERY_TIMEOUT_MS (milliseconds) — Default: 10s
	DeliveryTimeout time.Duration

	// NumPollers specifies the number of concurrent goroutines fetching from the queue.
	// Env: NUM_POLLERS — Default: 1
	NumPollers int

	// DryRun skips actual HTTP delivery and immediately returns success for benchmarking.
	// Env: DRY_RUN — Default: false
	DryRun bool
}

// Load reads configuration from the environment and returns a populated
// Config. Missing variables fall back to development-friendly defaults.
func Load() *Config {
	return &Config{
		ServerPort:       getEnv("SERVER_PORT", "8080"),
		DatabaseURL:      getEnv("DATABASE_URL", "postgres://kestrel:kestrel@localhost:5432/kestrel?sslmode=disable"),
		RedisURL:         getEnv("REDIS_URL", "localhost:6379"),
		APIKey:           getEnv("API_KEY", "kestrel-dev-key"),
		JWTSecret:        getEnv("JWT_SECRET", "kestrel-jwt-secret-dev"),
		WorkerPoolSize:   getEnvInt("WORKER_POOL_SIZE", 8),
		MaxConcurrent:    getEnvInt("MAX_CONCURRENT", 100),
		PollInterval:     getEnvDuration("POLL_INTERVAL_MS", 500),
		DequeueBatch:     getEnvInt("DEQUEUE_BATCH", 50),
		CBFailThreshold:  getEnvInt("CB_FAIL_THRESHOLD", 5),
		CBWindowSize:     getEnvDuration("CB_WINDOW_MS", 60_000),
		CBResetTimeout:   getEnvDuration("CB_RESET_TIMEOUT_MS", 30_000),
		RetryMaxAttempts: getEnvInt("RETRY_MAX_ATTEMPTS", 5),
		RetryBaseDelay:   getEnvDuration("RETRY_BASE_DELAY_MS", 1_000),
		RetryMaxDelay:    getEnvDuration("RETRY_MAX_DELAY_MS", 300_000),
		RateLimitRate:    getEnvInt("RATE_LIMIT_RATE", 100),
		RateLimitWindow:  getEnvDuration("RATE_LIMIT_WINDOW_MS", 60_000),
		DeliveryTimeout:  getEnvDuration("DELIVERY_TIMEOUT_MS", 10_000),
		NumPollers:       getEnvInt("NUM_POLLERS", 1),
		DryRun:           os.Getenv("DRY_RUN") == "true",
	}
}

// getEnv returns the value of the named environment variable or fallback if
// the variable is empty or unset.
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// getEnvInt returns the value of the named environment variable parsed as an
// integer, or fallback on error / absence.
func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

// getEnvDuration reads the named environment variable as an integer number of
// milliseconds and converts it to a time.Duration. Returns
// time.Duration(fallbackMS) * time.Millisecond if the variable is absent or
// unparseable.
func getEnvDuration(key string, fallbackMS int64) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return time.Duration(fallbackMS) * time.Millisecond
	}
	ms, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return time.Duration(fallbackMS) * time.Millisecond
	}
	return time.Duration(ms) * time.Millisecond
}
