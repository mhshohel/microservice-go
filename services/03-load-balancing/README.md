# 03 - Load Balancing

## What is Load Balancing?

When multiple instances of a service are running, **load balancing** decides
which instance handles each incoming request. Without it, all traffic would
pile up on one instance while the others sit idle.

A load balancer distributes requests across backends to:
- Avoid overloading any single server
- Use all available capacity
- Keep response times low
- Allow scaling up/down without changing client config

---

## ASCII Diagram

```
                        ┌──────────────────────┐
                        │     LOAD BALANCER    │
  Clients               │                      │
                        │  ┌────────────────┐  │
  [C1] ──┐              │  │   Algorithm    │  │
  [C2] ──┤─── request ──►  │ round-robin /  │  │
  [C3] ──┘              │  │ least-conn /   │  │
                        │  │ ip-hash / etc  │  │
                        │  └───────┬────────┘  │
                        └──────────┼───────────┘
                                   │
              ┌────────────────────┼────────────────────┐
              ▼                    ▼                     ▼
       ┌─────────────┐    ┌─────────────┐     ┌─────────────┐
       │  Backend 1  │    │  Backend 2  │     │  Backend 3  │
       │  :9001      │    │  :9002      │     │  :9003      │
       │  [load: 3]  │    │  [load: 1]  │     │  [load: 5]  │
       └─────────────┘    └─────────────┘     └─────────────┘
```

---

## The 5 Algorithms

### 1. Round Robin
Send each request to the next backend in a circular list.

```
Request 1 → Backend 1
Request 2 → Backend 2
Request 3 → Backend 3
Request 4 → Backend 1  (wraps around)
Request 5 → Backend 2
```

**Best for:** backends that are identical and requests take similar time.

---

### 2. Weighted Round Robin
Like round-robin, but backends with higher weight get more requests.

```
Backend 1: weight=3   → gets 3 out of every 5 requests
Backend 2: weight=1   → gets 1 out of every 5 requests
Backend 3: weight=1   → gets 1 out of every 5 requests
```

**Best for:** backends with different capacity (e.g., one server is 3x more powerful).

---

### 3. Least Connections
Always send the next request to the backend currently handling the fewest active connections.

```
Backend 1: 10 active connections
Backend 2:  2 active connections  ← pick this one
Backend 3:  7 active connections
```

**Best for:** when requests have very different processing times (some fast, some slow).

---

### 4. IP Hash
Hash the client's IP address to always pick the same backend for that client.

```
Client 192.168.1.5  → hash → Backend 2 (always)
Client 192.168.1.9  → hash → Backend 1 (always)
Client 10.0.0.3     → hash → Backend 3 (always)
```

**Best for:** sticky sessions — when the backend needs to remember the client
(e.g., in-memory session data).

---

### 5. Random
Pick a backend at random on each request.

```
Request 1 → Backend 3  (random)
Request 2 → Backend 1  (random)
Request 3 → Backend 3  (random)
```

**Best for:** quick demos or when all backends are identical and load is light.
Round-robin is almost always a better choice in production.

---

## Algorithm Comparison

| Algorithm        | Even spread | Sticky client | Handles slow backends | Complexity |
|------------------|-------------|---------------|-----------------------|------------|
| Round Robin      | Yes         | No            | No                    | Low        |
| Weighted RR      | By weight   | No            | No                    | Low        |
| Least Connections| Yes         | No            | Yes                   | Medium     |
| IP Hash          | Roughly     | Yes           | No                    | Low        |
| Random           | Roughly     | No            | No                    | Lowest     |

---

## How to Run

```bash
# From the project root
go run ./services/03-load-balancing/cmd/main.go
```

The demo starts on `:8082` with 3 fake backends. Try sending multiple requests
and watch the logs to see how each algorithm distributes them:

```bash
# Send 9 requests (3 per backend with round-robin)
for i in $(seq 9); do
  curl -s http://localhost:8082/api/data?algo=roundrobin | jq .backend
done

# Try least-connections
for i in $(seq 6); do
  curl -s "http://localhost:8082/api/data?algo=leastconn" | jq .backend
done

# IP hash (your IP always goes to the same backend)
curl "http://localhost:8082/api/data?algo=iphash"
```

---

## File Structure

```
03-load-balancing/
├── README.md
├── cmd/
│   └── main.go                          ← entry point, demo with 3 fake backends
├── internal/
│   └── balancer/
│       ├── balancer.go                  ← Balancer interface + Backend type
│       ├── round_robin.go               ← Round Robin algorithm
│       ├── weighted_round_robin.go      ← Weighted Round Robin
│       ├── least_connections.go         ← Least Connections
│       ├── ip_hash.go                   ← IP Hash
│       └── random.go                    ← Random pick
└── balancing_test.go                    ← tests for all 5 algorithms
```
