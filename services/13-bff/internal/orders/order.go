// order.go — Shared Order type used by all three BFF handlers.
//
// In a real system, this data would come from an order microservice over the
// network. Here we define it as a plain Go struct so the BFF demo can focus on
// the response-shaping logic without needing a real HTTP backend.
//
// All three BFFs (web, mobile, admin) import this package and receive the same
// []Order slice — they just shape the response differently for their client.

package orders

import "time"

// Order represents a customer order in the system.
// This is the "raw" domain object — all fields included.
// Each BFF will choose which subset of these fields to expose to its client.
type Order struct {
	// ID is the unique identifier for the order.
	ID string

	// CustomerName is the full name of the customer who placed the order.
	CustomerName string

	// Item is the name of the product ordered.
	Item string

	// Quantity is how many units of the item were ordered.
	Quantity int

	// TotalCents is the total order value in cents (e.g. 4999 = $49.99).
	// Using integer cents avoids floating-point rounding problems.
	TotalCents int

	// Status is the current state of the order (e.g. "pending", "shipped", "delivered").
	Status string

	// CreatedAt is when the order was placed.
	CreatedAt time.Time
}

// SampleOrders returns a fixed set of orders used by the demo server and tests.
// In production this would be replaced by a call to the order service.
func SampleOrders() []Order {
	return []Order{
		{
			ID:           "ord-001",
			CustomerName: "Alice Chen",
			Item:         "Wireless Keyboard",
			Quantity:     1,
			TotalCents:   4999,
			Status:       "delivered",
			CreatedAt:    time.Date(2026, 4, 10, 9, 0, 0, 0, time.UTC),
		},
		{
			ID:           "ord-002",
			CustomerName: "Bob Müller",
			Item:         "USB-C Hub",
			Quantity:     2,
			TotalCents:   3498,
			Status:       "shipped",
			CreatedAt:    time.Date(2026, 4, 12, 14, 30, 0, 0, time.UTC),
		},
		{
			ID:           "ord-003",
			CustomerName: "Carol Okafor",
			Item:         "Laptop Stand",
			Quantity:     1,
			TotalCents:   2999,
			Status:       "pending",
			CreatedAt:    time.Date(2026, 4, 14, 11, 15, 0, 0, time.UTC),
		},
		{
			ID:           "ord-004",
			CustomerName: "Dave Singh",
			Item:         "Mechanical Keyboard",
			Quantity:     1,
			TotalCents:   8999,
			Status:       "pending",
			CreatedAt:    time.Date(2026, 4, 15, 16, 45, 0, 0, time.UTC),
		},
		{
			ID:           "ord-005",
			CustomerName: "Eve Nakamura",
			Item:         "Monitor Light Bar",
			Quantity:     3,
			TotalCents:   5997,
			Status:       "shipped",
			CreatedAt:    time.Date(2026, 4, 16, 8, 0, 0, 0, time.UTC),
		},
	}
}
