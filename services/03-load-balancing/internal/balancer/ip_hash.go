// ip_hash.go — IP Hash load balancing.
//
// Hashes the client's IP address to always route the same client to the same backend.
//
//   Client 192.168.1.5  → hash(192.168.1.5) % 3 = 1 → Backend 1 (always)
//   Client 192.168.1.9  → hash(192.168.1.9) % 3 = 0 → Backend 0 (always)
//
// This is called "sticky sessions" or "session affinity": the same client is
// always "stuck" to the same backend. This matters when the backend stores
// per-user state in memory (like a shopping cart or login session) rather than
// in a shared database.
//
// Limitation: if a backend is removed (crashed or scaled down), all clients
// that were hashed to it get remapped — this disrupts their sessions.
// Consistent hashing (used by real systems) minimizes this disruption but
// is more complex to implement.
//
// We use FNV-1a — a fast, non-cryptographic hash that's built into Go's stdlib.

package balancer

import (
	"fmt"
	"hash/fnv"
)

// IPHash routes the same client IP to the same backend every time.
type IPHash struct {
	backends []Backend
}

// NewIPHash creates an IP-hash balancer.
func NewIPHash(backends []Backend) (*IPHash, error) {
	if len(backends) == 0 {
		return nil, fmt.Errorf("ip hash requires at least one backend")
	}
	return &IPHash{backends: backends}, nil
}

// Next hashes clientIP to select a backend.
// The same IP will always map to the same backend (as long as the backend list doesn't change).
func (h *IPHash) Next(clientIP string) (string, error) {
	// FNV-1a: fast hash, good distribution, included in Go stdlib
	hasher := fnv.New32a()
	hasher.Write([]byte(clientIP))
	hash := hasher.Sum32()

	// Map the hash to a backend index
	idx := hash % uint32(len(h.backends))
	return h.backends[idx].Address, nil
}
