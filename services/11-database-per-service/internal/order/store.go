// store.go — OrderStore for the "Database per Service" demo.
//
// The Order service owns this database. It stores orders — each order belongs
// to a user (referenced by userID) and contains a line item and quantity.
//
// IMPORTANT: The Order service stores userID as a plain integer. It does NOT
// have a foreign-key constraint pointing at the User service's database, because
// in a "Database per Service" architecture the two databases are completely
// separate — they may not even be on the same host. Referential integrity between
// services is maintained at the application layer (e.g., validate that the user
// exists by calling the User service's HTTP API before creating an order).
//
// This file uses SQLite via the pure-Go modernc.org/sqlite driver.

package order

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Order represents a row in the orders table.
type Order struct {
	ID     int64  // auto-incremented primary key
	UserID int64  // ID of the user who placed this order (from the User service)
	Item   string // name or SKU of the item ordered
	Qty    int    // quantity ordered (must be > 0)
}

// OrderStore holds the database connection for the Order service.
// This is a completely separate database from UserStore and ProductStore —
// that's the whole point of "Database per Service".
type OrderStore struct {
	db *sql.DB
}

// NewOrderStore opens (or creates) a SQLite database at the given path and
// ensures the orders table exists.
//
// Pass ":memory:" for tests to get a temporary in-memory database.
func NewOrderStore(dbPath string) (*OrderStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open order db: %w", err)
	}

	const createTable = `
		CREATE TABLE IF NOT EXISTS orders (
			id      INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			item    TEXT    NOT NULL,
			qty     INTEGER NOT NULL
		)
	`
	if _, err := db.Exec(createTable); err != nil {
		return nil, fmt.Errorf("create orders table: %w", err)
	}

	return &OrderStore{db: db}, nil
}

// CreateOrder inserts a new order and returns the created record with its assigned ID.
func (s *OrderStore) CreateOrder(userID int64, item string, qty int) (*Order, error) {
	result, err := s.db.Exec(
		"INSERT INTO orders (user_id, item, qty) VALUES (?, ?, ?)",
		userID, item, qty,
	)
	if err != nil {
		return nil, fmt.Errorf("insert order: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("get last insert id: %w", err)
	}

	return &Order{ID: id, UserID: userID, Item: item, Qty: qty}, nil
}

// GetOrder fetches an order by its ID. Returns an error wrapping sql.ErrNoRows
// if the order does not exist.
func (s *OrderStore) GetOrder(id int64) (*Order, error) {
	order := &Order{}

	err := s.db.QueryRow(
		"SELECT id, user_id, item, qty FROM orders WHERE id = ?", id,
	).Scan(&order.ID, &order.UserID, &order.Item, &order.Qty)

	if err != nil {
		return nil, fmt.Errorf("get order %d: %w", id, err)
	}

	return order, nil
}

// Close releases the database connection.
func (s *OrderStore) Close() error {
	return s.db.Close()
}
