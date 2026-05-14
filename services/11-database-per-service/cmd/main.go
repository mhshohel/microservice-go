// main.go — Demo for the "Database per Service" pattern.
//
// This program shows three services (User, Order, Product) each owning their
// own independent database. In a real system these three databases would live
// on separate hosts and each service would run in its own process (or container).
//
// Here we run them all in the same process to keep the demo self-contained,
// but the key insight is visible in the code: each store gets its own separate
// database connection and its own schema. No store can read another's tables.
//
// HOW TO RUN:
//
//	go run ./services/11-database-per-service/cmd/main.go

package main

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"microservices-go/services/11-database-per-service/internal/order"
	"microservices-go/services/11-database-per-service/internal/product"
	"microservices-go/services/11-database-per-service/internal/user"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	fmt.Println("══════════════════════════════════════════")
	fmt.Println("  Database per Service — Pattern Demo")
	fmt.Println("══════════════════════════════════════════")

	// ── User Service database ─────────────────────────────────────────────────
	// In production this would be something like postgres://user-svc-db/users.
	// We use a named file so you can inspect it with sqlite3 after running.
	// Each service has its OWN file — they never share.
	userStore, err := user.NewUserStore("users.db")
	if err != nil {
		slog.Error("failed to open user store", "error", err)
		os.Exit(1)
	}
	defer userStore.Close()
	defer os.Remove("users.db") // clean up demo file on exit

	// ── Order Service database ────────────────────────────────────────────────
	orderStore, err := order.NewOrderStore("orders.db")
	if err != nil {
		slog.Error("failed to open order store", "error", err)
		os.Exit(1)
	}
	defer orderStore.Close()
	defer os.Remove("orders.db")

	// ── Product Service database ──────────────────────────────────────────────
	productStore, err := product.NewProductStore("products.db")
	if err != nil {
		slog.Error("failed to open product store", "error", err)
		os.Exit(1)
	}
	defer productStore.Close()
	defer os.Remove("products.db")

	// ── Create sample data ────────────────────────────────────────────────────
	fmt.Println("\n─── Creating sample data ───")

	// User service creates a user
	alice, err := userStore.CreateUser("Alice", "alice@example.com")
	if err != nil {
		slog.Error("create user", "error", err)
		os.Exit(1)
	}
	slog.Info("[user-svc]    created user", "id", alice.ID, "name", alice.Name)

	// Product service creates a product — completely independent database
	widget, err := productStore.CreateProduct("Blue Widget", 999) // $9.99
	if err != nil {
		slog.Error("create product", "error", err)
		os.Exit(1)
	}
	slog.Info("[product-svc] created product", "id", widget.ID, "name", widget.Name, "price_cents", widget.PriceCents)

	// Order service creates an order referencing the user by ID.
	// The Order service does NOT query the User database — it trusts that
	// userID 1 was validated before this call (by the application layer).
	ord, err := orderStore.CreateOrder(alice.ID, widget.Name, 2)
	if err != nil {
		slog.Error("create order", "error", err)
		os.Exit(1)
	}
	slog.Info("[order-svc]   created order", "id", ord.ID, "user_id", ord.UserID, "item", ord.Item, "qty", ord.Qty)

	// ── Fetch and display ─────────────────────────────────────────────────────
	fmt.Println("\n─── Querying each service's own database ───")

	fetchedUser, err := userStore.GetUser(alice.ID)
	if err != nil {
		slog.Error("get user", "error", err)
		os.Exit(1)
	}
	slog.Info("[user-svc]    fetched user", "id", fetchedUser.ID, "name", fetchedUser.Name, "email", fetchedUser.Email)

	fetchedProduct, err := productStore.GetProduct(widget.ID)
	if err != nil {
		slog.Error("get product", "error", err)
		os.Exit(1)
	}
	slog.Info("[product-svc] fetched product", "id", fetchedProduct.ID, "name", fetchedProduct.Name, "price_cents", fetchedProduct.PriceCents)

	fetchedOrder, err := orderStore.GetOrder(ord.ID)
	if err != nil {
		slog.Error("get order", "error", err)
		os.Exit(1)
	}
	slog.Info("[order-svc]   fetched order", "id", fetchedOrder.ID, "user_id", fetchedOrder.UserID, "item", fetchedOrder.Item)

	// ── Demonstrate isolation ─────────────────────────────────────────────────
	fmt.Println("\n─── Demonstrating isolation ───")
	fmt.Println("Each store can only see its own data.")
	fmt.Println("Requesting user ID 999 from the User store:")

	_, err = userStore.GetUser(999)
	if errors.Is(err, sql.ErrNoRows) {
		fmt.Println("  → Not found (expected) — user 999 does not exist in the User DB")
	} else if err != nil {
		// Some other database error
		fmt.Printf("  → Error: %v\n", err)
	}

	fmt.Println("\nDone. Each service managed its own database independently.")
}
