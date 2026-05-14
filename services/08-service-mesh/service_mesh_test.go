// service_mesh_test.go — Tests for the sidecar proxy.

package service_mesh_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"microservices-go/services/08-service-mesh/internal/sidecar"
)

// ── Identity Verification Tests ───────────────────────────────────────────────

func TestSidecar_AllowedIdentity_Passes(t *testing.T) {
	sc := sidecar.New("user-svc", sidecar.Policy{
		AllowedIdentities: []string{"order-svc"},
	})

	backend := httptest.NewServer(sc.InboundMiddleware(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("ok"))
		}),
	))
	defer backend.Close()

	caller := sidecar.New("order-svc", sidecar.Policy{})
	body, err := caller.Call(backend.URL)
	if err != nil {
		t.Fatalf("expected allowed call to succeed, got: %v", err)
	}
	if body != "ok" {
		t.Errorf("expected body 'ok', got %q", body)
	}
}

func TestSidecar_UnknownIdentity_Rejected(t *testing.T) {
	sc := sidecar.New("user-svc", sidecar.Policy{
		AllowedIdentities: []string{"order-svc"},
	})

	backend := httptest.NewServer(sc.InboundMiddleware(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("secret data"))
		}),
	))
	defer backend.Close()

	caller := sidecar.New("unknown-svc", sidecar.Policy{})
	_, err := caller.Call(backend.URL)
	if err == nil {
		t.Error("expected rejection error for unknown identity, got nil")
	}

	// Rejected count should be 1
	if sc.Metrics.RejectedCount.Load() != 1 {
		t.Errorf("expected 1 rejected request, got %d", sc.Metrics.RejectedCount.Load())
	}
}

func TestSidecar_EmptyAllowedList_AllowsAll(t *testing.T) {
	sc := sidecar.New("user-svc", sidecar.Policy{
		AllowedIdentities: nil, // empty = allow all
	})

	backend := httptest.NewServer(sc.InboundMiddleware(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("ok"))
		}),
	))
	defer backend.Close()

	caller := sidecar.New("any-svc", sidecar.Policy{})
	_, err := caller.Call(backend.URL)
	if err != nil {
		t.Fatalf("expected empty allow list to permit all, got: %v", err)
	}
}

func TestSidecar_MultipleAllowedIdentities(t *testing.T) {
	sc := sidecar.New("user-svc", sidecar.Policy{
		AllowedIdentities: []string{"order-svc", "payment-svc", "admin-svc"},
	})

	backend := httptest.NewServer(sc.InboundMiddleware(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("ok"))
		}),
	))
	defer backend.Close()

	for _, callerName := range []string{"order-svc", "payment-svc", "admin-svc"} {
		caller := sidecar.New(callerName, sidecar.Policy{})
		_, err := caller.Call(backend.URL)
		if err != nil {
			t.Errorf("caller %q should be allowed, got error: %v", callerName, err)
		}
	}
}

// ── Retry Tests ───────────────────────────────────────────────────────────────

func TestSidecar_NoRetries_FailsImmediately(t *testing.T) {
	failCount := atomic.Int32{}
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		failCount.Add(1)
		http.Error(w, "error", http.StatusInternalServerError)
	}))
	defer backend.Close()

	sc := sidecar.New("caller", sidecar.Policy{MaxRetries: 0})
	_, err := sc.Call(backend.URL)

	if err == nil {
		t.Error("expected error with no retries")
	}
	if failCount.Load() != 1 {
		t.Errorf("expected 1 attempt (no retries), got %d", failCount.Load())
	}
}

func TestSidecar_Retries_EventuallySucceeds(t *testing.T) {
	attempts := atomic.Int32{}
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n < 3 { // fail first 2 times
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte("success"))
	}))
	defer backend.Close()

	sc := sidecar.New("caller", sidecar.Policy{MaxRetries: 3, RetryDelay: 1 * time.Millisecond})
	body, err := sc.Call(backend.URL)

	if err != nil {
		t.Fatalf("expected eventual success after retries, got: %v", err)
	}
	if body != "success" {
		t.Errorf("expected 'success', got %q", body)
	}
	if sc.Metrics.RetryCount.Load() != 2 {
		t.Errorf("expected 2 retries, got %d", sc.Metrics.RetryCount.Load())
	}
}

func TestSidecar_ExhaustsRetries_ReturnsError(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "always fails", http.StatusInternalServerError)
	}))
	defer backend.Close()

	sc := sidecar.New("caller", sidecar.Policy{MaxRetries: 2, RetryDelay: 1 * time.Millisecond})
	_, err := sc.Call(backend.URL)

	if err == nil {
		t.Error("expected error after exhausting retries")
	}
	if sc.Metrics.RetryCount.Load() != 2 {
		t.Errorf("expected 2 retries (max=2), got %d", sc.Metrics.RetryCount.Load())
	}
}

// ── Traffic Splitting Tests ───────────────────────────────────────────────────

func TestSidecar_TrafficSplit_RoutesSomeToCanary(t *testing.T) {
	stableHits := atomic.Int32{}
	stable := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stableHits.Add(1)
		w.Write([]byte("stable"))
	}))
	defer stable.Close()

	canaryHits := atomic.Int32{}
	canary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		canaryHits.Add(1)
		w.Write([]byte("canary"))
	}))
	defer canary.Close()

	sc := sidecar.New("api-svc", sidecar.Policy{
		CanaryBackend: canary.URL,
		CanaryPercent: 50, // 50% to canary
	})

	for range 200 {
		sc.Call(stable.URL) //nolint
	}

	// With 50% split and 200 requests, both should have gotten some traffic.
	// We use a loose check (> 20 each) to avoid flakiness.
	if stableHits.Load() < 20 {
		t.Errorf("stable should get roughly 50%% — got %d out of 200", stableHits.Load())
	}
	if canaryHits.Load() < 20 {
		t.Errorf("canary should get roughly 50%% — got %d out of 200", canaryHits.Load())
	}
	total := stableHits.Load() + canaryHits.Load()
	if total != 200 {
		t.Errorf("all 200 requests should have been handled (stable+canary), got %d", total)
	}
}

func TestSidecar_NoCanary_AllTrafficGoesToPrimary(t *testing.T) {
	primaryHits := atomic.Int32{}
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryHits.Add(1)
		w.Write([]byte("primary"))
	}))
	defer primary.Close()

	sc := sidecar.New("api-svc", sidecar.Policy{
		CanaryBackend: "", // no canary
		CanaryPercent: 0,
	})

	for range 50 {
		sc.Call(primary.URL) //nolint
	}

	if primaryHits.Load() != 50 {
		t.Errorf("without canary, all 50 requests should hit primary, got %d", primaryHits.Load())
	}
}

// ── Metrics Tests ─────────────────────────────────────────────────────────────

func TestSidecar_Metrics_Tracked(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	defer backend.Close()

	sc := sidecar.New("caller", sidecar.Policy{})
	for range 5 {
		sc.Call(backend.URL) //nolint
	}

	if sc.Metrics.TotalRequests.Load() != 5 {
		t.Errorf("expected 5 total requests, got %d", sc.Metrics.TotalRequests.Load())
	}
	if sc.Metrics.SuccessCount.Load() != 5 {
		t.Errorf("expected 5 successes, got %d", sc.Metrics.SuccessCount.Load())
	}
	if sc.Metrics.ErrorCount.Load() != 0 {
		t.Errorf("expected 0 errors, got %d", sc.Metrics.ErrorCount.Load())
	}
}
