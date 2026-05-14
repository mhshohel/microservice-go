// query_handler.go — Read side: handles queries and keeps the read DB up to date.
//
// The read DB has a denormalized schema optimized for fast reads.
// A single row in `order_summaries` has ALL the information for displaying
// an order — no joins needed.
//
// The Projector listens to events from the write side and updates the read DB.
// This is "eventual consistency": after a PlaceOrder command, the read model
// catches up asynchronously (in this demo, synchronously via channel).

package read

import (
	"database/sql"
	"fmt"
	"time"

	"microservices-go/services/06-cqrs/internal/events"

	_ "modernc.org/sqlite"
)

// OrderSummary is the denormalized read model for an order.
// One row = everything needed to display an order summary.
type OrderSummary struct {
	OrderID        string
	CustomerID     string
	Customer       string
	Item           string
	Quantity       int
	TotalCents     int
	Status         string
	TrackingNumber string
	PlacedAt       time.Time
	ShippedAt      *time.Time // pointer so it can be nil (not shipped yet)
}

// QueryHandler owns the read-side database and handles read queries.
type QueryHandler struct {
	db *sql.DB
}

// NewQueryHandler opens (or creates) the read DB and creates its schema.
// Pass ":memory:" for tests.
func NewQueryHandler(dbPath string) (*QueryHandler, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open read db: %w", err)
	}

	// Denormalized table: all order info in one row for fast reads
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS order_summaries (
			order_id        TEXT PRIMARY KEY,
			customer_id     TEXT NOT NULL,
			customer        TEXT NOT NULL,
			item            TEXT NOT NULL,
			quantity        INTEGER NOT NULL,
			total_cents     INTEGER NOT NULL,
			status          TEXT NOT NULL DEFAULT 'placed',
			tracking_number TEXT,
			placed_at       DATETIME NOT NULL,
			shipped_at      DATETIME
		)
	`)
	if err != nil {
		return nil, fmt.Errorf("create order_summaries table: %w", err)
	}

	return &QueryHandler{db: db}, nil
}

// GetOrder returns the summary for a single order by ID.
// Returns an error if not found.
func (h *QueryHandler) GetOrder(orderID string) (*OrderSummary, error) {
	row := h.db.QueryRow(`
		SELECT order_id, customer_id, customer, item, quantity, total_cents,
		       status, COALESCE(tracking_number,''), placed_at, shipped_at
		FROM order_summaries
		WHERE order_id = ?
	`, orderID)

	return scanSummary(row)
}

// ListOrders returns all order summaries, newest first.
func (h *QueryHandler) ListOrders() ([]OrderSummary, error) {
	rows, err := h.db.Query(`
		SELECT order_id, customer_id, customer, item, quantity, total_cents,
		       status, COALESCE(tracking_number,''), placed_at, shipped_at
		FROM order_summaries
		ORDER BY placed_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list orders: %w", err)
	}
	defer rows.Close()

	var summaries []OrderSummary
	for rows.Next() {
		summary, err := scanSummary(rows)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, *summary)
	}
	return summaries, rows.Err()
}

// ── Projector ─────────────────────────────────────────────────────────────────

// Project listens to the event bus and updates the read DB for each event.
// Run this in a goroutine. It exits when the bus channel is closed.
func (h *QueryHandler) Project(bus *events.Bus) {
	for event := range bus.Subscribe() {
		switch event.Type {

		case events.OrderPlaced:
			p := event.Payload.(events.OrderPlacedPayload)
			h.db.Exec(`
				INSERT OR REPLACE INTO order_summaries
				(order_id, customer_id, customer, item, quantity, total_cents, status, placed_at)
				VALUES (?, ?, ?, ?, ?, ?, 'placed', ?)
			`, p.OrderID, p.CustomerID, p.Customer, p.Item, p.Quantity, p.TotalCents, time.Now())

		case events.OrderShipped:
			p := event.Payload.(events.OrderShippedPayload)
			h.db.Exec(`
				UPDATE order_summaries
				SET status = 'shipped', tracking_number = ?, shipped_at = ?
				WHERE order_id = ?
			`, p.TrackingNumber, time.Now(), p.OrderID)
		}
	}
}

// ProjectSync processes one event synchronously (useful for tests).
func (h *QueryHandler) ProjectSync(event events.Event) {
	switch event.Type {
	case events.OrderPlaced:
		p := event.Payload.(events.OrderPlacedPayload)
		h.db.Exec(`
			INSERT OR REPLACE INTO order_summaries
			(order_id, customer_id, customer, item, quantity, total_cents, status, placed_at)
			VALUES (?, ?, ?, ?, ?, ?, 'placed', ?)
		`, p.OrderID, p.CustomerID, p.Customer, p.Item, p.Quantity, p.TotalCents, time.Now())

	case events.OrderShipped:
		p := event.Payload.(events.OrderShippedPayload)
		h.db.Exec(`
			UPDATE order_summaries
			SET status = 'shipped', tracking_number = ?, shipped_at = ?
			WHERE order_id = ?
		`, p.TrackingNumber, time.Now(), p.OrderID)
	}
}

// Close releases the database connection.
func (h *QueryHandler) Close() error {
	return h.db.Close()
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanSummary(s scanner) (*OrderSummary, error) {
	var sum OrderSummary
	var shippedAt sql.NullTime

	err := s.Scan(
		&sum.OrderID,
		&sum.CustomerID,
		&sum.Customer,
		&sum.Item,
		&sum.Quantity,
		&sum.TotalCents,
		&sum.Status,
		&sum.TrackingNumber,
		&sum.PlacedAt,
		&shippedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("order not found")
		}
		return nil, fmt.Errorf("scan order: %w", err)
	}

	if shippedAt.Valid {
		sum.ShippedAt = &shippedAt.Time
	}
	return &sum, nil
}
