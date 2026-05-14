// main.go — Entry point for the Circuit Breaker demo.
//
// This demo starts a server on :8083 with:
//   - A "flaky" backend that can be made to fail on demand
//   - A circuit breaker wrapping calls to that backend
//   - Endpoints to trigger calls, force failures, and inspect state
//
// HOW TO RUN (from the project root):
//   go run ./services/04-circuit-breaker/cmd/main.go
//
// HOW TO DEMO:
//
//   # 1. Make normal calls (circuit CLOSED, calls succeed)
//   curl http://localhost:8083/call
//
//   # 2. Trigger 5 failures to open the circuit
//   for i in $(seq 5); do curl http://localhost:8083/fail; done
//
//   # 3. Try calling again — circuit is OPEN, gets rejected immediately
//   curl http://localhost:8083/call
//
//   # 4. Check state
//   curl http://localhost:8083/state
//
//   # 5. Reset manually to try again
//   curl -X POST http://localhost:8083/reset

package main

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"microservices-go/services/04-circuit-breaker/internal/breaker"
)

// shouldFail controls whether the fake backend returns an error.
// Toggled by the /fail endpoint.
var shouldFail = false

// fakeBackend simulates calling a downstream service.
// Returns an error when shouldFail is true.
func fakeBackend() error {
	if shouldFail {
		return errors.New("backend error: connection refused")
	}
	return nil
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// Circuit breaker: opens after 5 failures, stays open for 10 seconds
	cb := breaker.New(5, 10*time.Second)

	mux := http.NewServeMux()

	// /call — make a call through the circuit breaker
	mux.HandleFunc("GET /call", func(w http.ResponseWriter, r *http.Request) {
		err := cb.Execute(fakeBackend)

		if err != nil {
			if errors.Is(err, breaker.ErrCircuitOpen) {
				slog.Warn("circuit OPEN — call rejected", "state", cb.State())
				http.Error(w, "circuit open: "+err.Error(), http.StatusServiceUnavailable)
				return
			}
			slog.Error("call failed", "error", err, "state", cb.State(), "failures", cb.Failures())
			http.Error(w, "backend error: "+err.Error(), http.StatusBadGateway)
			return
		}

		slog.Info("call succeeded", "state", cb.State())
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"result": "ok",
			"state":  cb.State().String(),
		})
	})

	// /fail — make the backend return an error (to simulate failures)
	mux.HandleFunc("GET /fail", func(w http.ResponseWriter, r *http.Request) {
		shouldFail = true
		err := cb.Execute(fakeBackend)
		shouldFail = false // reset after one forced failure

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"forced_failure": true,
			"error":          err.Error(),
			"state":          cb.State().String(),
			"failures":       cb.Failures(),
		})
	})

	// /state — inspect the current circuit breaker state
	mux.HandleFunc("GET /state", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"state":        cb.State().String(),
			"failures":     cb.Failures(),
			"max_failures": 5,
		})
	})

	// /reset — manually reset the circuit breaker to CLOSED
	mux.HandleFunc("POST /reset", func(w http.ResponseWriter, r *http.Request) {
		cb.Reset()
		slog.Info("circuit manually reset to CLOSED")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"state":   cb.State().String(),
			"message": "circuit reset to CLOSED",
		})
	})

	addr := ":8083"
	slog.Info("──────────────────────────────────────────────")
	slog.Info("Circuit Breaker demo running", "url", "http://localhost"+addr)
	slog.Info("Normal call  → curl http://localhost:8083/call")
	slog.Info("Force fail   → curl http://localhost:8083/fail")
	slog.Info("Check state  → curl http://localhost:8083/state")
	slog.Info("Reset        → curl -X POST http://localhost:8083/reset")
	slog.Info("──────────────────────────────────────────────")

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
