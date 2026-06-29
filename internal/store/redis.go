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
// string (e.g. "localhost:6379").
func NewRedisStore(redisURL string) (*RedisStore, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         redisURL,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     20,
		MinIdleConns: 5,
	})

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
