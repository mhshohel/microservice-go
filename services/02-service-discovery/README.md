# 02 - Service Discovery

## What is Service Discovery?

In a microservices system, services need to **find each other** to communicate.
But in modern deployments, services don't have fixed IP addresses — they start
on random ports, scale up/down, crash and restart. How does Service A find Service B?

**Service Discovery** solves this: a central **registry** keeps an up-to-date list
of all running services and where to reach them. Services **register** themselves
on startup and **deregister** on shutdown. Any service that needs to call another
service first **queries** the registry to find a healthy instance.

---

## ASCII Diagram

```
  ┌──────────────────────────────────────────────────────────────────┐
  │                     SERVICE REGISTRY                             │
  │                                                                  │
  │   ┌──────────────────────────────────────────────────────────┐  │
  │   │  Name       │ Instance ID    │ Address         │ Status  │  │
  │   │─────────────┼────────────────┼─────────────────┼─────────│  │
  │   │  user-svc   │ inst-abc-001   │ localhost:9001  │ healthy │  │
  │   │  user-svc   │ inst-abc-002   │ localhost:9011  │ healthy │  │
  │   │  order-svc  │ inst-xyz-001   │ localhost:9002  │ healthy │  │
  │   └──────────────────────────────────────────────────────────┘  │
  └──────────────────────────────────────────────────────────────────┘
         ▲             ▲                        ▲
         │ 1. Register │ 3. Heartbeat           │ 2. Query
         │  (on start) │  (every 10s)           │  "give me a user-svc"
         │             │                        │
  ┌──────┴──────┐ ┌────┴────────┐      ┌────────┴──────────┐
  │  User Svc   │ │  User Svc   │      │    Order Svc      │
  │ (inst-001)  │ │ (inst-002)  │      │  needs to call    │
  │  :9001      │ │  :9011      │      │    user-svc       │
  └─────────────┘ └─────────────┘      └───────────────────┘
```

**The lifecycle of a service:**

```
  [Service starts]
       │
       ▼
  POST /register  ──────►  Registry stores the entry
       │
       ▼
  [Every 10 seconds]
  PUT /heartbeat/{id} ──► Registry resets the "last seen" timer
       │
       ▼  (if no heartbeat received for 30 seconds)
  Registry auto-removes the dead entry
       │
  [Service shuts down gracefully]
  DELETE /deregister/{id} ──► Registry removes the entry immediately
```

---

## Why Do We Need This?

| Without Service Discovery | With Service Discovery |
|--------------------------|------------------------|
| Hardcoded IPs in config  | Dynamic lookup at runtime |
| Manual updates when services move | Auto-updated by the registry |
| All-or-nothing scaling   | Multiple instances, load spread |
| One crash = broken system | Failed instance quietly removed |

---

## Two Discovery Patterns

### Pattern A — Server-Side Discovery (what this demo uses)

```
  Client ──── query ────► Registry ──── picks instance ──► Backend
```

The client asks the registry: "Give me a user-svc to call."
The **registry** selects one healthy instance and returns its address.
The client doesn't need to know there are multiple instances.

### Pattern B — Client-Side Discovery

```
  Client ──── get all instances ────► Registry
  Client ──── (client picks one) ────► Backend
```

The client fetches all healthy instances and picks one itself (e.g., round-robin).
More flexible, but the client needs to implement selection logic.

This demo shows **both**: the registry supports both "give me one" (server-side)
and "give me all" (client-side) queries.

---

## How the Heartbeat Works

A **heartbeat** is a periodic "I'm still alive" ping. If the registry doesn't
receive a heartbeat within a timeout window (30 seconds in this demo), it
assumes the service crashed and removes it.

```
  Time: 0s   Service registers (LastSeen = 0s)
  Time: 10s  Heartbeat received (LastSeen = 10s)
  Time: 20s  Heartbeat received (LastSeen = 20s)
  Time: 30s  ... service crashes, no heartbeat
  Time: 50s  Registry checker runs: 50-20 = 30s > timeout → REMOVED
```

---

## HTTP API

| Method | Path                    | Description                              |
|--------|-------------------------|------------------------------------------|
| POST   | `/register`             | Register a new service instance          |
| DELETE | `/deregister/{id}`      | Remove a service instance                |
| PUT    | `/heartbeat/{id}`       | Reset the "last seen" timer              |
| GET    | `/services/{name}`      | Get all healthy instances of a service   |
| GET    | `/services`             | Get all registered services              |
| GET    | `/health`               | Registry health check                    |

### Register Request Body

```json
{
  "name":    "user-svc",
  "address": "localhost:9001",
  "metadata": {
    "version": "1.2.0",
    "region":  "us-east-1"
  }
}
```

### Register Response

```json
{
  "instance_id": "user-svc-a3f9c2b1",
  "name":        "user-svc",
  "address":     "localhost:9001",
  "registered_at": "2024-01-15T10:00:00Z"
}
```

---

## How to Run

```bash
# From the project root
go run ./services/02-service-discovery/cmd/main.go
```

The registry starts on `:8081`. Try these curl commands:

```bash
# Register a service
curl -X POST http://localhost:8081/register \
  -H "Content-Type: application/json" \
  -d '{"name":"user-svc","address":"localhost:9001"}'

# Register another instance of the same service
curl -X POST http://localhost:8081/register \
  -H "Content-Type: application/json" \
  -d '{"name":"user-svc","address":"localhost:9011"}'

# Query all user-svc instances
curl http://localhost:8081/services/user-svc

# Query all registered services
curl http://localhost:8081/services

# Send a heartbeat (replace INSTANCE_ID with the id from register response)
curl -X PUT http://localhost:8081/heartbeat/INSTANCE_ID

# Deregister (replace INSTANCE_ID)
curl -X DELETE http://localhost:8081/deregister/INSTANCE_ID

# Health check
curl http://localhost:8081/health
```

---

## File Structure

```
02-service-discovery/
├── README.md                        ← this file
├── cmd/
│   └── main.go                      ← entry point, starts the registry server
├── internal/
│   ├── registry/
│   │   └── registry.go              ← in-memory store: Register, Deregister, Heartbeat, Query
│   └── api/
│       └── handlers.go              ← HTTP handlers that wrap the registry
└── discovery_test.go                ← all tests
```

---

## Key Concepts to Remember

- **Instance ID** — a unique ID generated when a service registers (e.g., `user-svc-a3f9c2b1`). Used for heartbeat and deregister calls.
- **Heartbeat timeout** — if no heartbeat arrives within this window, the instance is assumed dead and removed.
- **Health check** — the registry's own `/health` endpoint, not the services it tracks.
- **Real-world registries** — Consul, etcd, Eureka, ZooKeeper all do exactly this, but with persistence, replication, and more features. This demo shows the core concept without external dependencies.
