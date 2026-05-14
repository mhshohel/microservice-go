// containerization_test.go — Tests for the Containerization demo HTTP handlers.
//
// These tests verify the HTTP handler behaviour without starting a real server.
// We use httptest.NewRecorder (a fake http.ResponseWriter) and httptest.NewRequest
// to call the handler functions directly — fast, no ports, no network needed.
//
// The handlers live in internal/handlers so they can be imported here.

package containerization_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"microservices-go/services/10-containerization/internal/handlers"
)

// TestHealth_Returns200 verifies that GET /health responds with HTTP 200.
func TestHealth_Returns200(t *testing.T) {
	// httptest.NewRecorder captures the response written by the handler.
	recorder := httptest.NewRecorder()

	// httptest.NewRequest builds a fake *http.Request — no real network call.
	request := httptest.NewRequest(http.MethodGet, "/health", nil)

	// Call the handler directly. In production this would be called by the HTTP server.
	handlers.HealthHandler(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", recorder.Code)
	}
}

// TestInfo_Returns200 verifies that GET /info responds with HTTP 200.
func TestInfo_Returns200(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/info", nil)

	handlers.InfoHandler(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", recorder.Code)
	}
}

// TestHealth_ResponseIsValidJSON verifies that the /health response body is
// valid JSON and contains the expected "status" and "service" fields.
func TestHealth_ResponseIsValidJSON(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)

	handlers.HealthHandler(recorder, request)

	// Attempt to decode the response body into a generic map.
	// If Decode returns an error, the body was not valid JSON.
	var body map[string]string
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("response body is not valid JSON: %v", err)
	}

	// Check the "status" field
	if body["status"] != "ok" {
		t.Errorf("expected status=ok, got %q", body["status"])
	}

	// Check the "service" field
	if body["service"] != "containerization-demo" {
		t.Errorf("expected service=containerization-demo, got %q", body["service"])
	}
}

// TestInfo_ContainsExpectedFields verifies that the /info response body is valid
// JSON and contains the expected fields: go_version, binary_size, os, arch.
func TestInfo_ContainsExpectedFields(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/info", nil)

	handlers.InfoHandler(recorder, request)

	var body map[string]string
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("response body is not valid JSON: %v", err)
	}

	// Each field should be present and non-empty.
	requiredFields := []string{"go_version", "binary_size", "os", "arch"}
	for _, field := range requiredFields {
		if body[field] == "" {
			t.Errorf("expected field %q to be present and non-empty in /info response", field)
		}
	}
}

// TestBuildMux_HealthRouteIsRegistered verifies that BuildMux registers /health.
// We send a request through the mux to confirm routing works end-to-end.
func TestBuildMux_HealthRouteIsRegistered(t *testing.T) {
	mux := handlers.BuildMux()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)

	mux.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Errorf("expected /health to return 200 via mux, got %d", recorder.Code)
	}
}

// TestBuildMux_InfoRouteIsRegistered verifies that BuildMux registers /info.
func TestBuildMux_InfoRouteIsRegistered(t *testing.T) {
	mux := handlers.BuildMux()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/info", nil)

	mux.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Errorf("expected /info to return 200 via mux, got %d", recorder.Code)
	}
}
