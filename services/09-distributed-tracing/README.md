# 09 - Distributed Tracing

## What is Distributed Tracing?

When a request flows through multiple services, something goes wrong and
you need to debug it — how do you find where it slowed down or failed?

Logs from Service A, B, and C are all mixed together. You can't tell which
log lines belong to YOUR request.

**Distributed Tracing** solves this by attaching a unique **Trace ID** to every
request at the entry point (the API Gateway). Every service that handles the
request uses the same Trace ID in its logs and records a **Span** — a timed
record of work done.

At the end, you can see the entire request journey: which services were called,
in what order, and how long each one took.

---

## Key Concepts

```
  Request enters → TraceID generated: "abc-123"
  │
  ├── [Span: api-gateway]    0ms → 45ms  (total request handling)
  │     │
  │     ├── [Span: user-svc]   2ms → 15ms  (DB lookup for user)
  │     │
  │     └── [Span: order-svc] 20ms → 40ms  (create order)
  │           │
  │           └── [Span: payment-svc] 22ms → 38ms (charge card)
```

### Trace
A trace is the complete record of one end-to-end request. It has a unique **Trace ID**.

### Span
A span records one unit of work within a trace:
- Which service did it (service name)
- What operation (e.g., "DB query", "HTTP call to user-svc")
- When it started and how long it took
- Whether it succeeded or failed

### Context Propagation
The Trace ID is passed through every HTTP call via a standard header: `X-Trace-ID`.
Each service reads the header on incoming requests and sets it on outgoing calls.

---

## ASCII Diagram

```
  Client ──── HTTP Request ────► API Gateway
                                    │
                       generates TraceID: "abc-123"
                       starts root span
                                    │
                   ┌────────────────┴──────────────────┐
                   │ X-Trace-ID: abc-123               │
                   ▼                                   ▼
              User Service                       Order Service
           starts child span               starts child span
           reads user from DB              creates order
           records span                   calls payment
                                               │
                                    X-Trace-ID: abc-123
                                               ▼
                                      Payment Service
                                   starts child span
                                   charges card
                                   records span
```

---

## File Structure

```
09-distributed-tracing/
├── README.md
├── cmd/
│   └── main.go                         ← demo: 3-hop request with trace propagation
├── internal/
│   └── tracer/
│       └── tracer.go                   ← Span, Trace, Tracer, middleware, HTTP propagation
└── tracing_test.go                     ← tests for span creation, propagation, middleware
```
