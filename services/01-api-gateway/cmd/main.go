// main.go — Entry point for the API Gateway demo.
//
// This program demonstrates a complete API Gateway by:
//   1. Starting 3 dummy backend services (Users, Products, Orders)
//   2. Starting the gateway on :8080 with auth + rate limiting + routing
//
// HOW TO RUN (from the project root):
//   go run ./services/01-api-gateway/cmd/main.go
//
// HOW TO TEST (in a second terminal):
//   # Get a demo token
//   curl http://localhost:8080/token
//
//   # Set the token in a variable (replace with the actual token from above)
//   TOKEN="paste-the-token-here"
//
//   # Call the different backend services through the gateway
//   curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/users
//   curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/products
//   curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/orders
//   curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/users/1
//
//   # Health check (no token needed)
//   curl http://localhost:8080/health
//
//   # Missing token → 401
//   curl http://localhost:8080/api/users
//
//   # Bad token → 401
//   curl -H "Authorization: Bearer bad.token.here" http://localhost:8080/api/users

package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"

	"microservices-go/services/01-api-gateway/internal/backend"
	"microservices-go/services/01-api-gateway/internal/jwt"
	"microservices-go/services/01-api-gateway/internal/middleware"
	"microservices-go/services/01-api-gateway/internal/proxy"
	"microservices-go/services/01-api-gateway/internal/router"
)

func main() {
	// Use plain text logging so the demo output is easy to read in a terminal.
	// In production you'd use slog.NewJSONHandler for structured/machine-readable logs.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// ── Step 1: Start dummy backend services ──────────────────────────────────
	// In real life these would already be running elsewhere.
	// We start them here so the demo is a single "go run" command.
	slog.Info("starting dummy backend services…")
	backends := backend.Start()

	// Give backends a moment to bind their ports before the gateway starts routing
	time.Sleep(50 * time.Millisecond)

	// ── Step 2: Define routing rules (path-based) ─────────────────────────────
	// Map URL path prefixes → backend service URLs.
	// The gateway strips "/api" before forwarding, so /api/users → /users on the backend.
	pathRoutes := []router.Route{
		{Prefix: "/api/users", Backend: backends.Users},
		{Prefix: "/api/products", Backend: backends.Products},
		{Prefix: "/api/orders", Backend: backends.Orders},
	}
	pathRouter := router.NewPathRouter(pathRoutes)

	// ── Step 3: Create a rate limiter ─────────────────────────────────────────
	// Allow 100 requests per minute per IP address.
	// Lower this to 5 if you want to quickly test rate limiting.
	rateLimiter := middleware.NewRateLimiter(100, time.Minute)

	// ── Step 4: Build the HTTP handler with middleware chain ──────────────────
	mux := http.NewServeMux()

	// /health is public — no auth needed.
	// Load balancers ping this to check if the gateway is alive.
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "ok",
			"service": "api-gateway",
		})
	})

	// /token is a convenience endpoint that generates a demo JWT.
	// In a real system, authentication lives in a separate Auth Service.
	mux.HandleFunc("GET /token", func(w http.ResponseWriter, r *http.Request) {
		token := jwt.Generate("demo-user-1", "customer")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"token": token,
			"hint":  "Use as: Authorization: Bearer " + token[:20] + "…",
		})
	})

	// All /api/* routes flow through: rate limit → auth → route → proxy
	//
	// The middleware chain is built by wrapping the core handler.
	// The outermost wrapper (RateLimit) runs first on each request.
	//
	//   Request → RateLimit → Auth → [route + proxy] → Response
	//
	coreHandler := buildCoreHandler(pathRouter)
	withAuth := middleware.Auth(coreHandler)
	withRateLimit := middleware.RateLimit(rateLimiter)(withAuth)
	mux.Handle("/api/", withRateLimit)

	// ── Step 5: Start the gateway ─────────────────────────────────────────────
	addr := ":8080"
	slog.Info("──────────────────────────────────────────────")
	slog.Info("API Gateway running", "url", "http://localhost"+addr)
	slog.Info("Backends",
		"users", backends.Users,
		"products", backends.Products,
		"orders", backends.Orders,
	)
	slog.Info("Get a demo token → curl http://localhost:8080/token")
	slog.Info("──────────────────────────────────────────────")

	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil {
		slog.Error("gateway stopped", "error", err)
		os.Exit(1)
	}
}

// buildCoreHandler creates the handler that routes a request and proxies it to the backend.
// This is the last step in the middleware chain — it runs after auth and rate limiting pass.
func buildCoreHandler(pathRouter *router.PathRouter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Ask the router which backend should handle this request
		backendURL, err := pathRouter.Route(r)
		if err != nil {
			slog.Warn("no route found", "path", r.URL.Path)
			http.Error(w, "not found: "+err.Error(), http.StatusNotFound)
			return
		}

		// Strip the "/api" prefix from the path before forwarding.
		// The gateway exposes /api/users but the User Service expects /users.
		if len(r.URL.Path) >= 4 && r.URL.Path[:4] == "/api" {
			r.URL.Path = r.URL.Path[4:]
			if r.URL.Path == "" {
				r.URL.Path = "/"
			}
		}

		// Forward the request to the selected backend service
		proxy.Forward(w, r, backendURL)
	})
}
