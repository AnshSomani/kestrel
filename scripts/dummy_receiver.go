package main

import (
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"
)

func main() {
	var count uint64

	http.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		
		// Read and discard the body to keep the connection alive
		_, _ = io.Copy(io.Discard, r.Body)
		r.Body.Close()

		// Increment and log every 100th request so we don't spam the console
		c := atomic.AddUint64(&count, 1)
		if c%100 == 0 {
			fmt.Printf("[%s] Received %d webhooks so far...\n", time.Now().Format("15:04:05"), c)
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	fmt.Println("🚀 Dummy Webhook Receiver listening on http://localhost:9090/webhook")
	fmt.Println("Ready to absorb unlimited Kestrel payloads!")
	if err := http.ListenAndServe(":9090", nil); err != nil {
		panic(err)
	}
}
