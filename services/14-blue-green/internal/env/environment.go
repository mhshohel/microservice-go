// environment.go — An environment (Blue or Green) that runs an HTTP service.
//
// In a real blue-green deployment, an "environment" would be a whole cluster
// of servers, a Kubernetes namespace, or a set of VMs.
// Here we simulate it with an httptest.Server so the demo needs no real ports.

package env

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
)

// Name identifies which environment this is.
type Name string

const (
	Blue  Name = "blue"
	Green Name = "green"
)

// Environment simulates one deployment slot (blue or green).
type Environment struct {
	mu      sync.RWMutex
	name    Name
	version string // the app version currently running (e.g., "v1", "v2")
	server  *httptest.Server
	live    bool // true = currently receiving production traffic
}

// New creates an environment with the given name and initial version.
// Starts the fake HTTP server immediately.
func New(name Name, version string) *Environment {
	e := &Environment{name: name, version: version}
	e.startServer()
	return e
}

// startServer starts the fake HTTP server for this environment.
func (e *Environment) startServer() {
	mux := http.NewServeMux()

	// /health — used by smoke tests before switching traffic
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		e.mu.RLock()
		v := e.version
		e.mu.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status":      "ok",
			"environment": string(e.name),
			"version":     v,
		})
	})

	// /api/hello — a sample endpoint to show which version is serving
	mux.HandleFunc("GET /api/hello", func(w http.ResponseWriter, r *http.Request) {
		e.mu.RLock()
		v := e.version
		e.mu.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"message":     "hello from " + string(e.name),
			"version":     v,
			"environment": string(e.name),
		})
	})

	e.server = httptest.NewServer(mux)
}

// URL returns the base URL of this environment's server.
func (e *Environment) URL() string {
	return e.server.URL
}

// Version returns the currently deployed version.
func (e *Environment) Version() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.version
}

// Deploy simulates deploying a new version to this environment.
// In a real system this would push a Docker image, update Kubernetes, etc.
func (e *Environment) Deploy(version string) {
	e.mu.Lock()
	e.version = version
	e.mu.Unlock()
}

// Name returns the environment name (blue or green).
func (e *Environment) Name() Name {
	return e.name
}

// IsLive returns whether this environment is currently receiving production traffic.
func (e *Environment) IsLive() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.live
}

// SetLive updates the live flag.
func (e *Environment) SetLive(live bool) {
	e.mu.Lock()
	e.live = live
	e.mu.Unlock()
}

// Close stops the environment's HTTP server.
func (e *Environment) Close() {
	if e.server != nil {
		e.server.Close()
	}
}
