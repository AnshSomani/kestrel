# 🦅 Kestrel: High-Throughput Webhook Delivery Engine

Modern SaaS applications frequently need to notify third-party systems when business events occur (payments, orders, subscriptions, etc.). Delivering webhooks synchronously increases request latency and makes applications vulnerable to slow or failing downstream services.

Kestrel decouples webhook delivery from the request lifecycle by buffering events in PostgreSQL and asynchronously dispatching them through concurrent workers with retries, rate limiting, circuit breakers, and Dead Letter Queues.

Engineered for extreme reliability and throughput, Kestrel protects downstream APIs from thundering herds by guaranteeing zero message loss during recoverable infrastructure failures while providing configurable retry and Dead Letter Queue handling for unrecoverable endpoint failures.

---

## ✨ Features

- **Multi-tenant architecture** (JWT Auth & Data Isolation)
- **PostgreSQL SKIP LOCKED queue**
- **Concurrent worker pools** (Horizontal Scaling)
- **Retry engine**
- **Decorrelated exponential backoff**
- **Dead Letter Queue (DLQ)**
- **Redis-backed distributed circuit breaker**
- **Redis sliding-window rate limiter**
- **React Dashboard**
- **Prometheus Metrics & Grafana Dashboard**
- **Docker Compose**
- **Chaos & Stress Testing Suite**

---

## 📊 Dashboard & Observability

Kestrel ships with a real-time React dashboard and comprehensive Grafana metrics to monitor queue depth, delivery throughput, and active subscriptions.

<div align="center">
  <img src="docs/kesterl-dashboard.png" alt="Kestrel Dashboard Overview" width="800"/>
</div>

<br/>

<div align="center">
  <img src="docs/grafana.png" alt="Grafana Metrics" width="800"/>
</div>

<br/>

<div align="center">
  <img src="docs/sub.png" alt="Kestrel Subscriptions" width="800"/>
</div>

---

## 🚀 Performance Highlights

During our rigorous 14-Phase stress testing and Chaos Engineering benchmarks, Kestrel achieved:
- **1 Million Simultaneous Event Burst:** Successfully drained 1,000,000 backlogged events across 32 concurrent Go pollers with a sustained throughput of **1,344 deliveries/sec**.
- **High-Concurrency Queue Processing:** Enabled concurrent dequeuing using PostgreSQL `FOR UPDATE SKIP LOCKED` and batched polling, allowing 32 pollers to process jobs with minimal row-lock contention.
- **Infrastructure Recovery:** Successfully recovered from sequential Redis, PostgreSQL, and application crashes without dropping queued events, validating reliable recovery under infrastructure failures.

---

## 🛠 Tech Stack

| Component | Technology | Description |
| :--- | :--- | :--- |
| **Backend Engine** | Go 1.24, Echo, `pgx`, `go-redis` | Ingests webhooks, authenticates requests (JWT/API Keys), and orchestrates the highly concurrent poller pool. |
| **Primary Database** | PostgreSQL 17 | Handles multi-tenant data storage and manages the high-throughput delivery queue utilizing `SKIP LOCKED`. |
| **Coordination Layer** | Redis 8 | Maintains distributed circuit breaker states and executes sliding-window tenant rate limits. |
| **Frontend** | React, Vite, TypeScript, Recharts | Provides a real-time observability dashboard for tracking queue depth, delivery success rates, and live payloads. |
| **Observability** | Prometheus, Grafana | Exposes and visualizes custom Go metrics for monitoring throughput, latency, and worker utilization. |
| **Infrastructure** | Docker, Docker Compose | Fully containerized multi-container deployment designed for zero-cost hosting and easy chaos testing. |

---

## 🏗 System Architecture

Kestrel isolates the high-I/O overhead of outgoing HTTP webhooks away from your core SaaS product, executing them asynchronously through a highly tuned worker pool.

```mermaid
graph TD
    Client["Core SaaS App"] -->|"POST /events"| API["Kestrel API Gateway"]
    User["Developer"] -->|"Login (JWT)"| Dashboard["React Portal"]
    
    Dashboard -.->|"GET /stats"| API

    subgraph "Kestrel Engine"
        API -->|"Insert Event"| DB[("(PostgreSQL)")]
        
        DB -->|"SKIP LOCKED"| W1["Worker 1"]
        DB -->|"SKIP LOCKED"| W2["Worker 2"]
        DB -->|"SKIP LOCKED"| W3["Worker 3"]
        
        W1 <-->|"Check State"| CB[("(Redis)")]
        W2 <-->|"Check State"| CB
        W3 <-->|"Check State"| CB
    end
    
    W1 -->|"HTTP POST"| Target1["Downstream API"]
    W2 -->|"HTTP POST"| Target2["Downstream API"]
    W3 -->|"HTTP POST"| Target3["Downstream API"]
```

---

## 🛡 Advanced Reliability Features

1. **Strict Multi-Tenancy:** 
   - JWT-based authentication and secure data isolation ensures events are strictly partitioned by `user_id`.
   - Redis-backed sliding window rate limiters enforce tenant quotas, preventing noisy neighbors from starving the worker pool.
2. **Circuit Breakers:** 
   - A Redis-backed distributed circuit breaker state machine instantly trips open after 5 consecutive endpoint failures, shunting traffic back to the queue to prevent hanging Go routines on dead APIs.
3. **Slow-Loris Mitigation:**
   - Strict 100ms HTTP delivery timeouts ensure that slow-responding webhooks cannot lock up the active worker pool.
4. **Dead Letter Queue (DLQ):**
   - After exhausted attempts (utilizing decorrelated jittered backoff), unrecoverable jobs transition to a `dead` state for manual inspection.

---

## 🚦 Getting Started (Docker)

Kestrel is fully containerized. You can spin up the entire multi-tenant stack (PostgreSQL, Redis, Go Engine, React Dashboard, and a mock webhook target) instantly.

### 1. Boot the Stack
```bash
cd deployments
docker compose up --build -d
```
*This exposes the API on `:8080` and the React Dashboard on `:5173`.*

### 2. Access the Dashboard
Navigate to `http://localhost:5173`. An admin account is seeded on boot:
- **Email:** `admin@kestrel.local`
- **Password:** `password`

*(Note: Run `npm install` and `npm run dev` inside the `/web` directory if you prefer to run the frontend hot-reloader locally).*

---

## 💥 Running the Benchmarks

Kestrel ships with a comprehensive CLI benchmarking tool to validate performance on your own hardware.

**Run the 1 Million Event Stress Test:**
```bash
go run ./cmd/bench -phase million
```

**Run the Chaos Engineering Suite:**
```bash
go run ./cmd/bench -phase chaos
```
