// balancer.go — Shared types used by all load-balancing algorithms.
//
// Every algorithm in this package implements the Balancer interface.
// This means you can swap algorithms at runtime without changing any calling code:
//
//   var lb Balancer = NewRoundRobin(backends)
//   lb = NewLeastConnections(backends)   // swap — same interface, different behavior
//   backend := lb.Next("192.168.1.1")    // same call either way
//
// The "strategy pattern": the Balancer interface is the strategy contract,
// and each algorithm file is a concrete strategy.

package balancer

// Backend represents one server that can handle requests.
//
// In a real system this might also hold region, version, health status, etc.
type Backend struct {
	Address     string // host:port, e.g. "localhost:9001"
	Weight      int    // relative capacity (only used by WeightedRoundRobin)
	ActiveConns int    // current in-flight requests (only used by LeastConnections)
}

// Balancer is the interface every load-balancing algorithm must implement.
//
// Next receives the client's IP address (for IP-hash based routing) and
// returns the address of the selected backend, or an error if no backend
// is available (e.g., the backend list is empty).
type Balancer interface {
	Next(clientIP string) (string, error)
}
