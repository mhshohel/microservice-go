// ratelimit.go — Rate limiting middleware using a sliding window algorithm.
//
// What is rate limiting?
// It caps how many requests a single client (identified by IP address) can make
// in a fixed time window. This protects backend services from being overwhelmed
// by a single client — whether that's a bug, abuse, or a DDoS attack.
//
// Example: allow 100 requests per minute per IP.
// The 101st request within the same minute gets a 429 Too Many Requests response.
//
// How the sliding window works:
// Instead of resetting a counter at a fixed clock boundary (e.g., every full minute),
// we keep a list of timestamps for each IP and look back exactly one window-length.
// This avoids the "burst at boundary" problem where a client sends 100 at :59 and 100 at :01.
//
//   window = 1 minute
//   now = 12:01:30
//   windowStart = 12:00:30
//   ─────────────────────────────────────────
//   requests: [12:00:25, 12:00:40, 12:01:10, 12:01:25]
//              ^^^^^^^^                                   ← outside window, ignored
//             kept: [12:00:40, 12:01:10, 12:01:25]  → count = 3

package middleware

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

// RateLimiter holds the request history for all IP addresses.
type RateLimiter struct {
	mu       sync.Mutex             // protects the map — multiple goroutines serve requests concurrently
	requests map[string][]time.Time // IP address → timestamps of recent requests
	limit    int                    // max requests allowed within the window
	window   time.Duration          // how far back to look (e.g., 1 minute)
}

// NewRateLimiter creates a new rate limiter.
//
//	limit  = max requests per window (e.g., 100)
//	window = time period  (e.g., time.Minute)
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
}

// Allow checks whether an IP address is within its rate limit.
// Returns true = request allowed, false = request should be rejected.
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-rl.window) // e.g., one minute ago

	// Keep only the timestamps that fall inside the current window.
	// Timestamps older than windowStart are discarded — they no longer count.
	var recent []time.Time
	for _, t := range rl.requests[ip] {
		if t.After(windowStart) {
			recent = append(recent, t)
		}
	}

	// If we're at or over the limit, reject this request
	if len(recent) >= rl.limit {
		rl.requests[ip] = recent // save the cleaned-up slice
		return false
	}

	// Under the limit — record this request and allow it
	rl.requests[ip] = append(recent, now)
	return true
}

// RateLimit returns an HTTP middleware that enforces rate limits per IP address.
//
// Usage:
//
//	limiter := middleware.NewRateLimiter(100, time.Minute)
//	protected := middleware.RateLimit(limiter)(myHandler)
func RateLimit(limiter *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)

			if !limiter.Allow(ip) {
				// 429 = Too Many Requests
				// Retry-After tells the client how long to wait before trying again
				w.Header().Set("Retry-After", "60")
				http.Error(w,
					"rate limit exceeded — too many requests, please wait before trying again",
					http.StatusTooManyRequests,
				)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// clientIP extracts the client's IP address from the request.
// It checks X-Forwarded-For first (set by load balancers and reverse proxies)
// and falls back to the direct connection address.
func clientIP(r *http.Request) string {
	// X-Forwarded-For can contain a comma-separated list; the first entry is the real client
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		return strings.TrimSpace(strings.Split(forwarded, ",")[0])
	}
	// r.RemoteAddr is "ip:port" — strip the port
	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		return ip[:idx]
	}
	return ip
}
