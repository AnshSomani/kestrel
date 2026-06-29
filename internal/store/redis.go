package store

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisStore wraps a go-redis client and provides lifecycle management.
type RedisStore struct {
	client *redis.Client
}

// NewRedisStore creates a new RedisStore, connects to the given address, and
// verifies reachability with a PING. The redisURL parameter is a host:port
// string (e.g. "localhost:6379") or a full URI (e.g. "rediss://...").
func NewRedisStore(redisURL string) (*RedisStore, error) {
	var opts *redis.Options
	var err error

	if len(redisURL) > 8 && (redisURL[:8] == "redis://" || redisURL[:9] == "rediss://") {
		opts, err = redis.ParseURL(redisURL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse redis URL: %w", err)
		}
	} else {
		opts = &redis.Options{Addr: redisURL}
	}

	opts.DialTimeout = 5 * time.Second
	opts.ReadTimeout = 3 * time.Second
	opts.WriteTimeout = 3 * time.Second
	opts.PoolSize = 20
	opts.MinIdleConns = 5

	client := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("pinging redis at %s: %w", redisURL, err)
	}

	slog.Info("connected to Redis", "addr", redisURL)

	return &RedisStore{client: client}, nil
}

// Close gracefully shuts down the Redis client connection.
func (s *RedisStore) Close() error {
	return s.client.Close()
}

// Client returns the underlying *redis.Client for direct use by other
// packages (e.g. rate limiter, circuit breaker state).
func (s *RedisStore) Client() *redis.Client {
	return s.client
}
