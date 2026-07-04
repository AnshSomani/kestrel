# Kestrel Benchmark Results & Resume Snippet

## 📄 Resume Bullet Points
```latex
\resumeSubheading
  {Kestrel: High-Throughput Webhook Delivery Engine}
  {\href{https://github.com/AnshSomani/kestrel}{\texttt{GitHub}} $|$ \emph{Go (Echo), PostgreSQL, Redis, React, TypeScript}}{June 2026}
  \resumeItemListStart
    \resumeItem{Engineered a highly concurrent webhook delivery platform using Go and PostgreSQL, successfully processing a \textbf{2-million-event burst} with \textbf{zero dropped payloads} while sustaining \textbf{1,652 webhook deliveries/sec} end-to-end.}
    \resumeItem{Optimized API ingestion to \textbf{3,846 requests/sec} by replacing synchronous database triggers with an asynchronous, Go-channel-based in-memory aggregator, eliminating PostgreSQL row-lock contention and slashing database CPU load.}
    \resumeItem{Architected resilient delivery workflows with Redis-backed distributed circuit breakers, decorrelated exponential backoff, Dead Letter Queues (DLQ), and HTTP timeout-based retries for fault-tolerant webhook delivery.}
    \resumeItem{Designed a \textbf{32-poller concurrent architecture} using PostgreSQL \texttt{FOR UPDATE SKIP LOCKED} and batched dequeuing for high-throughput job processing, monitored via a React observability dashboard with Prometheus and Grafana.}
  \resumeItemListEnd
```

---

## 💻 Hardware Specification (Local Docker Virtual Machine)

| Component | Detail |
|-----------|--------|
| **OS** | Windows (Docker Desktop Virtualization) |
| **Go Version** | 1.24 |
| **PostgreSQL** | 17 (Docker, SSD Volume) |
| **Redis** | 8 (Docker) |

## 🚀 Results — Event Ingestion (API Ingestion Benchmark)
*Isolated API capacity without matching subscriptions, bypassing the delivery pool.*

| Metric | Value |
|--------|-------|
| **Total Events Sent** | 2,000,000 |
| **Average Throughput** | 3,846 req/sec |
| **Peak Throughput** | 4,315 req/sec |
| **P50 API Latency** | 101 ms |
| **P97.5 API Latency** | 133 ms |
| **P99 API Latency** | 144 ms |
| **Delivery Success Rate** | 100% (Zero Dropped) |

## 🦅 Results — End-to-End Delivery Throughput
*Full pipeline test: API Ingestion -> Database Commit -> SKIP LOCKED Queue -> HTTP Delivery.*

| Metric | Value |
|--------|-------|
| **Total Events Processed** | 2,000,000 |
| **Average Delivery Throughput** | 1,652 req/sec |
| **Peak Delivery Throughput** | 2,023 req/sec |
| **P50 Webhook Delivery Latency** | 3.11 ms |
| **P99 Webhook Delivery Latency** | 23.4 ms |
| **Delivery Success Rate** | 100% |
| **Dead Lettered / Dropped** | 0 |

## 🧪 Results — Chaos Engineering & Reliability (Backend Suite)
*Internal testing bypassing API, pushing 50k - 2M rows directly into Postgres via raw SQL.*

| Metric | Value |
|--------|-------|
| **Queue Drain Speed (2M Event Burst)** | 1,652 events/sec |
| **Circuit Breaker Trips** | Instant trip after 5 consecutive failures |
| **Redis Rate Limiting P99** | < 1 ms |
| **Race Conditions (go test -race)** | 0 |

---

## 💥 Methodology

1. **Ingestion-Only test**: `autocannon -c 400 -a 2000000 -m POST` (No matching subscriptions)
2. **End-to-End test**: `autocannon -c 400 -a 2000000 -m POST` (Valid active subscription)
3. **Queue Drain test**: `go run ./cmd/bench -phase twomillion`
4. **Reliability & Chaos test**: `go run ./cmd/bench -phase chaos`
5. **Rate limiter test**: Distributed Redis sliding-window algorithms validated against concurrent pollers
6. **All tests run on local hardware** — throughput scales massively in native Linux cloud deployments.
