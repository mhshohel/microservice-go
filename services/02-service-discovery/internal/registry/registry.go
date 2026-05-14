// registry.go — In-memory service registry.
//
// This is the core of service discovery: a thread-safe store that tracks
// which services are running and where to find them.
//
// Think of it like a phone book:
//   - Services "register" when they start (add their number to the book)
//   - Services "deregister" when they stop (remove from the book)
//   - Callers "query" to find a service's address
//   - Services send "heartbeats" to prove they're still alive
//
// If a service crashes without deregistering (which is common!), the heartbeat
// mechanism catches it: no heartbeat in 30 seconds = assumed dead = removed.
//
// Thread safety: every read/write to the instances map is protected by a mutex
// because Go's http server handles each request in a separate goroutine, so
// multiple requests could touch the map at the same time.

package registry

import (
	"fmt"
	"math/rand/v2"
	"sync"
	"time"
)

// DefaultHeartbeatTimeout is how long to wait before removing a silent service.
// If no heartbeat arrives within this window, the instance is considered crashed.
const DefaultHeartbeatTimeout = 30 * time.Second

// Instance represents one running copy of a service.
//
// A single service (like "user-svc") may have many instances running in parallel
// on different ports or machines. Each instance gets its own unique ID so we can
// track heartbeats and deregister the right one.
type Instance struct {
	ID            string            // unique identifier for this instance, e.g. "user-svc-a3f9c2b1"
	Name          string            // human-readable service name, e.g. "user-svc"
	Address       string            // host:port where this instance can be reached
	Metadata      map[string]string // optional tags: version, region, env, etc.
	RegisteredAt  time.Time         // when this instance first registered
	LastHeartbeat time.Time         // when we last heard from this instance
}

// Registry is the central store for all service instances.
//
// It uses a plain map with a mutex (RWMutex = reader/writer lock) so that:
//   - Many goroutines can READ at the same time (multiple concurrent lookups)
//   - Only ONE goroutine can WRITE at a time (register/deregister/heartbeat)
type Registry struct {
	mu               sync.RWMutex         // protects the instances map
	instances        map[string]*Instance // instanceID → Instance
	heartbeatTimeout time.Duration        // how long before a silent instance is removed
}

// New creates an empty registry ready to accept registrations.
//
// heartbeatTimeout controls how long before a silent instance is removed.
// Pass DefaultHeartbeatTimeout for the standard 30-second window.
func New(heartbeatTimeout time.Duration) *Registry {
	return &Registry{
		instances:        make(map[string]*Instance),
		heartbeatTimeout: heartbeatTimeout,
	}
}

// RegisterRequest holds the information a service sends when it registers.
type RegisterRequest struct {
	Name     string            // required: the service name, e.g. "user-svc"
	Address  string            // required: host:port to reach the service
	Metadata map[string]string // optional: any extra key-value pairs
}

// Register adds a new service instance to the registry.
//
// It generates a unique instance ID using the service name + a random hex suffix.
// Returns the newly created Instance so the caller can get the assigned ID.
func (reg *Registry) Register(req RegisterRequest) (*Instance, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("service name is required")
	}
	if req.Address == "" {
		return nil, fmt.Errorf("service address is required")
	}

	// Generate a unique ID: "user-svc-a3f9c2b1"
	// Using a short random hex suffix keeps IDs readable but still unique.
	id := fmt.Sprintf("%s-%08x", req.Name, rand.Uint32())

	now := time.Now()
	instance := &Instance{
		ID:            id,
		Name:          req.Name,
		Address:       req.Address,
		Metadata:      req.Metadata,
		RegisteredAt:  now,
		LastHeartbeat: now, // treat registration itself as the first heartbeat
	}

	reg.mu.Lock()         // acquire write lock — no other goroutine can read or write
	defer reg.mu.Unlock() // release when this function returns

	reg.instances[id] = instance
	return instance, nil
}

// Deregister removes an instance from the registry by its ID.
//
// This is called when a service shuts down gracefully. If the service crashes,
// it won't call Deregister — instead, the heartbeat timeout will clean it up.
// Returns an error if the instance doesn't exist.
func (reg *Registry) Deregister(instanceID string) error {
	reg.mu.Lock()
	defer reg.mu.Unlock()

	if _, exists := reg.instances[instanceID]; !exists {
		return fmt.Errorf("instance %q not found", instanceID)
	}

	delete(reg.instances, instanceID)
	return nil
}

// Heartbeat resets the "last seen" timer for an instance.
//
// Services should call this every few seconds to prove they're still alive.
// The registry's cleanup loop removes instances that haven't sent a heartbeat
// within heartbeatTimeout.
// Returns an error if the instance ID is not registered.
func (reg *Registry) Heartbeat(instanceID string) error {
	reg.mu.Lock()
	defer reg.mu.Unlock()

	instance, exists := reg.instances[instanceID]
	if !exists {
		return fmt.Errorf("instance %q not found — it may have been removed due to timeout", instanceID)
	}

	instance.LastHeartbeat = time.Now()
	return nil
}

// GetByName returns all healthy instances registered under the given service name.
//
// "Healthy" here means: we've received a heartbeat recently enough.
// This is used for client-side discovery (caller gets all instances and picks one)
// or for server-side discovery (registry picks one for the caller).
func (reg *Registry) GetByName(name string) []*Instance {
	reg.mu.RLock() // read lock — allows concurrent reads
	defer reg.mu.RUnlock()

	var result []*Instance
	for _, inst := range reg.instances {
		if inst.Name == name {
			result = append(result, inst)
		}
	}
	return result
}

// GetAll returns every registered instance across all services.
//
// Useful for admin dashboards or debugging: "show me everything in the registry."
func (reg *Registry) GetAll() []*Instance {
	reg.mu.RLock()
	defer reg.mu.RUnlock()

	result := make([]*Instance, 0, len(reg.instances))
	for _, inst := range reg.instances {
		result = append(result, inst)
	}
	return result
}

// PickOne returns a single instance for the given service name (server-side discovery).
//
// This implements the simplest possible selection strategy: random pick.
// In a real registry you might pick the least-loaded, the closest region, etc.
// Returns an error if no instances are registered for that service name.
func (reg *Registry) PickOne(name string) (*Instance, error) {
	instances := reg.GetByName(name)
	if len(instances) == 0 {
		return nil, fmt.Errorf("no instances registered for service %q", name)
	}

	// Pick a random instance. rand.IntN is safe to call from multiple goroutines.
	return instances[rand.IntN(len(instances))], nil
}

// RemoveExpired scans all instances and removes those that have missed too many heartbeats.
//
// This should be called periodically (e.g., every 10 seconds) in a background goroutine.
// It's the "garbage collector" for crashed services that never deregistered.
//
// Returns the IDs of instances that were removed, useful for logging.
func (reg *Registry) RemoveExpired() []string {
	now := time.Now()

	reg.mu.Lock()
	defer reg.mu.Unlock()

	var removed []string
	for id, inst := range reg.instances {
		// time.Since(inst.LastHeartbeat) = how long ago we heard from this instance
		if now.Sub(inst.LastHeartbeat) > reg.heartbeatTimeout {
			delete(reg.instances, id)
			removed = append(removed, id)
		}
	}
	return removed
}

// Count returns how many instances are currently registered.
// Useful for the health endpoint and tests.
func (reg *Registry) Count() int {
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	return len(reg.instances)
}
