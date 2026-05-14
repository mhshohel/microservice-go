// handlers.go — HTTP handlers for the Containerization demo service.
//
// This file defines the HTTP handlers and the BuildMux function that wires them together.
// Keeping handlers in a separate package (instead of inside cmd/) means tests can import
// this package directly and call the handlers without spinning up a real HTTP server.
//
// The service itself is intentionally simple — its purpose is to be the binary that
// gets compiled and packaged inside a Docker container, demonstrating how small a
// Go binary can be when built with CGO_ENABLED=0 and stripped of debug info.

package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime"
)

// BuildMux creates and returns the HTTP ServeMux with all routes registered.
// Returning the mux (instead of calling http.ListenAndServe here) lets tests
// create the mux and pass it directly to httptest.NewRecorder — no real port needed.
func BuildMux() *http.ServeMux {
	mux := http.NewServeMux()

	// Register each endpoint. The handler functions are defined below so they
	// can also be called directly from tests.
	mux.HandleFunc("GET /health", HealthHandler)
	mux.HandleFunc("GET /info", InfoHandler)

	return mux
}

// HealthHandler responds with a simple JSON health check.
// Every service in this project exposes GET /health so that load balancers,
// Kubernetes liveness probes, and Docker healthchecks can verify the service is up.
func HealthHandler(w http.ResponseWriter, r *http.Request) {
	slog.Info("health check", "method", r.Method, "path", r.URL.Path)

	// Respond with JSON. We set the Content-Type header before writing the body —
	// once WriteHeader or Write is called, headers can no longer be changed.
	w.Header().Set("Content-Type", "application/json")

	response := map[string]string{
		"status":  "ok",
		"service": "containerization-demo",
	}

	// json.NewEncoder(w) writes directly to the ResponseWriter without buffering
	// the whole payload in memory first — a good habit for larger responses.
	if err := json.NewEncoder(w).Encode(response); err != nil {
		slog.Error("failed to encode health response", "error", err)
	}
}

// InfoHandler returns metadata about the running binary.
// In a real service this might expose build version, commit hash, or uptime.
// Here it demonstrates that a containerised Go binary can introspect itself
// (e.g., report the Go version it was compiled with) without any extra tooling.
func InfoHandler(w http.ResponseWriter, r *http.Request) {
	slog.Info("info request", "method", r.Method, "path", r.URL.Path)

	w.Header().Set("Content-Type", "application/json")

	// runtime.Version() returns the Go version this binary was compiled with,
	// e.g. "go1.26". This is baked into the binary at compile time — the
	// container image does NOT need the Go toolchain at runtime.
	response := map[string]string{
		"go_version":  runtime.Version(),
		"binary_size": "small", // stripped with -ldflags="-w -s" in Dockerfile
		"os":          runtime.GOOS,
		"arch":        runtime.GOARCH,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		slog.Error("failed to encode info response", "error", err)
	}
}
