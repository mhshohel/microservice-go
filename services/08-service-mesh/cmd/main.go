// main.go — Service Mesh demo.
//
// Shows:
//   1. Normal call: order-svc → user-svc (with identity header)
//   2. Rejected call: unknown-svc → user-svc (identity not in allow list)
//   3. Retry: flaky backend fails first time, sidecar retries → success
//   4. Traffic split: 50% canary routing
//
// HOW TO RUN:
//   go run ./services/08-service-mesh/cmd/main.go

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

	"microservices-go/services/08-service-mesh/internal/sidecar"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// ── Demo 1: Normal call with identity verification ────────────────────────
	fmt.Println("\n══ 1. Identity Verification ══")
	demoIdentity()

	// ── Demo 2: Retry on failure ──────────────────────────────────────────────
	fmt.Println("\n══ 2. Automatic Retries ══")
	demoRetry()

	// ── Demo 3: Traffic splitting ─────────────────────────────────────────────
	fmt.Println("\n══ 3. Traffic Splitting (Canary) ══")
	demoTrafficSplit()
}

func demoIdentity() {
	// user-svc with an inbound sidecar that only allows order-svc
	userSvc := httptest.NewServer(nil) // placeholder — we'll set handler below
	userSvc.Close()

	userSidecar := sidecar.New("user-svc", sidecar.Policy{
		AllowedIdentities: []string{"order-svc"}, // only order-svc is allowed
	})

	userSvc = httptest.NewServer(userSidecar.InboundMiddleware(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]string{"user": "alice"})
		}),
	))
	defer userSvc.Close()

	// order-svc calling user-svc — should succeed
	orderSidecar := sidecar.New("order-svc", sidecar.Policy{})
	body, err := orderSidecar.Call(userSvc.URL)
	if err != nil {
		slog.Error("order-svc call failed", "error", err)
	} else {
		fmt.Printf("  order-svc → user-svc: %s", body)
	}

	// unknown caller — should be rejected
	unknownSidecar := sidecar.New("unknown-svc", sidecar.Policy{})
	_, err = unknownSidecar.Call(userSvc.URL)
	if err != nil {
		fmt.Printf("  unknown-svc → user-svc: REJECTED (expected)\n")
	}
}

func demoRetry() {
	// Flaky backend: fails the first call, succeeds on retry
	attempts := atomic.Int32{}
	flakyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) <= 1 {
			http.Error(w, "service unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte(`{"result":"ok"}`))
	}))
	defer flakyServer.Close()

	// Sidecar with 2 retries
	sc := sidecar.New("caller-svc", sidecar.Policy{
		MaxRetries: 2,
		RetryDelay: 10 * time.Millisecond,
	})

	body, err := sc.Call(flakyServer.URL)
	if err != nil {
		slog.Error("call failed despite retries", "error", err)
	} else {
		fmt.Printf("  succeeded on retry: %s\n", body)
		fmt.Printf("  retries used: %d\n", sc.Metrics.RetryCount.Load())
	}
}

func demoTrafficSplit() {
	// Stable backend
	stableHits := atomic.Int32{}
	stable := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stableHits.Add(1)
		w.Write([]byte(`{"version":"v1"}`))
	}))
	defer stable.Close()

	// Canary backend (new version)
	canaryHits := atomic.Int32{}
	canary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		canaryHits.Add(1)
		w.Write([]byte(`{"version":"v2"}`))
	}))
	defer canary.Close()

	// Sidecar: 30% of traffic goes to canary
	sc := sidecar.New("api-svc", sidecar.Policy{
		CanaryBackend: canary.URL,
		CanaryPercent: 30,
	})

	// Send 100 requests
	for range 100 {
		sc.Call(stable.URL) //nolint
	}

	fmt.Printf("  100 requests — stable: %d  canary: %d\n",
		stableHits.Load(), canaryHits.Load())
	fmt.Printf("  (expected ~70 stable, ~30 canary)\n")
}
