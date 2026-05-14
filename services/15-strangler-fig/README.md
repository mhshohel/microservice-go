# 15 - Strangler Fig Pattern

## What is the Strangler Fig Pattern?

Named after the strangler fig tree that slowly grows around another tree until it
replaces it, this pattern migrates a monolith to microservices **incrementally**.

Instead of a risky "big bang" rewrite (rewrite everything at once, deploy on day X),
you:
1. Put a **router/proxy** in front of the existing "legacy" system
2. Incrementally migrate one route at a time to a new microservice
3. The router sends migrated routes to the new service, everything else to the legacy
4. Over time, the new system "strangles" the old one until nothing goes to legacy

```
  BEFORE MIGRATION:
  Client → Legacy Monolith (handles everything)

  DURING MIGRATION (phase 2):
  Client → ROUTER → /users/*     → New User Service (migrated)
                  → /products/*  → New Product Service (migrated)
                  → /*           → Legacy Monolith (everything else)

  AFTER MIGRATION:
  Client → ROUTER → /* → New Services (legacy is gone)
```

---

## Why Not Just Rewrite Everything?

| "Big Bang" Rewrite | Strangler Fig |
|--------------------|---------------|
| All-or-nothing risk | Low risk — migrate one route at a time |
| Long "feature freeze" | System stays functional throughout |
| Hard to test until done | Each migration is independently testable |
| Hard to roll back | Easy — just update the router rules |

---

## File Structure

```
15-strangler-fig/
├── README.md
├── cmd/
│   └── main.go                         ← demo: gradual migration of routes
├── internal/
│   ├── proxy/
│   │   └── router.go                   ← StranglerRouter: route table, proxy to new or legacy
│   ├── legacy/
│   │   └── server.go                   ← Fake legacy monolith handler
│   └── modern/
│       └── server.go                   ← Fake new microservices handler
└── strangler_fig_test.go               ← tests for routing, migration, rollback
```
