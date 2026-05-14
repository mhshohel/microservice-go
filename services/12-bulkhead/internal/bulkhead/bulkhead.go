// bulkhead.go — Bulkhead pattern implementation using a buffered channel semaphore.
//
// A Bulkhead limits how many concurrent calls can be in-flight to a given
// dependency at the same time. Each dependency gets its own Bulkhead with its
// own fixed-size slot pool. This is the Go equivalent of a thread-pool bulkhead
// in Java or .NET — but implemented with channels, which are idiomatic in Go.
//
// Analogy: a ship's watertight bulkhead divides the hull into isolated
// compartments. If one compartment floods, the others stay dry. Here, if one
// dependency (e.g. payment-svc) is slow and fills its pool, other dependencies
// (e.g. user-svc) keep their own pools unaffected.
//
// Concurrency:
//   Multiple goroutines call Execute/TryExecute at the same time. The semaphore
//   (buffered channel) is safe for concurrent use — Go's channel send/receive
//   operations are already synchronized. The Metrics fields use sync/atomic so
//   counter increments are also race-free.

package bulkhead

import (
	"errors"
	"sync/atomic"
)

// ErrBulkheadFull is returned when all slots in the bulkhead are occupied.
// Callers should handle this as a "capacity exceeded" signal and respond
// with an appropriate error to the client (e.g. HTTP 503 Service Unavailable).
var ErrBulkheadFull = errors.New("bulkhead is full: max concurrent limit reached")

// metrics holds counters that track how the bulkhead is being used.
// Using sync/atomic ensures the counters are safe when multiple goroutines
// update them concurrently — no mutex needed for simple integer counters.
type metrics struct {
	TotalRequests atomic.Int64 // every call to Execute or TryExecute
	Rejected      atomic.Int64 // calls turned away because the pool was full
	InFlight      atomic.Int64 // calls currently executing inside the bulkhead
}

// Bulkhead limits concurrent access to a resource using a counting semaphore.
//
// The semaphore is implemented as a buffered channel: the channel's capacity
// equals the maximum number of concurrent calls allowed. Sending a value into
// the channel "acquires" a slot; receiving (removing) a value "releases" it.
// If the channel is full, the send either blocks (Execute) or fails immediately
// (TryExecute), giving us two different backpressure strategies.
type Bulkhead struct {
	name    string        // human-readable label, used in metrics/logging
	sem     chan struct{} // counting semaphore: capacity = max concurrent slots
	Metrics metrics       // public so callers can read stats without extra getters
}

// New creates a Bulkhead with a fixed pool of maxConcurrent slots.
//
//	name:           a label for logging and metrics (e.g. "payment-bulkhead")
//	maxConcurrent:  how many calls may run at the same time inside this bulkhead
//
// Example:
//
//	paymentBulkhead := bulkhead.New("payment-bulkhead", 3)
//	err := paymentBulkhead.Execute(func() error {
//	    return callPaymentService()
//	})
func New(name string, maxConcurrent int) *Bulkhead {
	return &Bulkhead{
		name: name,
		// A buffered channel of struct{} is the classic Go semaphore.
		// struct{} uses zero bytes — it carries no data, only a slot reservation.
		sem: make(chan struct{}, maxConcurrent),
	}
}

// Execute runs fn inside the bulkhead.
//
// If a slot is available, fn runs immediately in the current goroutine.
// If all slots are full, Execute BLOCKS until one frees up, then runs fn.
// This is useful when you want to queue work rather than reject it outright.
//
// Returns fn's error on success, or ErrBulkheadFull only if the internal
// channel was somehow unavailable (which does not happen with blocking sends).
// Any error returned by fn is passed through unchanged to the caller.
func (b *Bulkhead) Execute(fn func() error) error {
	b.Metrics.TotalRequests.Add(1)

	// Send a token into the semaphore channel to acquire a slot.
	// This BLOCKS if the channel is full (all slots occupied).
	// When a running call finishes, it will release its slot and unblock us.
	b.sem <- struct{}{}

	// We now hold a slot. Track it as in-flight.
	b.Metrics.InFlight.Add(1)

	// Use defer to guarantee the slot is released even if fn panics.
	defer func() {
		// Receive from the channel to release our slot back to the pool.
		<-b.sem
		b.Metrics.InFlight.Add(-1)
	}()

	// Run the caller's function now that we have a slot.
	return fn()
}

// TryExecute runs fn inside the bulkhead without blocking.
//
// If a slot is available, fn runs immediately. If all slots are full,
// TryExecute returns ErrBulkheadFull right away — it never waits.
// This is the non-blocking variant: use it when you prefer to fail fast
// rather than queue up waiting goroutines.
func (b *Bulkhead) TryExecute(fn func() error) error {
	b.Metrics.TotalRequests.Add(1)

	// select with a default clause makes the channel send non-blocking.
	// If the channel is full (no free slot), the default branch runs instead.
	select {
	case b.sem <- struct{}{}:
		// Slot acquired — proceed with the call.
	default:
		// Channel is full — all slots are occupied. Reject immediately.
		b.Metrics.Rejected.Add(1)
		return ErrBulkheadFull
	}

	// We now hold a slot. Track it as in-flight.
	b.Metrics.InFlight.Add(1)

	// Release the slot when fn finishes, whether it succeeds or fails.
	defer func() {
		<-b.sem
		b.Metrics.InFlight.Add(-1)
	}()

	return fn()
}

// Available returns the number of free slots right now.
//
// This is a snapshot in time — by the time you read the value, another
// goroutine may have acquired or released a slot. Use for monitoring only.
func (b *Bulkhead) Available() int {
	// cap(sem) is the total pool size; len(sem) is how many are occupied.
	return cap(b.sem) - len(b.sem)
}

// Name returns the label this bulkhead was created with.
func (b *Bulkhead) Name() string {
	return b.name
}

// Capacity returns the maximum number of concurrent calls this bulkhead allows.
func (b *Bulkhead) Capacity() int {
	return cap(b.sem)
}
