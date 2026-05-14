// main.go — Strangler Fig Pattern demo.
//
// Shows incremental migration:
//   Phase 1: All traffic → legacy
//   Phase 2: /users migrated → modern user service
//   Phase 3: /products migrated → modern product service
//   Phase 4: /orders rolled back (new service had a problem)
//
// HOW TO RUN:
//   go run ./services/15-strangler-fig/cmd/main.go

package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"

	"microservices-go/services/15-strangler-fig/internal/legacy"
	"microservices-go/services/15-strangler-fig/internal/modern"
	"microservices-go/services/15-strangler-fig/internal/proxy"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// Start fake legacy and modern backends
	legacyServer := httptest.NewServer(legacy.Handler())
	defer legacyServer.Close()

	modernServer := httptest.NewServer(modern.Handler())
	defer modernServer.Close()

	// Create the strangler router pointing at legacy
	rt := proxy.New(legacyServer.URL)

	// Start the router as the public-facing server
	publicServer := httptest.NewServer(rt.Handler())
	defer publicServer.Close()

	// ── Phase 1: All traffic goes to legacy ───────────────────────────────────
	fmt.Println("\n═══ Phase 1: All routes → Legacy ═══")
	showRoutes(rt)
	call(publicServer.URL, "/users/alice")
	call(publicServer.URL, "/products/laptop")
	call(publicServer.URL, "/orders/123")

	// ── Phase 2: Migrate /users to modern ─────────────────────────────────────
	fmt.Println("\n═══ Phase 2: Migrate /users → Modern ═══")
	rt.Migrate("/users", modernServer.URL)
	showRoutes(rt)
	call(publicServer.URL, "/users/alice")     // → modern
	call(publicServer.URL, "/products/laptop") // → legacy
	call(publicServer.URL, "/orders/123")      // → legacy

	// ── Phase 3: Migrate /products to modern ──────────────────────────────────
	fmt.Println("\n═══ Phase 3: Migrate /products → Modern ═══")
	rt.Migrate("/products", modernServer.URL)
	showRoutes(rt)
	call(publicServer.URL, "/users/alice")     // → modern
	call(publicServer.URL, "/products/laptop") // → modern
	call(publicServer.URL, "/orders/123")      // → legacy

	// ── Phase 4: Rollback /users (new service has a bug) ─────────────────────
	fmt.Println("\n═══ Phase 4: Rollback /users (bug found) ═══")
	rt.Rollback("/users") //nolint
	showRoutes(rt)
	call(publicServer.URL, "/users/alice")     // → legacy again
	call(publicServer.URL, "/products/laptop") // → modern
}

func call(baseURL, path string) {
	resp, err := http.Get(baseURL + path)
	if err != nil {
		slog.Error("call failed", "path", path, "error", err)
		return
	}
	defer resp.Body.Close()

	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	fmt.Printf("  %-20s → %s\n", path, body["source"])
}

func showRoutes(rt *proxy.StranglerRouter) {
	routes := rt.Routes()
	if len(routes) == 0 {
		fmt.Println("  (no migrated routes — all traffic goes to legacy)")
		return
	}
	for _, r := range routes {
		fmt.Printf("  %-15s → modern ✓\n", r.Prefix)
	}
	fmt.Printf("  %-15s → legacy (catch-all)\n", "/*")
}

// ensure os is used
var _ = os.Stdout
