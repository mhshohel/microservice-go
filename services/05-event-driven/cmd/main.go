// main.go — Demo for Event-Driven Communication.
//
// This program shows both patterns side by side:
//   1. Pub-Sub: publish an "order.placed" event, 3 services receive it
//   2. Message Queue: dispatch 10 tasks across 3 workers
//
// HOW TO RUN:
//   go run ./services/05-event-driven/cmd/main.go

package main

import (
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"microservices-go/services/05-event-driven/internal/pubsub"
	"microservices-go/services/05-event-driven/internal/queue"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	fmt.Println("\n═══════════════════════════════════════")
	fmt.Println(" PART 1: Pub-Sub Demo")
	fmt.Println("═══════════════════════════════════════")
	demoPubSub()

	fmt.Println("\n═══════════════════════════════════════")
	fmt.Println(" PART 2: Message Queue Demo")
	fmt.Println("═══════════════════════════════════════")
	demoMessageQueue()
}

// demoPubSub shows fan-out: one event goes to all 3 subscribers.
func demoPubSub() {
	broker := pubsub.New(10)

	// Three services subscribe to the "order.placed" topic
	emailSub := broker.Subscribe("order.placed")
	inventorySub := broker.Subscribe("order.placed")
	analyticsSub := broker.Subscribe("order.placed")

	var wg sync.WaitGroup

	// Email service: listens for events and sends confirmation emails
	wg.Add(1)
	go func() {
		defer wg.Done()
		for event := range emailSub.Ch {
			order := event.Payload.(map[string]string)
			slog.Info("[email-svc] sending confirmation email",
				"order_id", order["id"],
				"customer", order["customer"],
			)
		}
	}()

	// Inventory service: listens and reserves stock
	wg.Add(1)
	go func() {
		defer wg.Done()
		for event := range inventorySub.Ch {
			order := event.Payload.(map[string]string)
			slog.Info("[inventory-svc] reserving items",
				"order_id", order["id"],
				"item", order["item"],
			)
		}
	}()

	// Analytics service: listens and records metrics
	wg.Add(1)
	go func() {
		defer wg.Done()
		for event := range analyticsSub.Ch {
			order := event.Payload.(map[string]string)
			slog.Info("[analytics-svc] recording event",
				"order_id", order["id"],
				"topic", event.Topic,
			)
		}
	}()

	// Publish 3 order events
	orders := []map[string]string{
		{"id": "ord-001", "customer": "alice", "item": "laptop"},
		{"id": "ord-002", "customer": "bob", "item": "keyboard"},
		{"id": "ord-003", "customer": "carol", "item": "monitor"},
	}
	for _, order := range orders {
		slog.Info("[order-svc] publishing order.placed event", "order_id", order["id"])
		broker.Publish("order.placed", order)
	}

	// Unsubscribe (closes each subscriber's channel → goroutines exit their range loops)
	broker.Unsubscribe("order.placed", emailSub)
	broker.Unsubscribe("order.placed", inventorySub)
	broker.Unsubscribe("order.placed", analyticsSub)

	wg.Wait()
	fmt.Println("→ Each order was received by ALL 3 services (fan-out)")
}

// demoMessageQueue shows work distribution: 10 tasks processed by 3 workers.
func demoMessageQueue() {
	var mu sync.Mutex
	workerCounts := map[int]int{} // track which worker handled how many tasks

	// Create a queue: capacity=20 tasks, 3 workers
	q := queue.New(20, 3, func(task queue.Task) {
		// Simulate processing time
		time.Sleep(5 * time.Millisecond)

		mu.Lock()
		workerCounts[os.Getpid()]++ // just use pid as a proxy — real workers would have IDs
		mu.Unlock()

		slog.Info("[worker] processed task", "id", task.ID, "payload", task.Payload)
	})

	// Push 10 tasks
	for i := range 10 {
		q.Push(queue.Task{
			ID:      fmt.Sprintf("task-%03d", i+1),
			Payload: fmt.Sprintf("do-work-%d", i+1),
		})
	}

	// Close waits for all tasks to finish
	q.Close()
	fmt.Println("→ 10 tasks were distributed across 3 workers (one task per worker)")
}
