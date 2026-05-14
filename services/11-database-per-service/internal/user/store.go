// store.go — UserStore for the "Database per Service" demo.
//
// In a real microservices system, the User service owns this database entirely.
// No other service can connect to it directly — they must call the User service
// over HTTP/gRPC if they need user data. This keeps the schema decoupled: the
// User service can rename columns, migrate tables, or switch database engines
// without breaking any other service.
//
// This file uses SQLite (via modernc.org/sqlite, a pure-Go driver) so the demo
// runs without installing any external database. In production this would be
// Postgres, MySQL, or another engine of the team's choice.

package user

import (
	"database/sql"
	"fmt"

	// Blank import registers the "sqlite" driver with database/sql.
	// We never call anything from this package directly — database/sql uses
	// it under the hood when we call sql.Open("sqlite", ...).
	_ "modernc.org/sqlite"
)

// User represents a row in the users table.
// Keeping the struct simple makes it easy to see what data this service owns.
type User struct {
	ID    int64  // auto-incremented primary key
	Name  string // display name
	Email string // unique email address
}

// UserStore holds the database connection for the User service.
// Only this package (and the service that imports it) can access user data.
// Other services — Order, Product — have their own separate stores and databases.
type UserStore struct {
	db *sql.DB // the open database connection
}

// NewUserStore opens (or creates) a SQLite database at the given path and
// sets up the users table if it does not already exist.
//
// Pass ":memory:" as the path to get a fresh in-memory database — perfect
// for tests because it vanishes automatically when the connection is closed.
func NewUserStore(dbPath string) (*UserStore, error) {
	// sql.Open does not actually connect yet — it just prepares the driver.
	// The real connection happens on the first query (or when we call Ping).
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open user db: %w", err)
	}

	// Create the users table if it doesn't exist yet.
	// We run this every time the store is created — "IF NOT EXISTS" makes it
	// idempotent (safe to call multiple times without duplicating the table).
	const createTable = `
		CREATE TABLE IF NOT EXISTS users (
			id    INTEGER PRIMARY KEY AUTOINCREMENT,
			name  TEXT    NOT NULL,
			email TEXT    NOT NULL UNIQUE
		)
	`
	if _, err := db.Exec(createTable); err != nil {
		return nil, fmt.Errorf("create users table: %w", err)
	}

	return &UserStore{db: db}, nil
}

// CreateUser inserts a new user into the database and returns the created record
// (including the database-assigned ID). Returns an error if the email already exists.
func (s *UserStore) CreateUser(name, email string) (*User, error) {
	// Prepared statement with ? placeholders prevents SQL injection.
	// Never build SQL by string concatenation with user input.
	result, err := s.db.Exec(
		"INSERT INTO users (name, email) VALUES (?, ?)",
		name, email,
	)
	if err != nil {
		return nil, fmt.Errorf("insert user: %w", err)
	}

	// LastInsertId returns the auto-incremented ID assigned to the new row.
	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("get last insert id: %w", err)
	}

	return &User{ID: id, Name: name, Email: email}, nil
}

// GetUser fetches a user by their ID. Returns an error wrapping sql.ErrNoRows
// if no user with that ID exists, so callers can distinguish "not found" from
// other database errors.
func (s *UserStore) GetUser(id int64) (*User, error) {
	user := &User{}

	// QueryRow returns exactly one row (or a row with an error inside it).
	// Scan reads each column value into the corresponding variable.
	err := s.db.QueryRow(
		"SELECT id, name, email FROM users WHERE id = ?", id,
	).Scan(&user.ID, &user.Name, &user.Email)

	if err != nil {
		// sql.ErrNoRows is the sentinel error for "no matching row found".
		// Wrapping it preserves that error type for callers using errors.Is.
		return nil, fmt.Errorf("get user %d: %w", id, err)
	}

	return user, nil
}

// Close releases the database connection. Call this when the store is no longer needed.
func (s *UserStore) Close() error {
	return s.db.Close()
}
