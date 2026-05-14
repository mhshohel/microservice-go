// weighted_round_robin.go — Weighted Round Robin load balancing.
//
// Like round-robin but backends with higher weight receive proportionally more requests.
//
// Example with weights [3, 1, 1]:
//   The expanded sequence is: [B0, B0, B0, B1, B2] (total = 5 slots)
//   Requests cycle through this sequence:
//     Request 1 → B0
//     Request 2 → B0
//     Request 3 → B0
//     Request 4 → B1
//     Request 5 → B2
//     Request 6 → B0  (wraps back to start)
//
// Implementation: we pre-expand the backend list at creation time.
// If backend B0 has weight=3, we insert B0 into the list 3 times.
// Then regular round-robin over the expanded list gives weighted distribution.
//
// This is the simplest weighted implementation. More advanced versions
// (like Nginx's "smooth weighted round-robin") avoid bursts by interleaving.

package balancer

import (
	"fmt"
	"sync/atomic"
)

// WeightedRoundRobin rotates through backends proportionally to their weight.
type WeightedRoundRobin struct {
	expanded []Backend // pre-expanded list: backend with weight N appears N times
	counter  atomic.Uint64
}

// NewWeightedRoundRobin creates a weighted round-robin balancer.
//
// Each Backend's Weight field controls how often it's selected.
// A Backend with Weight=0 is treated as Weight=1 (it's still included).
func NewWeightedRoundRobin(backends []Backend) (*WeightedRoundRobin, error) {
	if len(backends) == 0 {
		return nil, fmt.Errorf("weighted round robin requires at least one backend")
	}

	// Expand the backend list based on weights
	var expanded []Backend
	for _, b := range backends {
		weight := b.Weight
		if weight <= 0 {
			weight = 1 // default weight
		}
		for range weight {
			expanded = append(expanded, b)
		}
	}

	return &WeightedRoundRobin{expanded: expanded}, nil
}

// Next returns the next backend following the weighted distribution.
func (w *WeightedRoundRobin) Next(_ string) (string, error) {
	idx := w.counter.Add(1) - 1
	chosen := w.expanded[idx%uint64(len(w.expanded))]
	return chosen.Address, nil
}
