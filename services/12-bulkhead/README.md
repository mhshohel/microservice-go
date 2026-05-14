# Service 12 — Bulkhead Pattern

## What is the Bulkhead Pattern?

The name comes from ships. A ship's hull is divided into watertight **bulkheads** — separate compartments. If one compartment floods, the water cannot spread to the others, so the ship stays afloat.

In software, a bulkhead isolates resources per dependency. Instead of one shared pool of goroutines (or threads) for all downstream calls, each dependency gets its own fixed-size pool. If calls to one dependency slow down and fill their pool, they cannot overflow into the pools reserved for other dependencies.

## The Problem Without Bulkheads

Imagine a service that calls both `user-svc` and `payment-svc`. Without bulkheads, all requests share the same goroutine capacity:

```
Incoming requests
        |
        v
  [shared goroutine pool — 8 slots]
   slot 1 → user-svc call
   slot 2 → user-svc call
   slot 3 → payment-svc call
   slot 4 → user-svc call
   slot 5 → payment-svc call
   slot 6 → user-svc call
   slot 7 → user-svc call
   slot 8 → user-svc call   ← FULL!
```

If `payment-svc` becomes slow, its calls pile up and eventually fill all 8 slots. Now `user-svc` calls — which are perfectly fine — are also blocked. One slow dependency has cascaded into a full outage.

## The Solution: Separate Pools

With bulkheads, each dependency gets its own compartment:

```
Incoming requests
        |
   +----|----+
   |         |
   v         v
[user-pool] [payment-pool]
  5 slots      3 slots
  slot 1      slot 1
  slot 2      slot 2
  slot 3      slot 3
  slot 4      (full!)
  slot 5
```

If `payment-svc` is slow and fills its 3-slot pool, new payment calls are **immediately rejected** with an error. But the `user-pool` (5 slots) is completely unaffected — user calls continue to work normally.

## ASCII Diagram

```
                   ┌──────────────────────────────────────────┐
                   │           Bulkhead Service               │
                   │                                          │
  GET /api/users   │  ┌─────────────────────────────────┐    │
  ─────────────►   │  │  user-bulkhead  (max 5 slots)   │    │
                   │  │                                  │    │
                   │  │  [■][■][ ][ ][ ]  ← 2 in-flight │    │
                   │  └─────────────────────────────────┘    │
                   │                                          │
  GET /api/payments│  ┌─────────────────────────────────┐    │
  ─────────────►   │  │ payment-bulkhead (max 3 slots)  │    │
                   │  │                                  │    │
                   │  │  [■][■][■]  ← FULL, reject!     │    │
                   │  └─────────────────────────────────┘    │
                   └──────────────────────────────────────────┘

  ■ = occupied slot (in-flight request)
  □ = free slot
```

## Without Bulkhead vs With Bulkhead

| Scenario                        | Without Bulkhead              | With Bulkhead                    |
|---------------------------------|-------------------------------|----------------------------------|
| payment-svc is slow             | Goroutines pile up            | Payment pool fills up only       |
| payment-svc fills capacity      | user-svc calls also block     | user-svc calls unaffected        |
| Cascade failure                 | Yes — one dep kills all       | No — isolated per compartment    |
| Slow dependency detection       | Hard (mixed metrics)          | Easy (per-pool metrics)          |
| Graceful degradation            | Hard                          | Natural — just reject that pool  |

## How It Works in Go

The bulkhead is implemented with a **buffered channel as a counting semaphore**:

```go
// A buffered channel of size 5 acts as a pool with 5 slots.
// Sending a value "acquires" a slot; receiving "releases" it.
sem := make(chan struct{}, 5)
```

- `Execute` blocks until a slot is available (useful for queuing limited work)
- `TryExecute` returns immediately with `ErrBulkheadFull` if all slots are taken

## Running the Demo

```bash
# Start the server
go run ./services/12-bulkhead/cmd/main.go

# User API (5-slot pool)
curl http://localhost:8084/api/users

# Payment API (3-slot pool, artificially slow)
curl http://localhost:8084/api/payments

# Stats for both bulkheads
curl http://localhost:8084/stats
```

## Running Tests

```bash
go test -v -race ./services/12-bulkhead/...
```
