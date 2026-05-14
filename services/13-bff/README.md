# Service 13 — Backend for Frontend (BFF)

## What is the BFF Pattern?

The **Backend for Frontend** pattern solves a specific problem: different clients (web, mobile, admin) need different shapes of the same underlying data, but one generic API tries to serve all of them — and ends up doing it poorly.

A BFF is a thin aggregation and transformation layer that sits between a client and the backend services. Each client type gets its own BFF:

- **Web BFF** — returns rich, detailed data because a desktop browser can handle it
- **Mobile BFF** — returns minimal data to conserve bandwidth and battery on a phone
- **Admin BFF** — returns aggregated management data (counts, totals, breakdowns) that only admin dashboards need

The underlying backend services (order-svc, user-svc, payment-svc) stay generic. The BFFs do the client-specific shaping.

## ASCII Diagram

```
                        ┌────────────────────────────────────────────┐
                        │              BFF Service (:8085)            │
                        │                                            │
  ┌──────────────┐      │  ┌──────────────────────────────────────┐  │
  │   Web App    │─────►│  │  Web BFF  GET /web/orders            │  │
  │  (desktop)   │      │  │  Full order details — all fields     │  │
  └──────────────┘      │  └──────────────────────────────────────┘  │
                        │                                            │
  ┌──────────────┐      │  ┌──────────────────────────────────────┐  │   ┌─────────────┐
  │  Mobile App  │─────►│  │  Mobile BFF  GET /mobile/orders      │  │──►│  order-svc  │
  │   (phone)    │      │  │  Minimal — ID, status, total only    │  │   └─────────────┘
  └──────────────┘      │  └──────────────────────────────────────┘  │
                        │                                            │
  ┌──────────────┐      │  ┌──────────────────────────────────────┐  │
  │  Admin Panel │─────►│  │  Admin BFF  GET /admin/dashboard     │  │
  │              │      │  │  Aggregates — counts, revenue totals │  │
  └──────────────┘      │  └──────────────────────────────────────┘  │
                        └────────────────────────────────────────────┘
```

## Why Not One API for All?

Without BFFs, one generic API must return a response large enough for the most demanding client (admin), even when a mobile device only needs two fields. This leads to:

- **Over-fetching on mobile**: downloading 500 bytes to show 20 bytes of data
- **Under-fetching on admin**: making 5 API calls to get data that one BFF call could aggregate
- **Coupling**: every client must understand the same response shape, even if only 10% of it is relevant

With BFFs each client gets exactly what it needs — no more, no less.

## Response Shapes Comparison

Given the same `Order` data, each BFF returns a different shape:

| Field          | Web BFF | Mobile BFF | Admin BFF |
|----------------|---------|------------|-----------|
| id             | yes     | yes        | (aggregated) |
| customer_name  | yes     | no         | no        |
| item           | yes     | no         | no        |
| quantity       | yes     | no         | no        |
| total_cents    | yes     | yes        | summed    |
| status         | yes     | yes        | counted   |
| created_at     | yes     | no         | no        |

The Admin BFF goes further: instead of returning a list, it returns a summary dashboard with `total_orders`, `total_revenue_cents`, and `by_status` breakdown.

## Running the Demo

```bash
# Start the server
go run ./services/13-bff/cmd/main.go

# Web BFF — full order details
curl http://localhost:8085/web/orders | jq .

# Mobile BFF — minimal fields only
curl http://localhost:8085/mobile/orders | jq .

# Admin BFF — aggregate dashboard
curl http://localhost:8085/admin/dashboard | jq .
```

## Running Tests

```bash
go test -v -race ./services/13-bff/...
```
