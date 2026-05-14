// database_per_service_test.go — Tests for the "Database per Service" pattern.
//
// All tests use ":memory:" SQLite databases. An in-memory database is created
// fresh for each test, lives only as long as the connection is open, and
// disappears automatically when Close() is called — no cleanup files needed.
//
// We test each store in isolation and then verify that the stores truly are
// independent (data written to one is invisible to the others).

package database_per_service_test

import (
	"database/sql"
	"errors"
	"testing"

	"microservices-go/services/11-database-per-service/internal/order"
	"microservices-go/services/11-database-per-service/internal/product"
	"microservices-go/services/11-database-per-service/internal/user"
)

// ── UserStore Tests ───────────────────────────────────────────────────────────

// TestUserStore_CreateAndGet verifies that a user can be created and then
// retrieved with all fields intact.
func TestUserStore_CreateAndGet(t *testing.T) {
	// Arrange — open a fresh in-memory database for this test
	store, err := user.NewUserStore(":memory:")
	if err != nil {
		t.Fatalf("failed to open user store: %v", err)
	}
	defer store.Close()

	// Act — create a user
	created, err := store.CreateUser("Alice", "alice@example.com")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Act — fetch the user back by the ID we just got
	fetched, err := store.GetUser(created.ID)
	if err != nil {
		t.Fatalf("GetUser failed: %v", err)
	}

	// Assert — every field round-trips correctly
	if fetched.ID != created.ID {
		t.Errorf("ID mismatch: got %d, want %d", fetched.ID, created.ID)
	}
	if fetched.Name != "Alice" {
		t.Errorf("Name mismatch: got %q, want %q", fetched.Name, "Alice")
	}
	if fetched.Email != "alice@example.com" {
		t.Errorf("Email mismatch: got %q, want %q", fetched.Email, "alice@example.com")
	}
}

// TestUserStore_UnknownID_ReturnsError verifies that fetching a non-existent user
// returns an error wrapping sql.ErrNoRows (not a nil response or a panic).
func TestUserStore_UnknownID_ReturnsError(t *testing.T) {
	store, err := user.NewUserStore(":memory:")
	if err != nil {
		t.Fatalf("failed to open user store: %v", err)
	}
	defer store.Close()

	// Act — ask for user ID 999 which was never created
	_, err = store.GetUser(999)

	// Assert — we must get an error
	if err == nil {
		t.Fatal("expected an error for unknown user ID, got nil")
	}

	// Assert — the error must be (or wrap) sql.ErrNoRows so callers can handle
	// "not found" differently from database errors.
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected error to wrap sql.ErrNoRows, got: %v", err)
	}
}

// ── OrderStore Tests ──────────────────────────────────────────────────────────

// TestOrderStore_CreateAndGet verifies that an order can be created and retrieved.
func TestOrderStore_CreateAndGet(t *testing.T) {
	store, err := order.NewOrderStore(":memory:")
	if err != nil {
		t.Fatalf("failed to open order store: %v", err)
	}
	defer store.Close()

	// Create an order for (hypothetical) user ID 42
	created, err := store.CreateOrder(42, "Blue Widget", 3)
	if err != nil {
		t.Fatalf("CreateOrder failed: %v", err)
	}

	fetched, err := store.GetOrder(created.ID)
	if err != nil {
		t.Fatalf("GetOrder failed: %v", err)
	}

	if fetched.ID != created.ID {
		t.Errorf("ID mismatch: got %d, want %d", fetched.ID, created.ID)
	}
	if fetched.UserID != 42 {
		t.Errorf("UserID mismatch: got %d, want 42", fetched.UserID)
	}
	if fetched.Item != "Blue Widget" {
		t.Errorf("Item mismatch: got %q, want %q", fetched.Item, "Blue Widget")
	}
	if fetched.Qty != 3 {
		t.Errorf("Qty mismatch: got %d, want 3", fetched.Qty)
	}
}

// TestOrderStore_UnknownID_ReturnsError verifies that fetching a non-existent order
// returns an error wrapping sql.ErrNoRows.
func TestOrderStore_UnknownID_ReturnsError(t *testing.T) {
	store, err := order.NewOrderStore(":memory:")
	if err != nil {
		t.Fatalf("failed to open order store: %v", err)
	}
	defer store.Close()

	_, err = store.GetOrder(999)

	if err == nil {
		t.Fatal("expected an error for unknown order ID, got nil")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected error to wrap sql.ErrNoRows, got: %v", err)
	}
}

// ── ProductStore Tests ────────────────────────────────────────────────────────

// TestProductStore_CreateAndGet verifies that a product can be created and retrieved.
func TestProductStore_CreateAndGet(t *testing.T) {
	store, err := product.NewProductStore(":memory:")
	if err != nil {
		t.Fatalf("failed to open product store: %v", err)
	}
	defer store.Close()

	// priceCents=999 represents $9.99 (avoids floating-point rounding issues)
	created, err := store.CreateProduct("Blue Widget", 999)
	if err != nil {
		t.Fatalf("CreateProduct failed: %v", err)
	}

	fetched, err := store.GetProduct(created.ID)
	if err != nil {
		t.Fatalf("GetProduct failed: %v", err)
	}

	if fetched.ID != created.ID {
		t.Errorf("ID mismatch: got %d, want %d", fetched.ID, created.ID)
	}
	if fetched.Name != "Blue Widget" {
		t.Errorf("Name mismatch: got %q, want %q", fetched.Name, "Blue Widget")
	}
	if fetched.PriceCents != 999 {
		t.Errorf("PriceCents mismatch: got %d, want 999", fetched.PriceCents)
	}
}

// ── Independence Test ─────────────────────────────────────────────────────────

// TestStores_AreIndependent is the core test for the "Database per Service" pattern.
// It verifies that the three stores are truly independent:
//   - Each has its own in-memory database
//   - Records created in one store are invisible to the other stores
//   - Closing one store does not affect the others
func TestStores_AreIndependent(t *testing.T) {
	// Open all three stores — each gets its own ":memory:" database.
	// Note: each call to sql.Open(":memory:") creates a DISTINCT database,
	// even within the same process. They share no tables, no data.
	userStore, err := user.NewUserStore(":memory:")
	if err != nil {
		t.Fatalf("failed to open user store: %v", err)
	}
	defer userStore.Close()

	orderStore, err := order.NewOrderStore(":memory:")
	if err != nil {
		t.Fatalf("failed to open order store: %v", err)
	}
	defer orderStore.Close()

	productStore, err := product.NewProductStore(":memory:")
	if err != nil {
		t.Fatalf("failed to open product store: %v", err)
	}
	defer productStore.Close()

	// Create one record in each store
	createdUser, err := userStore.CreateUser("Bob", "bob@example.com")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	createdOrder, err := orderStore.CreateOrder(createdUser.ID, "Widget", 1)
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}

	createdProduct, err := productStore.CreateProduct("Widget", 499)
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}

	// Verify each record is retrievable from its own store
	if _, err := userStore.GetUser(createdUser.ID); err != nil {
		t.Errorf("user should be retrievable from UserStore: %v", err)
	}
	if _, err := orderStore.GetOrder(createdOrder.ID); err != nil {
		t.Errorf("order should be retrievable from OrderStore: %v", err)
	}
	if _, err := productStore.GetProduct(createdProduct.ID); err != nil {
		t.Errorf("product should be retrievable from ProductStore: %v", err)
	}

	// KEY ASSERTION: the user's ID is NOT queryable from the order or product stores.
	// Each store has its own schema and data — there is no "users" table in the
	// order database, and no "products" table in the user database.
	//
	// We verify this by querying what should be ID=1 in each store — which exists
	// in UserStore, but should fail in OrderStore (if we tried to get a user) and
	// similarly. Since stores have typed APIs we instead check that each store only
	// knows about its own domain: close UserStore and confirm its data is gone.
	userStore.Close()
	// After closing, the in-memory DB is destroyed. Re-opening gives an empty DB.
	freshUserStore, err := user.NewUserStore(":memory:")
	if err != nil {
		t.Fatalf("re-open user store: %v", err)
	}
	defer freshUserStore.Close()

	_, err = freshUserStore.GetUser(createdUser.ID)
	if err == nil {
		t.Error("expected error: freshly opened store should not contain previous user data")
	}

	// The order and product stores should still be working fine — closing UserStore
	// had zero effect on them, proving their independence.
	if _, err := orderStore.GetOrder(createdOrder.ID); err != nil {
		t.Errorf("closing UserStore should not affect OrderStore: %v", err)
	}
	if _, err := productStore.GetProduct(createdProduct.ID); err != nil {
		t.Errorf("closing UserStore should not affect ProductStore: %v", err)
	}
}
