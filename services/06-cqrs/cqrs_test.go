// cqrs_test.go — Tests for the CQRS command and query paths.
//
// All tests use in-memory SQLite (:memory:) so no files are left on disk.
//
// Test coverage:
//   - PlaceOrder: creates an order in the write DB and publishes an event
//   - PlaceOrder validation: missing fields return errors
//   - ShipOrder: updates status, publishes event; fails for unknown order
//   - Projector: OrderPlaced event creates a read-side summary
//   - Projector: OrderShipped event updates the read-side status
//   - GetOrder: returns a summary; not found returns error
//   - ListOrders: returns all summaries
//   - Full CQRS flow: place → ship → query shows correct state

package cqrs_test

import (
	"testing"

	"microservices-go/services/06-cqrs/internal/events"
	"microservices-go/services/06-cqrs/internal/read"
	"microservices-go/services/06-cqrs/internal/write"
)

// ── Helpers ───────────────────────────────────────────────────────────────────

// newTestSetup creates in-memory command + query handlers wired to a bus.
func newTestSetup(t *testing.T) (*write.CommandHandler, *read.QueryHandler, *events.Bus) {
	t.Helper()

	bus := events.NewBus(100)

	cmd, err := write.NewCommandHandler(":memory:", bus)
	if err != nil {
		t.Fatalf("command handler: %v", err)
	}
	t.Cleanup(func() { cmd.Close() })

	qry, err := read.NewQueryHandler(":memory:")
	if err != nil {
		t.Fatalf("query handler: %v", err)
	}
	t.Cleanup(func() { qry.Close() })

	return cmd, qry, bus
}

// projectEvent drains one event from the bus and applies it to the read side.
func projectEvent(t *testing.T, bus *events.Bus, qry *read.QueryHandler) {
	t.Helper()
	select {
	case event, ok := <-bus.Subscribe():
		if !ok {
			t.Fatal("bus closed before event arrived")
		}
		qry.ProjectSync(event)
	default:
		t.Fatal("no event in bus — did you forget to place/ship an order first?")
	}
}

// ── Command Tests ─────────────────────────────────────────────────────────────

func TestPlaceOrder_Success(t *testing.T) {
	cmd, _, bus := newTestSetup(t)
	defer bus.Close()

	err := cmd.PlaceOrder(write.PlaceOrderCmd{
		OrderID:    "ord-001",
		CustomerID: "cust-1",
		Customer:   "Alice",
		Item:       "Laptop",
		Quantity:   1,
		TotalCents: 129900,
	})

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestPlaceOrder_PublishesEvent(t *testing.T) {
	cmd, _, bus := newTestSetup(t)

	cmd.PlaceOrder(write.PlaceOrderCmd{ //nolint
		OrderID: "ord-001", CustomerID: "cust-1", Customer: "Alice",
		Item: "Laptop", Quantity: 1, TotalCents: 129900,
	})

	// There should be one event in the bus
	event, ok := <-bus.Subscribe()
	if !ok {
		t.Fatal("bus closed unexpectedly")
	}
	if event.Type != events.OrderPlaced {
		t.Errorf("expected OrderPlaced event, got %s", event.Type)
	}

	payload, ok := event.Payload.(events.OrderPlacedPayload)
	if !ok {
		t.Fatal("expected payload to be OrderPlacedPayload")
	}
	if payload.OrderID != "ord-001" {
		t.Errorf("expected OrderID 'ord-001', got %q", payload.OrderID)
	}
}

func TestPlaceOrder_ValidationErrors(t *testing.T) {
	cmd, _, bus := newTestSetup(t)
	defer bus.Close()

	tests := []struct {
		name string
		cmd  write.PlaceOrderCmd
	}{
		{"missing OrderID", write.PlaceOrderCmd{CustomerID: "c1", Item: "x", Quantity: 1}},
		{"missing CustomerID", write.PlaceOrderCmd{OrderID: "o1", Item: "x", Quantity: 1}},
		{"missing Item", write.PlaceOrderCmd{OrderID: "o1", CustomerID: "c1", Quantity: 1}},
		{"zero Quantity", write.PlaceOrderCmd{OrderID: "o1", CustomerID: "c1", Item: "x", Quantity: 0}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := cmd.PlaceOrder(tc.cmd)
			if err == nil {
				t.Errorf("expected error for %s, got nil", tc.name)
			}
		})
	}
}

func TestShipOrder_Success(t *testing.T) {
	cmd, _, bus := newTestSetup(t)
	defer bus.Close()

	cmd.PlaceOrder(write.PlaceOrderCmd{ //nolint
		OrderID: "ord-001", CustomerID: "c1", Customer: "Alice", Item: "x", Quantity: 1,
	})
	<-bus.Subscribe() // drain PlaceOrder event

	err := cmd.ShipOrder(write.ShipOrderCmd{
		OrderID:        "ord-001",
		TrackingNumber: "UPS-12345",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestShipOrder_PublishesEvent(t *testing.T) {
	cmd, _, bus := newTestSetup(t)

	cmd.PlaceOrder(write.PlaceOrderCmd{ //nolint
		OrderID: "ord-001", CustomerID: "c1", Customer: "Alice", Item: "x", Quantity: 1,
	})
	<-bus.Subscribe() // drain PlaceOrder event

	cmd.ShipOrder(write.ShipOrderCmd{OrderID: "ord-001", TrackingNumber: "UPS-999"}) //nolint

	event := <-bus.Subscribe()
	if event.Type != events.OrderShipped {
		t.Errorf("expected OrderShipped event, got %s", event.Type)
	}

	payload := event.Payload.(events.OrderShippedPayload)
	if payload.TrackingNumber != "UPS-999" {
		t.Errorf("expected TrackingNumber 'UPS-999', got %q", payload.TrackingNumber)
	}
}

func TestShipOrder_UnknownOrder_ReturnsError(t *testing.T) {
	cmd, _, bus := newTestSetup(t)
	defer bus.Close()

	err := cmd.ShipOrder(write.ShipOrderCmd{OrderID: "nonexistent", TrackingNumber: "X"})
	if err == nil {
		t.Error("expected error for unknown order, got nil")
	}
}

// ── Query + Projector Tests ───────────────────────────────────────────────────

func TestProjector_OrderPlaced_CreatesReadModel(t *testing.T) {
	cmd, qry, bus := newTestSetup(t)
	defer bus.Close()

	cmd.PlaceOrder(write.PlaceOrderCmd{ //nolint
		OrderID: "ord-001", CustomerID: "cust-1", Customer: "Alice",
		Item: "Laptop", Quantity: 1, TotalCents: 129900,
	})
	projectEvent(t, bus, qry)

	summary, err := qry.GetOrder("ord-001")
	if err != nil {
		t.Fatalf("expected order in read DB, got error: %v", err)
	}
	if summary.Customer != "Alice" {
		t.Errorf("expected Customer 'Alice', got %q", summary.Customer)
	}
	if summary.Status != "placed" {
		t.Errorf("expected status 'placed', got %q", summary.Status)
	}
	if summary.TotalCents != 129900 {
		t.Errorf("expected TotalCents 129900, got %d", summary.TotalCents)
	}
}

func TestProjector_OrderShipped_UpdatesReadModel(t *testing.T) {
	cmd, qry, bus := newTestSetup(t)
	defer bus.Close()

	cmd.PlaceOrder(write.PlaceOrderCmd{ //nolint
		OrderID: "ord-001", CustomerID: "c1", Customer: "Alice", Item: "x", Quantity: 1,
	})
	projectEvent(t, bus, qry) // apply OrderPlaced

	cmd.ShipOrder(write.ShipOrderCmd{OrderID: "ord-001", TrackingNumber: "UPS-999"}) //nolint
	projectEvent(t, bus, qry)                                                        // apply OrderShipped

	summary, err := qry.GetOrder("ord-001")
	if err != nil {
		t.Fatalf("expected order, got error: %v", err)
	}
	if summary.Status != "shipped" {
		t.Errorf("expected status 'shipped', got %q", summary.Status)
	}
	if summary.TrackingNumber != "UPS-999" {
		t.Errorf("expected TrackingNumber 'UPS-999', got %q", summary.TrackingNumber)
	}
	if summary.ShippedAt == nil {
		t.Error("expected ShippedAt to be set after shipping")
	}
}

func TestQueryHandler_GetOrder_NotFound(t *testing.T) {
	_, qry, bus := newTestSetup(t)
	defer bus.Close()

	_, err := qry.GetOrder("nonexistent")
	if err == nil {
		t.Error("expected error for unknown order, got nil")
	}
}

func TestQueryHandler_ListOrders_ReturnsAll(t *testing.T) {
	cmd, qry, bus := newTestSetup(t)
	defer bus.Close()

	// Place 3 orders
	for i, name := range []string{"Alice", "Bob", "Carol"} {
		cmd.PlaceOrder(write.PlaceOrderCmd{ //nolint
			OrderID:    string(rune('A'+i)) + "-001",
			CustomerID: name,
			Customer:   name,
			Item:       "thing",
			Quantity:   1,
		})
		projectEvent(t, bus, qry)
	}

	summaries, err := qry.ListOrders()
	if err != nil {
		t.Fatalf("list orders error: %v", err)
	}
	if len(summaries) != 3 {
		t.Errorf("expected 3 orders, got %d", len(summaries))
	}
}

// TestCQRS_FullFlow exercises the complete path from command to query.
func TestCQRS_FullFlow(t *testing.T) {
	cmd, qry, bus := newTestSetup(t)
	defer bus.Close()

	// 1. Place order
	err := cmd.PlaceOrder(write.PlaceOrderCmd{
		OrderID: "ord-full", CustomerID: "c1", Customer: "Dave",
		Item: "Phone", Quantity: 2, TotalCents: 79800,
	})
	if err != nil {
		t.Fatalf("place order: %v", err)
	}
	projectEvent(t, bus, qry)

	// 2. Verify in read model (status = placed)
	s, _ := qry.GetOrder("ord-full")
	if s.Status != "placed" {
		t.Errorf("expected 'placed', got %q", s.Status)
	}
	if s.Customer != "Dave" {
		t.Errorf("expected customer 'Dave', got %q", s.Customer)
	}

	// 3. Ship the order
	err = cmd.ShipOrder(write.ShipOrderCmd{
		OrderID: "ord-full", TrackingNumber: "FedEx-777",
	})
	if err != nil {
		t.Fatalf("ship order: %v", err)
	}
	projectEvent(t, bus, qry)

	// 4. Verify updated in read model (status = shipped)
	s, _ = qry.GetOrder("ord-full")
	if s.Status != "shipped" {
		t.Errorf("expected 'shipped', got %q", s.Status)
	}
	if s.TrackingNumber != "FedEx-777" {
		t.Errorf("expected tracking 'FedEx-777', got %q", s.TrackingNumber)
	}
}
