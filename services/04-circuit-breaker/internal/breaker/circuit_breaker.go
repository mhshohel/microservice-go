// circuit_breaker.go — Circuit Breaker state machine.
//
// A circuit breaker wraps outgoing calls to a service and tracks failures.
// When too many failures occur, it "opens" the circuit and stops making real
// calls — returning immediate errors instead. This prevents cascade failures.
//
// The three states:
//
//   CLOSED   → normal. All calls pass through. Failures are counted.
//   OPEN     → tripped. All calls fail immediately. No real calls made.
//   HALF-OPEN → testing. One probe call passes through. If it succeeds,
//               reset to CLOSED. If it fails, back to OPEN.
//
// Thread safety:
//   Multiple goroutines call Execute concurrently. We use sync.Mutex to
//   protect the state machine — only one goroutine may read/modify state at a time.
//   The actual user function (fn) runs outside the lock so slow calls don't
//   block state changes for other goroutines.

package breaker

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// State represents which of the three circuit breaker states we're in.
type State int

const (
	StateClosed   State = iota // normal operation — calls pass through
	StateOpen                  // tripped — calls fail immediately
	StateHalfOpen              // testing recovery — one probe call allowed
)

// String returns a human-readable state name for logging and debugging.
func (s State) String() string {
	switch s {
	case StateClosed:
		return "CLOSED"
	case StateOpen:
		return "OPEN"
	case StateHalfOpen:
		return "HALF-OPEN"
	default:
		return "UNKNOWN"
	}
}

// ErrCircuitOpen is returned when the circuit is OPEN and a call is rejected.
// Callers can check for this specific error to distinguish "circuit open"
// from actual downstream errors.
var ErrCircuitOpen = errors.New("circuit breaker is OPEN: call rejected")

// CircuitBreaker wraps outgoing calls and implements the state machine.
type CircuitBreaker struct {
	mu            sync.Mutex    // protects all fields below
	state         State         // current state: CLOSED, OPEN, or HALF-OPEN
	failures      int           // consecutive failures in the CLOSED state
	maxFailures   int           // failure threshold before opening the circuit
	openTimeout   time.Duration // how long to stay OPEN before trying HALF-OPEN
	lastOpenedAt  time.Time     // when we last transitioned to OPEN
	halfOpenProbe bool          // true if a probe is currently running in HALF-OPEN
}

// New creates a circuit breaker with the given configuration.
//
//	maxFailures:  how many consecutive failures trigger the OPEN state
//	openTimeout:  how long to stay OPEN before trying a probe call
//
// Example:
//
//	cb := breaker.New(5, 60*time.Second)
//	err := cb.Execute(func() error {
//	    return callPaymentService()
//	})
func New(maxFailures int, openTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:       StateClosed,
		maxFailures: maxFailures,
		openTimeout: openTimeout,
	}
}

// Execute runs fn if the circuit allows it.
//
// CLOSED state:  fn runs normally. On success, failure count resets.
//
//	On failure, count increments. At maxFailures, opens circuit.
//
// OPEN state:    fn is NOT called. Returns ErrCircuitOpen immediately.
//
//	After openTimeout, transitions to HALF-OPEN.
//
// HALF-OPEN state: if no probe is in flight, fn runs as a probe.
//
//	On success: reset to CLOSED. On failure: back to OPEN.
//	If a probe is already in flight, reject with ErrCircuitOpen.
func (cb *CircuitBreaker) Execute(fn func() error) error {
	// Check whether we should allow this call (and update state if needed)
	if err := cb.allowRequest(); err != nil {
		return err
	}

	// Run the actual call OUTSIDE the mutex.
	// We don't want to hold the lock while waiting for a network call —
	// that would block all other goroutines from checking the breaker state.
	err := fn()

	// Record the result (success or failure)
	cb.recordResult(err)
	return err
}

// allowRequest checks the current state and decides whether to allow the call.
// Returns nil if the call should proceed, ErrCircuitOpen if it should be rejected.
func (cb *CircuitBreaker) allowRequest() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {

	case StateClosed:
		// Normal operation — allow the call
		return nil

	case StateOpen:
		// Check if the timeout has elapsed — time to try a probe
		if time.Since(cb.lastOpenedAt) >= cb.openTimeout {
			// Transition to HALF-OPEN to allow one probe call
			cb.state = StateHalfOpen
			cb.halfOpenProbe = true // mark that a probe is in flight
			return nil
		}
		// Still within the timeout — reject the call
		return fmt.Errorf("%w (opens at %s, retry after %s)",
			ErrCircuitOpen,
			cb.lastOpenedAt.Format(time.RFC3339),
			cb.openTimeout,
		)

	case StateHalfOpen:
		// If a probe is already in flight, reject additional calls
		if cb.halfOpenProbe {
			return fmt.Errorf("%w (probe already in flight)", ErrCircuitOpen)
		}
		// Allow (shouldn't normally reach here, but just in case)
		cb.halfOpenProbe = true
		return nil
	}

	return nil
}

// recordResult updates the circuit breaker state based on whether the call succeeded.
func (cb *CircuitBreaker) recordResult(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {

	case StateClosed:
		if err == nil {
			// Successful call — reset the failure counter
			cb.failures = 0
		} else {
			// Failed call — increment counter and check threshold
			cb.failures++
			if cb.failures >= cb.maxFailures {
				// Too many failures — open the circuit
				cb.state = StateOpen
				cb.lastOpenedAt = time.Now()
			}
		}

	case StateHalfOpen:
		cb.halfOpenProbe = false // probe is no longer in flight
		if err == nil {
			// Probe succeeded — service recovered, close the circuit
			cb.state = StateClosed
			cb.failures = 0
		} else {
			// Probe failed — service still broken, go back to OPEN
			cb.state = StateOpen
			cb.lastOpenedAt = time.Now()
		}
	}
	// In StateOpen we don't track results (calls are rejected before fn runs)
}

// State returns the current state of the circuit breaker.
// Safe to call from any goroutine.
func (cb *CircuitBreaker) State() State {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

// Failures returns the current consecutive failure count.
// Useful for monitoring and tests.
func (cb *CircuitBreaker) Failures() int {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.failures
}

// Reset forces the circuit breaker back to CLOSED with zero failures.
// Useful for tests and manual recovery.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = StateClosed
	cb.failures = 0
	cb.halfOpenProbe = false
}
