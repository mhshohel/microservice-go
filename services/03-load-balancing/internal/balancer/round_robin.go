// round_robin.go — Round Robin load balancing.
//
// Sends requests to backends in a repeating circular order:
//   Request 1 → Backend 0
//   Request 2 → Backend 1
//   Request 3 → Backend 2
//   Request 4 → Backend 0  (wraps around)
//
// This is the simplest fair algorithm: every backend gets the same number of
// requests over time, regardless of how long each request takes.
//
// Thread safety: the current index is shared across goroutines (each HTTP request
// runs in its own goroutine). We use atomic.Uint64 to increment the counter
// without a mutex — atomic operations are faster than locking for a single integer.

package balancer

import (
	"fmt"
	"sync/atomic"
)

// RoundRobin cycles through backends one by one.
type RoundRobin struct {
	backends []Backend
	counter  atomic.Uint64 // how many requests have been seen so far
}

// NewRoundRobin creates a round-robin balancer for the given backends.
// Returns an error if no backends are provided.
func NewRoundRobin(backends []Backend) (*RoundRobin, error) {
	if len(backends) == 0 {
		return nil, fmt.Errorf("round robin requires at least one backend")
	}
	return &RoundRobin{backends: backends}, nil
}

// Next returns the next backend address in round-robin order.
// clientIP is ignored — round-robin doesn't care who the client is.
func (rr *RoundRobin) Next(_ string) (string, error) {
	// Add 1, then take the remainder when dividing by number of backends.
	// Example with 3 backends:
	//   call 0 → (0+1) % 3 = 1  → backend[1]
	//   call 1 → (1+1) % 3 = 2  → backend[2]
	//   call 2 → (2+1) % 3 = 0  → backend[0]  (wraps)
	//
	// atomic.Add returns the NEW value after adding 1.
	// Subtracting 1 gives us a zero-based index.
	idx := rr.counter.Add(1) - 1
	chosen := rr.backends[idx%uint64(len(rr.backends))]
	return chosen.Address, nil
}
