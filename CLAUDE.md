# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Go 1.26.2 microservices project demonstrating 15 real-world patterns from `Microservice-Concepts.pdf`. Each concept becomes its own runnable service example. Module: `microservices-go`.

Use the `/go` skill (`.claude/skills/go/SKILL.md`) for Go 1.26.2 idioms, stdlib usage, and patterns. Track active work in `workflow.md`.

---

## Workflow → Agents → Tools

All development follows a three-layer model:

```
Workflow  — the goal (e.g., "build Circuit Breaker service")
  └── Agent  — Claude Code executing a workflow step autonomously
        └── Tools — file edits, bash commands, test runs
```

- **Workflow** lives in `workflow.md` — one entry per microservice concept, with status and current task
- **Agent** is Claude Code operating within a workflow step — it plans, implements, tests, and marks the step done
- **Tools** are the concrete actions: `go build`, `go test`, file edits, docker commands

When starting any implementation task: check `workflow.md` first, update status to `in_progress`, then proceed. Mark `done` when tests pass.

---

## The 15 Microservice Concepts (build order)

| # | Concept | Pattern Type | Go Focus |
|---|---------|-------------|----------|
| 1 | API Gateway | Entry point, routing | `net/http` reverse proxy, JWT middleware |
| 2 | Service Discovery | Dynamic service location | DNS, health checks, registry client |
| 3 | Load Balancing | Distribute requests | Round-robin, least-conn client-side LB |
| 4 | Circuit Breaker | Fault tolerance | State machine: CLOSED → OPEN → HALF-OPEN |
| 5 | Event-Driven Communication | Async inter-service | RabbitMQ/NATS pub-sub, message queues |
| 6 | CQRS | Separate read/write | Dual DB handlers, event bus sync |
| 7 | Saga Pattern | Distributed transactions | Orchestration coordinator, compensations |
| 8 | Service Mesh | Communication layer | Sidecar proxy concept, mTLS |
| 9 | Distributed Tracing | Request tracking | `slog` trace IDs, OpenTelemetry spans |
| 10 | Containerization | Lightweight deployment | Multi-stage Dockerfile, distroless images |
| 11 | Database per Service | Independent data stores | Each service owns its schema/connection |
| 12 | Bulkhead Pattern | Fault isolation | Separate goroutine pools per dependency |
| 13 | Backend for Frontend (BFF) | Client-specific services | Web/Mobile/Admin response shaping |
| 14 | Blue-Green Deployment | Zero-downtime releases | Traffic switching, health gate |
| 15 | Strangler Fig Pattern | Legacy modernization | Router proxy, incremental extraction |

---

## Repository Layout

```
services/
  01-api-gateway/
  02-service-discovery/
  03-load-balancing/
  ...
  15-strangler-fig/
    cmd/main.go
    internal/
      handler/      # HTTP handlers
      service/      # business logic
      repository/   # data access (if needed)
    config/
    Dockerfile
pkg/                # shared: tracing, logging, middleware, errors
docker-compose.yml  # spin up all services together
Makefile
workflow.md         # active workflow and task tracker
```

Each service is self-contained: its own `go.work` workspace is **not** used — all services share the root `go.mod`. Shared code lives in `pkg/` only.

---

## Commands

```bash
go build ./...                                      # build all
go test ./...                                       # test all
go test ./services/01-api-gateway/... -run TestX -v # single test
go vet ./...                                        # vet
make run SERVICE=01-api-gateway                     # run one service
docker compose up                                   # run all
```

---

## Code Style — Beginner-Friendly

This project is educational. Every service must be readable by someone new to Go and new to the pattern being demonstrated.

### Simplicity first
- Write the **simplest code that correctly shows the pattern** — no premature abstractions
- Prefer flat, linear code over clever one-liners
- Use short, descriptive variable names: `userID` not `uid`, `server` not `srv`
- One concept per file where possible — keep files focused and short

### Comments are required
Write a comment whenever the WHY or HOW is not immediately obvious to a beginner:

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

**Always comment:**
- The top of every file: one sentence explaining what this file does and why it exists
- Every struct: what it represents and its role in the pattern
- Every non-trivial function: what it does, what its parameters mean, what it returns
- Any concurrency (goroutines, channels, mutexes): explain why concurrency is needed here
- Any pattern-specific logic (e.g., state transitions, compensation steps, retry logic)

**Example — good file header:**
```go
// handler.go — HTTP handler for the Circuit Breaker service.
// It wraps outgoing calls to a downstream service with a circuit breaker,
// so if that service keeps failing, we stop hammering it and return a friendly error.
```

### Pattern fidelity
Each service must clearly demonstrate its named pattern — do not mix in unrelated patterns:
- The Circuit Breaker service demonstrates fault tolerance with state transitions — keep that front and centre
- The Saga service demonstrates compensating transactions — show the rollback path clearly
- The BFF service demonstrates response shaping per client — show at least two different response shapes

### No magic — make it visible
- Do not hide the pattern inside a third-party library if you can show it with plain Go
- When a library is used (e.g., NATS for messaging), show the usage clearly with comments explaining what the library call does

---

## Testing Requirements

Every service **must have tests**. Tests are not optional.

### What to test
- The core pattern behaviour — this is the most important test
- Happy path (normal operation)
- Failure path (what happens when something goes wrong)
- Edge cases specific to the pattern

### Pattern-specific test examples

| Concept | Must test |
|---------|-----------|
| API Gateway | Request is proxied to correct downstream; invalid token is rejected |
| Circuit Breaker | Breaker opens after N failures; half-open allows one probe; closes on success |
| Load Balancer | Requests distribute across instances; failed instance is skipped |
| Saga | All steps succeed → committed; step 2 fails → step 1 compensation runs |
| CQRS | Write goes to write model; read comes from read model; sync happens |
| Bulkhead | Pool A failure does not block Pool B requests |
| Event-Driven | Published event is received by all subscribers |

### Test structure
```go
func TestCircuitBreaker_OpensAfterFailures(t *testing.T) {
    // Arrange — set up the breaker with a low threshold so the test runs fast
    cb := NewCircuitBreaker(CircuitBreakerConfig{
        FailureThreshold: 3,
        Timeout:          100 * time.Millisecond,
    })

    // Act — simulate failures until the breaker should trip
    for i := 0; i < 3; i++ {
        cb.RecordFailure()
    }

    // Assert — breaker must now be OPEN and reject calls
    if cb.State() != StateOpen {
        t.Fatalf("expected OPEN state after 3 failures, got %s", cb.State())
    }
}
```

Run before marking any concept `done`:
```bash
go test ./services/XX-name/... -v -race
```

The `-race` flag catches data races in concurrent pattern implementations.

---

## Architecture Rules

- **Database per Service**: never share a DB connection across service boundaries
- **No cross-service internal imports**: `services/A` must not import `services/B/internal/...`
- **Async by default for events**: use message broker (NATS/RabbitMQ), not direct HTTP, for event-driven concepts
- **Config via env vars**: each service reads its own env; use `mustEnv()` for required vars
- **Trace ID on every request**: propagate via `X-Trace-ID` header and include in all `slog` calls
- **Health endpoint**: every service exposes `GET /health` returning `{"status":"ok"}`
