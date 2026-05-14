// main.go — Entry point for the Bulkhead Pattern demo.
//
// This demo runs an HTTP server on :8084 with two separate bulkheads:
//
//   user-bulkhead    — allows up to 5 concurrent calls to the "user service"
//   payment-bulkhead — allows up to 3 concurrent calls to the "payment service"
//
// The key point to observe: even when the payment pool is completely saturated
// (all 3 slots occupied), the user endpoints continue to work normally because
// they draw from a completely separate pool.
//
// HOW TO RUN (from the project root):
//
//	go run ./services/12-bulkhead/cmd/main.go
//
// HOW TO DEMO:
//
//	# Normal user call (fast, 5-slot pool)
//	curl http://localhost:8084/api/users
//
//	# Normal payment call (slightly slower, 3-slot pool)
//	curl http://localhost:8084/api/payments
//
//	# Slow payment call — holds a slot for 5 seconds
//	curl "http://localhost:8084/api/payments?slow=true"
//
//	# After 3 slow payment calls are in-flight, new payment calls are rejected
//	# but user calls still work fine — that is the bulkhead in action.
//
//	# View live stats for both bulkheads
//	curl http://localhost:8084/stats

package main

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"microservices-go/services/12-bulkhead/internal/bulkhead"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// Create two isolated bulkheads — one per downstream dependency.
	// Notice the different capacities: user calls get more slots because
	// they are expected to be higher traffic.
	userBulkhead := bulkhead.New("user-bulkhead", 5)
	paymentBulkhead := bulkhead.New("payment-bulkhead", 3)

	mux := http.NewServeMux()

	// GET /api/users — proxies to the "user service" via the user bulkhead.
	// Uses TryExecute (non-blocking): if the pool is full, returns 503 immediately.
	mux.HandleFunc("GET /api/users", func(w http.ResponseWriter, r *http.Request) {
		err := userBulkhead.TryExecute(func() error {
			// Simulate a fast user service call (10ms)
			time.Sleep(10 * time.Millisecond)
			return nil
		})

		if err != nil {
			if errors.Is(err, bulkhead.ErrBulkheadFull) {
				slog.Warn("user-bulkhead full — request rejected",
					"available", userBulkhead.Available(),
					"capacity", userBulkhead.Capacity(),
				)
				http.Error(w, "user service capacity exceeded, try again later", http.StatusServiceUnavailable)
				return
			}
			http.Error(w, "user service error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		slog.Info("user call succeeded",
			"bulkhead", userBulkhead.Name(),
			"available", userBulkhead.Available(),
		)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"users": []map[string]string{
				{"id": "u1", "name": "Alice"},
				{"id": "u2", "name": "Bob"},
			},
			"bulkhead":        userBulkhead.Name(),
			"slots_available": userBulkhead.Available(),
		})
	})

	// GET /api/payments — proxies to the "payment service" via the payment bulkhead.
	// Add ?slow=true to hold a slot for 5 seconds, simulating a slow payment service.
	// When 3 slow calls are in flight, the bulkhead is full and new calls are rejected.
	mux.HandleFunc("GET /api/payments", func(w http.ResponseWriter, r *http.Request) {
		slow := r.URL.Query().Get("slow") == "true"

		err := paymentBulkhead.TryExecute(func() error {
			if slow {
				// Hold the slot for 5 seconds to simulate a slow/stuck payment call.
				// While this goroutine sleeps, the slot is occupied.
				// If 3 goroutines are all sleeping here, the bulkhead is full.
				slog.Info("slow payment call — holding slot for 5s",
					"available_after_acquire", paymentBulkhead.Available(),
				)
				time.Sleep(5 * time.Second)
			} else {
				// Normal payment call takes 50ms
				time.Sleep(50 * time.Millisecond)
			}
			return nil
		})

		if err != nil {
			if errors.Is(err, bulkhead.ErrBulkheadFull) {
				slog.Warn("payment-bulkhead full — request rejected",
					"available", paymentBulkhead.Available(),
					"capacity", paymentBulkhead.Capacity(),
					"rejected_total", paymentBulkhead.Metrics.Rejected.Load(),
				)
				http.Error(w, "payment service capacity exceeded, try again later", http.StatusServiceUnavailable)
				return
			}
			http.Error(w, "payment service error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		slog.Info("payment call succeeded",
			"bulkhead", paymentBulkhead.Name(),
			"available", paymentBulkhead.Available(),
		)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"payments": []map[string]any{
				{"id": "p1", "amount_cents": 4999, "status": "settled"},
				{"id": "p2", "amount_cents": 1200, "status": "pending"},
			},
			"bulkhead":        paymentBulkhead.Name(),
			"slots_available": paymentBulkhead.Available(),
		})
	})

	// GET /stats — returns current metrics for both bulkheads.
	// Useful for monitoring how full each pool is and how many calls have been rejected.
	mux.HandleFunc("GET /stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"bulkheads": []map[string]any{
				{
					"name":           userBulkhead.Name(),
					"capacity":       userBulkhead.Capacity(),
					"available":      userBulkhead.Available(),
					"in_flight":      userBulkhead.Metrics.InFlight.Load(),
					"total_requests": userBulkhead.Metrics.TotalRequests.Load(),
					"rejected":       userBulkhead.Metrics.Rejected.Load(),
				},
				{
					"name":           paymentBulkhead.Name(),
					"capacity":       paymentBulkhead.Capacity(),
					"available":      paymentBulkhead.Available(),
					"in_flight":      paymentBulkhead.Metrics.InFlight.Load(),
					"total_requests": paymentBulkhead.Metrics.TotalRequests.Load(),
					"rejected":       paymentBulkhead.Metrics.Rejected.Load(),
				},
			},
		})
	})

	// GET /health — standard health check endpoint required by all services.
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	addr := ":8084"
	slog.Info("────────────────────────────────────────────────────────")
	slog.Info("Bulkhead Pattern demo running", "url", "http://localhost"+addr)
	slog.Info("User API    → curl http://localhost:8084/api/users")
	slog.Info("Payment API → curl http://localhost:8084/api/payments")
	slog.Info("Slow pay    → curl 'http://localhost:8084/api/payments?slow=true'")
	slog.Info("Stats       → curl http://localhost:8084/stats")
	slog.Info("Health      → curl http://localhost:8084/health")
	slog.Info("────────────────────────────────────────────────────────")

	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
