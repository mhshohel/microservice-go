# Microservices in Go — 15 Real-World Patterns

A hands-on reference implementation of **15 production microservice patterns**, each built as a self-contained, runnable Go service. Every concept is fully tested, documented, and containerised.

> **Goal:** Learn microservice architecture by reading and running real code — not toy snippets, not framework magic. Each service is written in plain Go so the pattern is front and centre.

---

## Table of Contents

- [Overview](#overview)
- [Patterns at a Glance](#patterns-at-a-glance)
- [Tech Stack](#tech-stack)
- [Repository Layout](#repository-layout)
- [Prerequisites](#prerequisites)
- [Getting Started](#getting-started)
- [Running Individual Services](#running-individual-services)
- [Running All Services with Docker](#running-all-services-with-docker)
- [Testing](#testing)
- [Service Deep-Dives](#service-deep-dives)
  - [01 — API Gateway](#01--api-gateway)
  - [02 — Service Discovery](#02--service-discovery)
  - [03 — Load Balancing](#03--load-balancing)
  - [04 — Circuit Breaker](#04--circuit-breaker)
  - [05 — Event-Driven Communication](#05--event-driven-communication)
  - [06 — CQRS](#06--cqrs)
  - [07 — Saga Pattern](#07--saga-pattern)
  - [08 — Service Mesh](#08--service-mesh)
  - [09 — Distributed Tracing](#09--distributed-tracing)
  - [10 — Containerization](#10--containerization)
  - [11 — Database per Service](#11--database-per-service)
  - [12 — Bulkhead Pattern](#12--bulkhead-pattern)
  - [13 — Backend for Frontend (BFF)](#13--backend-for-frontend-bff)
  - [14 — Blue-Green Deployment](#14--blue-green-deployment)
  - [15 — Strangler Fig Pattern](#15--strangler-fig-pattern)
- [Architecture Principles](#architecture-principles)
- [Code Style](#code-style)

---

## Overview

This project implements 15 battle-tested microservice patterns in **Go 1.26**. Each service lives in its own directory under `services/`, has its own `README.md` explaining the concept in depth, and ships with:

- A working implementation using only the Go standard library (plus SQLite where persistence is needed)
- A full test suite covering the happy path, failure paths, and edge cases
- A multi-stage `Dockerfile`
- Beginner-friendly comments that explain *why* the code is written the way it is

The services are independent — you can read and run them in any order, though the table below lists them from foundational to advanced.

---

## Patterns at a Glance

| # | Pattern | Category | Key Concepts | Tests |
|---|---------|----------|--------------|-------|
| 01 | [API Gateway](#01--api-gateway) | Entry point | JWT auth, rate limiting, reverse proxy, path/header routing | 14 |
| 02 | [Service Discovery](#02--service-discovery) | Dynamic location | In-memory registry, heartbeat, auto-expiry, HTTP API | 24 |
| 03 | [Load Balancing](#03--load-balancing) | Traffic distribution | Round-robin, weighted RR, least-connections, IP hash, random | 17 |
| 04 | [Circuit Breaker](#04--circuit-breaker) | Fault tolerance | CLOSED → OPEN → HALF-OPEN state machine, concurrent probe | 16 |
| 05 | [Event-Driven Communication](#05--event-driven-communication) | Async messaging | Pub-sub broker, bounded message queue, pure Go channels | 12 |
| 06 | [CQRS](#06--cqrs) | Read/write separation | Write DB, denormalized read DB (SQLite), projector/event sync | 11 |
| 07 | [Saga Pattern](#07--saga-pattern) | Distributed transactions | Orchestration coordinator, compensating rollbacks | 14 |
| 08 | [Service Mesh](#08--service-mesh) | Communication layer | Sidecar proxy, mTLS simulation, retries, canary traffic split | 10 |
| 09 | [Distributed Tracing](#09--distributed-tracing) | Observability | X-Trace-ID propagation, span/trace store, HTTP middleware | 13 |
| 10 | [Containerization](#10--containerization) | Deployment | Multi-stage Dockerfile, distroless image, health/version endpoints | 6 |
| 11 | [Database per Service](#11--database-per-service) | Data isolation | Three isolated SQLite stores (user/order/product), no cross-store access | 6 |
| 12 | [Bulkhead Pattern](#12--bulkhead-pattern) | Fault isolation | Buffered-channel semaphore, blocking Execute, non-blocking TryExecute | 7 |
| 13 | [Backend for Frontend (BFF)](#13--backend-for-frontend-bff) | Client-specific APIs | Web, Mobile, and Admin response shapes from a shared data source | 12 |
| 14 | [Blue-Green Deployment](#14--blue-green-deployment) | Zero-downtime release | Two environments, smoke-test gate, atomic traffic switch, rollback | 11 |
| 15 | [Strangler Fig Pattern](#15--strangler-fig-pattern) | Legacy migration | Proxy router, incremental prefix migration, longest-prefix match, rollback | 8 |

**Total: 181 tests across 15 services.**

---

## Tech Stack

| Tool | Version | Purpose |
|------|---------|---------|
| Go | 1.26.2 | All service implementation |
| SQLite (`modernc.org/sqlite`) | latest | Embedded persistence (CQRS, DB-per-Service) |
| `github.com/google/uuid` | v1.6.0 | Trace IDs and request correlation |
| Docker | 24+ | Multi-stage builds, container images |
| Docker Compose | v2 | Running all 15 services together |

No Kubernetes, no service mesh framework, no gRPC — the patterns are shown in plain Go so the mechanics are transparent.

---

## Repository Layout

```
microservices-go/
├── go.mod                        # Single module for all services
├── go.sum
├── Makefile                      # make run SERVICE=01-api-gateway
├── docker-compose.yml            # spin up all 15 services
├── workflow.md                   # build tracker (all 15 done)
│
├── pkg/                          # Shared utilities (logging, tracing, errors)
│
└── services/
    ├── 01-api-gateway/
    │   ├── cmd/main.go
    │   ├── internal/
    │   │   ├── jwt/              # JWT generate + validate
    │   │   ├── middleware/       # auth + rate limiter
    │   │   ├── proxy/            # reverse proxy
    │   │   └── router/           # path routing + header routing
    │   ├── gateway_test.go
    │   ├── Dockerfile
    │   └── README.md
    ├── 02-service-discovery/
    ├── 03-load-balancing/
    ├── 04-circuit-breaker/
    ├── 05-event-driven/
    ├── 06-cqrs/
    ├── 07-saga/
    ├── 08-service-mesh/
    ├── 09-distributed-tracing/
    ├── 10-containerization/
    ├── 11-database-per-service/
    ├── 12-bulkhead/
    ├── 13-bff/
    ├── 14-blue-green/
    └── 15-strangler-fig/
```

Each service follows the same layout:

```
services/XX-name/
├── cmd/main.go          # entry point — start the service
├── internal/            # service-private packages
│   ├── handler/         # HTTP handlers
│   ├── service/         # business / pattern logic
│   └── repository/      # data access (where needed)
├── XX_test.go           # full test suite
├── Dockerfile           # multi-stage build
└── README.md            # concept explanation + diagrams + run instructions
```

---

## Prerequisites

- **Go 1.26+** — `go version`
- **Docker** (optional, for containerised runs) — `docker version`
- **Docker Compose v2** (optional) — `docker compose version`

No external databases, message brokers, or cloud accounts required. Everything runs locally.

---

## Getting Started

```bash
# Clone the repository
git clone https://github.com/shohelshamim/microservice-go.git
cd microservice-go

# Verify the build — all 15 services must compile
go build ./...

# Run all tests
go test ./...

# Vet for common mistakes
go vet ./...
```

---

## Running Individual Services

```bash
# Run a service directly
go run ./services/01-api-gateway/cmd/main.go

# Or use the Makefile shorthand
make run SERVICE=01-api-gateway
```

Each service's `README.md` lists the exact `curl` commands to exercise it.

**Quick example — API Gateway:**

```bash
# Terminal 1: start the gateway
go run ./services/01-api-gateway/cmd/main.go

# Terminal 2: get a demo token, then make an authenticated request
TOKEN=$(curl -s http://localhost:8080/token)
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/users
```

---

## Running All Services with Docker

```bash
# Build and start all 15 services
docker compose up --build

# Run in the background
docker compose up --build -d

# Tear everything down
docker compose down
```

---

## Testing

Every service has a test file that covers:

- **Core pattern behaviour** — the thing being demonstrated
- **Happy path** — normal operation
- **Failure path** — what happens when something goes wrong
- **Edge cases** — concurrent access, boundary conditions

```bash
# Test all services
go test ./... -v -race

# Test one service
go test ./services/04-circuit-breaker/... -v -race

# Test a specific case
go test ./services/03-load-balancing/... -run TestWeightedRoundRobin -v
```

The `-race` flag is important — several services use goroutines and channels, and the race detector catches data races in concurrent pattern implementations.

---

## Service Deep-Dives

### 01 — API Gateway

**Pattern:** Single entry point for all client traffic.

The gateway enforces three layers before forwarding a request:

```
Client → Rate Limiter (100 req/min) → JWT Auth → Router → Reverse Proxy → Backend Service
```

**Key implementation details:**
- JWT is generated and validated with HMAC-SHA256, no third-party library
- Rate limiter uses a token bucket keyed by client IP
- Two routing strategies: by URL path prefix (`/api/users/` → `:9001`) and by `X-Service` header
- The reverse proxy strips the prefix and forwards headers including a `X-Trace-ID`

**Run it:**
```bash
go run ./services/01-api-gateway/cmd/main.go
curl http://localhost:8080/token          # get a demo JWT
curl -H "Authorization: Bearer <token>" http://localhost:8080/api/users
curl http://localhost:8080/health
```

---

### 02 — Service Discovery

**Pattern:** Services register themselves; other services look them up by name instead of hard-coded address.

```
Service A starts → POST /register (name, address, port)
                              ↓
                     In-memory registry
                              ↑
Service B needs A → GET /lookup?name=service-a → [address:port, ...]
```

**Key implementation details:**
- Registry is an in-memory map protected by a `sync.RWMutex`
- Services send a `POST /heartbeat` every N seconds; entries without a recent heartbeat expire automatically
- Supports multiple instances per service name (returns a list)
- HTTP API: `POST /register`, `DELETE /deregister`, `POST /heartbeat`, `GET /lookup`, `GET /services`

**Run it:**
```bash
go run ./services/02-service-discovery/cmd/main.go
curl -X POST http://localhost:8082/register \
  -d '{"name":"my-service","address":"localhost","port":9001}'
curl "http://localhost:8082/lookup?name=my-service"
```

---

### 03 — Load Balancing

**Pattern:** Distribute incoming requests across multiple backend instances to avoid overloading any single one.

Five algorithms are implemented — each in its own file:

| Algorithm | Best for |
|-----------|---------|
| Round-robin | Equal servers, stateless requests |
| Weighted round-robin | Servers with different capacities |
| Least connections | Long-lived or variable-duration requests |
| IP hash | Session affinity (same client → same server) |
| Random | Simple, low-overhead distribution |

**Key implementation details:**
- A common `Balancer` interface with `Next(clientIP string) (*Backend, error)` means the load balancer can be swapped without changing the caller
- Backends track active connection counts (used by least-connections)
- Unhealthy backends are skipped automatically

---

### 04 — Circuit Breaker

**Pattern:** Stop calling a failing downstream service so it has time to recover, and return fast errors instead of waiting for timeouts.

```
CLOSED ──(failures >= threshold)──► OPEN ──(timeout elapsed)──► HALF-OPEN
  ▲                                                                  │
  └────────────(probe succeeds)────────────────────────────────────-┘
                                       │ probe fails
                                       ▼
                                     OPEN (reset timer)
```

**Key implementation details:**
- State is stored in a struct with a `sync.Mutex` to handle concurrent requests safely
- `RecordSuccess()` and `RecordFailure()` drive state transitions
- HALF-OPEN allows exactly one probe call; subsequent concurrent calls are rejected until the probe resolves
- `Execute(fn func() error)` wraps any function call with circuit breaker protection

---

### 05 — Event-Driven Communication

**Pattern:** Services communicate asynchronously by publishing events to a broker; subscribers react when relevant events arrive.

Two messaging primitives are implemented:

**Pub-Sub broker** — fan-out to all subscribers:
```
Publisher → Broker.Publish("user.created", event)
                 ├──► Subscriber A (email service)
                 └──► Subscriber B (analytics service)
```

**Message queue** — single consumer per message:
```
Producer → Queue.Enqueue(job) → Queue.Dequeue() → Worker
```

**Key implementation details:**
- Both primitives are built on buffered Go channels — no external broker required
- The pub-sub broker is safe for concurrent publishers and subscribers
- Queues have a configurable capacity; `Enqueue` blocks when full (back-pressure)

---

### 06 — CQRS

**Pattern:** Separate the write model (commands) from the read model (queries). Writes update the authoritative store; a projector syncs changes to a denormalised read store optimised for queries.

```
Command (CreateProduct) → Write Handler → Write DB (SQLite)
                                              │
                                          Projector (event)
                                              │
                                              ▼
Query (GetProduct)     ← Read Handler  ← Read DB (SQLite, denormalised)
```

**Key implementation details:**
- Two separate SQLite databases — the write DB is normalised, the read DB is a flat projection
- An in-memory event channel connects the write handler to the projector
- The projector runs in its own goroutine, consuming events and updating the read DB

---

### 07 — Saga Pattern

**Pattern:** Coordinate a multi-step distributed transaction where each step has a compensating action that reverses it if a later step fails.

**Order placement saga (Payment → Inventory → Shipping):**

```
Step 1: Charge payment       ← compensate: Refund payment
Step 2: Reserve inventory    ← compensate: Release inventory
Step 3: Create shipment      ← compensate: Cancel shipment

If step 3 fails:
  → Cancel shipment  (step 3 compensation)
  → Release inventory (step 2 compensation)
  → Refund payment   (step 1 compensation)
```

**Key implementation details:**
- Orchestration pattern — a central `Saga` struct controls the sequence and compensation
- Each step is defined as `{Action, Compensation}` function pairs
- Steps are executed in order; on failure, compensations run in reverse
- The orchestrator records which steps completed so it only compensates what ran

---

### 08 — Service Mesh

**Pattern:** Offload cross-cutting network concerns (retries, mTLS, traffic splitting) to a sidecar proxy that runs alongside every service, so the service itself stays simple.

```
Service A → Sidecar Proxy A ──(mTLS)──► Sidecar Proxy B → Service B
                │
                ├── Retry failed requests (up to N times)
                ├── Verify caller identity (simulated mTLS)
                └── Split traffic (canary: 10% new, 90% stable)
```

**Key implementation details:**
- Sidecar wraps outbound calls with retry logic (exponential-ish backoff)
- Identity verification simulates mTLS by checking a pre-shared service identity header
- Canary traffic splitting uses weighted random selection between two backend pools

---

### 09 — Distributed Tracing

**Pattern:** Assign a unique trace ID to every incoming request and propagate it through every downstream call, so you can reconstruct the full journey of a request across services.

```
Request arrives → Middleware generates Trace-ID (or reads X-Trace-ID header)
                        │
              ┌─────────▼────────┐
              │  Start Span A    │ ← record start time, service name
              │    ┌─────────────▼──────────┐
              │    │  Start Span B (child)  │ ← downstream call
              │    │  End Span B            │
              │    └────────────────────────┘
              │  End Span A      │
              └──────────────────┘
                        │
               Trace Store (in-memory)
                        │
              GET /traces/:traceID  → full span tree
```

**Key implementation details:**
- `X-Trace-ID` header carries the trace ID across HTTP boundaries
- Each span records service name, operation, start/end time, parent span ID
- An in-memory store collects spans; `GET /traces/:id` returns the reconstructed trace

---

### 10 — Containerization

**Pattern:** Package a service and all its dependencies into a container image so it runs identically in development, CI, and production.

The Dockerfile is commented line-by-line to explain every decision:

```dockerfile
# Stage 1 — builder: full Go toolchain, compiles the binary
FROM golang:1.26-alpine AS builder

# Stage 2 — runtime: minimal image, only the compiled binary
FROM gcr.io/distroless/static-debian12
COPY --from=builder /app/service /service
ENTRYPOINT ["/service"]
```

**Key implementation details:**
- Multi-stage build keeps the final image small (no compiler, no source)
- Distroless base image has no shell — reduces attack surface
- `GET /health` and `GET /version` endpoints for container orchestration

---

### 11 — Database per Service

**Pattern:** Each service owns its own database and schema. No service may directly query another service's database — it must go through the other service's API.

```
User Service    ──► users.db    (SQLite)
Order Service   ──► orders.db   (SQLite)
Product Service ──► products.db (SQLite)

✗  Order Service SELECT * FROM users.db   — FORBIDDEN
✓  Order Service GET /users/:id            — via HTTP API
```

**Key implementation details:**
- Three completely separate SQLite files, opened independently
- Each service package (`internal/user`, `internal/order`, `internal/product`) has no imports from the others
- The demo shows cross-service data assembly via HTTP calls, not DB joins

---

### 12 — Bulkhead Pattern

**Pattern:** Isolate resources (goroutine pools, connection pools) so that a failure or overload in one area cannot consume resources needed by other areas.

Named after watertight compartments in a ship's hull — if one compartment floods, the others stay dry.

```
Request for Service A → Bulkhead A (max 5 concurrent) ─► Service A
Request for Service B → Bulkhead B (max 5 concurrent) ─► Service B

If Service A is overwhelmed:
  → Bulkhead A rejects new requests to Service A
  → Bulkhead B still accepts requests to Service B  ← isolation works
```

**Key implementation details:**
- Implemented as a buffered channel used as a semaphore
- `Execute(fn)` — blocks until a slot is free, then runs `fn`, then releases the slot
- `TryExecute(fn)` — returns `ErrBulkheadFull` immediately if no slot is available (non-blocking)
- Each dependency gets its own `Bulkhead` instance with its own capacity

---

### 13 — Backend for Frontend (BFF)

**Pattern:** Instead of one generic API, build separate backend services tailored to the needs of each frontend client (web, mobile, admin).

```
                         ┌──────────────────────┐
                         │   Shared Data Source  │
                         └──────────────────────┘
                               │        │
                    ┌──────────┘        └─────────┐
                    ▼                             ▼
            ┌──────────────┐           ┌──────────────┐
            │   Web BFF    │           │  Mobile BFF  │
            │  7 fields    │           │  3 fields    │
            │  (rich data) │           │  (minimal)   │
            └──────────────┘           └──────────────┘
                                             │
                                    ┌────────────────┐
                                    │   Admin BFF    │
                                    │  aggregated    │
                                    │  stats + audit │
                                    └────────────────┘
```

**Key implementation details:**
- Three separate HTTP servers on different ports, each with their own response struct
- Web BFF returns full product detail (name, description, price, stock, images, reviews, metadata)
- Mobile BFF returns minimal payload (name, price, thumbnail) to save mobile bandwidth
- Admin BFF aggregates statistics and includes audit fields not exposed to end users

---

### 14 — Blue-Green Deployment

**Pattern:** Run two identical production environments (Blue = current, Green = new). Deploy the new version to Green, run smoke tests, then atomically switch traffic. If anything fails, switch back to Blue in seconds.

```
Before deploy:  100% traffic → Blue (v1.0)

Deploy:         Green gets new code (v2.0), smoke tests run

Switch:         100% traffic → Green (v2.0)   ← atomic, ~0ms downtime

Rollback:       100% traffic → Blue (v1.0)    ← instant if needed
```

**Key implementation details:**
- An `atomic.Value` holds the pointer to the active environment, making the switch lock-free
- Smoke tests run against the inactive environment before the switch — traffic only flips if they pass
- The router's `ServeHTTP` reads the active pointer on every request, so mid-flight requests are not affected
- Both environments stay warm (running), so rollback is immediate

---

### 15 — Strangler Fig Pattern

**Pattern:** Gradually migrate a legacy system to a new implementation by routing traffic for specific features to the new system while the rest still goes to the legacy system. Over time, the legacy system is "strangled" as more routes are migrated.

```
Client → Strangler Router
              ├── /api/users/*   → New Service   (migrated)
              ├── /api/products/* → New Service  (migrated)
              └── /*             → Legacy System (not yet migrated)
```

**Key implementation details:**
- Longest-prefix match ensures `/api/users/v2` routes correctly even when `/api/users` and `/api` are both registered
- Routes can be added and removed at runtime without restarting the proxy
- Rollback: deregister a route prefix to send traffic back to legacy instantly
- Proxy transparently forwards headers, body, and status code

---

## Architecture Principles

These rules are enforced across all 15 services:

| Principle | Rule |
|-----------|------|
| **Database per Service** | Never share a DB connection across service boundaries |
| **No internal cross-imports** | `services/A` must not import `services/B/internal/...` |
| **Async for events** | Use message broker or channels, not direct HTTP, for event-driven patterns |
| **Config via env vars** | Each service reads its own env; required vars use `mustEnv()` |
| **Trace ID on every request** | Propagate via `X-Trace-ID` header; include in all `slog` log calls |
| **Health endpoint** | Every service exposes `GET /health` returning `{"status":"ok"}` |

---

## Code Style

This project is intentionally beginner-friendly. Every service is written so someone new to Go *and* new to the pattern can follow along.

- **Comments explain WHY and HOW**, not what — the code speaks for itself, comments add context a beginner would not have
- **Every struct, function, and concurrency construct is commented**
- **Flat, linear code** — no clever one-liners, no deep abstraction chains
- **Pattern fidelity** — each service demonstrates exactly its named pattern, nothing more

Example:

```go
// Circuit breaker starts in CLOSED state, meaning requests pass through normally.
// We track consecutive failures; once they exceed the threshold, we OPEN the breaker
// and stop forwarding requests so the broken service gets time to recover.
state := StateClosed

// We use a mutex here because multiple goroutines may check and update the state
// at the same time (concurrent HTTP requests). Without it, state reads/writes
// would be a data race.
var mu sync.Mutex
```

---

## License

MIT — free to use, study, and adapt.