// Package metrics exposes Prometheus instrumentation for the Kestrel webhook
// delivery engine. All metrics are auto-registered under the "kestrel"
// namespace.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus collectors used throughout the service.
type Metrics struct {
	// EventsIngested counts the total number of events accepted by the API.
	EventsIngested prometheus.Counter

	// DeliveriesTotal counts delivery outcomes partitioned by status.
	// Labels: "status" — one of: delivered, failed, dead, rate_limited, circuit_open.
	DeliveriesTotal *prometheus.CounterVec

	// DeliveryDuration observes the latency of individual webhook POST calls.
	DeliveryDuration prometheus.Histogram

	// ActiveDeliveries tracks the number of HTTP deliveries currently in-flight.
	ActiveDeliveries prometheus.Gauge

	// QueueDepth reports the approximate number of pending jobs in the queue.
	QueueDepth prometheus.Gauge

	// RateLimitTotal counts rate limiter decisions.
	// Labels: "result" — one of: allowed, rejected.
	RateLimitTotal *prometheus.CounterVec

	// RetryTotal counts retries bucketed by attempt number.
	// Labels: "attempt" — one of: "1", "2", "3", "4", "5".
	RetryTotal *prometheus.CounterVec

	// CircuitBreaks counts the total number of times a circuit breaker tripped.
	CircuitBreaks prometheus.Counter

	// PanicsTotal counts recovered panics inside worker goroutines.
	PanicsTotal prometheus.Counter

	// DBStats flushes metrics to the database to avoid row contention.
	DBStats *DBStatsFlusher
}

// NewMetrics creates and auto-registers all Prometheus metrics for Kestrel.
func NewMetrics(dbStats *DBStatsFlusher) *Metrics {
	return &Metrics{
		DBStats: dbStats,
		EventsIngested: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "kestrel",
			Name:      "events_ingested_total",
			Help:      "Total number of events ingested via the API.",
		}),

		DeliveriesTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: "kestrel",
			Name:      "deliveries_total",
			Help:      "Total delivery outcomes by status (delivered, failed, dead, rate_limited, circuit_open).",
		}, []string{"status"}),

		DeliveryDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Namespace: "kestrel",
			Name:      "delivery_duration_seconds",
			Help:      "Histogram of webhook delivery latencies in seconds.",
			Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		}),

		ActiveDeliveries: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: "kestrel",
			Name:      "active_deliveries",
			Help:      "Number of webhook deliveries currently in-flight.",
		}),

		QueueDepth: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: "kestrel",
			Name:      "queue_depth",
			Help:      "Approximate number of pending delivery jobs in the queue.",
		}),

		RateLimitTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: "kestrel",
			Name:      "rate_limit_total",
			Help:      "Rate limiter decisions by result (allowed, rejected).",
		}, []string{"result"}),

		RetryTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: "kestrel",
			Name:      "retry_total",
			Help:      "Retry count by attempt number (1–5).",
		}, []string{"attempt"}),

		CircuitBreaks: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "kestrel",
			Name:      "circuit_breaks_total",
			Help:      "Total number of circuit breaker trips.",
		}),

		PanicsTotal: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "kestrel",
			Name:      "panics_total",
			Help:      "Total number of recovered panics inside worker goroutines.",
		}),
	}
}
