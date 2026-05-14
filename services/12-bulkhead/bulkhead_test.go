// bulkhead_test.go — Tests for the Bulkhead pattern implementation.
//
// We test the core bulkhead behaviours:
//   - Normal execution within capacity
//   - Rejection when the pool is full (TryExecute, non-blocking)
//   - Blocking until a slot frees up (Execute, blocking)
//   - Accurate metrics tracking
//   - Available() reflects current usage
//   - Concurrent safety under the -race detector
//   - Error propagation from the wrapped function

package bulkhead_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"microservices-go/services/12-bulkhead/internal/bulkhead"
)

// ── Basic Execution Tests ─────────────────────────────────────────────────────

// TestBulkhead_ExecutesWithinLimit verifies that when we are below capacity,
// both Execute and TryExecute run the function and return its result.
func TestBulkhead_ExecutesWithinLimit(t *testing.T) {
	bh := bulkhead.New("test-bulkhead", 5)

	// Execute should run fn and return nil when there is capacity.
	err := bh.Execute(func() error {
		return nil
	})
	if err != nil {
		t.Errorf("Execute: expected nil error, got %v", err)
	}

	// TryExecute should also succeed when there is capacity.
	err = bh.TryExecute(func() error {
		return nil
	})
	if err != nil {
		t.Errorf("TryExecute: expected nil error, got %v", err)
	}
}

// TestBulkhead_TryExecute_RejectsWhenFull verifies the non-blocking path:
// when all slots are occupied, TryExecute returns ErrBulkheadFull immediately.
func TestBulkhead_TryExecute_RejectsWhenFull(t *testing.T) {
	// Small capacity makes it easy to saturate.
	bh := bulkhead.New("test-bulkhead", 3)

	// We need goroutines to hold all 3 slots while we attempt TryExecute.
	// Use a channel to keep the goroutines alive (holding their slots) until
	// we have run our assertion.
	release := make(chan struct{})
	var slotHolders sync.WaitGroup

	// Launch 3 goroutines that each acquire a slot and hold it.
	// Use Execute (blocking) so they wait for a slot — but since we start
	// them before the pool is full, each one should get a slot immediately.
	acquired := make(chan struct{}, 3)
	for i := 0; i < 3; i++ {
		slotHolders.Add(1)
		go func() {
			defer slotHolders.Done()
			bh.Execute(func() error { //nolint:errcheck
				acquired <- struct{}{} // signal that this slot is now held
				<-release              // wait until the test releases us
				return nil
			})
		}()
	}

	// Wait until all 3 goroutines have acquired their slots.
	for i := 0; i < 3; i++ {
		<-acquired
	}

	// Now the bulkhead is full (0 slots available).
	// TryExecute must return ErrBulkheadFull without blocking.
	err := bh.TryExecute(func() error {
		return nil
	})

	if !errors.Is(err, bulkhead.ErrBulkheadFull) {
		t.Errorf("expected ErrBulkheadFull when pool is full, got: %v", err)
	}

	// Release all slot-holding goroutines so they can finish cleanly.
	close(release)
	slotHolders.Wait()
}

// TestBulkhead_Execute_BlocksUntilSlotFree verifies the blocking path:
// when all slots are occupied, Execute waits until one is freed, then runs.
func TestBulkhead_Execute_BlocksUntilSlotFree(t *testing.T) {
	// Capacity of 1 makes the blocking behaviour easy to demonstrate:
	// if one goroutine holds the slot, the next Execute must wait.
	bh := bulkhead.New("test-bulkhead", 1)

	// Goroutine A acquires the only slot.
	slotAcquired := make(chan struct{})
	releaseSlot := make(chan struct{})

	go func() {
		bh.Execute(func() error { //nolint:errcheck
			close(slotAcquired) // signal: slot is now held
			<-releaseSlot       // hold the slot until released
			return nil
		})
	}()

	// Wait for goroutine A to acquire the slot.
	<-slotAcquired

	// Now Execute in the current goroutine — it must BLOCK because the only
	// slot is occupied. We run it in a goroutine so we can time it.
	executed := make(chan struct{})
	go func() {
		bh.Execute(func() error { //nolint:errcheck
			return nil
		})
		close(executed) // signal: Execute completed (slot was freed)
	}()

	// Give the second Execute a moment — it should NOT have completed yet
	// because the slot is still held.
	select {
	case <-executed:
		t.Fatal("Execute completed before slot was released — it should have blocked")
	case <-time.After(50 * time.Millisecond):
		// Good: Execute is still waiting (blocking) as expected.
	}

	// Release goroutine A's slot.
	close(releaseSlot)

	// Now the second Execute should unblock and complete quickly.
	select {
	case <-executed:
		// Good: Execute completed after slot was freed.
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Execute did not unblock after slot was released")
	}
}

// ── Metrics Tests ─────────────────────────────────────────────────────────────

// TestBulkhead_Metrics_Tracked verifies that TotalRequests and Rejected counters
// are updated correctly as calls succeed and are rejected.
func TestBulkhead_Metrics_Tracked(t *testing.T) {
	bh := bulkhead.New("metrics-test", 2)

	// Two successful calls — TotalRequests should be 2, Rejected 0.
	bh.Execute(func() error { return nil })    //nolint:errcheck
	bh.TryExecute(func() error { return nil }) //nolint:errcheck

	if got := bh.Metrics.TotalRequests.Load(); got != 2 {
		t.Errorf("expected TotalRequests=2 after 2 calls, got %d", got)
	}
	if got := bh.Metrics.Rejected.Load(); got != 0 {
		t.Errorf("expected Rejected=0 after 2 successful calls, got %d", got)
	}

	// Saturate the pool and reject one more call.
	release := make(chan struct{})
	var wg sync.WaitGroup

	acquired := make(chan struct{}, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bh.Execute(func() error { //nolint:errcheck
				acquired <- struct{}{}
				<-release
				return nil
			})
		}()
	}

	// Wait for both slots to be occupied.
	<-acquired
	<-acquired

	// This TryExecute should be rejected — pool is full.
	bh.TryExecute(func() error { return nil }) //nolint:errcheck

	close(release)
	wg.Wait()

	// Total: 2 (earlier) + 2 (slot holders) + 1 (rejected) = 5
	if got := bh.Metrics.TotalRequests.Load(); got != 5 {
		t.Errorf("expected TotalRequests=5, got %d", got)
	}
	// Rejected: 1
	if got := bh.Metrics.Rejected.Load(); got != 1 {
		t.Errorf("expected Rejected=1, got %d", got)
	}
}

// ── Available() Tests ─────────────────────────────────────────────────────────

// TestBulkhead_Available_ReflectsCurrentUsage verifies that Available() returns
// the correct count of free slots as goroutines acquire and release them.
func TestBulkhead_Available_ReflectsCurrentUsage(t *testing.T) {
	bh := bulkhead.New("avail-test", 4)

	// Initially all 4 slots are free.
	if got := bh.Available(); got != 4 {
		t.Errorf("expected Available=4 initially, got %d", got)
	}

	// Occupy 2 slots.
	release := make(chan struct{})
	var wg sync.WaitGroup
	acquired := make(chan struct{}, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bh.Execute(func() error { //nolint:errcheck
				acquired <- struct{}{}
				<-release
				return nil
			})
		}()
	}

	// Wait for both goroutines to have acquired their slots.
	<-acquired
	<-acquired

	// 2 slots occupied → 2 available.
	if got := bh.Available(); got != 2 {
		t.Errorf("expected Available=2 with 2 slots occupied, got %d", got)
	}

	// Release all slots.
	close(release)
	wg.Wait()

	// Allow a brief moment for the goroutines to complete their cleanup.
	time.Sleep(10 * time.Millisecond)

	// Back to full capacity.
	if got := bh.Available(); got != 4 {
		t.Errorf("expected Available=4 after releasing all slots, got %d", got)
	}
}

// ── Concurrency Safety Tests ──────────────────────────────────────────────────

// TestBulkhead_ConcurrentSafe launches 100 goroutines all calling TryExecute
// simultaneously. The -race flag will catch any data races.
// This is the most important test for verifying thread safety.
func TestBulkhead_ConcurrentSafe(t *testing.T) {
	// Capacity of 10 means ~90 of the 100 calls will be rejected,
	// but no goroutine should race on any shared state.
	bh := bulkhead.New("concurrent-test", 10)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// We don't care about the error — we just want concurrent activity.
			bh.TryExecute(func() error { //nolint:errcheck
				// Tiny sleep to increase the chance of overlapping goroutines.
				time.Sleep(1 * time.Millisecond)
				return nil
			})
		}()
	}
	wg.Wait()

	// After all goroutines finish, the pool should be fully available again.
	if got := bh.Available(); got != 10 {
		t.Errorf("expected all 10 slots free after completion, got %d available", got)
	}

	// Total requests must equal 100 (every goroutine made one call).
	if got := bh.Metrics.TotalRequests.Load(); got != 100 {
		t.Errorf("expected TotalRequests=100, got %d", got)
	}

	// Accepted + Rejected must equal Total.
	rejected := bh.Metrics.Rejected.Load()
	accepted := bh.Metrics.TotalRequests.Load() - rejected
	if accepted+rejected != 100 {
		t.Errorf("accepted(%d) + rejected(%d) should equal 100", accepted, rejected)
	}
}

// ── Error Propagation Tests ───────────────────────────────────────────────────

// TestBulkhead_ErrorPropagated verifies that errors returned by fn are passed
// back to the caller unchanged. The bulkhead is transparent to fn's errors.
func TestBulkhead_ErrorPropagated(t *testing.T) {
	bh := bulkhead.New("error-test", 5)

	expectedErr := errors.New("downstream service unavailable")

	// Execute should return exactly the same error fn returned.
	err := bh.Execute(func() error {
		return expectedErr
	})
	if !errors.Is(err, expectedErr) {
		t.Errorf("Execute: expected error %q, got: %v", expectedErr, err)
	}

	// TryExecute should also propagate fn's error (not confuse it with ErrBulkheadFull).
	err = bh.TryExecute(func() error {
		return expectedErr
	})
	if !errors.Is(err, expectedErr) {
		t.Errorf("TryExecute: expected error %q, got: %v", expectedErr, err)
	}

	// A rejected call should return ErrBulkheadFull, not the fn's error.
	// To test: fill the pool, then try.
	bh2 := bulkhead.New("error-test-2", 1)
	hold := make(chan struct{})
	acquired := make(chan struct{})

	go func() {
		bh2.Execute(func() error { //nolint:errcheck
			close(acquired)
			<-hold
			return nil
		})
	}()
	<-acquired

	err = bh2.TryExecute(func() error {
		return errors.New("this should not be returned")
	})
	if !errors.Is(err, bulkhead.ErrBulkheadFull) {
		t.Errorf("expected ErrBulkheadFull when pool is full, got: %v", err)
	}

	close(hold)
}
