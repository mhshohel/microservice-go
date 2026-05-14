// strangler_fig_test.go — Tests for the Strangler Fig routing pattern.

package strangler_fig_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"microservices-go/services/15-strangler-fig/internal/legacy"
	"microservices-go/services/15-strangler-fig/internal/modern"
	"microservices-go/services/15-strangler-fig/internal/proxy"
)

// newTestEnv creates legacy and modern test servers and a strangler router.
func newTestEnv(t *testing.T) (legacySrv, modernSrv *httptest.Server, rt *proxy.StranglerRouter) {
	t.Helper()
	legacySrv = httptest.NewServer(legacy.Handler())
	modernSrv = httptest.NewServer(modern.Handler())
	t.Cleanup(func() {
		legacySrv.Close()
		modernSrv.Close()
	})
	rt = proxy.New(legacySrv.URL)
	return
}

// getSource calls the given URL path through the router and returns the "source" field.
func getSource(t *testing.T, routerURL, path string) string {
	t.Helper()
	resp, err := http.Get(routerURL + path)
	if err != nil {
		t.Fatalf("request to %s failed: %v", path, err)
	}
	defer resp.Body.Close()

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return body["source"]
}

// ── Initial State Tests ───────────────────────────────────────────────────────

func TestStranglerFig_InitialState_AllTrafficToLegacy(t *testing.T) {
	_, _, rt := newTestEnv(t)
	routerSrv := httptest.NewServer(rt.Handler())
	defer routerSrv.Close()

	paths := []string{"/users/alice", "/products/laptop", "/orders/123", "/anything"}
	for _, path := range paths {
		source := getSource(t, routerSrv.URL, path)
		if source != "legacy" {
			t.Errorf("path %q: expected 'legacy' initially, got %q", path, source)
		}
	}
}

func TestStranglerFig_InitialMigratedCount_IsZero(t *testing.T) {
	_, _, rt := newTestEnv(t)

	if rt.MigratedCount() != 0 {
		t.Errorf("expected 0 migrated routes initially, got %d", rt.MigratedCount())
	}
}

// ── Migration Tests ───────────────────────────────────────────────────────────

func TestStranglerFig_Migrate_RoutesMigratedPathToModern(t *testing.T) {
	_, modernSrv, rt := newTestEnv(t)
	routerSrv := httptest.NewServer(rt.Handler())
	defer routerSrv.Close()

	rt.Migrate("/users", modernSrv.URL)

	source := getSource(t, routerSrv.URL, "/users/alice")
	if source != "modern" {
		t.Errorf("/users/alice should go to modern after migration, got %q", source)
	}
}

func TestStranglerFig_Migrate_UnmigratedPathStaysLegacy(t *testing.T) {
	_, modernSrv, rt := newTestEnv(t)
	routerSrv := httptest.NewServer(rt.Handler())
	defer routerSrv.Close()

	rt.Migrate("/users", modernSrv.URL) // only users migrated

	source := getSource(t, routerSrv.URL, "/products/laptop")
	if source != "legacy" {
		t.Errorf("/products should still go to legacy, got %q", source)
	}
}

func TestStranglerFig_Migrate_SubpathsMatch(t *testing.T) {
	_, modernSrv, rt := newTestEnv(t)
	routerSrv := httptest.NewServer(rt.Handler())
	defer routerSrv.Close()

	rt.Migrate("/users", modernSrv.URL)

	subPaths := []string{"/users", "/users/123", "/users/alice/profile"}
	for _, path := range subPaths {
		source := getSource(t, routerSrv.URL, path)
		if source != "modern" {
			t.Errorf("sub-path %q should match /users migration, got %q", path, source)
		}
	}
}

func TestStranglerFig_MultipleMigrations(t *testing.T) {
	_, modernSrv, rt := newTestEnv(t)
	routerSrv := httptest.NewServer(rt.Handler())
	defer routerSrv.Close()

	rt.Migrate("/users", modernSrv.URL)
	rt.Migrate("/products", modernSrv.URL)

	if rt.MigratedCount() != 2 {
		t.Errorf("expected 2 migrated routes, got %d", rt.MigratedCount())
	}

	if getSource(t, routerSrv.URL, "/users/x") != "modern" {
		t.Error("/users should be modern")
	}
	if getSource(t, routerSrv.URL, "/products/y") != "modern" {
		t.Error("/products should be modern")
	}
	if getSource(t, routerSrv.URL, "/orders/z") != "legacy" {
		t.Error("/orders should still be legacy")
	}
}

// ── Rollback Tests ────────────────────────────────────────────────────────────

func TestStranglerFig_Rollback_SendsTrafficBackToLegacy(t *testing.T) {
	_, modernSrv, rt := newTestEnv(t)
	routerSrv := httptest.NewServer(rt.Handler())
	defer routerSrv.Close()

	rt.Migrate("/users", modernSrv.URL)

	// Verify it's on modern
	if getSource(t, routerSrv.URL, "/users/x") != "modern" {
		t.Fatal("setup: expected modern before rollback")
	}

	// Rollback
	if err := rt.Rollback("/users"); err != nil {
		t.Fatalf("rollback error: %v", err)
	}

	// Should be back on legacy
	source := getSource(t, routerSrv.URL, "/users/x")
	if source != "legacy" {
		t.Errorf("after rollback, /users should go to legacy, got %q", source)
	}
}

func TestStranglerFig_Rollback_UnknownPrefix_ReturnsError(t *testing.T) {
	_, _, rt := newTestEnv(t)

	err := rt.Rollback("/not-migrated")
	if err == nil {
		t.Error("expected error for rolling back non-migrated route, got nil")
	}
}

func TestStranglerFig_Rollback_DecrementsMigratedCount(t *testing.T) {
	_, modernSrv, rt := newTestEnv(t)

	rt.Migrate("/users", modernSrv.URL)
	rt.Migrate("/products", modernSrv.URL)

	rt.Rollback("/users") //nolint

	if rt.MigratedCount() != 1 {
		t.Errorf("expected 1 migrated route after rollback, got %d", rt.MigratedCount())
	}
}

// ── Routes Snapshot Test ──────────────────────────────────────────────────────

func TestStranglerFig_Routes_ReturnsCorrectEntries(t *testing.T) {
	_, modernSrv, rt := newTestEnv(t)

	rt.Migrate("/users", modernSrv.URL)
	rt.Migrate("/products", modernSrv.URL)

	routes := rt.Routes()
	if len(routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(routes))
	}

	prefixes := map[string]bool{}
	for _, r := range routes {
		prefixes[r.Prefix] = true
		if !r.Migrated {
			t.Errorf("route %q should be marked as migrated", r.Prefix)
		}
	}

	if !prefixes["/users"] {
		t.Error("expected /users in routes")
	}
	if !prefixes["/products"] {
		t.Error("expected /products in routes")
	}
}

// ── Full Migration Flow Test ──────────────────────────────────────────────────

func TestStranglerFig_FullMigrationFlow(t *testing.T) {
	_, modernSrv, rt := newTestEnv(t)
	routerSrv := httptest.NewServer(rt.Handler())
	defer routerSrv.Close()

	// Phase 1: all legacy
	for _, path := range []string{"/users/x", "/products/y", "/orders/z"} {
		if getSource(t, routerSrv.URL, path) != "legacy" {
			t.Errorf("phase 1: %s should be legacy", path)
		}
	}

	// Phase 2: migrate /users
	rt.Migrate("/users", modernSrv.URL)
	if getSource(t, routerSrv.URL, "/users/x") != "modern" {
		t.Error("phase 2: /users should be modern")
	}
	if getSource(t, routerSrv.URL, "/products/y") != "legacy" {
		t.Error("phase 2: /products should still be legacy")
	}

	// Phase 3: migrate /products
	rt.Migrate("/products", modernSrv.URL)
	if getSource(t, routerSrv.URL, "/products/y") != "modern" {
		t.Error("phase 3: /products should be modern")
	}
	if getSource(t, routerSrv.URL, "/orders/z") != "legacy" {
		t.Error("phase 3: /orders should still be legacy")
	}

	// Phase 4: rollback /users (bug)
	rt.Rollback("/users") //nolint
	if getSource(t, routerSrv.URL, "/users/x") != "legacy" {
		t.Error("phase 4: /users should be legacy after rollback")
	}
	if getSource(t, routerSrv.URL, "/products/y") != "modern" {
		t.Error("phase 4: /products should still be modern")
	}
}
