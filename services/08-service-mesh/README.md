# 08 - Service Mesh

## What is a Service Mesh?

A **Service Mesh** is an infrastructure layer that handles service-to-service
communication automatically — without any code changes in the services themselves.

Instead of each service implementing its own:
- Retries
- Circuit breaking
- TLS encryption
- Observability (metrics, tracing)
- Traffic routing

A service mesh moves all that logic into a **sidecar proxy** — a helper process
that runs alongside each service, intercepting all its network traffic.

---

## The Sidecar Pattern

```
  ┌───────────────────────────────────────────────────────┐
  │  Pod / Container Group                                │
  │                                                       │
  │  ┌─────────────────┐       ┌─────────────────┐       │
  │  │   Your Service  │ ──────► Sidecar Proxy   │       │
  │  │   (user-svc)    │ ◄───── (Envoy/Linkerd)  │       │
  │  └─────────────────┘       └────────┬────────┘       │
  └───────────────────────────────────── │ ───────────────┘
                                         │ encrypted (mTLS)
                                         │
  ┌─────────────────────────────────────  │ ──────────────┐
  │  Pod / Container Group               │               │
  │                                      │               │
  │  ┌─────────────────┐       ┌──────── ▼ ──────────┐   │
  │  │  Order Service  │ ◄──── │  Sidecar Proxy      │   │
  │  │  (order-svc)    │ ──────► (Envoy/Linkerd)     │   │
  │  └─────────────────┘       └─────────────────────┘   │
  └───────────────────────────────────────────────────────┘
```

The service only talks to `localhost`. The sidecar intercepts, handles TLS,
applies policies, and forwards to the right destination.

---

## What This Demo Shows

Since we're using plain Go (no Envoy, no Istio), we simulate the sidecar concept:

1. **Sidecar wraps a service** — intercepts calls before/after
2. **mTLS simulation** — each sidecar verifies the caller's identity header
3. **Traffic policies** — retries, request logging, latency injection
4. **Traffic splitting** — route a percentage of requests to a new version

```
  Client  ──►  Sidecar A  ───(mTLS)───►  Sidecar B  ──►  Service B
                │                             │
            [log, retry,              [verify identity,
             rate limit]               log, forward]
```

---

## Key Concepts

| Concept | Real Mesh | This Demo |
|---------|-----------|-----------|
| Sidecar proxy | Envoy/Linkerd binary | Go struct wrapping an http.Handler |
| mTLS | X.509 client certs | `X-Service-Identity` header check |
| Retries | Built into proxy | Retry loop in sidecar |
| Traffic split | Weight-based routing | Percent-based backend selection |
| Observability | Metrics/traces | Request/error counters |

---

## File Structure

```
08-service-mesh/
├── README.md
├── cmd/
│   └── main.go                          ← demo: two services with sidecars
├── internal/
│   └── sidecar/
│       └── sidecar.go                   ← Sidecar: identity check, retries, traffic split, metrics
└── service_mesh_test.go                 ← tests for all sidecar features
```
