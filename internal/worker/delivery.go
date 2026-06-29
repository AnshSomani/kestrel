// Package worker implements the core webhook delivery pipeline, including
// HTTP delivery with HMAC-SHA256 signing, a concurrent worker pool, and
// integration with circuit breaking, rate limiting, and retry logic.
package worker

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"kestrel/internal/queue"
)

// DeliveryResult captures the outcome of a single webhook delivery attempt.
// It provides all the information needed by the worker pool to decide whether
// to mark the job as delivered, retry, or dead-letter.
type DeliveryResult struct {
	Success    bool
	StatusCode int
	Error      string
	Duration   time.Duration
}

// Deliverer handles the HTTP mechanics of delivering webhook payloads
// to subscriber endpoints. It manages a shared http.Client with connection
// pooling tuned for high-throughput, many-endpoint delivery patterns.
type Deliverer struct {
	client *http.Client
	dryRun bool
}

// NewDeliverer creates a Deliverer with a pre-configured HTTP client.
//
// The transport is tuned for webhook delivery workloads:
//   - MaxIdleConns=200: supports many concurrent connections across endpoints
//   - MaxIdleConnsPerHost=50: allows connection reuse for hot endpoints
//   - IdleConnTimeout=90s: keeps connections warm without leaking resources
func NewDeliverer(timeout time.Duration, dryRun bool) *Deliverer {
	return &Deliverer{
		client: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:        200,
				MaxIdleConnsPerHost: 50,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		dryRun: dryRun,
	}
}

// Deliver sends the webhook payload to the job's endpoint URL via HTTP POST.
//
// Every request includes security headers:
//   - X-Kestrel-Signature: HMAC-SHA256 of "timestamp.payload" for verification
//   - X-Kestrel-Timestamp: unix timestamp to prevent replay attacks
//   - X-Kestrel-Event-Type: the event type for routing on the subscriber side
//   - X-Kestrel-Delivery-ID: unique delivery ID for idempotency tracking
//
// Success is defined as any 2xx HTTP status code. The response body is always
// fully drained to allow TCP connection reuse via keep-alive.
func (d *Deliverer) Deliver(ctx context.Context, job *queue.Job) *DeliveryResult {
	start := time.Now()

	if d.dryRun {
		return &DeliveryResult{
			Success:    true,
			StatusCode: 200,
			Duration:   time.Since(start),
		}
	}

	// Build the POST request with the event payload as the body.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, job.EndpointURL, bytes.NewReader(job.Payload))
	if err != nil {
		return &DeliveryResult{
			Error:    err.Error(),
			Duration: time.Since(start),
		}
	}

	// Generate the HMAC-SHA256 signature using the shared secret.
	// The timestamp is included in the signed message to bind the signature
	// to a specific point in time, allowing subscribers to reject stale deliveries.
	timestamp := time.Now().Unix()
	signature := signPayload(job.Payload, job.Secret, timestamp)

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Kestrel-Signature", signature)
	req.Header.Set("X-Kestrel-Event-Type", job.EventType)
	req.Header.Set("X-Kestrel-Delivery-ID", job.ID.String())
	req.Header.Set("X-Kestrel-Timestamp", strconv.FormatInt(timestamp, 10))
	req.Header.Set("User-Agent", "Kestrel-Webhook/1.0")

	// Execute the HTTP request.
	resp, err := d.client.Do(req)
	if err != nil {
		return &DeliveryResult{
			Error:    err.Error(),
			Duration: time.Since(start),
		}
	}
	defer resp.Body.Close()

	// Drain the response body to enable TCP connection reuse.
	// Without this, the connection cannot be returned to the pool for keep-alive.
	_, _ = io.Copy(io.Discard, resp.Body)

	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	result := &DeliveryResult{
		Success:    success,
		StatusCode: resp.StatusCode,
		Duration:   time.Since(start),
	}
	if !success {
		result.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	return result
}

// signPayload produces an HMAC-SHA256 signature of the webhook payload.
//
// The signed message is "timestamp.payload" — concatenating the unix timestamp
// with the raw payload bytes separated by a period. This format:
//   - Binds the signature to a specific timestamp (replay protection)
//   - Includes the full payload (tamper protection)
//   - Uses a simple, unambiguous format that subscribers can easily reproduce
//
// The output is prefixed with "sha256=" to indicate the algorithm, following
// the convention established by GitHub webhooks and Stripe.
func signPayload(payload []byte, secret string, timestamp int64) string {
	message := fmt.Sprintf("%d.%s", timestamp, payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
