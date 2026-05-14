// gateway_test.go — Tests for the API Gateway.
//
// We use Go's httptest package to test HTTP handlers without starting real servers.
// httptest.NewRecorder() captures what the handler writes as the response.
// httptest.NewRequest() builds a fake HTTP request.
//
// Test coverage:
//   - JWT: generate → validate, invalid signature, malformed token
//   - Auth middleware: missing token, bad token, valid token
//   - Rate limiter: limit is enforced, different IPs have separate limits
//   - Path router: correct backend selected per path, unknown path returns error
//   - Header router: correct backend selected per header value, missing header
//   - Full gateway: health endpoint accessible without auth, routing with live backends

package gateway_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"microservices-go/services/01-api-gateway/internal/jwt"
	"microservices-go/services/01-api-gateway/internal/middleware"
	"microservices-go/services/01-api-gateway/internal/proxy"
	"microservices-go/services/01-api-gateway/internal/router"
)

// ── Helpers ──────────────────────────────────────────────────────────────────

// newTestGateway builds a gateway handler wired up to the given backend servers.
// We pass httptest.Server URLs so no real ports are needed.
func newTestGateway(usersURL, productsURL, ordersURL string, rateLimit int) http.Handler {
	routes := []router.Route{
		{Prefix: "/api/users", Backend: usersURL},
		{Prefix: "/api/products", Backend: productsURL},
		{Prefix: "/api/orders", Backend: ordersURL},
	}
	pathRouter := router.NewPathRouter(routes)
	rateLimiter := middleware.NewRateLimiter(rateLimit, time.Minute)

	mux := http.NewServeMux()

	// Health check — public, no auth
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Core handler: route → strip /api prefix → proxy to backend
	coreHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backendURL, err := pathRouter.Route(r)
		if err != nil {
			http.Error(w, "not found: "+err.Error(), http.StatusNotFound)
			return
		}
		if len(r.URL.Path) >= 4 && r.URL.Path[:4] == "/api" {
			r.URL.Path = r.URL.Path[4:]
			if r.URL.Path == "" {
				r.URL.Path = "/"
			}
		}
		proxy.Forward(w, r, backendURL)
	})

	withAuth := middleware.Auth(coreHandler)
	withRateLimit := middleware.RateLimit(rateLimiter)(withAuth)
	mux.Handle("/api/", withRateLimit)

	return mux
}

// fakeBacked starts an httptest.Server that always returns 200 with the given JSON body.
func fakeBackend(responseBody string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(responseBody))
	}))
}

// ── JWT Tests ─────────────────────────────────────────────────────────────────

func TestJWT_GenerateAndValidate(t *testing.T) {
	// Arrange: generate a token for a known user
	token := jwt.Generate("user-99", "admin")

	// Act: validate it
	claims, err := jwt.Validate(token)

	// Assert: should succeed with the right claims
	if err != nil {
		t.Fatalf("expected valid token but got error: %v", err)
	}
	if claims.UserID != "user-99" {
		t.Errorf("expected UserID 'user-99', got %q", claims.UserID)
	}
	if claims.Role != "admin" {
		t.Errorf("expected Role 'admin', got %q", claims.Role)
	}
}

func TestJWT_TamperedSignature(t *testing.T) {
	// Arrange: generate a valid token then change the last character of the signature
	token := jwt.Generate("user-1", "customer")
	tampered := token[:len(token)-1] + "X" // flip the last character

	// Act + Assert: validation must fail
	_, err := jwt.Validate(tampered)
	if err == nil {
		t.Error("expected error for tampered token but got nil")
	}
}

func TestJWT_MalformedTokens(t *testing.T) {
	// Each of these is structurally invalid — they don't have three dot-separated parts
	badTokens := []string{
		"",                   // empty string
		"nodots",             // no dots at all
		"only.two",           // only two parts
		"too.many.dots.here", // four parts
	}

	for _, bad := range badTokens {
		_, err := jwt.Validate(bad)
		if err == nil {
			t.Errorf("expected error for malformed token %q, but validation passed", bad)
		}
	}
}

// ── Auth Middleware Tests ─────────────────────────────────────────────────────

func TestAuth_MissingHeader_Returns401(t *testing.T) {
	// Arrange: gateway with no actual backend needed (auth will reject before routing)
	gateway := newTestGateway("", "", "", 100)

	// Act: send a request with NO Authorization header
	req := httptest.NewRequest("GET", "/api/users", nil)
	rec := httptest.NewRecorder()
	gateway.ServeHTTP(rec, req)

	// Assert: must be 401 Unauthorized
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("missing token: expected 401, got %d", rec.Code)
	}
}

func TestAuth_WrongHeaderFormat_Returns401(t *testing.T) {
	gateway := newTestGateway("", "", "", 100)

	// Send the token without the "Bearer " prefix
	req := httptest.NewRequest("GET", "/api/users", nil)
	req.Header.Set("Authorization", "Token abc123") // wrong scheme
	rec := httptest.NewRecorder()
	gateway.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("wrong header format: expected 401, got %d", rec.Code)
	}
}

func TestAuth_InvalidToken_Returns401(t *testing.T) {
	gateway := newTestGateway("", "", "", 100)

	req := httptest.NewRequest("GET", "/api/users", nil)
	req.Header.Set("Authorization", "Bearer this.is.not.valid")
	rec := httptest.NewRecorder()
	gateway.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("invalid token: expected 401, got %d", rec.Code)
	}
}

func TestAuth_ValidToken_PassesThrough(t *testing.T) {
	// Arrange: a real fake backend so the proxy has somewhere to forward to
	backend := fakeBackend(`[{"id":"1","name":"Alice"}]`)
	defer backend.Close()

	gateway := newTestGateway(backend.URL, backend.URL, backend.URL, 100)

	// Generate a real valid token
	token := jwt.Generate("user-1", "customer")

	req := httptest.NewRequest("GET", "/api/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	gateway.ServeHTTP(rec, req)

	// Should NOT be 401 — auth passed
	if rec.Code == http.StatusUnauthorized {
		t.Errorf("valid token should pass auth but got 401")
	}
	// Should be a success response from the fake backend
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 from backend, got %d — body: %s", rec.Code, rec.Body.String())
	}
}

// ── Health Endpoint Test ──────────────────────────────────────────────────────

func TestHealth_NoAuthRequired(t *testing.T) {
	gateway := newTestGateway("", "", "", 100)

	// Send request to /health with NO token
	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	gateway.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("health endpoint: expected 200, got %d", rec.Code)
	}

	// Verify the response body is valid JSON with status: ok
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("health response is not valid JSON: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("health response status: expected 'ok', got %q", body["status"])
	}
}

// ── Rate Limiter Tests ────────────────────────────────────────────────────────

func TestRateLimit_EnforcesLimit(t *testing.T) {
	// Arrange: a limiter that allows only 3 requests per minute
	limiter := middleware.NewRateLimiter(3, time.Minute)
	handler := middleware.RateLimit(limiter)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	ip := "10.0.0.1:1234"

	// Act: send 3 requests — all should succeed
	for i := range 3 {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = ip
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i+1, rec.Code)
		}
	}

	// 4th request must be rate-limited
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = ip
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("4th request: expected 429 Too Many Requests, got %d", rec.Code)
	}
}

func TestRateLimit_SeparateLimitsPerIP(t *testing.T) {
	// Two different IPs must have independent counters
	limiter := middleware.NewRateLimiter(2, time.Minute)
	handler := middleware.RateLimit(limiter)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Use up the limit for IP 1
	for range 2 {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "192.168.1.1:5000"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	// IP 2 should still be allowed (fresh counter)
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.2:5000" // different IP
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("different IP should not be rate-limited, but got %d", rec.Code)
	}
}

// ── Path Router Tests ─────────────────────────────────────────────────────────

func TestPathRouter_RoutesCorrectly(t *testing.T) {
	pathRouter := router.NewPathRouter([]router.Route{
		{Prefix: "/api/users", Backend: "http://users:9001"},
		{Prefix: "/api/products", Backend: "http://products:9002"},
		{Prefix: "/api/orders", Backend: "http://orders:9003"},
	})

	tests := []struct {
		path            string
		expectedBackend string
		expectError     bool
	}{
		{"/api/users", "http://users:9001", false},
		{"/api/users/123", "http://users:9001", false}, // sub-path still matches
		{"/api/products", "http://products:9002", false},
		{"/api/products/p1", "http://products:9002", false},
		{"/api/orders", "http://orders:9003", false},
		{"/api/orders/o99", "http://orders:9003", false},
		{"/api/unknown", "", true}, // no route → error
		{"/health", "", true},      // not an /api route
	}

	for _, tc := range tests {
		req := httptest.NewRequest("GET", tc.path, nil)
		got, err := pathRouter.Route(req)

		if tc.expectError {
			if err == nil {
				t.Errorf("path %q: expected error but got backend %q", tc.path, got)
			}
		} else {
			if err != nil {
				t.Errorf("path %q: unexpected error: %v", tc.path, err)
			}
			if got != tc.expectedBackend {
				t.Errorf("path %q: expected %q, got %q", tc.path, tc.expectedBackend, got)
			}
		}
	}
}

// ── Header Router Tests ───────────────────────────────────────────────────────

func TestHeaderRouter_RoutesCorrectly(t *testing.T) {
	headerRouter := router.NewHeaderRouter("X-Service", []router.HeaderRoute{
		{HeaderValue: "users", Backend: "http://users:9001"},
		{HeaderValue: "products", Backend: "http://products:9002"},
		{HeaderValue: "orders", Backend: "http://orders:9003"},
	})

	tests := []struct {
		headerValue     string
		expectedBackend string
		expectError     bool
	}{
		{"users", "http://users:9001", false},
		{"products", "http://products:9002", false},
		{"orders", "http://orders:9003", false},
		{"unknown", "", true}, // no matching route
		{"", "", true},        // missing header
	}

	for _, tc := range tests {
		req := httptest.NewRequest("GET", "/api/data", nil)
		if tc.headerValue != "" {
			req.Header.Set("X-Service", tc.headerValue)
		}

		got, err := headerRouter.Route(req)

		if tc.expectError {
			if err == nil {
				t.Errorf("header %q: expected error but got backend %q", tc.headerValue, got)
			}
		} else {
			if err != nil {
				t.Errorf("header %q: unexpected error: %v", tc.headerValue, err)
			}
			if got != tc.expectedBackend {
				t.Errorf("header %q: expected %q, got %q", tc.headerValue, tc.expectedBackend, got)
			}
		}
	}
}

// ── Full Gateway Integration Test ─────────────────────────────────────────────

func TestGateway_RoutesToCorrectBackend(t *testing.T) {
	// Arrange: three fake backends that echo which service they are
	usersBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"service":"users"}`))
	}))
	defer usersBackend.Close()

	productsBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"service":"products"}`))
	}))
	defer productsBackend.Close()

	gateway := newTestGateway(usersBackend.URL, productsBackend.URL, "", 100)
	token := jwt.Generate("user-1", "customer")
	authHeader := "Bearer " + token

	tests := []struct {
		path            string
		wantBodyContain string
	}{
		{"/api/users", `"service":"users"`},
		{"/api/products", `"service":"products"`},
	}

	for _, tc := range tests {
		req := httptest.NewRequest("GET", tc.path, nil)
		req.Header.Set("Authorization", authHeader)
		rec := httptest.NewRecorder()
		gateway.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("path %q: expected 200, got %d — body: %s", tc.path, rec.Code, rec.Body.String())
			continue
		}
		body := rec.Body.String()
		if !containsString(body, tc.wantBodyContain) {
			t.Errorf("path %q: expected body to contain %q, got: %s", tc.path, tc.wantBodyContain, body)
		}
	}
}

func TestGateway_UnknownPath_Returns404(t *testing.T) {
	gateway := newTestGateway("", "", "", 100)
	token := jwt.Generate("user-1", "customer")

	req := httptest.NewRequest("GET", "/api/unknown-service", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	gateway.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown path: expected 404, got %d", rec.Code)
	}
}

// containsString is a simple substring check used in assertions.
func containsString(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := range len(s) - len(sub) + 1 {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
