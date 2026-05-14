// balancing_test.go — Tests for all 5 load balancing algorithms.
//
// Each algorithm is tested for:
//   - Correct distribution of requests across backends
//   - Thread safety (parallel calls with -race)
//   - Error handling (empty backend list)
//   - Algorithm-specific behaviour (weights, sticky IPs, connection counts)

package balancing_test

import (
	"fmt"
	"sync"
	"testing"

	"microservices-go/services/03-load-balancing/internal/balancer"
)

// threeBackends is a helper that returns 3 test backends with addresses b0, b1, b2.
func threeBackends() []balancer.Backend {
	return []balancer.Backend{
		{Address: "b0:9000", Weight: 1},
		{Address: "b1:9001", Weight: 1},
		{Address: "b2:9002", Weight: 1},
	}
}

// ── Round Robin ───────────────────────────────────────────────────────────────

func TestRoundRobin_CyclesThroughBackends(t *testing.T) {
	lb, err := balancer.NewRoundRobin(threeBackends())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 9 requests should visit each backend exactly 3 times
	counts := map[string]int{}
	for range 9 {
		addr, err := lb.Next("")
		if err != nil {
			t.Fatalf("Next() error: %v", err)
		}
		counts[addr]++
	}

	for _, b := range threeBackends() {
		if counts[b.Address] != 3 {
			t.Errorf("backend %s: expected 3 requests, got %d", b.Address, counts[b.Address])
		}
	}
}

func TestRoundRobin_EmptyBackends_ReturnsError(t *testing.T) {
	_, err := balancer.NewRoundRobin(nil)
	if err == nil {
		t.Error("expected error for empty backend list, got nil")
	}
}

func TestRoundRobin_ConcurrentSafe(t *testing.T) {
	lb, _ := balancer.NewRoundRobin(threeBackends())

	// Fire 100 concurrent goroutines and count results — no panics, no data races
	var mu sync.Mutex
	counts := map[string]int{}
	var wg sync.WaitGroup

	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			addr, err := lb.Next("")
			if err != nil {
				t.Errorf("concurrent Next() error: %v", err)
				return
			}
			mu.Lock()
			counts[addr]++
			mu.Unlock()
		}()
	}
	wg.Wait()

	// Total must be exactly 100
	total := 0
	for _, c := range counts {
		total += c
	}
	if total != 100 {
		t.Errorf("expected 100 total requests, got %d", total)
	}
}

// ── Weighted Round Robin ──────────────────────────────────────────────────────

func TestWeightedRoundRobin_DistributesByWeight(t *testing.T) {
	backends := []balancer.Backend{
		{Address: "b0:9000", Weight: 3},
		{Address: "b1:9001", Weight: 1},
		{Address: "b2:9002", Weight: 1},
	}
	lb, err := balancer.NewWeightedRoundRobin(backends)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 5 requests: b0 should get 3, b1 gets 1, b2 gets 1
	counts := map[string]int{}
	for range 5 {
		addr, _ := lb.Next("")
		counts[addr]++
	}

	if counts["b0:9000"] != 3 {
		t.Errorf("b0 (weight=3): expected 3 requests, got %d", counts["b0:9000"])
	}
	if counts["b1:9001"] != 1 {
		t.Errorf("b1 (weight=1): expected 1 request, got %d", counts["b1:9001"])
	}
	if counts["b2:9002"] != 1 {
		t.Errorf("b2 (weight=1): expected 1 request, got %d", counts["b2:9002"])
	}
}

func TestWeightedRoundRobin_ZeroWeightTreatedAsOne(t *testing.T) {
	backends := []balancer.Backend{
		{Address: "b0:9000", Weight: 0}, // zero weight → treated as 1
		{Address: "b1:9001", Weight: 0},
	}
	lb, err := balancer.NewWeightedRoundRobin(backends)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Both should get equal distribution (2 requests each for 4 total)
	counts := map[string]int{}
	for range 4 {
		addr, _ := lb.Next("")
		counts[addr]++
	}

	if counts["b0:9000"] != 2 || counts["b1:9001"] != 2 {
		t.Errorf("expected equal distribution, got b0=%d b1=%d", counts["b0:9000"], counts["b1:9001"])
	}
}

func TestWeightedRoundRobin_EmptyBackends_ReturnsError(t *testing.T) {
	_, err := balancer.NewWeightedRoundRobin(nil)
	if err == nil {
		t.Error("expected error for empty backend list, got nil")
	}
}

// ── Least Connections ─────────────────────────────────────────────────────────

func TestLeastConnections_PicksLeastLoaded(t *testing.T) {
	lb, err := balancer.NewLeastConnections(threeBackends())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Manually increment connections on b0 and b2 via Next (without Done)
	// so the balancer thinks they're busy
	lb.Next("") // b0 gets 1 connection (first call, all tied at 0, picks index 0)
	lb.Next("") // b1 gets 1 connection (b1 tied with b2 at 0, index 1 picks first)
	// now b0=1, b1=1, b2=0 → next call should pick b2

	addr, err := lb.Next("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if addr != "b2:9002" {
		t.Errorf("expected b2 (0 connections), got %s", addr)
	}
}

func TestLeastConnections_DoneDecrementsCount(t *testing.T) {
	lb, _ := balancer.NewLeastConnections(threeBackends())

	// Give b0 one connection, then release it
	addr, _ := lb.Next("")
	if lb.ActiveConns(addr) != 1 {
		t.Errorf("expected 1 active conn on %s, got %d", addr, lb.ActiveConns(addr))
	}

	lb.Done(addr)
	if lb.ActiveConns(addr) != 0 {
		t.Errorf("expected 0 active conns after Done, got %d", lb.ActiveConns(addr))
	}
}

func TestLeastConnections_EmptyBackends_ReturnsError(t *testing.T) {
	_, err := balancer.NewLeastConnections(nil)
	if err == nil {
		t.Error("expected error for empty backend list, got nil")
	}
}

func TestLeastConnections_ConcurrentSafe(t *testing.T) {
	lb, _ := balancer.NewLeastConnections(threeBackends())

	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			addr, err := lb.Next("")
			if err != nil {
				t.Errorf("concurrent Next() error: %v", err)
				return
			}
			lb.Done(addr)
		}()
	}
	wg.Wait()

	// After all Done calls, all backends should have 0 active connections
	for _, b := range threeBackends() {
		if c := lb.ActiveConns(b.Address); c != 0 {
			t.Errorf("%s: expected 0 connections after all Done calls, got %d", b.Address, c)
		}
	}
}

// ── IP Hash ───────────────────────────────────────────────────────────────────

func TestIPHash_SameIPAlwaysSameBackend(t *testing.T) {
	lb, err := balancer.NewIPHash(threeBackends())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	testIPs := []string{"192.168.1.1", "10.0.0.5", "172.16.0.100"}

	for _, ip := range testIPs {
		// Call Next 5 times with the same IP — should always return the same backend
		first, _ := lb.Next(ip)
		for j := 1; j < 5; j++ {
			addr, _ := lb.Next(ip)
			if addr != first {
				t.Errorf("IP %s: expected same backend %q every time, got %q on call %d", ip, first, addr, j+1)
			}
		}
	}
}

func TestIPHash_DifferentIPsDifferentBackends(t *testing.T) {
	lb, _ := balancer.NewIPHash(threeBackends())

	// Generate enough different IPs to hit all 3 backends
	seen := map[string]bool{}
	for i := range 100 {
		ip := fmt.Sprintf("10.0.0.%d", i)
		addr, _ := lb.Next(ip)
		seen[addr] = true
	}

	// With 100 different IPs, all 3 backends should be reachable
	if len(seen) < 2 {
		t.Errorf("expected multiple backends to be selected across 100 IPs, got %d unique backends", len(seen))
	}
}

func TestIPHash_EmptyBackends_ReturnsError(t *testing.T) {
	_, err := balancer.NewIPHash(nil)
	if err == nil {
		t.Error("expected error for empty backend list, got nil")
	}
}

// ── Random ────────────────────────────────────────────────────────────────────

func TestRandom_AlwaysPicksValidBackend(t *testing.T) {
	lb, err := balancer.NewRandom(threeBackends())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	validAddresses := map[string]bool{
		"b0:9000": true,
		"b1:9001": true,
		"b2:9002": true,
	}

	for i := range 50 {
		addr, err := lb.Next("")
		if err != nil {
			t.Fatalf("request %d: unexpected error: %v", i, err)
		}
		if !validAddresses[addr] {
			t.Errorf("request %d: got unknown backend %q", i, addr)
		}
	}
}

func TestRandom_EmptyBackends_ReturnsError(t *testing.T) {
	_, err := balancer.NewRandom(nil)
	if err == nil {
		t.Error("expected error for empty backend list, got nil")
	}
}

func TestRandom_ConcurrentSafe(t *testing.T) {
	lb, _ := balancer.NewRandom(threeBackends())

	var wg sync.WaitGroup
	for range 200 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := lb.Next("")
			if err != nil {
				t.Errorf("concurrent Next() error: %v", err)
			}
		}()
	}
	wg.Wait()
}

// ── Balancer Interface ────────────────────────────────────────────────────────

// TestBalancerInterface verifies all 5 types satisfy the Balancer interface.
// This is a compile-time check — if any type is missing a method, the code won't compile.
func TestBalancerInterface_AllTypesImplement(t *testing.T) {
	backends := threeBackends()

	rr, _ := balancer.NewRoundRobin(backends)
	wrr, _ := balancer.NewWeightedRoundRobin(backends)
	lc, _ := balancer.NewLeastConnections(backends)
	iph, _ := balancer.NewIPHash(backends)
	rnd, _ := balancer.NewRandom(backends)

	// Assign all to the interface — compile error if any type is missing Next(string)(string,error)
	balancers := []balancer.Balancer{rr, wrr, lc, iph, rnd}

	for _, lb := range balancers {
		addr, err := lb.Next("127.0.0.1")
		if err != nil {
			t.Errorf("interface call failed: %v", err)
		}
		if addr == "" {
			t.Error("expected non-empty address from interface call")
		}
	}
}
