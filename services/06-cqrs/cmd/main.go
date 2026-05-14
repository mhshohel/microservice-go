// main.go — CQRS demo.
//
// Shows the full CQRS flow:
//   1. Place orders (commands → write DB → events)
//   2. Ship an order (command → write DB → event)
//   3. Query order summaries (queries → read DB)
//
// HOW TO RUN:
//   go run ./services/06-cqrs/cmd/main.go

package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"microservices-go/services/06-cqrs/internal/events"
	"microservices-go/services/06-cqrs/internal/read"
	"microservices-go/services/06-cqrs/internal/write"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// ── Set up infrastructure ─────────────────────────────────────────────────
	bus := events.NewBus(100)

	// Both DBs use in-memory SQLite so the demo leaves no files behind
	cmdHandler, err := write.NewCommandHandler(":memory:", bus)
	if err != nil {
		slog.Error("failed to create command handler", "error", err)
		os.Exit(1)
	}
	defer cmdHandler.Close()

	qryHandler, err := read.NewQueryHandler(":memory:")
	if err != nil {
		slog.Error("failed to create query handler", "error", err)
		os.Exit(1)
	}
	defer qryHandler.Close()

	// ── Start the projector ───────────────────────────────────────────────────
	// The projector runs in a goroutine, listening to the event bus
	// and updating the read DB whenever something changes.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		qryHandler.Project(bus)
	}()

	// ── Phase 1: Place some orders (commands) ─────────────────────────────────
	fmt.Println("\n── WRITE SIDE: placing orders ──")

	orders := []write.PlaceOrderCmd{
		{OrderID: "ord-001", CustomerID: "cust-1", Customer: "Alice", Item: "Laptop", Quantity: 1, TotalCents: 129900},
		{OrderID: "ord-002", CustomerID: "cust-2", Customer: "Bob", Item: "Keyboard", Quantity: 2, TotalCents: 15000},
		{OrderID: "ord-003", CustomerID: "cust-1", Customer: "Alice", Item: "Monitor", Quantity: 1, TotalCents: 39900},
	}

	for _, cmd := range orders {
		if err := cmdHandler.PlaceOrder(cmd); err != nil {
			slog.Error("place order failed", "id", cmd.OrderID, "error", err)
			continue
		}
		slog.Info("order placed", "id", cmd.OrderID, "customer", cmd.Customer, "item", cmd.Item)
	}

	// ── Phase 2: Ship one order ───────────────────────────────────────────────
	fmt.Println("\n── WRITE SIDE: shipping order ──")
	if err := cmdHandler.ShipOrder(write.ShipOrderCmd{
		OrderID:        "ord-001",
		TrackingNumber: "UPS-12345",
	}); err != nil {
		slog.Error("ship order failed", "error", err)
	} else {
		slog.Info("order shipped", "id", "ord-001", "tracking", "UPS-12345")
	}

	// Give the projector time to process events
	bus.Close()
	wg.Wait()

	// ── Phase 3: Query the read side ──────────────────────────────────────────
	fmt.Println("\n── READ SIDE: querying order summaries ──")

	summaries, err := qryHandler.ListOrders()
	if err != nil {
		slog.Error("list orders failed", "error", err)
		os.Exit(1)
	}

	for _, s := range summaries {
		shipped := "-"
		if s.ShippedAt != nil {
			shipped = "✓ " + s.TrackingNumber
		}
		fmt.Printf("  [%s] %-10s %-8s %-10s $%6.2f  shipped: %s\n",
			s.OrderID, s.Customer, s.Status, s.Item,
			float64(s.TotalCents)/100, shipped,
		)
	}

	// Show one order in detail
	fmt.Println("\n── READ SIDE: get order ord-001 ──")
	detail, err := qryHandler.GetOrder("ord-001")
	if err != nil {
		slog.Error("get order failed", "error", err)
		return
	}
	b, _ := json.MarshalIndent(detail, "  ", "  ")
	fmt.Println(" ", string(b))
}
