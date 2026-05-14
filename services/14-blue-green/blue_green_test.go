// blue_green_test.go — Tests for blue-green deployment.

package blue_green_test

import (
	"net/http"
	"testing"

	"microservices-go/services/14-blue-green/internal/env"
	"microservices-go/services/14-blue-green/internal/router"
)

func newTestSetup(t *testing.T) (*env.Environment, *env.Environment, *router.Router) {
	t.Helper()
	blue := env.New(env.Blue, "v1")
	green := env.New(env.Green, "v1-idle")
	t.Cleanup(func() { blue.Close(); green.Close() })
	rt := router.New(blue, green)
	return blue, green, rt
}

// ── Initial State Tests ───────────────────────────────────────────────────────

func TestRouter_InitialLiveEnvironment_IsBlue(t *testing.T) {
	_, _, rt := newTestSetup(t)

	if rt.LiveName() != env.Blue {
		t.Errorf("expected Blue to be live initially, got %s", rt.LiveName())
	}
}

func TestRouter_BlueIsLive_GreenIsIdle(t *testing.T) {
	blue, green, _ := newTestSetup(t)

	if !blue.IsLive() {
		t.Error("blue should be live")
	}
	if green.IsLive() {
		t.Error("green should be idle initially")
	}
}

// ── Traffic Forwarding Tests ──────────────────────────────────────────────────

func TestRouter_Forward_ReachesLiveEnvironment(t *testing.T) {
	blue, _, rt := newTestSetup(t)

	resp, err := http.Get(rt.LiveURL() + "/health")
	if err != nil {
		t.Fatalf("request to live env failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 from live env, got %d", resp.StatusCode)
	}
	_ = blue // used to set up the test
}

// ── Switch Tests ──────────────────────────────────────────────────────────────

func TestRouter_Switch_ChangesLiveToGreen(t *testing.T) {
	_, green, rt := newTestSetup(t)

	result, err := rt.Switch()
	if err != nil {
		t.Fatalf("switch failed: %v", err)
	}

	if rt.LiveName() != env.Green {
		t.Errorf("expected Green to be live after switch, got %s", rt.LiveName())
	}
	if !green.IsLive() {
		t.Error("green.IsLive() should be true after switch")
	}
	if result.From != env.Blue {
		t.Errorf("expected From=Blue, got %s", result.From)
	}
	if result.To != env.Green {
		t.Errorf("expected To=Green, got %s", result.To)
	}
	if !result.SmokeTests {
		t.Error("expected SmokeTests=true when switch succeeds")
	}
}

func TestRouter_Switch_UpdatesBlueToIdle(t *testing.T) {
	blue, _, rt := newTestSetup(t)

	rt.Switch() //nolint

	if blue.IsLive() {
		t.Error("blue should be idle after switching to green")
	}
}

func TestRouter_Switch_SwitchTwice_ReturnsToBlue(t *testing.T) {
	_, _, rt := newTestSetup(t)

	rt.Switch() //nolint — Blue → Green
	rt.Switch() //nolint — Green → Blue

	if rt.LiveName() != env.Blue {
		t.Errorf("expected Blue to be live after two switches, got %s", rt.LiveName())
	}
}

func TestRouter_Switch_SmokeTestRun(t *testing.T) {
	_, _, rt := newTestSetup(t)

	result, err := rt.Switch()
	if err != nil {
		t.Fatalf("switch failed: %v", err)
	}
	if !result.SmokeTests {
		t.Error("expected smoke tests to run before switch")
	}
}

// ── Rollback Tests ────────────────────────────────────────────────────────────

func TestRouter_Rollback_SwitchesBackToPrevious(t *testing.T) {
	_, _, rt := newTestSetup(t)

	rt.Switch() //nolint — Blue → Green

	result := rt.Rollback() // Green → Blue

	if rt.LiveName() != env.Blue {
		t.Errorf("expected Blue after rollback, got %s", rt.LiveName())
	}
	if result.From != env.Green {
		t.Errorf("expected From=Green, got %s", result.From)
	}
	if result.To != env.Blue {
		t.Errorf("expected To=Blue, got %s", result.To)
	}
}

func TestRouter_Rollback_SkipsSmokeTests(t *testing.T) {
	_, _, rt := newTestSetup(t)
	rt.Switch() //nolint

	result := rt.Rollback()

	// Rollback is an emergency action — no time for smoke tests
	if result.SmokeTests {
		t.Error("rollback should skip smoke tests (it's an emergency action)")
	}
}

// ── Version Tracking Tests ────────────────────────────────────────────────────

func TestRouter_LiveVersion_ReflectsDeployedVersion(t *testing.T) {
	_, green, rt := newTestSetup(t)

	// Deploy v2 to green
	green.Deploy("v2")

	// Still on blue (v1)
	if rt.LiveVersion() != "v1" {
		t.Errorf("expected live version v1 before switch, got %q", rt.LiveVersion())
	}

	// Switch to green
	rt.Switch() //nolint

	if rt.LiveVersion() != "v2" {
		t.Errorf("expected live version v2 after switching to green, got %q", rt.LiveVersion())
	}
}

// ── Full Flow Test ────────────────────────────────────────────────────────────

func TestBlueGreen_FullDeploymentFlow(t *testing.T) {
	blue, green, rt := newTestSetup(t)

	// 1. Blue is live at v1
	if rt.LiveName() != env.Blue || rt.LiveVersion() != "v1" {
		t.Fatalf("step 1: expected blue/v1, got %s/%s", rt.LiveName(), rt.LiveVersion())
	}

	// 2. Deploy v2 to idle (green)
	green.Deploy("v2")

	// 3. Switch traffic to green (smoke tests pass)
	if _, err := rt.Switch(); err != nil {
		t.Fatalf("step 3: switch failed: %v", err)
	}
	if rt.LiveName() != env.Green || rt.LiveVersion() != "v2" {
		t.Fatalf("step 3: expected green/v2, got %s/%s", rt.LiveName(), rt.LiveVersion())
	}

	// 4. Problem detected → rollback
	rt.Rollback()
	if rt.LiveName() != env.Blue {
		t.Fatalf("step 4: expected blue after rollback, got %s", rt.LiveName())
	}

	// 5. Blue (v1) is live again — requests should succeed
	resp, err := http.Get(rt.LiveURL() + "/health")
	if err != nil {
		t.Fatalf("step 5: health check failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("step 5: expected 200, got %d", resp.StatusCode)
	}

	_ = blue
}
