package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// ANSI color codes
const (
	reset   = "\033[0m"
	bold    = "\033[1m"
	red     = "\033[31m"
	green   = "\033[32m"
	yellow  = "\033[33m"
	blue    = "\033[34m"
	magenta = "\033[35m"
	cyan    = "\033[36m"
	gray    = "\033[90m"
)

func main() {
	url := flag.String("url", "http://localhost:8080", "Kestrel server URL")
	targetURL := flag.String("target-url", "http://webhook-target:9999/webhook", "Webhook target URL")
	apiKey := flag.String("api-key", "kestrel-dev-key", "API key for authentication")
	flag.Parse()

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Banner
	fmt.Println()
	printBanner()
	fmt.Println()

	// Step 1: Create a subscription
	step("Creating webhook subscription...")
	sub := createSubscription(client, *url, *apiKey, *targetURL)
	success("Subscription created: %s", sub["id"])
	info("  Endpoint: %s", sub["endpoint_url"])
	info("  Events:   [user.signed_up, subscription.upgraded, invoice.payment_failed]")
	pause(1)

	// Step 2: Fire 1000 events
	fmt.Println()
	step("Firing 1000 user.signed_up events...")
	var (
		sentCount   int64
		errorCount  int64
	)
	startTime := time.Now()

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 50) // 50 concurrent goroutines

	for i := 0; i < 1000; i++ {
		wg.Add(1)
		semaphore <- struct{}{}
		go func(idx int) {
			defer wg.Done()
			defer func() { <-semaphore }()

			payload := map[string]interface{}{
				"type": "user.signed_up",
				"payload": map[string]interface{}{
					"user_id":    fmt.Sprintf("usr_%06d", idx),
					"email":      fmt.Sprintf("demo_user_%06d@kestrel.local", idx),
					"plan":       "free",
					"created_at": time.Now().Unix(),
				},
			}

			body, _ := json.Marshal(payload)
			req, _ := http.NewRequest("POST", *url+"/api/events", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-API-Key", *apiKey)

			resp, err := client.Do(req)
			if err != nil {
				atomic.AddInt64(&errorCount, 1)
				atomic.AddInt64(&sentCount, 1)
				return
			}
			resp.Body.Close()

			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				atomic.AddInt64(&sentCount, 1)
			} else {
				atomic.AddInt64(&errorCount, 1)
				atomic.AddInt64(&sentCount, 1)
			}

			// Progress indicator every 100
			cur := atomic.LoadInt64(&sentCount)
			if cur%100 == 0 {
				progress("  Sent %d/1000 events...", cur)
			}
		}(i)
	}

	wg.Wait()
	fireElapsed := time.Since(startTime)

	sent := atomic.LoadInt64(&sentCount)
	errors := atomic.LoadInt64(&errorCount)
	success("All events fired in %v", fireElapsed.Round(time.Millisecond))
	info("  Sent: %d  |  Errors: %d  |  Rate: %.0f events/s",
		sent, errors, float64(sent)/fireElapsed.Seconds())

	pause(3)

	// Step 3: Check health and queue depth
	fmt.Println()
	step("Checking system health...")
	health := checkHealth(client, *url)
	if health != nil {
		info("  Status:      %s", health["status"])
		info("  Postgres:    %s", health["postgres"])
		info("  Queue Depth: %.0f", health["queue_depth"])
	} else {
		warn("  Could not reach health endpoint")
	}

	// Step 4: Wait for queue to drain
	fmt.Println()
	step("Waiting for delivery queue to drain...")
	drained := waitForDrain(client, *url, 60*time.Second)
	if drained {
		success("Queue drained — all deliveries completed! ✓")
	} else {
		warn("Timeout waiting for queue to drain")
	}

	// Step 5: Final stats
	fmt.Println()
	fmt.Printf("%s═══════════════════════════════════════════%s\n", cyan, reset)
	fmt.Printf("%s  🦅 Kestrel Demo — Final Results%s\n", bold, reset)
	fmt.Printf("%s═══════════════════════════════════════════%s\n", cyan, reset)
	fmt.Printf("  Events Sent:       %d\n", sent)
	fmt.Printf("  Send Errors:       %d\n", errors)
	fmt.Printf("  Fire Duration:     %v\n", fireElapsed.Round(time.Millisecond))
	fmt.Printf("  Events/s:          %.0f\n", float64(sent)/fireElapsed.Seconds())

	// Check final queue state
	finalHealth := checkHealth(client, *url)
	if finalHealth != nil {
		depth := finalHealth["queue_depth"].(float64)
		if depth == 0 {
			fmt.Printf("  Queue Depth:       %s0 (all delivered)%s\n", green, reset)
		} else {
			fmt.Printf("  Queue Depth:       %s%.0f (still processing)%s\n", yellow, depth, reset)
		}
	}

	fmt.Printf("%s═══════════════════════════════════════════%s\n", cyan, reset)
	fmt.Println()
}

// createSubscription creates a webhook subscription via the API.
func createSubscription(client *http.Client, baseURL, apiKey, targetURL string) map[string]interface{} {
	payload := map[string]interface{}{
		"endpoint_url": targetURL,
		"secret":       "demo-secret-key",
		"event_types":  []string{"user.signed_up", "subscription.upgraded", "invoice.payment_failed"},
	}

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", baseURL+"/api/subscriptions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", apiKey)

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("%s  ✗ Failed to create subscription: %v%s\n", red, err, reset)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		respBody, _ := io.ReadAll(resp.Body)
		fmt.Printf("%s  ✗ Subscription creation failed (status %d): %s%s\n",
			red, resp.StatusCode, string(respBody), reset)
		os.Exit(1)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	return result
}

// checkHealth queries the /health endpoint and returns the parsed response.
func checkHealth(client *http.Client, baseURL string) map[string]interface{} {
	resp, err := client.Get(baseURL + "/health")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	return result
}

// waitForDrain polls the health endpoint until queue_depth reaches 0 or timeout.
func waitForDrain(client *http.Client, baseURL string, timeout time.Duration) bool {
	deadline := time.After(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			return false
		case <-ticker.C:
			health := checkHealth(client, baseURL)
			if health == nil {
				continue
			}
			depth, ok := health["queue_depth"].(float64)
			if !ok {
				continue
			}
			progress("  Queue depth: %.0f", depth)
			if depth == 0 {
				return true
			}
		}
	}
}

// Output helpers with color
func printBanner() {
	fmt.Printf("%s%s", bold, cyan)
	fmt.Println("  ╔════════════════════════════════════════════════╗")
	fmt.Println("  ║                                                ║")
	fmt.Println("  ║   🦅 Kestrel Demo — Webhook Delivery Engine    ║")
	fmt.Println("  ║                                                ║")
	fmt.Println("  ╚════════════════════════════════════════════════╝")
	fmt.Printf("%s", reset)
}

func step(msg string, args ...interface{}) {
	fmt.Printf("%s▶ %s%s\n", blue+bold, fmt.Sprintf(msg, args...), reset)
}

func success(msg string, args ...interface{}) {
	fmt.Printf("%s  ✓ %s%s\n", green, fmt.Sprintf(msg, args...), reset)
}

func info(msg string, args ...interface{}) {
	fmt.Printf("%s%s%s\n", gray, fmt.Sprintf(msg, args...), reset)
}

func progress(msg string, args ...interface{}) {
	fmt.Printf("%s%s%s\n", yellow, fmt.Sprintf(msg, args...), reset)
}

func warn(msg string, args ...interface{}) {
	fmt.Printf("%s  ⚠ %s%s\n", yellow, fmt.Sprintf(msg, args...), reset)
}

func pause(seconds int) {
	time.Sleep(time.Duration(seconds) * time.Second)
}
