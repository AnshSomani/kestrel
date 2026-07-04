package metrics

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// StatUpdate represents a single delta to apply to a system stat.
type StatUpdate struct {
	UserID uuid.UUID
	Key    string
	Delta  int64
}

// DBStatsFlusher aggregates in-memory metric updates and flushes them to the
// PostgreSQL system_stats table in bulk to avoid row contention.
type DBStatsFlusher struct {
	pool    *pgxpool.Pool
	logger  *slog.Logger
	updates chan StatUpdate

	mu     sync.Mutex
	buffer map[uuid.UUID]map[string]int64
}

// NewDBStatsFlusher creates a new flusher.
func NewDBStatsFlusher(pool *pgxpool.Pool, logger *slog.Logger) *DBStatsFlusher {
	return &DBStatsFlusher{
		pool:    pool,
		logger:  logger,
		updates: make(chan StatUpdate, 100000), // Buffer for ultra high throughput
		buffer:  make(map[uuid.UUID]map[string]int64),
	}
}

// TrackUpdate enqueues a delta for a user's metric in a non-blocking way.
func (f *DBStatsFlusher) TrackUpdate(userID uuid.UUID, key string, delta int64) {
	select {
	case f.updates <- StatUpdate{UserID: userID, Key: key, Delta: delta}:
	default:
		// If channel is full, drop it (rare unless DB is entirely unreachable for a long time)
		if f.logger != nil {
			f.logger.Warn("db stats flusher channel full, dropping metric update", "user_id", userID, "key", key)
		}
	}
}

// Start runs the ingestion loop and periodic flusher.
func (f *DBStatsFlusher) Start(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				f.flush()
				return
			case u := <-f.updates:
				f.mu.Lock()
				if _, ok := f.buffer[u.UserID]; !ok {
					f.buffer[u.UserID] = make(map[string]int64)
				}
				f.buffer[u.UserID][u.Key] += u.Delta
				f.mu.Unlock()
			case <-ticker.C:
				f.flush()
			}
		}
	}()
}

func (f *DBStatsFlusher) flush() {
	f.mu.Lock()
	if len(f.buffer) == 0 {
		f.mu.Unlock()
		return
	}

	// Swap out the buffer instantly to minimize lock time
	current := f.buffer
	f.buffer = make(map[uuid.UUID]map[string]int64)
	f.mu.Unlock()

	// Use pgx.Batch to execute the updates in a single round-trip
	batch := &pgx.Batch{}

	for userID, keys := range current {
		for key, delta := range keys {
			if delta == 0 {
				continue
			}
			batch.Queue(
				`INSERT INTO system_stats (key, value, user_id) 
				 VALUES ($1, $2, $3)
				 ON CONFLICT (key, user_id) 
				 DO UPDATE SET value = system_stats.value + $2`,
				key, delta, userID,
			)
		}
	}

	if batch.Len() == 0 {
		return
	}

	br := f.pool.SendBatch(context.Background(), batch)
	defer br.Close()

	for i := 0; i < batch.Len(); i++ {
		_, err := br.Exec()
		if err != nil && f.logger != nil {
			f.logger.Error("failed to flush stat update", "error", err)
		}
	}
}
