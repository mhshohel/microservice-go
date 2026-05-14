// servers.go — Dummy backend services for the API Gateway demo.
//
// In a real microservices system, User Service, Product Service, and Order Service
// would be three separate applications running on different machines or containers.
//
// For this demo, we run all three as lightweight goroutines in the same process.
// This lets you see the gateway in action without running 4 separate programs.
//
// Each service is deliberately simple — it just returns hardcoded JSON data
// so we can focus on how the gateway routes and proxies, not on business logic.

package backend

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// Addresses holds the base URLs for all three dummy backend services.
type Addresses struct {
	Users    string // e.g. "http://localhost:9001"
	Products string // e.g. "http://localhost:9002"
	Orders   string // e.g. "http://localhost:9003"
}

// Start launches all three dummy services as background goroutines and returns
// their addresses so the gateway can route to them.
func Start() Addresses {
	addrs := Addresses{
		Users:    "http://localhost:9001",
		Products: "http://localhost:9002",
		Orders:   "http://localhost:9003",
	}

	go startUsers(":9001")
	go startProducts(":9002")
	go startOrders(":9003")

	return addrs
}

// startUsers runs a minimal HTTP server representing the User Service.
func startUsers(addr string) {
	mux := http.NewServeMux()

	// GET /users → return all users
	mux.HandleFunc("GET /users", func(w http.ResponseWriter, r *http.Request) {
		slog.Info("UserService: GET /users")
		writeJSON(w, []map[string]any{
			{"id": "1", "name": "Alice Johnson", "email": "alice@example.com", "role": "customer"},
			{"id": "2", "name": "Bob Smith", "email": "bob@example.com", "role": "customer"},
			{"id": "3", "name": "Carol Admin", "email": "carol@example.com", "role": "admin"},
		})
	})

	// GET /users/{id} → return a single user
	mux.HandleFunc("GET /users/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		slog.Info("UserService: GET /users/{id}", "id", id)
		writeJSON(w, map[string]any{
			"id":    id,
			"name":  "Alice Johnson",
			"email": "alice@example.com",
			"role":  "customer",
		})
	})

	slog.Info("User Service started", "addr", "http://localhost"+addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("User Service crashed", "error", err)
	}
}

// startProducts runs a minimal HTTP server representing the Product Service.
func startProducts(addr string) {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /products", func(w http.ResponseWriter, r *http.Request) {
		slog.Info("ProductService: GET /products")
		writeJSON(w, []map[string]any{
			{"id": "p1", "name": "Laptop Pro", "price": 1299.99, "stock": 15},
			{"id": "p2", "name": "Wireless Mouse", "price": 29.99, "stock": 200},
			{"id": "p3", "name": "Mechanical Keyboard", "price": 89.99, "stock": 75},
		})
	})

	mux.HandleFunc("GET /products/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		slog.Info("ProductService: GET /products/{id}", "id", id)
		writeJSON(w, map[string]any{
			"id":          id,
			"name":        "Laptop Pro",
			"price":       1299.99,
			"stock":       15,
			"description": "High-performance laptop for developers",
		})
	})

	slog.Info("Product Service started", "addr", "http://localhost"+addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("Product Service crashed", "error", err)
	}
}

// startOrders runs a minimal HTTP server representing the Order Service.
func startOrders(addr string) {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /orders", func(w http.ResponseWriter, r *http.Request) {
		slog.Info("OrderService: GET /orders")
		writeJSON(w, []map[string]any{
			{"id": "o1", "userID": "1", "product": "Laptop Pro", "status": "delivered", "total": 1299.99},
			{"id": "o2", "userID": "2", "product": "Wireless Mouse", "status": "pending", "total": 29.99},
		})
	})

	mux.HandleFunc("GET /orders/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		slog.Info("OrderService: GET /orders/{id}", "id", id)
		writeJSON(w, map[string]any{
			"id":      id,
			"userID":  "1",
			"product": "Laptop Pro",
			"status":  "delivered",
			"total":   1299.99,
		})
	})

	slog.Info("Order Service started", "addr", "http://localhost"+addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("Order Service crashed", "error", err)
	}
}

// writeJSON writes any value as a JSON response.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
