// command_handler.go — Write side: handles commands that change state.
//
// Commands are actions that modify data:
//   - PlaceOrder:  create a new order in the write DB, publish OrderPlaced event
//   - ShipOrder:   mark an order as shipped, publish OrderShipped event
//
// The write DB schema is normalized — just what we need to store the facts.
// It's NOT optimized for reading (no joins, no denormalization).
//
// After every successful write, we publish an event so the read side can update.

package write

import (
	"database/sql"
	"fmt"
	"time"

	"microservices-go/services/06-cqrs/internal/events"

	_ "modernc.org/sqlite" // register the sqlite3 driver
)

// CommandHandler owns the write-side database and publishes events.
type CommandHandler struct {
	db  *sql.DB
	bus *events.Bus
}

// NewCommandHandler opens (or creates) the write DB at the given path and
// creates the schema. Pass ":memory:" for tests.
func NewCommandHandler(dbPath string, bus *events.Bus) (*CommandHandler, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open write db: %w", err)
	}

	// Create the orders table if it doesn't exist yet
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS orders (
			id            TEXT PRIMARY KEY,
			customer_id   TEXT NOT NULL,
			customer      TEXT NOT NULL,
			item          TEXT NOT NULL,
			quantity      INTEGER NOT NULL,
			total_cents   INTEGER NOT NULL,
			status        TEXT NOT NULL DEFAULT 'placed',
			tracking      TEXT,
			placed_at     DATETIME NOT NULL,
			shipped_at    DATETIME
		)
	`)
	if err != nil {
		return nil, fmt.Errorf("create orders table: %w", err)
	}

	return &CommandHandler{db: db, bus: bus}, nil
}

// PlaceOrderCmd is the data needed to place an order.
type PlaceOrderCmd struct {
	OrderID    string
	CustomerID string
	Customer   string
	Item       string
	Quantity   int
	TotalCents int
}

// PlaceOrder creates a new order in the write DB and publishes an OrderPlaced event.
func (h *CommandHandler) PlaceOrder(cmd PlaceOrderCmd) error {
	if cmd.OrderID == "" {
		return fmt.Errorf("OrderID is required")
	}
	if cmd.CustomerID == "" {
		return fmt.Errorf("CustomerID is required")
	}
	if cmd.Item == "" {
		return fmt.Errorf("Item is required")
	}
	if cmd.Quantity <= 0 {
		return fmt.Errorf("Quantity must be > 0")
	}

	_, err := h.db.Exec(`
		INSERT INTO orders (id, customer_id, customer, item, quantity, total_cents, status, placed_at)
		VALUES (?, ?, ?, ?, ?, ?, 'placed', ?)
	`, cmd.OrderID, cmd.CustomerID, cmd.Customer, cmd.Item, cmd.Quantity, cmd.TotalCents, time.Now())
	if err != nil {
		return fmt.Errorf("insert order: %w", err)
	}

	// Publish event so the read side can update its denormalized view
	h.bus.Publish(events.Event{
		Type: events.OrderPlaced,
		Payload: events.OrderPlacedPayload{
			OrderID:    cmd.OrderID,
			CustomerID: cmd.CustomerID,
			Customer:   cmd.Customer,
			Item:       cmd.Item,
			Quantity:   cmd.Quantity,
			TotalCents: cmd.TotalCents,
		},
	})

	return nil
}

// ShipOrderCmd is the data needed to mark an order as shipped.
type ShipOrderCmd struct {
	OrderID        string
	TrackingNumber string
}

// ShipOrder updates the order status and publishes an OrderShipped event.
func (h *CommandHandler) ShipOrder(cmd ShipOrderCmd) error {
	if cmd.OrderID == "" {
		return fmt.Errorf("OrderID is required")
	}

	result, err := h.db.Exec(`
		UPDATE orders
		SET status = 'shipped', tracking = ?, shipped_at = ?
		WHERE id = ? AND status = 'placed'
	`, cmd.TrackingNumber, time.Now(), cmd.OrderID)
	if err != nil {
		return fmt.Errorf("update order: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("order %q not found or already shipped", cmd.OrderID)
	}

	h.bus.Publish(events.Event{
		Type: events.OrderShipped,
		Payload: events.OrderShippedPayload{
			OrderID:        cmd.OrderID,
			TrackingNumber: cmd.TrackingNumber,
		},
	})

	return nil
}

// Close releases the database connection.
func (h *CommandHandler) Close() error {
	return h.db.Close()
}
