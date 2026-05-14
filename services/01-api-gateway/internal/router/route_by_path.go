// route_by_path.go — Routes requests to backend services based on the URL path.
//
// This is the most common routing strategy in API Gateways.
// You look at the beginning of the URL path (called the "prefix") and decide
// which backend service should handle the request.
//
// Routing table example:
//   /api/users/*    → http://localhost:9001  (User Service)
//   /api/products/* → http://localhost:9002  (Product Service)
//   /api/orders/*   → http://localhost:9003  (Order Service)
//
// The "*" means "anything after this prefix".
// So /api/users/123 and /api/users/profile both go to the User Service.
//
// Rules are checked in order — put more specific paths before less specific ones.
// If two rules could match, the first one wins.

package router

import (
	"fmt"
	"net/http"
	"strings"
)

// Route is one routing rule: when the URL starts with Prefix, send to Backend.
type Route struct {
	Prefix  string // URL path prefix to match, e.g. "/api/users"
	Backend string // backend service base URL, e.g. "http://localhost:9001"
}

// PathRouter routes requests by matching the URL path against a list of prefixes.
type PathRouter struct {
	routes []Route // routing rules, checked in order
}

// NewPathRouter creates a PathRouter with the given routing rules.
func NewPathRouter(routes []Route) *PathRouter {
	return &PathRouter{routes: routes}
}

// Route finds the backend URL for the given HTTP request.
// It returns the URL of the first route whose prefix matches the request's path.
// Returns an error if no route matches.
func (pr *PathRouter) Route(r *http.Request) (string, error) {
	path := r.URL.Path

	for _, route := range pr.routes {
		// strings.HasPrefix checks if the path starts with the rule's prefix.
		// For example: "/api/users/123" HasPrefix "/api/users" → true
		if strings.HasPrefix(path, route.Prefix) {
			return route.Backend, nil
		}
	}

	return "", fmt.Errorf("no backend route found for path %q — check your routing table", path)
}
