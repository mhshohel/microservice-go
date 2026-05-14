// main.go — Entry point for the Load Balancing demo.
//
// This program starts a demo load balancer on :8082.
// It spins up 3 fake backends and lets you pick an algorithm via query param.
//
// HOW TO RUN (from the project root):
//   go run ./services/03-load-balancing/cmd/main.go
//
// HOW TO TEST (in a second terminal):
//
//   # Round robin — watch the backend rotate evenly
//   for i in $(seq 9); do curl -s "http://localhost:8082/api?algo=roundrobin" | grep backend; done
//
//   # Weighted round robin — backend1 gets 3x more traffic
//   for i in $(seq 9); do curl -s "http://localhost:8082/api?algo=weighted" | grep backend; done
//
//   # Least connections
//   for i in $(seq 6); do curl -s "http://localhost:8082/api?algo=leastconn" | grep backend; done
//
//   # IP hash — your IP always goes to the same backend
//   for i in $(seq 6); do curl -s "http://localhost:8082/api?algo=iphash" | grep backend; done
//
//   # Random
//   for i in $(seq 9); do curl -s "http://localhost:8082/api?algo=random" | grep backend; done
//
//   # Show backend stats
//   curl http://localhost:8082/stats

package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"time"

	"microservices-go/services/03-load-balancing/internal/balancer"
)

// backendStats tracks how many requests each fake backend has handled.
type backendStats struct {
	address string
	count   atomic.Uint64
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// ── Step 1: Start 3 fake backends ─────────────────────────────────────────
	stats := []*backendStats{
		{address: ""},
		{address: ""},
		{address: ""},
	}

	for i, s := range stats {
		idx := i // capture for closure
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			stats[idx].count.Add(1)
			json.NewEncoder(w).Encode(map[string]string{
				"backend": fmt.Sprintf("backend-%d", idx+1),
				"address": stats[idx].address,
			})
		}))
		defer srv.Close()
		s.address = srv.URL
	}

	// ── Step 2: Build backends slice ──────────────────────────────────────────
	backends := []balancer.Backend{
		{Address: stats[0].address, Weight: 3}, // backend1 has 3x weight
		{Address: stats[1].address, Weight: 1},
		{Address: stats[2].address, Weight: 1},
	}

	// ── Step 3: Create all 5 balancers ────────────────────────────────────────
	rr, _ := balancer.NewRoundRobin(backends)
	wrr, _ := balancer.NewWeightedRoundRobin(backends)
	lc, _ := balancer.NewLeastConnections(backends)
	iph, _ := balancer.NewIPHash(backends)
	rnd, _ := balancer.NewRandom(backends)

	// ── Step 4: Build HTTP handler ────────────────────────────────────────────
	mux := http.NewServeMux()

	// /api?algo=roundrobin|weighted|leastconn|iphash|random
	mux.HandleFunc("GET /api", func(w http.ResponseWriter, r *http.Request) {
		algo := r.URL.Query().Get("algo")
		if algo == "" {
			algo = "roundrobin"
		}

		// Extract client IP for IP-hash routing
		clientIP := r.RemoteAddr

		// Pick the algorithm
		var lb balancer.Balancer
		switch algo {
		case "roundrobin":
			lb = rr
		case "weighted":
			lb = wrr
		case "leastconn":
			lb = lc
		case "iphash":
			lb = iph
		case "random":
			lb = rnd
		default:
			http.Error(w, "unknown algo — use: roundrobin, weighted, leastconn, iphash, random", http.StatusBadRequest)
			return
		}

		// Select a backend
		backendURL, err := lb.Next(clientIP)
		if err != nil {
			http.Error(w, "no backends available: "+err.Error(), http.StatusServiceUnavailable)
			return
		}

		// Forward the request
		resp, err := http.Get(backendURL)
		if err != nil {
			http.Error(w, "backend error: "+err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		// Mark the connection done for least-connections
		if algo == "leastconn" {
			lc.Done(backendURL)
		}

		slog.Info("routed request", "algo", algo, "backend", backendURL)

		var body any
		json.NewDecoder(resp.Body).Decode(&body)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"algo":    algo,
			"backend": body,
		})
	})

	// /stats — show how many requests each backend has handled
	mux.HandleFunc("GET /stats", func(w http.ResponseWriter, r *http.Request) {
		result := make([]map[string]any, len(stats))
		for i, s := range stats {
			result[i] = map[string]any{
				"backend": fmt.Sprintf("backend-%d", i+1),
				"address": s.address,
				"count":   s.count.Load(),
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})

	addr := ":8082"
	slog.Info("──────────────────────────────────────────────")
	slog.Info("Load Balancer demo running", "url", "http://localhost"+addr)
	slog.Info("Try: curl 'http://localhost:8082/api?algo=roundrobin'")
	slog.Info("Stats: curl http://localhost:8082/stats")
	slog.Info("──────────────────────────────────────────────")

	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil {
		slog.Error("load balancer stopped", "error", err)
		os.Exit(1)
	}
}
