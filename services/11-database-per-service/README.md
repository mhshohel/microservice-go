# Service 11 — Database per Service

## What is "Database per Service"?

In a microservices architecture, each service owns its own database. No other service can connect to it directly — they must communicate through the owning service's API.

This means:
- The **User service** owns `users.db` — only it can read or write user records
- The **Order service** owns `orders.db` — only it can read or write orders
- The **Product service** owns `products.db` — only it can read or write products

If the Order service needs to validate that a user exists, it calls the **User service's HTTP API**, not the User database directly.

---

## Why Services Must NOT Share a Database

Sharing a database creates tight coupling between services:

| Problem | Consequence |
|---------|-------------|
| Schema changes break other services | Can't rename a column without updating every service that reads it |
| One service can corrupt another's data | A buggy Order service can accidentally delete user records |
| Can't scale independently | If the User DB is slow, Order queries are also slow |
| Can't choose the right DB per service | User service might benefit from Postgres; Order service might need Cassandra |

---

## Architecture Diagram

```
  ┌──────────────────┐   HTTP API    ┌──────────────────┐
  │   User Service   │◄─────────────►│  Order Service   │
  │                  │               │                  │
  │  ┌────────────┐  │               │  ┌────────────┐  │
  │  │  users.db  │  │               │  │  orders.db │  │
  │  │ ─────────  │  │               │  │ ─────────  │  │
  │  │ id         │  │               │  │ id         │  │
  │  │ name       │  │               │  │ user_id    │  │
  │  │ email      │  │               │  │ item       │  │
  │  └────────────┘  │               │  │ qty        │  │
  └──────────────────┘               │  └────────────┘  │
                                     └──────────────────┘

  ┌──────────────────┐
  │ Product Service  │
  │                  │
  │  ┌────────────┐  │
  │  │products.db │  │
  │  │ ─────────  │  │
  │  │ id         │  │
  │  │ name       │  │
  │  │price_cents │  │
  │  └────────────┘  │
  └──────────────────┘

  Each service has its OWN database.
  No service can connect to another service's database directly.
```

---

## Shared DB vs Per-Service DB

| Concern | Shared Database | Per-Service Database |
|---------|----------------|---------------------|
| **Coupling** | High — schema changes ripple everywhere | Low — each service owns its schema |
| **Independent deployment** | Hard — all services must agree on schema | Easy — each service deploys independently |
| **Independent scaling** | No — one connection pool for all | Yes — scale each DB as needed |
| **Technology choice** | One DB engine for all | Best tool per service (Postgres, Redis, etc.) |
| **Fault isolation** | One slow query can starve all services | DB problems are contained to one service |
| **Data integrity** | DB-level foreign keys across services | Application-level validation via APIs |

---

## Code Structure

```
internal/
  user/store.go     — UserStore: owns the users table
  order/store.go    — OrderStore: owns the orders table
  product/store.go  — ProductStore: owns the products table
cmd/main.go         — demo that uses all three stores
database_per_service_test.go — tests for all stores
```

Each store:
1. Opens its own SQLite database file (or `:memory:` for tests)
2. Creates its own schema on startup
3. Provides typed methods (CreateUser, GetUser, etc.)
4. Has no knowledge of the other stores

---

## How to Run

```bash
# Run the demo (no Docker needed)
go run ./services/11-database-per-service/cmd/main.go

# Run tests
go test -v -race ./services/11-database-per-service/...

# Build and run with Docker
docker build \
  -t database-per-service-demo \
  -f services/11-database-per-service/Dockerfile \
  .
docker run database-per-service-demo
```

---

## Why SQLite Here?

This demo uses `modernc.org/sqlite` — a pure-Go SQLite implementation that requires no C compiler and no installed SQLite library. The choice of database engine is an **implementation detail** of each service. In production you might use:

- Postgres or MySQL for transactional workloads
- Redis for session/cache data
- Cassandra or DynamoDB for write-heavy event stores
- Elasticsearch for full-text search

The pattern works the same regardless of the engine.
