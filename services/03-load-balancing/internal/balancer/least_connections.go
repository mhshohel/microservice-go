// least_connections.go — Least Connections load balancing.
//
// Always routes the next request to the backend with the fewest active connections.
//
//   Backend 1: [|||||||||||] 11 connections
//   Backend 2: [||]          2 connections  ← send here
//   Backend 3: [|||||||]     7 connections
//
// Why is this better than round-robin for some workloads?
//
// Round-robin assumes all requests take the same time. If some requests are
// fast (1ms) and some are slow (5s), round-robin can still pile up on one backend
// that happens to have gotten many slow requests.
//
// Least-connections naturally adapts: slow backends accumulate connections and
// get bypassed until they clear their backlog.
//
// Thread safety: we use a sync.Mutex because we need to read ALL connection counts
// atomically to pick the minimum. An atomic read per backend wouldn't prevent
// two goroutines from both picking the same "least loaded" backend.

package balancer

import (
	"fmt"
	"sync"
)

// LeastConnections routes to whichever backend has the fewest active connections.
type LeastConnections struct {
	mu       sync.Mutex // protects backends during pick + increment
	backends []lcBackend
}

// lcBackend tracks a backend and its current connection count.
type lcBackend struct {
	address     string
	activeConns int // how many in-flight requests are currently on this backend
}

// NewLeastConnections creates a least-connections balancer.
func NewLeastConnections(backends []Backend) (*LeastConnections, error) {
	if len(backends) == 0 {
		return nil, fmt.Errorf("least connections requires at least one backend")
	}

	lc := &LeastConnections{
		backends: make([]lcBackend, len(backends)),
	}
	for i, b := range backends {
		lc.backends[i] = lcBackend{address: b.Address}
	}
	return lc, nil
}

// Next picks the backend with the fewest active connections and increments
// its count by 1 (the caller is responsible for calling Done when the request finishes).
func (lc *LeastConnections) Next(_ string) (string, error) {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	// Find the backend with the minimum active connections
	minIdx := 0
	for i := 1; i < len(lc.backends); i++ {
		if lc.backends[i].activeConns < lc.backends[minIdx].activeConns {
			minIdx = i
		}
	}

	// Register that we're sending one more request to this backend
	lc.backends[minIdx].activeConns++
	return lc.backends[minIdx].address, nil
}

// Done signals that a request to the given backend address has completed.
// This decrements the active connection count so future picks reflect reality.
//
// The caller MUST call Done for every successful Next call, typically via defer:
//
//	addr, _ := lc.Next(clientIP)
//	defer lc.Done(addr)
//	// ... proxy the request ...
func (lc *LeastConnections) Done(address string) {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	for i := range lc.backends {
		if lc.backends[i].address == address {
			if lc.backends[i].activeConns > 0 {
				lc.backends[i].activeConns--
			}
			return
		}
	}
}

// ActiveConns returns the current connection count for the given address.
// Useful for tests and monitoring.
func (lc *LeastConnections) ActiveConns(address string) int {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	for _, b := range lc.backends {
		if b.address == address {
			return b.activeConns
		}
	}
	return -1 // not found
}
