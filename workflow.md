# Workflow

Active workflow tracker for the microservices-go project.  
Update status here before starting any implementation and when completing a step.

**Statuses**: `todo` | `in_progress` | `done` | `blocked`

---

## Current Task

> **All 15 services complete.** Project done.

---

## Microservice Concepts — Build Queue

| # | Service | Status | Notes |
|---|---------|--------|-------|
| 1 | API Gateway | `done` | Single entry point; JWT auth middleware; reverse proxy to downstream services |
| 2 | Service Discovery | `done` | In-memory registry; Register/Deregister/Heartbeat/Query HTTP API; auto-expiry |
| 3 | Load Balancing | `done` | 5 algorithms: round-robin, weighted RR, least-conn, IP hash, random; 17 tests pass |
| 4 | Circuit Breaker | `done` | CLOSED/OPEN/HALF-OPEN state machine; concurrent probe; 16 tests pass |
| 5 | Event-Driven Communication | `done` | Pub-sub broker + message queue using pure Go channels; 12 tests pass |
| 6 | CQRS | `done` | Write DB + read DB (SQLite); projector syncs via events; 11 tests pass |
| 7 | Saga Pattern | `done` | Orchestration saga: Payment→Inventory→Shipping with compensating rollbacks; 14 tests pass |
| 8 | Service Mesh | `done` | Sidecar proxy; identity verification; retries; traffic splitting; 10 tests pass |
| 9 | Distributed Tracing | `done` | Trace ID via X-Trace-ID; spans; in-memory store; HTTP middleware propagation; 13 tests |
| 10 | Containerization | `done` | Multi-stage Dockerfile; commented best practices; HTTP handlers; 6 tests pass |
| 11 | Database per Service | `done` | Three isolated SQLite stores (user/order/product); no cross-store access; 6 tests pass |
| 12 | Bulkhead Pattern | `done` | Buffered-channel semaphore; Execute (blocking) + TryExecute (non-blocking); 7 tests pass |
| 13 | Backend for Frontend (BFF) | `done` | Web/Mobile/Admin BFFs with tailored response shapes; 9 tests pass |
| 14 | Blue-Green Deployment | `done` | Two envs; smoke-test gate; atomic traffic switch; rollback path; 11 tests pass |
| 15 | Strangler Fig Pattern | `done` | Proxy router; incremental prefix migration; rollback; longest-prefix match; 11 tests pass |

---

## Workflow Steps (per concept)

When an agent picks up a concept, follow these steps in order:

1. **Plan** — Update `Current Task` above; outline service responsibilities and key interfaces
2. **Scaffold** — Create `services/XX-name/` directory structure
3. **Implement** — Build `cmd/main.go`, handlers, service logic, config
4. **Test** — Write tests; `go test ./services/XX-name/...` must pass
5. **Dockerfile** — Add multi-stage build
6. **Integrate** — Add to `docker-compose.yml` and `Makefile`
7. **Done** — Mark status `done` in table above; clear `Current Task`

---

## Completed

| # | Service | Notes |
|---|---------|-------|
| 1 | API Gateway | JWT auth + rate limiter + path/header routing + reverse proxy; 14 tests pass |
| 2 | Service Discovery | In-memory registry + heartbeat expiry + HTTP API; 23 tests pass |
| 3 | Load Balancing | 5 algorithms (round-robin, weighted RR, least-conn, IP hash, random); 17 tests pass |
| 4 | Circuit Breaker | CLOSED/OPEN/HALF-OPEN state machine; concurrent probe handling; 16 tests pass |
| 5 | Event-Driven Communication | Pub-sub broker + bounded message queue; pure Go channels; 12 tests pass |
| 6 | CQRS | Write DB + denormalized read DB (SQLite); projector syncs via events; 11 tests pass |
| 7 | Saga Pattern | Orchestration saga: Payment→Inventory→Shipping; compensating rollbacks; 14 tests pass |
| 8 | Service Mesh | Sidecar proxy; identity verification (mTLS sim); retries; canary split; 10 tests pass |
| 9 | Distributed Tracing | X-Trace-ID propagation; span/trace store; HTTP middleware; end-to-end; 13 tests pass |
| 10 | Containerization | Multi-stage Dockerfile with detailed comments; HTTP health/version handlers; 6 tests pass |
| 11 | Database per Service | Three isolated SQLite stores (user/order/product); no cross-store access; 6 tests pass |
| 12 | Bulkhead Pattern | Buffered-channel semaphore; Execute (blocking) + TryExecute (non-blocking); 7 tests pass |
| 13 | Backend for Frontend (BFF) | Web (7 fields), Mobile (3 fields), Admin (aggregated stats) BFFs; 9 tests pass |
| 14 | Blue-Green Deployment | Two envs; smoke-test gate; atomic traffic switch; rollback path; 11 tests pass |
| 15 | Strangler Fig Pattern | Proxy router; incremental prefix migration; rollback; longest-prefix match; 11 tests pass |
