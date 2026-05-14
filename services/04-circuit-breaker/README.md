# 04 - Circuit Breaker

## What is a Circuit Breaker?

When Service A calls Service B, Service B might fail or slow down.
Without protection, Service A will keep retrying failed calls,
waste resources waiting, and eventually slow down or crash itself
— a "cascade failure" that takes down the whole system.

A **Circuit Breaker** sits in front of those outgoing calls.
It watches for failures and when too many happen, it "opens the circuit" —
just like a fuse in your home's electrical panel.

Once open, instead of making real calls to the failing service,
it immediately returns an error. This:
- Stops wasting resources on calls that will fail anyway
- Gives the downstream service time to recover
- Prevents cascade failures spreading through the system

---

## The Three States

```
                    ┌─────────────────────────────────────────────────┐
                    │                                                  │
          failures >= threshold                              timeout elapsed
                    │                                                  │
                    ▼                                                  │
         ┌─────────────────┐                             ┌────────────┴──────┐
         │                 │                             │                   │
         │     CLOSED      │                             │    HALF-OPEN      │
         │   (normal)      │                             │   (testing)       │
         │                 │                             │                   │
         │  calls pass     │                             │  1 probe call     │
         │  through        │                             │  passes through   │
         └────────┬────────┘                             └─────────┬─────────┘
                  │                                                │
                  │                                    ┌───────────┴──────────┐
                  │                              probe │ success?             │ failure?
                  │                                    ▼                      ▼
                  │                             reset to CLOSED        stay OPEN
                  │                                                           │
                  └─────────────────────────────────── OPEN ─────────────────┘
                                                    │
                                          immediately returns error
                                          (no real call made)
```

### CLOSED (normal operation)
- All calls pass through to the real service
- Failures are counted
- When failures reach the threshold → switch to **OPEN**

### OPEN (circuit tripped)
- All calls immediately return an error (no real calls made)
- After a timeout period → switch to **HALF-OPEN** to test recovery

### HALF-OPEN (testing recovery)
- One probe call is allowed through
- If it **succeeds** → reset counter, switch back to **CLOSED**
- If it **fails** → switch back to **OPEN** for another timeout period

---

## Why This Matters

Without circuit breaker — cascade failure:

```
Client → API Gateway → [User Service → Payment Service → FAILING DB]
                         ↓ timeout after 30s
                         ↓ retry
                         ↓ timeout again
                         ↓ thread pool exhausted
                         ↓ API Gateway starts timing out too
                         ↓ Client gets errors everywhere
```

With circuit breaker — fast failure:

```
Client → API Gateway → [User Service → CB → OPEN → immediate error]
                         ↓ 1ms error response
                         ↓ "payment service unavailable"
                         ↓ API Gateway still works fine
                         ↓ Other services unaffected
```

---

## Configuration

| Parameter          | Description                              | Example |
|--------------------|------------------------------------------|---------|
| `maxFailures`      | How many failures trigger OPEN state     | 5       |
| `timeout`          | How long to stay OPEN before HALF-OPEN   | 60s     |

---

## How to Run

```bash
go run ./services/04-circuit-breaker/cmd/main.go
```

The demo starts on `:8083`. It simulates a flaky backend:

```bash
# Normal call (backend succeeds)
curl http://localhost:8083/call

# Force backend to fail (fills the failure counter)
curl http://localhost:8083/fail

# Check circuit state
curl http://localhost:8083/state

# Reset to CLOSED manually
curl -X POST http://localhost:8083/reset
```

---

## File Structure

```
04-circuit-breaker/
├── README.md
├── cmd/
│   └── main.go                     ← demo: flaky backend + circuit breaker in front
├── internal/
│   └── breaker/
│       └── circuit_breaker.go      ← CLOSED/OPEN/HALF-OPEN state machine
└── circuit_breaker_test.go         ← tests for state transitions and concurrency
```
