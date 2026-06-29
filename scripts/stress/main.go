package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	url := flag.String("url", "http://localhost:8080", "Kestrel server URL")
	apiKey := flag.String("api-key", "kestrel-dev-key", "API key for authentication")
	rps := flag.Int("rps", 1000, "Target requests per second")
	duration := flag.Duration("duration", 30*time.Second, "Test duration")
	workers := flag.Int("workers", 50, "Number of concurrent worker goroutines")
	flag.Parse()

	fmt.Println()
	fmt.Println("╔═══════════════════════════════════════════╗")
	fmt.Println("║   🦅 Kestrel Stress Test                  ║")
	fmt.Println("╚═══════════════════════════════════════════╝")
	fmt.Printf("  Target:     %s\n", *url)
	fmt.Printf("  RPS:        %d\n", *rps)
	fmt.Printf("  Duration:   %s\n", *duration)
	fmt.Printf("  Workers:    %d\n", *workers)
	fmt.Println()

	eventTypes := []string{"user.signed_up", "subscription.upgraded", "invoice.payment_failed"}

	// Channels and counters
	var (
		totalSent   int64
		successCnt  int64
		failureCnt  int64
		latencies   []time.Duration
		latencyMu   sync.Mutex
	)

	// Worker pool
	jobs := make(chan struct{}, *workers*2)
	var wg sync.WaitGroup

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConnsPerHost: *workers,
			MaxIdleConns:        *workers * 2,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	// Spawn workers
	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				start := time.Now()

				evtType := eventTypes[rand.Intn(len(eventTypes))]
				var evtPayload map[string]interface{}
				
				switch evtType {
				case "user.signed_up":
					evtPayload = map[string]interface{}{
						"user_id":    fmt.Sprintf("usr_%d", rand.Int63()),
						"email":      fmt.Sprintf("user%d@example.com", rand.Int63()),
						"plan":       "free",
						"created_at": time.Now().Unix(),
					}
				case "subscription.upgraded":
					evtPayload = map[string]interface{}{
						"user_id":    fmt.Sprintf("usr_%d", rand.Int63()),
						"plan_from":  "free",
						"plan_to":    "pro",
						"mrr_change": 2900,
						"timestamp":  time.Now().Unix(),
					}
				case "invoice.payment_failed":
					evtPayload = map[string]interface{}{
						"invoice_id": fmt.Sprintf("inv_%d", rand.Int63()),
						"amount":     2900,
						"currency":   "USD",
						"reason":     "insufficient_funds",
						"timestamp":  time.Now().Unix(),
					}
				}

				payload := map[string]interface{}{
					"type":           evtType,
					"payload":        evtPayload,
					"idempotency_key": fmt.Sprintf("stress-%d-%d", time.Now().UnixNano(), rand.Int63()),
				}
				body, _ := json.Marshal(payload)

				req, err := http.NewRequest("POST", *url+"/api/events", bytes.NewReader(body))
				if err != nil {
					atomic.AddInt64(&failureCnt, 1)
					atomic.AddInt64(&totalSent, 1)
					continue
				}
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-API-Key", *apiKey)

				resp, err := client.Do(req)
				elapsed := time.Since(start)

				atomic.AddInt64(&totalSent, 1)

				if err != nil {
					atomic.AddInt64(&failureCnt, 1)
				} else {
					resp.Body.Close()
					if resp.StatusCode >= 200 && resp.StatusCode < 300 {
						atomic.AddInt64(&successCnt, 1)
					} else {
						atomic.AddInt64(&failureCnt, 1)
					}
				}

				latencyMu.Lock()
				latencies = append(latencies, elapsed)
				latencyMu.Unlock()
			}
		}()
	}

	// Pace requests using a ticker
	interval := time.Second / time.Duration(*rps)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	deadline := time.After(*duration)
	fmt.Printf("  Starting... (sending ~%d req/s)\n\n", *rps)

	// Progress reporting
	progressTicker := time.NewTicker(5 * time.Second)
	defer progressTicker.Stop()

	startTime := time.Now()

loop:
	for {
		select {
		case <-deadline:
			break loop
		case <-ticker.C:
			select {
			case jobs <- struct{}{}:
			default:
				// Workers are saturated, skip this tick
			}
		case <-progressTicker.C:
			elapsed := time.Since(startTime).Seconds()
			sent := atomic.LoadInt64(&totalSent)
			ok := atomic.LoadInt64(&successCnt)
			fail := atomic.LoadInt64(&failureCnt)
			currentRPS := float64(sent) / elapsed
			fmt.Printf("  [%.0fs] sent=%d ok=%d fail=%d rps=%.1f\n",
				elapsed, sent, ok, fail, currentRPS)
		}
	}

	// Close jobs channel and wait for workers to finish
	close(jobs)
	wg.Wait()

	totalElapsed := time.Since(startTime)

	// Calculate latency percentiles
	latencyMu.Lock()
	sort.Slice(latencies, func(i, j int) bool {
		return latencies[i] < latencies[j]
	})
	totalLatencies := latencies
	latencyMu.Unlock()

	total := atomic.LoadInt64(&totalSent)
	success := atomic.LoadInt64(&successCnt)
	failures := atomic.LoadInt64(&failureCnt)
	actualRPS := float64(total) / totalElapsed.Seconds()

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════")
	fmt.Println("  🦅 Kestrel Stress Test Results")
	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("  Duration:     %.1fs\n", totalElapsed.Seconds())
	fmt.Printf("  Total Sent:   %s\n", formatInt(total))
	fmt.Printf("  Successful:   %s\n", formatInt(success))
	fmt.Printf("  Failed:       %s\n", formatInt(failures))
	fmt.Printf("  Actual RPS:   %.1f\n", actualRPS)
	fmt.Println()

	if len(totalLatencies) > 0 {
		fmt.Println("  Latency:")
		fmt.Printf("    Min:        %s\n", formatDuration(totalLatencies[0]))
		fmt.Printf("    Avg:        %s\n", formatDuration(avgDuration(totalLatencies)))
		fmt.Printf("    P50:        %s\n", formatDuration(percentile(totalLatencies, 0.50)))
		fmt.Printf("    P95:        %s\n", formatDuration(percentile(totalLatencies, 0.95)))
		fmt.Printf("    P99:        %s\n", formatDuration(percentile(totalLatencies, 0.99)))
		fmt.Printf("    Max:        %s\n", formatDuration(totalLatencies[len(totalLatencies)-1]))
	}
	fmt.Println("═══════════════════════════════════════════")
	fmt.Println()

	// Exit with non-zero status if error rate is above 1%
	if total > 0 && float64(failures)/float64(total) > 0.01 {
		fmt.Println("⚠️  Error rate above 1%, exiting with status 1")
		os.Exit(1)
	}
}

// percentile returns the value at the given percentile from a sorted slice.
func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)-1) * p)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// avgDuration calculates the average of a slice of durations.
func avgDuration(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	var total time.Duration
	for _, d := range durations {
		total += d
	}
	return total / time.Duration(len(durations))
}

// formatDuration formats a duration in a human-friendly way (ms).
func formatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%.1fµs", float64(d.Microseconds()))
	}
	return fmt.Sprintf("%.1fms", float64(d.Microseconds())/1000.0)
}

// formatInt formats an integer with comma separators.
func formatInt(n int64) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}

	var result []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	return string(result)
}
