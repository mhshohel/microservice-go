// router.go — Strangler Fig router: proxy in front of legacy + new services.
//
// The StranglerRouter sits in front of everything. It has a routing table
// that maps URL path prefixes to either the legacy system or a new service.
//
// Starting state: ALL routes go to legacy.
//
// As routes are migrated:
//   router.Migrate("/users", newUserServiceURL)
//
// Now /users/* goes to the new service, everything else still goes to legacy.
//
// The legacy system is "strangled" route by route until nothing flows there.

package proxy

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

// RouteEntry maps a path prefix to a backend URL.
type RouteEntry struct {
	Prefix     string // e.g. "/users" or "/products"
	BackendURL string // URL of the service handling this prefix
	Migrated   bool   // true = goes to new service; false = goes to legacy
}

// StranglerRouter routes requests: migrated paths go to new services,
// everything else goes to the legacy backend.
type StranglerRouter struct {
	mu        sync.RWMutex
	legacyURL string       // where the old monolith lives
	routes    []RouteEntry // migrated routes (in order — first match wins)
}

// New creates a strangler router with all traffic initially going to the legacy URL.
func New(legacyURL string) *StranglerRouter {
	return &StranglerRouter{legacyURL: legacyURL}
}

// Migrate registers a path prefix to route to a new microservice.
// After calling Migrate("/users", newURL), all requests with paths starting
// with "/users" will go to newURL instead of legacy.
//
// Call Migrate for each route as it's ready in the new service.
func (s *StranglerRouter) Migrate(pathPrefix, newBackendURL string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if this prefix is already registered; update if so
	for i, r := range s.routes {
		if r.Prefix == pathPrefix {
			s.routes[i].BackendURL = newBackendURL
			s.routes[i].Migrated = true
			return
		}
	}

	s.routes = append(s.routes, RouteEntry{
		Prefix:     pathPrefix,
		BackendURL: newBackendURL,
		Migrated:   true,
	})
}

// Rollback moves a previously migrated route back to the legacy system.
// Use this if the new service has a problem and you need to revert.
func (s *StranglerRouter) Rollback(pathPrefix string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, r := range s.routes {
		if r.Prefix == pathPrefix {
			s.routes = append(s.routes[:i], s.routes[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("prefix %q not found in migration table", pathPrefix)
}

// Routes returns a snapshot of the current migration table.
func (s *StranglerRouter) Routes() []RouteEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snapshot := make([]RouteEntry, len(s.routes))
	copy(snapshot, s.routes)
	return snapshot
}

// MigratedCount returns how many routes have been migrated to new services.
func (s *StranglerRouter) MigratedCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.routes)
}

// Handler returns an http.Handler that proxies requests to the right backend.
func (s *StranglerRouter) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backendURL := s.resolveBackend(r.URL.Path)
		s.proxy(w, r, backendURL)
	})
}

// resolveBackend finds which backend should handle the given path.
// Checks migrated routes first (longest prefix wins), falls back to legacy.
func (s *StranglerRouter) resolveBackend(path string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Find the most specific (longest) matching prefix
	bestMatch := ""
	bestURL := s.legacyURL

	for _, route := range s.routes {
		if strings.HasPrefix(path, route.Prefix) {
			if len(route.Prefix) > len(bestMatch) {
				bestMatch = route.Prefix
				bestURL = route.BackendURL
			}
		}
	}

	return bestURL
}

// proxy forwards the request to backendURL and copies the response.
func (s *StranglerRouter) proxy(w http.ResponseWriter, r *http.Request, backendURL string) {
	targetURL := backendURL + r.URL.Path
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	resp, err := http.Get(targetURL)
	if err != nil {
		http.Error(w, "strangler router: backend error: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for key, vals := range resp.Header {
		for _, v := range vals {
			w.Header().Add(key, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
