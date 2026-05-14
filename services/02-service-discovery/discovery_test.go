// discovery_test.go — Tests for the Service Discovery registry.
//
// We test two layers:
//   1. Registry logic (unit tests): Register, Deregister, Heartbeat, Query, expiry
//   2. HTTP handlers (integration tests): the full HTTP layer on top of the registry
//
// All tests use httptest so no real ports are bound.
//
// Test coverage:
//   - Register: valid registration returns an instance with a non-empty ID
//   - Register: missing name or address returns an error
//   - Deregister: registered instance is removed; unknown ID returns error
//   - Heartbeat: resets last-seen timer; unknown ID returns error
//   - Query by name: returns the right instances; empty when none registered
//   - GetAll: returns all instances across all services
//   - PickOne: returns a random instance; error when no instances for that name
//   - Expiry: instance missing heartbeat is removed by RemoveExpired
//   - HTTP register: POST /register creates an instance (201)
//   - HTTP deregister: DELETE /deregister/{id} removes it (200); missing id → 404
//   - HTTP heartbeat: PUT /heartbeat/{id} returns ok; unknown → 404
//   - HTTP get by name: GET /services/{name} returns matching instances
//   - HTTP get all: GET /services returns all instances
//   - HTTP health: GET /health returns ok with count

package discovery_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"microservices-go/services/02-service-discovery/internal/api"
	"microservices-go/services/02-service-discovery/internal/registry"
)

// ── Registry Unit Tests ───────────────────────────────────────────────────────

func TestRegistry_Register_Success(t *testing.T) {
	// Arrange
	reg := registry.New(registry.DefaultHeartbeatTimeout)

	// Act
	inst, err := reg.Register(registry.RegisterRequest{
		Name:    "user-svc",
		Address: "localhost:9001",
	})

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if inst.ID == "" {
		t.Error("expected non-empty instance ID")
	}
	if inst.Name != "user-svc" {
		t.Errorf("expected name 'user-svc', got %q", inst.Name)
	}
	if inst.Address != "localhost:9001" {
		t.Errorf("expected address 'localhost:9001', got %q", inst.Address)
	}
	if reg.Count() != 1 {
		t.Errorf("expected 1 instance in registry, got %d", reg.Count())
	}
}

func TestRegistry_Register_ValidationErrors(t *testing.T) {
	reg := registry.New(registry.DefaultHeartbeatTimeout)

	tests := []struct {
		name    string
		request registry.RegisterRequest
	}{
		{
			name:    "missing name",
			request: registry.RegisterRequest{Name: "", Address: "localhost:9001"},
		},
		{
			name:    "missing address",
			request: registry.RegisterRequest{Name: "user-svc", Address: ""},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := reg.Register(tc.request)
			if err == nil {
				t.Errorf("expected error for %s, got nil", tc.name)
			}
		})
	}
}

func TestRegistry_Register_MultipleInstancesSameName(t *testing.T) {
	// Registering the same service name twice should create two separate instances
	reg := registry.New(registry.DefaultHeartbeatTimeout)

	inst1, _ := reg.Register(registry.RegisterRequest{Name: "user-svc", Address: "localhost:9001"})
	inst2, _ := reg.Register(registry.RegisterRequest{Name: "user-svc", Address: "localhost:9011"})

	// IDs must be different
	if inst1.ID == inst2.ID {
		t.Error("two registrations should produce different instance IDs")
	}

	// Both should be found when querying by name
	instances := reg.GetByName("user-svc")
	if len(instances) != 2 {
		t.Errorf("expected 2 instances for 'user-svc', got %d", len(instances))
	}
}

func TestRegistry_Deregister_RemovesInstance(t *testing.T) {
	reg := registry.New(registry.DefaultHeartbeatTimeout)

	inst, _ := reg.Register(registry.RegisterRequest{Name: "user-svc", Address: "localhost:9001"})

	// Deregister should succeed
	err := reg.Deregister(inst.ID)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Registry should be empty now
	if reg.Count() != 0 {
		t.Errorf("expected 0 instances after deregister, got %d", reg.Count())
	}
}

func TestRegistry_Deregister_UnknownID_ReturnsError(t *testing.T) {
	reg := registry.New(registry.DefaultHeartbeatTimeout)

	err := reg.Deregister("nonexistent-id")
	if err == nil {
		t.Error("expected error for unknown instance ID, got nil")
	}
}

func TestRegistry_Heartbeat_ResetsTimer(t *testing.T) {
	reg := registry.New(registry.DefaultHeartbeatTimeout)

	inst, _ := reg.Register(registry.RegisterRequest{Name: "user-svc", Address: "localhost:9001"})

	// Small sleep so LastHeartbeat will change
	time.Sleep(5 * time.Millisecond)

	beforeHeartbeat := inst.LastHeartbeat

	err := reg.Heartbeat(inst.ID)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// We need to re-fetch the instance to see the updated LastHeartbeat.
	// GetByName returns a pointer so the value should be updated.
	instances := reg.GetByName("user-svc")
	if len(instances) == 0 {
		t.Fatal("expected instance to still exist after heartbeat")
	}
	if !instances[0].LastHeartbeat.After(beforeHeartbeat) {
		t.Error("expected LastHeartbeat to be updated after heartbeat call")
	}
}

func TestRegistry_Heartbeat_UnknownID_ReturnsError(t *testing.T) {
	reg := registry.New(registry.DefaultHeartbeatTimeout)

	err := reg.Heartbeat("nonexistent-id")
	if err == nil {
		t.Error("expected error for unknown instance ID, got nil")
	}
}

func TestRegistry_RemoveExpired_RemovesSilentInstances(t *testing.T) {
	// Use a very short timeout (1ms) so the instance expires immediately
	reg := registry.New(1 * time.Millisecond)

	inst, _ := reg.Register(registry.RegisterRequest{Name: "user-svc", Address: "localhost:9001"})
	_ = inst // just to confirm it registered

	// Wait long enough for the heartbeat to expire
	time.Sleep(5 * time.Millisecond)

	removed := reg.RemoveExpired()
	if len(removed) != 1 {
		t.Errorf("expected 1 expired instance to be removed, got %d", len(removed))
	}
	if reg.Count() != 0 {
		t.Errorf("expected 0 instances after expiry cleanup, got %d", reg.Count())
	}
}

func TestRegistry_RemoveExpired_KeepsActiveInstances(t *testing.T) {
	// Short timeout but we'll send a heartbeat before it expires
	reg := registry.New(50 * time.Millisecond)

	inst, _ := reg.Register(registry.RegisterRequest{Name: "user-svc", Address: "localhost:9001"})

	// Send a heartbeat immediately — resets the timer
	_ = reg.Heartbeat(inst.ID)

	// Call RemoveExpired right away — the instance is fresh, should NOT be removed
	removed := reg.RemoveExpired()
	if len(removed) != 0 {
		t.Errorf("expected 0 removed (instance is alive), got %d", len(removed))
	}
	if reg.Count() != 1 {
		t.Errorf("expected instance to still be registered, count = %d", reg.Count())
	}
}

func TestRegistry_PickOne_ReturnsInstance(t *testing.T) {
	reg := registry.New(registry.DefaultHeartbeatTimeout)
	reg.Register(registry.RegisterRequest{Name: "user-svc", Address: "localhost:9001"}) //nolint

	inst, err := reg.PickOne("user-svc")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if inst.Name != "user-svc" {
		t.Errorf("expected name 'user-svc', got %q", inst.Name)
	}
}

func TestRegistry_PickOne_ErrorWhenNoInstances(t *testing.T) {
	reg := registry.New(registry.DefaultHeartbeatTimeout)

	_, err := reg.PickOne("nonexistent-svc")
	if err == nil {
		t.Error("expected error when no instances registered, got nil")
	}
}

func TestRegistry_GetAll_ReturnsAllServices(t *testing.T) {
	reg := registry.New(registry.DefaultHeartbeatTimeout)

	reg.Register(registry.RegisterRequest{Name: "user-svc", Address: "localhost:9001"})    //nolint
	reg.Register(registry.RegisterRequest{Name: "order-svc", Address: "localhost:9002"})   //nolint
	reg.Register(registry.RegisterRequest{Name: "product-svc", Address: "localhost:9003"}) //nolint

	all := reg.GetAll()
	if len(all) != 3 {
		t.Errorf("expected 3 instances across all services, got %d", len(all))
	}
}

// ── HTTP Handler Tests ────────────────────────────────────────────────────────

// newTestServer creates a test HTTP server backed by a fresh registry.
// The returned *httptest.Server is NOT closed automatically — call Close() in your test.
func newTestServer() (*httptest.Server, *registry.Registry) {
	reg := registry.New(registry.DefaultHeartbeatTimeout)
	mux := http.NewServeMux()
	h := api.New(reg)
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	return server, reg
}

func TestHTTP_Health(t *testing.T) {
	srv, _ := newTestServer()
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Errorf("expected status 'ok', got %v", body["status"])
	}
}

func TestHTTP_Register_Success(t *testing.T) {
	srv, _ := newTestServer()
	defer srv.Close()

	body := `{"name":"user-svc","address":"localhost:9001"}`
	resp, err := http.Post(srv.URL+"/register", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected 201 Created, got %d", resp.StatusCode)
	}

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)

	if result["instance_id"] == "" {
		t.Error("expected non-empty instance_id in response")
	}
	if result["name"] != "user-svc" {
		t.Errorf("expected name 'user-svc', got %v", result["name"])
	}
}

func TestHTTP_Register_InvalidJSON(t *testing.T) {
	srv, _ := newTestServer()
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/register", "application/json", bytes.NewBufferString("not json"))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestHTTP_Register_MissingFields(t *testing.T) {
	srv, _ := newTestServer()
	defer srv.Close()

	// Name is missing — should get 400
	body := `{"address":"localhost:9001"}`
	resp, err := http.Post(srv.URL+"/register", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestHTTP_Deregister_Success(t *testing.T) {
	srv, reg := newTestServer()
	defer srv.Close()

	// Register an instance directly through the registry
	inst, _ := reg.Register(registry.RegisterRequest{Name: "user-svc", Address: "localhost:9001"})

	// Deregister it via HTTP
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/deregister/"+inst.ID, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	// Verify it's gone
	if reg.Count() != 0 {
		t.Errorf("expected registry to be empty after deregister, got %d", reg.Count())
	}
}

func TestHTTP_Deregister_UnknownID(t *testing.T) {
	srv, _ := newTestServer()
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/deregister/does-not-exist", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestHTTP_Heartbeat_Success(t *testing.T) {
	srv, reg := newTestServer()
	defer srv.Close()

	inst, _ := reg.Register(registry.RegisterRequest{Name: "user-svc", Address: "localhost:9001"})

	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/heartbeat/"+inst.ID, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestHTTP_Heartbeat_UnknownID(t *testing.T) {
	srv, _ := newTestServer()
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/heartbeat/does-not-exist", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestHTTP_GetByName_ReturnsMatchingInstances(t *testing.T) {
	srv, reg := newTestServer()
	defer srv.Close()

	// Register two user-svc and one order-svc
	reg.Register(registry.RegisterRequest{Name: "user-svc", Address: "localhost:9001"})  //nolint
	reg.Register(registry.RegisterRequest{Name: "user-svc", Address: "localhost:9011"})  //nolint
	reg.Register(registry.RegisterRequest{Name: "order-svc", Address: "localhost:9002"}) //nolint

	resp, err := http.Get(srv.URL + "/services/user-svc")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		Name      string `json:"name"`
		Count     int    `json:"count"`
		Instances []any  `json:"instances"`
	}
	json.NewDecoder(resp.Body).Decode(&body)

	if body.Count != 2 {
		t.Errorf("expected 2 user-svc instances, got %d", body.Count)
	}
	if body.Name != "user-svc" {
		t.Errorf("expected name 'user-svc', got %q", body.Name)
	}
}

func TestHTTP_GetByName_UnknownServiceReturnsEmpty(t *testing.T) {
	srv, _ := newTestServer()
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/services/nonexistent-svc")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	// Should be 200 with count=0, not a 404 — empty is a valid result
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 with empty list, got %d", resp.StatusCode)
	}

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)

	count, ok := body["count"].(float64)
	if !ok || count != 0 {
		t.Errorf("expected count=0 for unknown service, got %v", body["count"])
	}
}

func TestHTTP_GetAll_ReturnsAllInstances(t *testing.T) {
	srv, reg := newTestServer()
	defer srv.Close()

	reg.Register(registry.RegisterRequest{Name: "user-svc", Address: "localhost:9001"})    //nolint
	reg.Register(registry.RegisterRequest{Name: "order-svc", Address: "localhost:9002"})   //nolint
	reg.Register(registry.RegisterRequest{Name: "product-svc", Address: "localhost:9003"}) //nolint

	resp, err := http.Get(srv.URL + "/services")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		Count     int   `json:"count"`
		Instances []any `json:"instances"`
	}
	json.NewDecoder(resp.Body).Decode(&body)

	if body.Count != 3 {
		t.Errorf("expected 3 total instances, got %d", body.Count)
	}
}

// TestHTTP_FullLifecycle exercises the complete register → heartbeat → deregister flow.
func TestHTTP_FullLifecycle(t *testing.T) {
	srv, _ := newTestServer()
	defer srv.Close()

	// Step 1: Register
	body := `{"name":"lifecycle-svc","address":"localhost:9999","metadata":{"version":"1.0"}}`
	resp, err := http.Post(srv.URL+"/register", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("register request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register: expected 201, got %d", resp.StatusCode)
	}

	var registered map[string]any
	json.NewDecoder(resp.Body).Decode(&registered)
	instanceID := fmt.Sprintf("%v", registered["instance_id"])

	// Step 2: Verify it appears in /services/lifecycle-svc
	resp2, _ := http.Get(srv.URL + "/services/lifecycle-svc")
	defer resp2.Body.Close()
	var serviceList struct {
		Count int `json:"count"`
	}
	json.NewDecoder(resp2.Body).Decode(&serviceList)
	if serviceList.Count != 1 {
		t.Errorf("after register: expected 1 instance, got %d", serviceList.Count)
	}

	// Step 3: Heartbeat
	req3, _ := http.NewRequest(http.MethodPut, srv.URL+"/heartbeat/"+instanceID, nil)
	resp3, _ := http.DefaultClient.Do(req3)
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusOK {
		t.Errorf("heartbeat: expected 200, got %d", resp3.StatusCode)
	}

	// Step 4: Deregister
	req4, _ := http.NewRequest(http.MethodDelete, srv.URL+"/deregister/"+instanceID, nil)
	resp4, _ := http.DefaultClient.Do(req4)
	defer resp4.Body.Close()
	if resp4.StatusCode != http.StatusOK {
		t.Errorf("deregister: expected 200, got %d", resp4.StatusCode)
	}

	// Step 5: Verify it's gone
	resp5, _ := http.Get(srv.URL + "/services/lifecycle-svc")
	defer resp5.Body.Close()
	var afterDeregister struct {
		Count int `json:"count"`
	}
	json.NewDecoder(resp5.Body).Decode(&afterDeregister)
	if afterDeregister.Count != 0 {
		t.Errorf("after deregister: expected 0 instances, got %d", afterDeregister.Count)
	}
}
