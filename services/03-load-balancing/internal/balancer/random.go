// random.go — Random load balancing.
//
// Picks a backend at random on each request.
//
// This is the simplest possible algorithm — no state, no coordination.
// Over many requests it tends toward an even distribution, but there are
// no guarantees for short bursts (unlike round-robin which is exactly even).
//
// Use cases:
//   - Quick prototypes where algorithmic correctness doesn't matter yet
//   - Extremely high-throughput systems where atomic counters become bottlenecks
//     (though round-robin with atomic.Uint64 is nearly as fast)
//
// In practice, round-robin is almost always the better default.

package balancer

import (
	"fmt"
	"math/rand/v2"
)

// Random picks backends uniformly at random.
type Random struct {
	backends []Backend
}

// NewRandom creates a random balancer.
func NewRandom(backends []Backend) (*Random, error) {
	if len(backends) == 0 {
		return nil, fmt.Errorf("random balancer requires at least one backend")
	}
	return &Random{backends: backends}, nil
}

// Next picks a backend at random. clientIP is ignored.
// rand.IntN is safe for concurrent use in Go 1.20+ (uses a per-goroutine source).
func (r *Random) Next(_ string) (string, error) {
	idx := rand.IntN(len(r.backends))
	return r.backends[idx].Address, nil
}
