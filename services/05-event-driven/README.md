# 05 - Event-Driven Communication

## What is Event-Driven Communication?

Instead of Service A directly calling Service B (synchronous),
**event-driven communication** lets services talk through a message bus:

- Service A **publishes** an event ("order placed")
- Service B, C, D — whoever is interested — **subscribe** and react

Neither side knows about the other. They're decoupled.

---

## Two Patterns in This Demo

### Pattern 1 — Pub-Sub (Publish-Subscribe)

One event is delivered to **ALL** subscribers on that topic.

```
  Publisher                     Event Bus                   Subscribers
                                                             (all receive it)
  [Order Svc] ──── "order.placed" ────► ┌─────────────┐
                                         │   TOPIC:    │ ──► [Email Svc]
                                         │ order.placed│ ──► [Inventory Svc]
                                         │             │ ──► [Analytics Svc]
                                         └─────────────┘
```

Use when: multiple services care about the same event (notifications, audit logs, cache invalidation).

### Pattern 2 — Message Queue (Work Queue)

One message is delivered to **ONE** consumer. Consumers compete for messages.

```
  Producer                     Queue                     Workers
  (sends tasks)                                          (one processes each)

  [API] ──► [task] ──► ┌─────────────────────┐
  [API] ──► [task] ──► │ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ │ ──► [Worker 1]
  [API] ──► [task] ──► │ (FIFO buffer)       │ ──► [Worker 2]
                        └─────────────────────┘ ──► [Worker 3]
                                                     (only one gets each task)
```

Use when: work needs to be distributed across multiple workers (email sending, image processing, order fulfillment).

---

## Pub-Sub vs Message Queue

| Feature              | Pub-Sub                      | Message Queue             |
|----------------------|------------------------------|---------------------------|
| Delivery             | ALL subscribers get a copy   | ONE worker gets the task  |
| Use case             | Notifications, fan-out       | Background jobs, work distribution |
| Ordering             | No guarantee                 | FIFO within the queue     |
| Backpressure         | No (slow subscriber blocks)  | Yes (bounded buffer)      |

---

## How It Works in Go

Go channels are a perfect fit for event-driven patterns:
- A channel IS a message queue — buffered = async, unbuffered = sync
- Multiple goroutines can subscribe (pub-sub) or compete for work (queue)
- Select statements allow non-blocking sends (useful for slow subscribers)

### Pub-Sub using channels

```go
// Each subscriber gets its own channel.
// Publisher sends to all channels.
subscribers := map[string]chan Event{
    "email-svc":     make(chan Event, 10),
    "inventory-svc": make(chan Event, 10),
}
// Publish: fan out to all
for _, ch := range subscribers {
    ch <- event
}
```

### Message Queue using a single channel

```go
// Single buffered channel — one queue, many workers
queue := make(chan Task, 100)
// Workers read from the same channel
for i := 0; i < 3; i++ {
    go func() {
        for task := range queue {
            process(task)
        }
    }()
}
```

---

## File Structure

```
05-event-driven/
├── README.md
├── cmd/
│   └── main.go                         ← demo: pub-sub and message queue side by side
├── internal/
│   ├── pubsub/
│   │   └── broker.go                   ← topic-based pub-sub broker
│   └── queue/
│       └── queue.go                    ← bounded message queue with worker pool
└── event_driven_test.go                ← tests for both patterns
```

---

## How to Run

```bash
go run ./services/05-event-driven/cmd/main.go
```

Watch the logs to see events fan out to multiple subscribers and tasks get
distributed across workers.
