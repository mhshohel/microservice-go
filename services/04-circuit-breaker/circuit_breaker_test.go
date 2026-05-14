// circuit_breaker_test.go — Tests for the circuit breaker state machine.
//
// We test every state transition:
//   CLOSED → OPEN    (enough failures)
//   OPEN   → reject  (while within timeout)
//   OPEN   → HALF-OPEN (after timeout)
//   HALF-OPEN → CLOSED (probe succeeds)
//   HALF-OPEN → OPEN   (probe fails)
//
// We also test thread safety (concurrent Execute calls under -race).

package circuit_breaker_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"microservices-go/services/04-circuit-breaker/internal/breaker"
)

// ── Helpers ───────────────────────────────────────────────────────────────────

// failFn always returns an error. Use as the fn argument to Execute.
func failFn() error { return errors.New("simulated failure") }

// successFn always returns nil. Use as the fn argument to Execute.
func successFn() error { return nil }

// openCircuit triggers enough failures to open the circuit.
// maxFailures is how many calls it takes.
func openCircuit(t *testing.T, cb *breaker.CircuitBreaker, maxFailures int) {
	t.Helper()
	for i := range maxFailures {
		if err := cb.Execute(failFn); err == nil {
			t.Fatalf("call %d: expected failure from failFn but got nil", i+1)
		}
	}
}

// ── CLOSED State Tests ────────────────────────────────────────────────────────

func TestCircuitBreaker_InitialState_IsClosed(t *testing.T) {
	cb := breaker.New(5, 10*time.Second)

	if cb.State() != breaker.StateClosed {
		t.Errorf("expected CLOSED state initially, got %s", cb.State())
	}
	if cb.Failures() != 0 {
		t.Errorf("expected 0 failures initially, got %d", cb.Failures())
	}
}

func TestCircuitBreaker_Closed_SuccessPassesThrough(t *testing.T) {
	cb := breaker.New(5, 10*time.Second)

	err := cb.Execute(successFn)
	if err != nil {
		t.Errorf("expected no error on success, got: %v", err)
	}
	if cb.State() != breaker.StateClosed {
		t.Errorf("expected CLOSED after success, got %s", cb.State())
	}
}

func TestCircuitBreaker_Closed_FailureIncrementsCounter(t *testing.T) {
	cb := breaker.New(5, 10*time.Second)

	cb.Execute(failFn) //nolint
	cb.Execute(failFn) //nolint
	cb.Execute(failFn) //nolint

	if cb.Failures() != 3 {
		t.Errorf("expected 3 failures, got %d", cb.Failures())
	}
	if cb.State() != breaker.StateClosed {
		t.Errorf("expected still CLOSED (threshold not reached), got %s", cb.State())
	}
}

func TestCircuitBreaker_Closed_SuccessResetsCounter(t *testing.T) {
	cb := breaker.New(5, 10*time.Second)

	// 3 failures, then a success
	cb.Execute(failFn)    //nolint
	cb.Execute(failFn)    //nolint
	cb.Execute(failFn)    //nolint
	cb.Execute(successFn) //nolint

	// Counter should reset to 0 after success
	if cb.Failures() != 0 {
		t.Errorf("expected 0 failures after success, got %d", cb.Failures())
	}
}

// ── CLOSED → OPEN Transition ──────────────────────────────────────────────────

func TestCircuitBreaker_OpensAfterMaxFailures(t *testing.T) {
	maxFailures := 5
	cb := breaker.New(maxFailures, 10*time.Second)

	openCircuit(t, cb, maxFailures)

	if cb.State() != breaker.StateOpen {
		t.Errorf("expected OPEN after %d failures, got %s", maxFailures, cb.State())
	}
}

// ── OPEN State Tests ──────────────────────────────────────────────────────────

func TestCircuitBreaker_Open_RejectsCallsImmediately(t *testing.T) {
	cb := breaker.New(3, 10*time.Second)
	openCircuit(t, cb, 3)

	// Track whether fn was actually called
	called := false
	err := cb.Execute(func() error {
		called = true
		return nil
	})

	if called {
		t.Error("fn should NOT be called when circuit is OPEN")
	}
	if !errors.Is(err, breaker.ErrCircuitOpen) {
		t.Errorf("expected ErrCircuitOpen, got: %v", err)
	}
}

func TestCircuitBreaker_Open_StaysOpenWithinTimeout(t *testing.T) {
	cb := breaker.New(3, 10*time.Second) // long timeout — won't expire in this test
	openCircuit(t, cb, 3)

	// Multiple rejected calls should not change state
	for range 5 {
		cb.Execute(successFn) //nolint
	}

	if cb.State() != breaker.StateOpen {
		t.Errorf("expected to stay OPEN within timeout, got %s", cb.State())
	}
}

// ── OPEN → HALF-OPEN Transition ───────────────────────────────────────────────

func TestCircuitBreaker_Open_TransitionsToHalfOpenAfterTimeout(t *testing.T) {
	// Use a very short timeout so the test doesn't wait long
	cb := breaker.New(3, 1*time.Millisecond)
	openCircuit(t, cb, 3)

	// Wait for the timeout to expire
	time.Sleep(5 * time.Millisecond)

	// The first call after timeout should transition to HALF-OPEN and run the probe
	probeRan := false
	cb.Execute(func() error { //nolint
		probeRan = true
		return nil
	})

	if !probeRan {
		t.Error("expected probe call to run after timeout elapsed")
	}
}

// ── HALF-OPEN State Tests ─────────────────────────────────────────────────────

func TestCircuitBreaker_HalfOpen_ProbeSuccessCloses(t *testing.T) {
	cb := breaker.New(3, 1*time.Millisecond)
	openCircuit(t, cb, 3)
	time.Sleep(5 * time.Millisecond)

	// Probe succeeds → should close the circuit
	err := cb.Execute(successFn)
	if err != nil {
		t.Fatalf("probe call should succeed but got error: %v", err)
	}

	if cb.State() != breaker.StateClosed {
		t.Errorf("expected CLOSED after successful probe, got %s", cb.State())
	}
	if cb.Failures() != 0 {
		t.Errorf("expected failures reset to 0 after probe success, got %d", cb.Failures())
	}
}

func TestCircuitBreaker_HalfOpen_ProbeFailureReopens(t *testing.T) {
	cb := breaker.New(3, 1*time.Millisecond)
	openCircuit(t, cb, 3)
	time.Sleep(5 * time.Millisecond)

	// Probe fails → should go back to OPEN
	cb.Execute(failFn) //nolint

	if cb.State() != breaker.StateOpen {
		t.Errorf("expected OPEN after failed probe, got %s", cb.State())
	}
}

func TestCircuitBreaker_HalfOpen_RejectsAdditionalCallsDuringProbe(t *testing.T) {
	cb := breaker.New(3, 1*time.Millisecond)
	openCircuit(t, cb, 3)
	time.Sleep(5 * time.Millisecond)

	// Simulate a slow probe: hold the state in HALF-OPEN by not completing it yet
	// We do this by calling Execute with a blocking function in a goroutine,
	// then immediately trying a second call.

	probeDone := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		cb.Execute(func() error { //nolint
			<-probeDone // block until main goroutine signals
			return nil
		})
	}()

	// Give the goroutine time to reach the slow function and acquire HALF-OPEN probe slot
	time.Sleep(2 * time.Millisecond)

	// Now try another call — should be rejected because probe is in flight
	err := cb.Execute(successFn)
	if !errors.Is(err, breaker.ErrCircuitOpen) {
		t.Errorf("expected ErrCircuitOpen while probe is in flight, got: %v", err)
	}

	// Let the probe finish
	close(probeDone)
	wg.Wait()
}

// ── Reset Tests ───────────────────────────────────────────────────────────────

func TestCircuitBreaker_Reset_ClosesFromOpen(t *testing.T) {
	cb := breaker.New(3, 10*time.Second)
	openCircuit(t, cb, 3)

	cb.Reset()

	if cb.State() != breaker.StateClosed {
		t.Errorf("expected CLOSED after Reset, got %s", cb.State())
	}
	if cb.Failures() != 0 {
		t.Errorf("expected 0 failures after Reset, got %d", cb.Failures())
	}
}

func TestCircuitBreaker_Reset_AllowsCallsAgain(t *testing.T) {
	cb := breaker.New(3, 10*time.Second)
	openCircuit(t, cb, 3)
	cb.Reset()

	called := false
	err := cb.Execute(func() error {
		called = true
		return nil
	})

	if err != nil {
		t.Errorf("expected no error after Reset, got: %v", err)
	}
	if !called {
		t.Error("expected fn to be called after Reset")
	}
}

// ── Thread Safety Tests ───────────────────────────────────────────────────────

func TestCircuitBreaker_ConcurrentExecute_NoDataRace(t *testing.T) {
	// This test is mainly to catch data races under -race flag.
	// 50 goroutines all calling Execute simultaneously.
	cb := breaker.New(5, 10*time.Second)

	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cb.Execute(successFn) //nolint
		}()
	}
	wg.Wait()

	// 50 successes — still CLOSED, 0 failures
	if cb.State() != breaker.StateClosed {
		t.Errorf("expected CLOSED after all successes, got %s", cb.State())
	}
}

func TestCircuitBreaker_ConcurrentFailures_OpensCorrectly(t *testing.T) {
	cb := breaker.New(5, 10*time.Second)

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cb.Execute(failFn) //nolint
		}()
	}
	wg.Wait()

	// After enough concurrent failures, the circuit should be OPEN
	if cb.State() != breaker.StateOpen {
		t.Errorf("expected OPEN after many concurrent failures, got %s", cb.State())
	}
}

// ── Full Lifecycle Test ───────────────────────────────────────────────────────

func TestCircuitBreaker_FullLifecycle(t *testing.T) {
	cb := breaker.New(3, 5*time.Millisecond)

	// Phase 1: CLOSED — normal calls succeed
	for range 3 {
		if err := cb.Execute(successFn); err != nil {
			t.Fatalf("phase 1: unexpected error: %v", err)
		}
	}
	if cb.State() != breaker.StateClosed {
		t.Fatalf("phase 1: expected CLOSED, got %s", cb.State())
	}

	// Phase 2: CLOSED → OPEN — 3 failures trigger the circuit
	for range 3 {
		cb.Execute(failFn) //nolint
	}
	if cb.State() != breaker.StateOpen {
		t.Fatalf("phase 2: expected OPEN after failures, got %s", cb.State())
	}

	// Phase 3: OPEN — calls rejected immediately
	if err := cb.Execute(successFn); !errors.Is(err, breaker.ErrCircuitOpen) {
		t.Fatalf("phase 3: expected ErrCircuitOpen, got: %v", err)
	}

	// Phase 4: OPEN → HALF-OPEN — after timeout, probe is allowed
	time.Sleep(10 * time.Millisecond)

	// Phase 5: HALF-OPEN → CLOSED — probe succeeds
	if err := cb.Execute(successFn); err != nil {
		t.Fatalf("phase 5: probe should succeed, got: %v", err)
	}
	if cb.State() != breaker.StateClosed {
		t.Fatalf("phase 5: expected CLOSED after successful probe, got %s", cb.State())
	}

	// Phase 6: Back to normal operation
	if err := cb.Execute(successFn); err != nil {
		t.Fatalf("phase 6: expected success after recovery, got: %v", err)
	}
}
