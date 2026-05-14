// sidecar.go — Simulated sidecar proxy for the Service Mesh demo.
//
// In a real service mesh (Istio, Linkerd), a sidecar is a separate binary
// (like Envoy) that runs next to each service. Here we simulate the concept
// as a Go struct that wraps HTTP requests/responses.
//
// The Sidecar provides:
//   1. Identity verification: reject requests that don't identify themselves
//   2. Retries: retry failed calls up to N times
//   3. Traffic splitting: send X% of traffic to a "canary" backend
//   4. Metrics: count requests, successes, and errors
//
// How to think about it:
//   Without sidecar: Client → Service
//   With sidecar:    Client → Sidecar → Service
//                             (all policies enforced here)

package sidecar

import (
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"sync/atomic"
	"time"
)

// Policy holds the configuration for this sidecar instance.
type Policy struct {
	// AllowedIdentities is the list of service names that may call this sidecar.
	// If empty, all callers are allowed.
	AllowedIdentities []string

	// MaxRetries is how many times to retry a failed upstream call.
	// 0 means no retries (fail immediately).
	MaxRetries int

	// RetryDelay is how long to wait between retries.
	RetryDelay time.Duration

	// CanaryBackend is an optional secondary backend URL.
	// When set, CanaryPercent% of traffic is sent there instead of the primary.
	CanaryBackend string
	CanaryPercent int // 0-100: percentage of requests that go to canary
}

// Metrics tracks how many requests passed through this sidecar.
type Metrics struct {
	TotalRequests atomic.Int64
	SuccessCount  atomic.Int64
	ErrorCount    atomic.Int64
	RetryCount    atomic.Int64
	RejectedCount atomic.Int64 // rejected due to identity check
}

// Sidecar wraps outgoing HTTP calls with policies.
type Sidecar struct {
	name    string // this service's identity (sent as X-Service-Identity header)
	policy  Policy
	client  *http.Client
	Metrics Metrics
}

// New creates a sidecar for a service.
//
//	name:   this service's identity (e.g., "order-svc")
//	policy: traffic and security policies
func New(name string, policy Policy) *Sidecar {
	return &Sidecar{
		name:   name,
		policy: policy,
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

// Call makes an HTTP GET to the given URL via this sidecar.
// It enforces retries and traffic splitting.
// Returns the response body as a string, or an error.
func (s *Sidecar) Call(url string) (string, error) {
	s.Metrics.TotalRequests.Add(1)

	// Traffic splitting: maybe route to the canary backend
	targetURL := url
	if s.policy.CanaryBackend != "" && s.policy.CanaryPercent > 0 {
		if rand.IntN(100) < s.policy.CanaryPercent {
			targetURL = s.policy.CanaryBackend
		}
	}

	// Build request with identity header
	req, err := http.NewRequest(http.MethodGet, targetURL, nil)
	if err != nil {
		s.Metrics.ErrorCount.Add(1)
		return "", fmt.Errorf("sidecar: build request: %w", err)
	}
	// Attach this service's identity so the destination sidecar can verify it
	req.Header.Set("X-Service-Identity", s.name)

	// Retry loop
	maxAttempts := s.policy.MaxRetries + 1
	var lastErr error
	for attempt := range maxAttempts {
		if attempt > 0 {
			s.Metrics.RetryCount.Add(1)
			if s.policy.RetryDelay > 0 {
				time.Sleep(s.policy.RetryDelay)
			}
		}

		body, err := s.doRequest(req)
		if err == nil {
			s.Metrics.SuccessCount.Add(1)
			return body, nil
		}
		lastErr = err
	}

	s.Metrics.ErrorCount.Add(1)
	return "", lastErr
}

// doRequest executes the HTTP request and reads the response body.
func (s *Sidecar) doRequest(req *http.Request) (string, error) {
	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("sidecar: call failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("sidecar: read body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("sidecar: upstream error %d: %s", resp.StatusCode, string(body))
	}

	return string(body), nil
}

// InboundMiddleware returns an HTTP middleware that enforces identity verification.
//
// Wrap your service handler with this to make it part of the mesh:
//
//	http.Handle("/", sidecar.InboundMiddleware(myHandler))
//
// The middleware checks the X-Service-Identity header against AllowedIdentities.
// If AllowedIdentities is empty, all callers are permitted.
func (s *Sidecar) InboundMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity := r.Header.Get("X-Service-Identity")

		if len(s.policy.AllowedIdentities) > 0 {
			if !s.isAllowed(identity) {
				s.Metrics.RejectedCount.Add(1)
				http.Error(w,
					fmt.Sprintf("service mesh: identity %q is not allowed to call %q", identity, s.name),
					http.StatusForbidden,
				)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// isAllowed checks if the given identity is in the allowed list.
func (s *Sidecar) isAllowed(identity string) bool {
	for _, allowed := range s.policy.AllowedIdentities {
		if allowed == identity {
			return true
		}
	}
	return false
}
