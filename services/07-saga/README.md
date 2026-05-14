# 07 - Saga Pattern

## What is the Saga Pattern?

In microservices, a business transaction often spans **multiple services**.
For example, placing an order requires:
1. Charging the customer's payment
2. Reserving inventory
3. Creating a shipment

Each step runs in a different service. There's no single database transaction
that can wrap all three — they're separate processes on separate databases.

If step 3 fails after steps 1 and 2 succeeded, we need to **undo** those changes:
- Refund the payment (step 1)
- Release the reserved inventory (step 2)

**Saga** is the pattern for managing this. Each step has a matching **compensating action**
that reverses it if something goes wrong later.

---

## Orchestration vs Choreography

### Orchestration (this demo)

A central **Orchestrator** controls the flow. It tells each service what to do
and handles failures by running compensations in reverse order.

```
             ┌────────────────────────────────┐
             │           ORCHESTRATOR          │
             │                                │
             │  1. call Payment               │
             │  2. call Inventory             │
             │  3. call Shipping              │
             │  (if any fails → compensate)   │
             └────────────────────────────────┘
                  │            │           │
                  ▼            ▼           ▼
             [Payment]    [Inventory]  [Shipping]
```

**Pro**: Easy to understand — one place to see the whole flow.
**Con**: The orchestrator becomes a central point that knows about all services.

### Choreography

Services react to events from each other. No central controller.

```
  [Order Service] ─► OrderCreated event
                          │
          ┌───────────────┼───────────────┐
          ▼               ▼               ▼
     [Payment]       [Inventory]      [Analytics]
  publishes           publishes
  PaymentCharged      StockReserved
       │
       ▼
  [Shipping]
  creates shipment
```

**Pro**: Fully decoupled — services don't know about each other.
**Con**: Hard to visualize the overall flow; debugging distributed events is complex.

---

## The Happy Path

```
Orchestrator
  │
  ├─► Payment.Charge($50)      ✓ success
  │
  ├─► Inventory.Reserve(qty:1)  ✓ success
  │
  └─► Shipping.Create(addr)    ✓ success
       │
       └── ORDER COMPLETE
```

## The Failure Path (Compensation)

```
Orchestrator
  │
  ├─► Payment.Charge($50)      ✓ success  [step 1 done]
  │
  ├─► Inventory.Reserve(qty:2)  ✓ success  [step 2 done]
  │
  ├─► Shipping.Create(addr)    ✗ FAIL ← step 3 fails
  │
  └── Begin compensation (reverse order):
       ├─► Inventory.Release(qty:2)  ✓ [step 2 compensated]
       └─► Payment.Refund($50)       ✓ [step 1 compensated]
                                       ORDER CANCELLED
```

---

## File Structure

```
07-saga/
├── README.md
├── cmd/
│   └── main.go                         ← demo: success case + failure/compensation case
├── internal/
│   ├── saga/
│   │   └── orchestrator.go             ← Step + Orchestrator: run steps, compensate on failure
│   └── services/
│       ├── payment.go                  ← fake Payment service (Charge + Refund)
│       ├── inventory.go                ← fake Inventory service (Reserve + Release)
│       └── shipping.go                 ← fake Shipping service (Create + Cancel)
└── saga_test.go                        ← tests for happy path, failure, compensation
```
