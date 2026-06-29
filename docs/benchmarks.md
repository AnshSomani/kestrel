# Kestrel Benchmark Results

## Hardware Specification

| Component | Detail |
|-----------|--------|
| **CPU** | [TO BE MEASURED] |
| **RAM** | [TO BE MEASURED] |
| **Disk** | [TO BE MEASURED] |
| **OS** | [TO BE MEASURED] |
| **Go Version** | 1.23.x |
| **PostgreSQL** | 16 (Docker, default config) |
| **Redis** | 7 (Docker, default config) |

## Test Configuration

| Parameter | Value |
|-----------|-------|
| Duration | 60s |
| Target RPS | 5,000 |
| Worker goroutines | 50 |
| Max concurrent deliveries | 100 |
| Dequeue batch size | 50 |
| Poll interval | 500ms |
| Subscriber failure rate | 0% (for throughput test) |

## Results — Event Ingestion (POST /api/events)

| Metric | Value |
|--------|-------|
| **Total Events Sent** | [TO BE MEASURED] |
| **Successful** | [TO BE MEASURED] |
| **Failed** | [TO BE MEASURED] |
| **Actual RPS** | [TO BE MEASURED] |
| **P50 Latency** | [TO BE MEASURED] |
| **P95 Latency** | [TO BE MEASURED] |
| **P99 Latency** | [TO BE MEASURED] |
| **Max Latency** | [TO BE MEASURED] |

## Results — Delivery Throughput

| Metric | Value |
|--------|-------|
| **Total Delivered** | [TO BE MEASURED] |
| **Delivery Rate** | [TO BE MEASURED]% |
| **Dead Lettered** | [TO BE MEASURED] |
| **Avg Delivery Latency** | [TO BE MEASURED] |
| **P99 Delivery Latency** | [TO BE MEASURED] |
| **Concurrent Deliveries (peak)** | [TO BE MEASURED] |

## Results — Reliability (30% failure rate)

| Metric | Value |
|--------|-------|
| **Total Events** | 1,000,000 |
| **Eventual Delivery Rate** | [TO BE MEASURED]% |
| **Duplicate Deliveries** | [TO BE MEASURED] (target: 0) |
| **Circuit Breaker Trips** | [TO BE MEASURED] |
| **Dead Letter Queue Size** | [TO BE MEASURED] |
| **Race Conditions (go test -race)** | [TO BE MEASURED] (target: 0) |

## Results — Rate Limiter Performance

| Metric | Value |
|--------|-------|
| **Decision Latency P50** | [TO BE MEASURED] |
| **Decision Latency P99** | [TO BE MEASURED] |
| **Concurrent Requestors** | [TO BE MEASURED] |
| **False Allows (over limit)** | [TO BE MEASURED] (target: 0) |

## Results — Worker Pool Scaling

| Workers | Throughput (events/sec) | P99 Latency | Memory Usage |
|---------|------------------------|-------------|--------------|
| 1 | [TO BE MEASURED] | [TO BE MEASURED] | [TO BE MEASURED] |
| 2 | [TO BE MEASURED] | [TO BE MEASURED] | [TO BE MEASURED] |
| 4 | [TO BE MEASURED] | [TO BE MEASURED] | [TO BE MEASURED] |
| 8 | [TO BE MEASURED] | [TO BE MEASURED] | [TO BE MEASURED] |
| 16 | [TO BE MEASURED] | [TO BE MEASURED] | [TO BE MEASURED] |
| 32 | [TO BE MEASURED] | [TO BE MEASURED] | [TO BE MEASURED] |

> Find where throughput plateaus — that plateau IS your interview number.
> On a 4-core machine, expect ~8-12 workers to be the sweet spot.

## Methodology

1. **Ingestion test**: `go run scripts/stress/main.go --rps 5000 --duration 60s`
2. **Delivery test**: 1M events with 1 subscriber, measure until queue drains
3. **Reliability test**: 1M events with `FAIL_RATE=0.3` on webhook target, measure eventual delivery rate
4. **Rate limiter test**: Go benchmark with `b.SetParallelism(100)` against Allow()
5. **Worker scaling**: Run stress test with `MAX_CONCURRENT=N` for N in {1,2,4,8,16,32}
6. **Race detection**: `go test -race ./...` — must be zero
7. **All tests run on same hardware** — numbers are comparable across components

## How to Reproduce

```bash
# 1. Start the stack
cd deployments && docker compose up --build -d

# 2. Create a subscription
curl -X POST http://localhost:8080/api/subscriptions \
  -H "X-API-Key: kestrel-dev-key" \
  -H "Content-Type: application/json" \
  -d '{"endpoint_url":"http://webhook-target:9999/webhook","secret":"bench-secret","event_types":["stress.test"]}'

# 3. Run the stress test
go run scripts/stress/main.go --rps 5000 --duration 60s

# 4. Check Grafana at http://localhost:3000 for live metrics
# 5. Record numbers in this file
```
