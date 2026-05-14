// store.go — ProductStore for the "Database per Service" demo.
//
// The Product service owns this database. It stores product catalogue entries:
// name and price. No other service connects to this database directly.
//
// Why priceCents instead of a float?
// Storing money as an integer number of cents (or the smallest currency unit)
// avoids floating-point rounding errors. $9.99 is stored as 999, never 9.990000001.
//
// This file uses SQLite via the pure-Go modernc.org/sqlite driver.

package product

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Product represents a row in the products table.
type Product struct {
	ID         int64  // auto-incremented primary key
	Name       string // product display name
	PriceCents int    // price in the smallest currency unit (e.g. US cents)
}

// ProductStore holds the database connection for the Product service.
// Its schema is entirely independent from UserStore and OrderStore.
type ProductStore struct {
	db *sql.DB
}

// NewProductStore opens (or creates) a SQLite database at the given path and
// ensures the products table exists.
//
// Pass ":memory:" for tests to get a disposable in-memory database.
func NewProductStore(dbPath string) (*ProductStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open product db: %w", err)
	}

	const createTable = `
		CREATE TABLE IF NOT EXISTS products (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			name        TEXT    NOT NULL,
			price_cents INTEGER NOT NULL
		)
	`
	if _, err := db.Exec(createTable); err != nil {
		return nil, fmt.Errorf("create products table: %w", err)
	}

	return &ProductStore{db: db}, nil
}

// CreateProduct inserts a new product and returns the created record with its assigned ID.
func (s *ProductStore) CreateProduct(name string, priceCents int) (*Product, error) {
	result, err := s.db.Exec(
		"INSERT INTO products (name, price_cents) VALUES (?, ?)",
		name, priceCents,
	)
	if err != nil {
		return nil, fmt.Errorf("insert product: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("get last insert id: %w", err)
	}

	return &Product{ID: id, Name: name, PriceCents: priceCents}, nil
}

// GetProduct fetches a product by its ID. Returns an error wrapping sql.ErrNoRows
// if no product with that ID exists.
func (s *ProductStore) GetProduct(id int64) (*Product, error) {
	product := &Product{}

	err := s.db.QueryRow(
		"SELECT id, name, price_cents FROM products WHERE id = ?", id,
	).Scan(&product.ID, &product.Name, &product.PriceCents)

	if err != nil {
		return nil, fmt.Errorf("get product %d: %w", id, err)
	}

	return product, nil
}

// Close releases the database connection.
func (s *ProductStore) Close() error {
	return s.db.Close()
}
