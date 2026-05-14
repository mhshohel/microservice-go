// router.go — Traffic router for blue-green deployment.
//
// The router knows about two environments (blue and green).
// At any time, one is "live" (receives traffic) and the other is "idle".
//
// The router provides:
//   - Forward: proxy a request to the current live environment
//   - Switch:  run smoke tests on idle, then atomically flip traffic
//   - Rollback: immediately switch back to the previous live environment

package router

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"microservices-go/services/14-blue-green/internal/env"
)

// SwitchResult describes what happened during a traffic switch.
type SwitchResult struct {
	From       env.Name // which environment was live before
	To         env.Name // which environment is live after
	SmokeTests bool     // whether smoke tests were run and passed
}

// Router manages which environment receives traffic and handles switching.
type Router struct {
	mu      sync.RWMutex
	blue    *env.Environment
	green   *env.Environment
	liveEnv env.Name // which environment is currently live
}

// New creates a router with blue as the initial live environment.
func New(blue, green *env.Environment) *Router {
	blue.SetLive(true)
	green.SetLive(false)
	return &Router{
		blue:    blue,
		green:   green,
		liveEnv: env.Blue,
	}
}

// LiveURL returns the URL of the currently live environment.
func (r *Router) LiveURL() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.envByName(r.liveEnv).URL()
}

// LiveName returns the name of the currently live environment.
func (r *Router) LiveName() env.Name {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.liveEnv
}

// LiveVersion returns the app version running in the live environment.
func (r *Router) LiveVersion() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.envByName(r.liveEnv).Version()
}

// Forward proxies the incoming request to the live environment.
// This is the main entry point for production traffic.
func (r *Router) Forward(w http.ResponseWriter, req *http.Request) {
	r.mu.RLock()
	liveURL := r.envByName(r.liveEnv).URL()
	r.mu.RUnlock()

	targetURL := liveURL + req.URL.Path
	if req.URL.RawQuery != "" {
		targetURL += "?" + req.URL.RawQuery
	}

	resp, err := http.Get(targetURL)
	if err != nil {
		http.Error(w, "router: backend unreachable: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy all response headers
	for key, vals := range resp.Header {
		for _, v := range vals {
			w.Header().Add(key, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// Switch runs smoke tests against the idle environment.
// If they pass, atomically flips traffic to it.
// Returns an error if smoke tests fail (traffic stays on current live environment).
func (r *Router) Switch() (*SwitchResult, error) {
	r.mu.Lock()
	idleName := r.idleName()
	idleEnv := r.envByName(idleName)
	r.mu.Unlock()

	// Run smoke tests against the idle environment BEFORE switching
	if err := r.smokeTest(idleEnv.URL()); err != nil {
		return nil, fmt.Errorf("smoke tests failed — keeping traffic on %s: %w", r.liveEnv, err)
	}

	// Smoke tests passed — atomically switch traffic
	r.mu.Lock()
	fromName := r.liveEnv
	r.envByName(r.liveEnv).SetLive(false)
	r.liveEnv = idleName
	r.envByName(r.liveEnv).SetLive(true)
	r.mu.Unlock()

	return &SwitchResult{
		From:       fromName,
		To:         idleName,
		SmokeTests: true,
	}, nil
}

// Rollback immediately switches traffic back to the idle environment
// without running smoke tests. Use when the new version has a problem.
func (r *Router) Rollback() *SwitchResult {
	r.mu.Lock()
	defer r.mu.Unlock()

	fromName := r.liveEnv
	idleName := r.idleName()

	r.envByName(r.liveEnv).SetLive(false)
	r.liveEnv = idleName
	r.envByName(r.liveEnv).SetLive(true)

	return &SwitchResult{
		From:       fromName,
		To:         idleName,
		SmokeTests: false, // rollback skips smoke tests
	}
}

// smokeTest calls the /health endpoint of the given environment URL.
// Returns nil if the health check returns 200 with status="ok".
func (r *Router) smokeTest(baseURL string) error {
	resp, err := http.Get(baseURL + "/health")
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned %d", resp.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return fmt.Errorf("health check response is not valid JSON: %w", err)
	}
	if body["status"] != "ok" {
		return fmt.Errorf("health check returned status=%q (expected 'ok')", body["status"])
	}

	return nil
}

// idleName returns whichever environment is NOT currently live.
func (r *Router) idleName() env.Name {
	if r.liveEnv == env.Blue {
		return env.Green
	}
	return env.Blue
}

// envByName returns the environment with the given name.
func (r *Router) envByName(name env.Name) *env.Environment {
	if name == env.Blue {
		return r.blue
	}
	return r.green
}
