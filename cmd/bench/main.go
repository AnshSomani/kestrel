package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	phase := flag.String("phase", "all", "Which phase to run (correctness, queue, failure, circuit-breaker, million, all)")
	flag.Parse()

	ctx := context.Background()
	
	// Connect to PG
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://kestrel:kestrel@localhost:5432/kestrel?sslmode=disable"
	}
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to PG: %v", err)
	}
	defer pool.Close()

	fmt.Println("🚀 Starting Kestrel Benchmark Suite")
	
	// Ensure a user exists
	var userID string
	err = pool.QueryRow(ctx, "SELECT id FROM users WHERE email='admin@kestrel.local'").Scan(&userID)
	if err != nil {
		log.Fatalf("Admin user not found. Run server first: %v", err)
	}

	// Ensure subscription exists
	var subID string
	err = pool.QueryRow(ctx, `
		INSERT INTO subscriptions (user_id, endpoint_url, secret, event_types, is_active) 
		VALUES ($1, 'http://webhook-target:9999/', 'benchsecret', '{bench.test}', true)
		ON CONFLICT DO NOTHING
		RETURNING id
	`, userID).Scan(&subID)
	
	if err != nil {
		// Just grab it
		pool.QueryRow(ctx, "SELECT id FROM subscriptions WHERE endpoint_url='http://webhook-target:9999/' LIMIT 1").Scan(&subID)
	}

	fmt.Printf("Using Subscription ID: %s\n", subID)

	switch *phase {
	case "correctness":
		runCorrectness(ctx, pool, subID)
	case "queue":
		runQueueBench(ctx, pool, subID)
	case "failure":
		runFailureBench(ctx, pool, subID)
	case "ratelimit":
		runRateLimitBench(ctx, pool, subID)
	case "timeout":
		runTimeoutBench(ctx, pool, subID)
	case "million":
		runMillion(ctx, pool, subID)
	case "chaos":
		runChaosBench(ctx, pool, subID)
	default:
		fmt.Printf("Unknown phase: %s\n", *phase)
	}
}

func runChaosBench(ctx context.Context, pool *pgxpool.Pool, subID string) {
	fmt.Println("\n--- Phase 13: Chaos Engineering & Recovery Simulation ---")
	
	setTargetConfig(0.0, 0) // Target is perfectly reliable, infrastructure is not.
	
	fmt.Println("Seeding 50,000 events...")
	start := time.Now()
	
	pool.Exec(ctx, "ALTER TABLE events DISABLE TRIGGER ALL;")
	pool.Exec(ctx, "ALTER TABLE delivery_jobs DISABLE TRIGGER ALL;")
	insertEvents(ctx, pool, subID, 50000)
	pool.Exec(ctx, "ALTER TABLE events ENABLE TRIGGER ALL;")
	pool.Exec(ctx, "ALTER TABLE delivery_jobs ENABLE TRIGGER ALL;")
	
	pool.Exec(ctx, `
		UPDATE system_stats SET value = (SELECT COUNT(*) FROM events) WHERE key = 'total_events';
		UPDATE system_stats SET value = (SELECT COUNT(*) FROM delivery_jobs WHERE status = 'pending') WHERE key = 'delivery_pending';
	`)
	
	fmt.Printf("Seeded 50k events in %v\n", time.Since(start))
	
	fmt.Println("Waiting for queue to drain under chaotic conditions...")
	start = time.Now()
	waitForDrained(ctx, pool)
	duration := time.Since(start)
	
	stats := getStats(ctx, pool)
	
	fmt.Printf("\n=== CHAOS BENCHMARK RESULTS ===\n")
	fmt.Printf("Time taken:      %v\n", duration)
	fmt.Printf("Delivered:       %d\n", stats["delivery_delivered"])
	fmt.Printf("DLQ:             %d\n", stats["delivery_dead"])
	fmt.Printf("Failed:          %d\n", stats["delivery_failed"])
	fmt.Printf("Pending:         %d\n", stats["delivery_pending"])
	fmt.Println("If Delivered is exactly 50,000, Kestrel successfully recovered from all faults!")
}

func runTimeoutBench(ctx context.Context, pool *pgxpool.Pool, subID string) {
	fmt.Println("\n--- Phase 4: Slow Receiver (Timeout Simulation) ---")
	
	fmt.Println("Configuring target receiver for 100% SLOW rate (3s delay)...")
	setTargetConfig(0.0, 1.0) // 0% fail, 100% slow
	
	fmt.Println("Seeding 200 events...")
	start := time.Now()
	insertEvents(ctx, pool, subID, 200)
	fmt.Printf("Seeded 200 events in %v\n", time.Since(start))
	
	fmt.Println("Waiting 10 seconds for initial burst to be processed/timed-out...")
	time.Sleep(10 * time.Second)
	
	stats := getStats(ctx, pool)
	fmt.Printf("\n=== TIMEOUT BENCHMARK RESULTS ===\n")
	fmt.Printf("Delivered: %d\n", stats["delivery_delivered"])
	fmt.Printf("Pending:   %d\n", stats["delivery_pending"])
	fmt.Printf("In Flight: %d\n", stats["delivery_in_flight"])
	fmt.Printf("Failed/Dead: %d\n", stats["delivery_failed"]+stats["delivery_dead"])
	fmt.Println("Note: If DELIVERY_TIMEOUT_MS < target delay, they should fail and go to DLQ/pending.")
}

func runRateLimitBench(ctx context.Context, pool *pgxpool.Pool, subID string) {
	fmt.Println("\n--- Phase 11: Rate Limit Simulation ---")
	
	// Configure target to succeed always
	fmt.Println("Configuring target receiver for 100% success rate...")
	setTargetConfig(0.0, 0)
	
	fmt.Println("Seeding 500 events...")
	start := time.Now()
	insertEvents(ctx, pool, subID, 500)
	fmt.Printf("Seeded 500 events in %v\n", time.Since(start))
	
	fmt.Println("Waiting 10 seconds for initial burst to be processed/rate-limited...")
	time.Sleep(10 * time.Second)
	
	stats := getStats(ctx, pool)
	fmt.Printf("\n=== RATE LIMIT BENCHMARK RESULTS ===\n")
	fmt.Printf("Delivered: %d\n", stats["delivery_delivered"])
	fmt.Printf("Pending:   %d\n", stats["delivery_pending"])
	fmt.Printf("In Flight: %d\n", stats["delivery_in_flight"])
	fmt.Println("Note: Delivered should equal RATE_LIMIT_RATE, the rest should be Pending (re-queued).")
}

func runFailureBench(ctx context.Context, pool *pgxpool.Pool, subID string) {
	fmt.Println("\n--- Phase 8: Failure Simulation (100% Fail Rate) ---")
	
	// Configure target to fail 100% of the time
	fmt.Println("Configuring target receiver for 100% failure rate...")
	setTargetConfig(1.0, 0)
	
	fmt.Println("Seeding 1,000 events...")
	start := time.Now()
	insertEvents(ctx, pool, subID, 1000)
	fmt.Printf("Seeded 1,000 events in %v\n", time.Since(start))
	
	fmt.Println("Waiting for queue to drain (retries will occur)...")
	waitForDrained(ctx, pool)
	
	stats := getStats(ctx, pool)
	fmt.Printf("\n=== FAILURE BENCHMARK RESULTS ===\n")
	fmt.Printf("Delivered: %d\n", stats["delivery_delivered"])
	fmt.Printf("Dead:      %d\n", stats["delivery_dead"])
	fmt.Printf("Failed:    %d\n", stats["delivery_failed"])
}

func runQueueBench(ctx context.Context, pool *pgxpool.Pool, subID string) {
	fmt.Println("\n--- Phase 2: Queue Benchmark (100k events) ---")
	// Insert 100k
	fmt.Println("Seeding 100,000 events...")
	start := time.Now()
	pool.Exec(ctx, "ALTER TABLE events DISABLE TRIGGER ALL;")
	pool.Exec(ctx, "ALTER TABLE delivery_jobs DISABLE TRIGGER ALL;")
	insertEvents(ctx, pool, subID, 100000)
	pool.Exec(ctx, "ALTER TABLE events ENABLE TRIGGER ALL;")
	pool.Exec(ctx, "ALTER TABLE delivery_jobs ENABLE TRIGGER ALL;")
	
	pool.Exec(ctx, `
		UPDATE system_stats SET value = (SELECT COUNT(*) FROM events) WHERE key = 'total_events';
		UPDATE system_stats SET value = (SELECT COUNT(*) FROM delivery_jobs WHERE status = 'pending') WHERE key = 'delivery_pending';
	`)
	fmt.Printf("Seeded 100k events in %v\n", time.Since(start))
	
	fmt.Println("Waiting for queue to drain...")
	start = time.Now()
	waitForDrained(ctx, pool)
	duration := time.Since(start)
	
	stats := getStats(ctx, pool)
	throughput := 100000.0 / duration.Seconds()
	
	fmt.Printf("\n=== QUEUE BENCHMARK RESULTS ===\n")
	fmt.Printf("Time taken:      %v\n", duration)
	fmt.Printf("Peak throughput: %.2f events/sec\n", throughput)
	fmt.Printf("Delivered:       %d\n", stats["delivery_delivered"])
	fmt.Printf("Pending:         %d\n", stats["delivery_pending"])
}

func runCorrectness(ctx context.Context, pool *pgxpool.Pool, subID string) {
	fmt.Println("\n--- Phase 1: Correctness (10 events) ---")
	setTargetConfig(0, 0)
	insertEvents(ctx, pool, subID, 10)
	waitForDrained(ctx, pool)
	
	stats := getStats(ctx, pool)
	fmt.Printf("Stats: Pending=%d, InFlight=%d, Delivered=%d, Failed=%d, Dead=%d\n", 
		stats["delivery_pending"], stats["delivery_in_flight"], stats["delivery_delivered"], stats["delivery_failed"], stats["delivery_dead"])
}

func runMillion(ctx context.Context, pool *pgxpool.Pool, subID string) {
	fmt.Println("\n--- Phase 14: One Million Event Test ---")
	
	setTargetConfig(0, 0) // Perfect receiver
	
	fmt.Println("Seeding 1,000,000 events... (this may take a minute)")
	
	start := time.Now()
	// Disable triggers
	pool.Exec(ctx, "ALTER TABLE events DISABLE TRIGGER ALL;")
	pool.Exec(ctx, "ALTER TABLE delivery_jobs DISABLE TRIGGER ALL;")
	
	// Insert 1M
	_, err := pool.Exec(ctx, `
		WITH new_events AS (
			INSERT INTO events (type, payload, user_id)
			SELECT 'bench.test', '{"bench": true}'::jsonb, user_id 
			FROM subscriptions 
			CROSS JOIN generate_series(1, 1000000)
			WHERE id = $1
			RETURNING id, user_id
		)
		INSERT INTO delivery_jobs (event_id, subscription_id, user_id)
		SELECT id, $1, user_id FROM new_events;
	`, subID)
	if err != nil {
		log.Fatalf("Failed to seed 1M: %v", err)
	}
	
	pool.Exec(ctx, "ALTER TABLE events ENABLE TRIGGER ALL;")
	pool.Exec(ctx, "ALTER TABLE delivery_jobs ENABLE TRIGGER ALL;")
	
	// Hydrate system_stats manually
	pool.Exec(ctx, `
		UPDATE system_stats SET value = (SELECT COUNT(*) FROM events) WHERE key = 'total_events';
		UPDATE system_stats SET value = (SELECT COUNT(*) FROM delivery_jobs WHERE status = 'pending') WHERE key = 'delivery_pending';
	`)
	
	fmt.Printf("Seeded 1M events in %v\n", time.Since(start))
	
	fmt.Println("Waiting for queue to drain...")
	start = time.Now()
	waitForDrained(ctx, pool)
	duration := time.Since(start)
	
	stats := getStats(ctx, pool)
	throughput := 1000000.0 / duration.Seconds()
	
	fmt.Printf("\n=== MILLION EVENT TEST RESULTS ===\n")
	fmt.Printf("Time taken:      %v\n", duration)
	fmt.Printf("Peak throughput: %.2f events/sec\n", throughput)
	fmt.Printf("Delivered:       %d\n", stats["delivery_delivered"])
	fmt.Printf("DLQ:             %d\n", stats["delivery_dead"])
	fmt.Printf("Pending:         %d\n", stats["delivery_pending"])
}

func setTargetConfig(failRate, slowRate float64) {
	url := fmt.Sprintf("http://webhook-target:9999/config?failRate=%f&slowRate=%f", failRate, slowRate)
	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		log.Printf("Failed to config target: %v", err)
		return
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
}

func insertEvents(ctx context.Context, pool *pgxpool.Pool, subID string, count int) {
	_, err := pool.Exec(ctx, `
		WITH new_events AS (
			INSERT INTO events (type, payload, user_id)
			SELECT 'bench.test', '{"bench": true}'::jsonb, user_id 
			FROM subscriptions 
			CROSS JOIN generate_series(1, $2)
			WHERE id = $1
			RETURNING id, user_id
		)
		INSERT INTO delivery_jobs (event_id, subscription_id, user_id)
		SELECT id, $1, user_id FROM new_events;
	`, subID, count)
	if err != nil {
		log.Fatalf("Insert failed: %v", err)
	}
}

func waitForDrained(ctx context.Context, pool *pgxpool.Pool) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	
	var lastPending int64
	
	for range ticker.C {
		stats := getStats(ctx, pool)
		pending := stats["delivery_pending"]
		inFlight := stats["delivery_in_flight"]
		delivered := stats["delivery_delivered"]
		
		fmt.Printf("Queue: Pending=%d, InFlight=%d, Delivered=%d\n", pending, inFlight, delivered)
		
		if pending == 0 && inFlight == 0 {
			break
		}
		
		// Break if stalled
		if pending == lastPending && inFlight == 0 && pending > 0 {
			fmt.Println("Warning: Queue appears stalled (no in-flight, pending unchanged)")
			// Don't break immediately, could be backoff retry
		}
		lastPending = pending
	}
}

func getStats(ctx context.Context, pool *pgxpool.Pool) map[string]int64 {
	rows, err := pool.Query(ctx, "SELECT key, SUM(value) FROM system_stats GROUP BY key")
	if err != nil {
		log.Printf("Error getting stats: %v", err)
		return nil
	}
	defer rows.Close()
	
	stats := make(map[string]int64)
	for rows.Next() {
		var k string
		var v int64
		rows.Scan(&k, &v)
		stats[k] = v
	}
	return stats
}
