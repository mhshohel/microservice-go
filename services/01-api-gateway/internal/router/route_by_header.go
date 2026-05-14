// route_by_header.go — Routes requests based on a custom HTTP request header.
//
// Instead of reading the URL path, this strategy reads a header the client sends
// and uses its value to pick the backend service.
//
// Example — using the "X-Service" header:
//   X-Service: users    → http://localhost:9001  (User Service)
//   X-Service: products → http://localhost:9002  (Product Service)
//   X-Service: orders   → http://localhost:9003  (Order Service)
//
// When to prefer header routing over path routing:
//   - Multiple services share the same URL pattern (e.g., all use /api/data)
//   - Supporting multiple API versions (X-API-Version: v1 vs X-API-Version: v2)
//   - A/B testing (X-Variant: experiment vs X-Variant: control)
//   - Routing based on client type (X-Client: mobile vs X-Client: web)
//
// Downside: clients must know to set the header — it's less "discoverable"
// than path-based routing.

package router

import (
	"fmt"
	"net/http"
)

// HeaderRoute maps one header value to a backend service URL.
type HeaderRoute struct {
	HeaderValue string // the header value to match (e.g., "users")
	Backend     string // backend URL to route to (e.g., "http://localhost:9001")
}

// HeaderRouter routes requests by reading a specific HTTP header.
type HeaderRouter struct {
	headerName string        // which header to read (e.g., "X-Service")
	routes     []HeaderRoute // mapping of header values to backend URLs
}

// NewHeaderRouter creates a HeaderRouter that routes based on the given header name.
//
// Example:
//
//	router := NewHeaderRouter("X-Service", []HeaderRoute{
//	    {HeaderValue: "users",    Backend: "http://localhost:9001"},
//	    {HeaderValue: "products", Backend: "http://localhost:9002"},
//	})
func NewHeaderRouter(headerName string, routes []HeaderRoute) *HeaderRouter {
	return &HeaderRouter{
		headerName: headerName,
		routes:     routes,
	}
}

// Route finds the backend URL by reading the routing header from the request.
// Returns an error if the header is missing or its value has no matching route.
func (hr *HeaderRouter) Route(r *http.Request) (string, error) {
	// Read the routing header from the incoming request
	headerValue := r.Header.Get(hr.headerName)
	if headerValue == "" {
		return "", fmt.Errorf("missing routing header %q — set it to one of the known service names", hr.headerName)
	}

	// Find a route whose value matches the header
	for _, route := range hr.routes {
		if route.HeaderValue == headerValue {
			return route.Backend, nil
		}
	}

	return "", fmt.Errorf("no backend found for %s: %q — known values: check your routing table", hr.headerName, headerValue)
}
