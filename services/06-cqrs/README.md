# 06 - CQRS (Command Query Responsibility Segregation)

## What is CQRS?

**CQRS** separates the way you **write** data from the way you **read** data.

- **Command** = something that changes state ("place order", "update user")
- **Query** = something that reads state ("get order by ID", "list all orders")

In a traditional system, both use the same data model and database.
CQRS puts them on separate paths — different models, different storage, optimized for each.

---

## ASCII Diagram

```
  ┌─────────────────────────────────────────────────────────────────────────┐
  │                          CQRS SYSTEM                                    │
  │                                                                         │
  │   WRITE SIDE                               READ SIDE                   │
  │   (Commands)                               (Queries)                   │
  │                                                                         │
  │   Client                                   Client                      │
  │     │                                        │                         │
  │     ▼                                        ▼                         │
  │  ┌──────────┐                           ┌──────────┐                   │
  │  │ Command  │                           │  Query   │                   │
  │  │ Handler  │                           │ Handler  │                   │
  │  └────┬─────┘                           └────┬─────┘                   │
  │       │                                      │                         │
  │       ▼                                      ▼                         │
  │  ┌──────────┐  event  ┌──────────┐     ┌──────────┐                   │
  │  │  Write   │────────►│  Event   │────►│  Read    │                   │
  │  │   DB     │         │   Bus    │     │   DB     │                   │
  │  │(orders)  │         │(channel) │     │(denorm.) │                   │
  │  └──────────┘         └──────────┘     └──────────┘                   │
  └─────────────────────────────────────────────────────────────────────────┘
```

**Write DB**: normalized, good for transactional writes (SQLite in this demo)
**Read DB**: denormalized, optimized for fast reads (also SQLite, different schema)
**Event Bus**: syncs changes from write to read side (Go channel in this demo)

---

## Why CQRS?

| Traditional | CQRS |
|-------------|------|
| One model for reads AND writes | Separate optimized models |
| Complex queries on a write-optimized schema | Read model pre-joined and denormalized |
| Scaling reads = scaling writes too | Read side can scale independently |
| Every write must maintain query indexes | Write side stays simple |

**Example**: an e-commerce order has 10 tables (order, items, address, payment...).
A read query like "show order summary" joins all 10 tables — slow!
In CQRS, the read model has a single `order_summaries` table with all the data
pre-joined and ready to serve. Blazing fast reads.

---

## This Demo

Models an **Order** domain:
- **Write**: `orders` table (normalized, append-only style)
- **Read**: `order_summaries` table (denormalized: order + customer info in one row)
- **Events**: `OrderPlaced`, `OrderShipped` published when write side changes
- **Projector**: listens to events and updates the read DB

---

## File Structure

```
06-cqrs/
├── README.md
├── cmd/
│   └── main.go                          ← demo: place orders, query summaries
├── internal/
│   ├── events/
│   │   └── events.go                    ← event types and event bus
│   ├── write/
│   │   └── command_handler.go           ← handles commands: PlaceOrder, ShipOrder
│   └── read/
│       └── query_handler.go             ← handles queries: GetOrder, ListOrders
└── cqrs_test.go                         ← tests for command + query paths
```
