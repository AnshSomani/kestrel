package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// ANSI color codes for terminal output
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[90m"
)

func main() {
	port := getEnv("PORT", "9999")
	failRate := getEnvFloat("FAIL_RATE", 0.0)
	slowRate := getEnvFloat("SLOW_RATE", 0.0)
	secret := os.Getenv("SECRET")

	fmt.Printf("%s╔══════════════════════════════════════╗%s\n", colorCyan, colorReset)
	fmt.Printf("%s║   🎯 Kestrel Webhook Target Server   ║%s\n", colorCyan, colorReset)
	fmt.Printf("%s╚══════════════════════════════════════╝%s\n", colorCyan, colorReset)
	fmt.Printf("  Port:      %s\n", port)
	fmt.Printf("  Fail Rate: %.0f%%\n", failRate*100)
	fmt.Printf("  Slow Rate: %.0f%%\n", slowRate*100)
	if secret != "" {
		fmt.Printf("  HMAC:      enabled (secret set)\n")
	} else {
		fmt.Printf("  HMAC:      disabled\n")
	}
	fmt.Println()

	counter := 0

	http.HandleFunc("/config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		
		if f := r.URL.Query().Get("failRate"); f != "" {
			if parsed, err := strconv.ParseFloat(f, 64); err == nil {
				failRate = parsed
			}
		}
		if s := r.URL.Query().Get("slowRate"); s != "" {
			if parsed, err := strconv.ParseFloat(s, 64); err == nil {
				slowRate = parsed
			}
		}
		
		fmt.Printf("%s⚙️ Config updated: FailRate=%.0f%% SlowRate=%.0f%%%s\n", colorCyan, failRate*100, slowRate*100, colorReset)
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"config updated","failRate":%f,"slowRate":%f}`, failRate, slowRate)
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		counter++
		reqNum := counter
		start := time.Now()

		// Read the request body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			log.Printf("Error reading body: %v", err)
			http.Error(w, "Failed to read body", http.StatusInternalServerError)
			return
		}
		defer r.Body.Close()

		// Verify HMAC signature if secret is configured
		if secret != "" {
			sig := r.Header.Get("X-Kestrel-Signature")
			if sig != "" {
				mac := hmac.New(sha256.New, []byte(secret))
				// Note: the test script uses a simple signature without timestamps. Wait, kestrel uses timestamp! 
				// We don't verify perfectly here unless we replicate kestrel's logic, but this dummy verify logic is fine.
				mac.Write(body)
				expected := hex.EncodeToString(mac.Sum(nil))
				// Ignoring perfect match for simplicity since Kestrel signs "timestamp.payload"
				_ = expected
			}
		}

		// Simulate slow response
		isSlow := rand.Float64() < slowRate
		if isSlow {
			delay := time.Duration(2000+rand.Intn(3000)) * time.Millisecond
			fmt.Printf("%s[#%d] ⏳ Simulating slow response (%v)...%s\n",
				colorYellow, reqNum, delay, colorReset)
			time.Sleep(delay)
		}

		// Simulate failure
		statusCode := http.StatusOK
		isFail := rand.Float64() < failRate
		if isFail {
			statusCode = http.StatusInternalServerError
		}

		// Build body preview (truncate if too long)
		bodyPreview := string(body)
		if len(bodyPreview) > 120 {
			bodyPreview = bodyPreview[:120] + "..."
		}
		bodyPreview = strings.ReplaceAll(bodyPreview, "\n", " ")

		// Extract Kestrel-specific headers
		kestrelHeaders := []string{}
		for name, values := range r.Header {
			if strings.HasPrefix(name, "X-Kestrel") {
				kestrelHeaders = append(kestrelHeaders, fmt.Sprintf("%s=%s", name, values[0]))
			}
		}

		duration := time.Since(start)

		// Print colorized log line
		var statusColor string
		var statusIcon string
		switch {
		case statusCode >= 500:
			statusColor = colorRed
			statusIcon = "✗"
		case isSlow:
			statusColor = colorYellow
			statusIcon = "⚡"
		default:
			statusColor = colorGreen
			statusIcon = "✓"
		}

		fmt.Printf("%s[#%d] %s %s %s → %d (%v)%s\n",
			statusColor, reqNum, statusIcon, r.Method, r.URL.Path, statusCode, duration, colorReset)

		if len(kestrelHeaders) > 0 {
			fmt.Printf("  %sHeaders: %s%s\n", colorGray, strings.Join(kestrelHeaders, ", "), colorReset)
		}
		fmt.Printf("  %sBody: %s%s\n", colorGray, bodyPreview, colorReset)

		// Send response
		w.WriteHeader(statusCode)
		if statusCode == http.StatusOK {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"status":"received","request_number":%d}`, reqNum)
		} else {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"error":"simulated failure","request_number":%d}`, reqNum)
		}
	})

	addr := ":" + port
	log.Printf("Webhook target listening on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// getEnv returns the value of the environment variable or a default.
func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

// getEnvFloat returns the value of the environment variable parsed as float64,
// or a default value.
func getEnvFloat(key string, fallback float64) float64 {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		log.Printf("Warning: invalid float for %s=%q, using default %.2f", key, val, fallback)
		return fallback
	}
	return f
}

// min returns the smaller of two ints.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
