# 01 — API Gateway

## 1.1 Explanation

An **API Gateway** is a server that acts as the **single entry point** for all client requests
in a microservices system. Clients never talk directly to individual services — they always
go through the gateway.

Think of it like the **front desk of a hotel**:
- Every guest (client) checks in at the front desk (gateway)
- The front desk authenticates them (are you a guest here?)
- The front desk directs them to the right room (which service?)
- The guest never needs to know the hotel's internal layout (service addresses)

---

## 1.1.1 How a Request Flows

```
  Client (browser / mobile app / curl)
           │
           │  GET /api/users/123
           │  Authorization: Bearer eyJhb...
           ▼
  ┌─────────────────────────────────────┐
  │           API GATEWAY :8080         │
  │                                     │
  │  ┌──────────────────────────────┐   │
  │  │  Step 1 — Rate Limiter       │   │  ← Too many requests? → 429
  │  │  max 100 req/min per IP      │   │
  │  └──────────────┬───────────────┘   │
  │                 │ OK                │
  │  ┌──────────────▼───────────────┐   │
  │  │  Step 2 — Auth Middleware    │   │  ← Bad/missing token? → 401
  │  │  validate JWT token          │   │
  │  └──────────────┬───────────────┘   │
  │                 │ OK                │
  │  ┌──────────────▼───────────────┐   │
  │  │  Step 3 — Router             │   │  ← Unknown path? → 404
  │  │  /api/users/*   → :9001      │   │
  │  │  /api/products/* → :9002     │   │
  │  │  /api/orders/*  → :9003      │   │
  │  └──────────────┬───────────────┘   │
  │                 │ backend URL       │
  │  ┌──────────────▼───────────────┐   │
  │  │  Step 4 — Reverse Proxy      │   │  ← Backend down? → 502
  │  │  forward request to backend  │   │
  │  └──────────────┬───────────────┘   │
  └─────────────────┼───────────────────┘
                    │
        ┌───────────┼───────────────┐
        ▼           ▼               ▼
  ┌──────────┐ ┌──────────┐ ┌──────────┐
  │  Users   │ │ Products │ │  Orders  │
  │ :9001    │ │  :9002   │ │  :9003   │
  └──────────┘ └──────────┘ └──────────┘
    SQLite?        SQLite?      SQLite?
  (own data)    (own data)   (own data)
```

---

## 1.1.2 Definition

> An **API Gateway** is a reverse proxy that sits in front of all your microservices.
> It handles cross-cutting concerns (auth, rate limiting, logging, routing) in one place
> so individual services don't have to repeat that logic.

**Key terms:**
- **Reverse proxy** — a server that forwards requests on behalf of clients (opposite of a forward proxy)
- **JWT** — JSON Web Token, a signed string proving who the client is
- **Rate limiting** — capping how many requests a client can make in a time window
- **Routing** — deciding which backend service should handle a request

---

## 1.1.3 Why to Use

Without a gateway, every microservice must handle its own:
- Authentication (validating tokens)
- Rate limiting (preventing abuse)
- Logging (tracking requests)
- CORS headers
- SSL termination

That's a lot of repeated code. If you have 10 services, you write that logic 10 times.
With a gateway, you write it **once**.

Also, without a gateway, clients must know the address of every service:
```
GET http://users-service:9001/users/1
GET http://products-service:9002/products/p1
GET http://orders-service:9003/orders/o1
```

With a gateway, clients only need one address:
```
GET http://gateway:8080/api/users/1
GET http://gateway:8080/api/products/p1
GET http://gateway:8080/api/orders/o1
```

---

## 1.1.4 When to Use

Use an API Gateway when:
- You have **multiple microservices** that clients need to access
- You want **one place** to handle auth, logging, and rate limiting
- You need to **hide internal service addresses** from clients
- You want to **change backend services** without updating clients

Do NOT use it when:
- You only have one service (adds unnecessary complexity)
- Services communicate only with each other (internal traffic doesn't need a gateway)

---

## 1.1.5 Benefits

| Benefit | Description |
|---------|-------------|
| Single entry point | Clients only need one URL |
| Centralised auth | Token validation in one place |
| Rate limiting | Protect backends from abuse |
| Service isolation | Backends can change without clients knowing |
| Observability | Log all traffic in one place |
| Load balancing | Spread requests across instances |

---

## 1.1.6 Practical Application — E-Commerce Example

Imagine you're building an online shop with three services:

```
User visits product page
  → browser calls GET /api/products/p1
  → gateway checks JWT (is user logged in?)
  → gateway routes to Product Service
  → Product Service returns product details
  → browser displays the page

User places an order
  → browser calls POST /api/orders
  → gateway checks JWT
  → gateway checks rate limit (prevent order spam)
  → gateway routes to Order Service
  → Order Service creates the order
```

The browser never knows that Product Service and Order Service are separate applications.

---

## 1.2 Routing Strategies (two separate files)

This example includes two ways to route requests:

### Strategy 1 — Route by URL Path (`route_by_path.go`)
Reads the URL path and picks the backend:
```
/api/users/*    → User Service
/api/products/* → Product Service
/api/orders/*   → Order Service
```
Most common approach. Easy to read, no special client setup required.

### Strategy 2 — Route by Header (`route_by_header.go`)
Reads a custom `X-Service` header:
```
X-Service: users    → User Service
X-Service: products → Product Service
X-Service: orders   → Order Service
```
Useful when multiple services share the same URL pattern, or when supporting multiple
API versions (`X-API-Version: v1` vs `X-API-Version: v2`).

---

## How to Run

```bash
# From the project root:
go run ./services/01-api-gateway/cmd/main.go
```

Then in another terminal:

```bash
# Step 1: get a demo JWT token
curl http://localhost:8080/token

# Step 2: copy the token and use it
TOKEN="paste-token-here"

# Request the users list
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/users

# Request a single product
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/products/p1

# Request orders
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/orders

# Health check (no token needed)
curl http://localhost:8080/health

# No token → 401 Unauthorized
curl http://localhost:8080/api/users

# Bad token → 401 Unauthorized
curl -H "Authorization: Bearer fake.token.here" http://localhost:8080/api/users
```

## How to Test

```bash
go test ./services/01-api-gateway/... -v -race
```

---

## File Structure

```
01-api-gateway/
├── README.md                          ← you are here
├── cmd/
│   └── main.go                        ← start the demo
├── internal/
│   ├── backend/
│   │   └── servers.go                 ← dummy User/Product/Order services
│   ├── jwt/
│   │   └── jwt.go                     ← generate + validate JWT tokens
│   ├── middleware/
│   │   ├── auth.go                    ← JWT auth middleware
│   │   └── ratelimit.go               ← rate limiter middleware
│   ├── proxy/
│   │   └── proxy.go                   ← reverse proxy (forwards requests)
│   └── router/
│       ├── route_by_path.go           ← routing strategy 1: by URL path
│       └── route_by_header.go         ← routing strategy 2: by header
└── gateway_test.go                    ← all tests
```
