// main.go — Entry point for the Backend for Frontend (BFF) demo.
//
// This server demonstrates the BFF pattern by hosting three different BFFs
// on a single port (:8085), each serving a different client type:
//
//   GET /web/orders      — Web BFF: full order details for a desktop browser
//   GET /mobile/orders   — Mobile BFF: minimal fields for a phone app
//   GET /admin/dashboard — Admin BFF: aggregated stats for a management panel
//
// All three BFFs read from the SAME underlying order data (sampleOrders),
// but they each shape it differently for their specific client.
//
// HOW TO RUN (from the project root):
//
//	go run ./services/13-bff/cmd/main.go
//
// HOW TO DEMO:
//
//	# Web — richest response (all fields)
//	curl http://localhost:8085/web/orders | jq .
//
//	# Mobile — minimal response (id, status, total only)
//	curl http://localhost:8085/mobile/orders | jq .
//
//	# Admin — aggregated dashboard (counts, revenue, by-status breakdown)
//	curl http://localhost:8085/admin/dashboard | jq .

package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"

	"microservices-go/services/13-bff/internal/admin"
	"microservices-go/services/13-bff/internal/mobile"
	"microservices-go/services/13-bff/internal/orders"
	"microservices-go/services/13-bff/internal/web"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// sampleOrders is the shared data store for this demo.
	// In a real system each BFF would call downstream microservices over HTTP;
	// here we use a fixed in-memory slice so the demo runs without dependencies.
	sampleOrders := orders.SampleOrders()

	mux := http.NewServeMux()

	// Mount each BFF handler at its own route.
	// Notice how each Handler() receives the same sampleOrders but each returns
	// a completely different JSON shape tailored to its client.

	// Web BFF — full order details (most data, for desktop browsers)
	mux.Handle("GET /web/orders", web.Handler(sampleOrders))

	// Mobile BFF — minimal order data (least data, for bandwidth-constrained phones)
	mux.Handle("GET /mobile/orders", mobile.Handler(sampleOrders))

	// Admin BFF — aggregated dashboard (no list, just totals and counts)
	mux.Handle("GET /admin/dashboard", admin.Handler(sampleOrders))

	// Health check endpoint — required by all services in this project.
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	addr := ":8085"
	slog.Info("────────────────────────────────────────────────────────")
	slog.Info("BFF Pattern demo running", "url", "http://localhost"+addr)
	slog.Info("Web BFF    → curl http://localhost:8085/web/orders")
	slog.Info("Mobile BFF → curl http://localhost:8085/mobile/orders")
	slog.Info("Admin BFF  → curl http://localhost:8085/admin/dashboard")
	slog.Info("Health     → curl http://localhost:8085/health")
	slog.Info("────────────────────────────────────────────────────────")

	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
