// bff_test.go — Tests for all three BFF handlers.
//
// Each test passes a controlled slice of Order data directly to the Handler()
// function and inspects the JSON response. This makes tests:
//   - Fast: no real HTTP server needed (httptest.NewRecorder is used)
//   - Deterministic: we control the exact input data
//   - Focused: each test verifies the response shaping, not network code
//
// The key assertion in each test is that each BFF returns the right SHAPE for
// its client — web gets all fields, mobile gets a subset, admin gets aggregates.

package bff_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"microservices-go/services/13-bff/internal/admin"
	"microservices-go/services/13-bff/internal/mobile"
	"microservices-go/services/13-bff/internal/orders"
	"microservices-go/services/13-bff/internal/web"
)

// ── Test Helpers ──────────────────────────────────────────────────────────────

// testOrders returns a small, controlled set of orders for use in tests.
// Using a fixed set (not SampleOrders()) keeps tests self-contained and
// makes expected values easy to calculate by hand.
func testOrders() []orders.Order {
	return []orders.Order{
		{
			ID:           "ord-001",
			CustomerName: "Alice Test",
			Item:         "Widget A",
			Quantity:     2,
			TotalCents:   1000,
			Status:       "delivered",
			CreatedAt:    time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
		},
		{
			ID:           "ord-002",
			CustomerName: "Bob Test",
			Item:         "Widget B",
			Quantity:     1,
			TotalCents:   500,
			Status:       "pending",
			CreatedAt:    time.Date(2026, 1, 2, 11, 0, 0, 0, time.UTC),
		},
		{
			ID:           "ord-003",
			CustomerName: "Carol Test",
			Item:         "Widget C",
			Quantity:     3,
			TotalCents:   750,
			Status:       "pending",
			CreatedAt:    time.Date(2026, 1, 3, 12, 0, 0, 0, time.UTC),
		},
	}
}

// callHandler is a helper that fires a GET request at the given handler and
// returns the response recorder. We use httptest so no real network is needed.
func callHandler(t *testing.T, handler http.Handler) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// ── Web BFF Tests ─────────────────────────────────────────────────────────────

// TestWebBFF_ReturnsFullOrderDetails verifies that the web BFF includes all
// seven order fields (id, customer_name, item, quantity, total_cents, status,
// created_at) for each order.
func TestWebBFF_ReturnsFullOrderDetails(t *testing.T) {
	handler := web.Handler(testOrders())
	rec := callHandler(t, handler)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	// Decode into a flexible map so we can check individual keys.
	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode JSON: %v\nbody: %s", err, rec.Body.String())
	}

	// The response must have an "orders" array.
	ordersRaw, ok := response["orders"]
	if !ok {
		t.Fatal("response missing 'orders' key")
	}
	orderList, ok := ordersRaw.([]any)
	if !ok {
		t.Fatal("'orders' is not an array")
	}
	if len(orderList) != 3 {
		t.Fatalf("expected 3 orders, got %d", len(orderList))
	}

	// Check that the first order has all the expected full-detail fields.
	firstOrder := orderList[0].(map[string]any)
	requiredFields := []string{"id", "customer_name", "item", "quantity", "total_cents", "status", "created_at"}
	for _, field := range requiredFields {
		if _, exists := firstOrder[field]; !exists {
			t.Errorf("web BFF response missing field %q", field)
		}
	}

	// Verify specific values to make sure the data is actually shaped correctly.
	if firstOrder["id"] != "ord-001" {
		t.Errorf("expected id='ord-001', got %v", firstOrder["id"])
	}
	if firstOrder["customer_name"] != "Alice Test" {
		t.Errorf("expected customer_name='Alice Test', got %v", firstOrder["customer_name"])
	}
	if firstOrder["item"] != "Widget A" {
		t.Errorf("expected item='Widget A', got %v", firstOrder["item"])
	}
}

// TestWebBFF_ReturnsTotalCount verifies the total_count field in the web response.
func TestWebBFF_ReturnsTotalCount(t *testing.T) {
	handler := web.Handler(testOrders())
	rec := callHandler(t, handler)

	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	totalCount, ok := response["total_count"]
	if !ok {
		t.Fatal("response missing 'total_count' key")
	}
	// JSON numbers decode as float64 in Go's interface{} unmarshalling.
	if int(totalCount.(float64)) != 3 {
		t.Errorf("expected total_count=3, got %v", totalCount)
	}
}

// ── Mobile BFF Tests ──────────────────────────────────────────────────────────

// TestMobileBFF_ReturnsMinimalFields verifies that the mobile BFF includes only
// the minimal set of fields (id, status, total_cents) and excludes the others.
func TestMobileBFF_ReturnsMinimalFields(t *testing.T) {
	handler := mobile.Handler(testOrders())
	rec := callHandler(t, handler)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode JSON: %v\nbody: %s", err, rec.Body.String())
	}

	ordersRaw, ok := response["orders"]
	if !ok {
		t.Fatal("response missing 'orders' key")
	}
	orderList := ordersRaw.([]any)
	if len(orderList) != 3 {
		t.Fatalf("expected 3 orders, got %d", len(orderList))
	}

	firstOrder := orderList[0].(map[string]any)

	// The mobile response MUST have these minimal fields.
	minimalFields := []string{"id", "status", "total_cents"}
	for _, field := range minimalFields {
		if _, exists := firstOrder[field]; !exists {
			t.Errorf("mobile BFF response missing required field %q", field)
		}
	}

	// The mobile response MUST NOT include verbose fields.
	excludedFields := []string{"customer_name", "item", "quantity", "created_at"}
	for _, field := range excludedFields {
		if _, exists := firstOrder[field]; exists {
			t.Errorf("mobile BFF should NOT include field %q (mobile payload should be minimal)", field)
		}
	}
}

// ── Admin BFF Tests ───────────────────────────────────────────────────────────

// TestAdminBFF_ReturnsDashboardStats verifies that the admin BFF returns
// aggregate statistics rather than a raw order list.
func TestAdminBFF_ReturnsDashboardStats(t *testing.T) {
	handler := admin.Handler(testOrders())
	rec := callHandler(t, handler)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode JSON: %v\nbody: %s", err, rec.Body.String())
	}

	// Check total_orders.
	totalOrders, ok := response["total_orders"]
	if !ok {
		t.Fatal("admin dashboard missing 'total_orders'")
	}
	if int(totalOrders.(float64)) != 3 {
		t.Errorf("expected total_orders=3, got %v", totalOrders)
	}

	// Check total_revenue_cents: 1000 + 500 + 750 = 2250.
	totalRevenue, ok := response["total_revenue_cents"]
	if !ok {
		t.Fatal("admin dashboard missing 'total_revenue_cents'")
	}
	if int(totalRevenue.(float64)) != 2250 {
		t.Errorf("expected total_revenue_cents=2250, got %v", totalRevenue)
	}

	// Check by_status breakdown.
	byStatusRaw, ok := response["by_status"]
	if !ok {
		t.Fatal("admin dashboard missing 'by_status'")
	}
	byStatus := byStatusRaw.(map[string]any)

	// 1 delivered, 2 pending (see testOrders())
	if int(byStatus["delivered"].(float64)) != 1 {
		t.Errorf("expected by_status.delivered=1, got %v", byStatus["delivered"])
	}
	if int(byStatus["pending"].(float64)) != 2 {
		t.Errorf("expected by_status.pending=2, got %v", byStatus["pending"])
	}
}

// TestAdminBFF_NoRawOrderList verifies that the admin BFF does NOT return a
// list of individual orders — it should only return aggregated data.
func TestAdminBFF_NoRawOrderList(t *testing.T) {
	handler := admin.Handler(testOrders())
	rec := callHandler(t, handler)

	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	// The admin dashboard should not contain a raw "orders" list.
	if _, hasOrders := response["orders"]; hasOrders {
		t.Error("admin BFF should NOT return a raw 'orders' list — only aggregated stats")
	}
}

// ── Cross-BFF Comparison Tests ────────────────────────────────────────────────

// TestWebBFF_HasMoreFieldsThanMobile verifies that the web response for a single
// order contains more fields than the mobile response. This is the core BFF
// contract: web is richer than mobile.
func TestWebBFF_HasMoreFieldsThanMobile(t *testing.T) {
	data := testOrders()

	webHandler := web.Handler(data)
	mobileHandler := mobile.Handler(data)

	webRec := callHandler(t, webHandler)
	mobileRec := callHandler(t, mobileHandler)

	var webResponse map[string]any
	var mobileResponse map[string]any

	if err := json.Unmarshal(webRec.Body.Bytes(), &webResponse); err != nil {
		t.Fatalf("failed to decode web JSON: %v", err)
	}
	if err := json.Unmarshal(mobileRec.Body.Bytes(), &mobileResponse); err != nil {
		t.Fatalf("failed to decode mobile JSON: %v", err)
	}

	// Extract the first order from each response.
	webOrders := webResponse["orders"].([]any)
	mobileOrders := mobileResponse["orders"].([]any)

	webFirst := webOrders[0].(map[string]any)
	mobileFirst := mobileOrders[0].(map[string]any)

	webFieldCount := len(webFirst)
	mobileFieldCount := len(mobileFirst)

	if webFieldCount <= mobileFieldCount {
		t.Errorf(
			"web BFF should return more fields per order than mobile: web=%d fields, mobile=%d fields",
			webFieldCount, mobileFieldCount,
		)
	}

	t.Logf("web order fields: %d, mobile order fields: %d (web has %d more)",
		webFieldCount, mobileFieldCount, webFieldCount-mobileFieldCount)
}

// TestBFFs_AllReturnValidJSON verifies that all three BFF endpoints return
// valid JSON with a 200 status code and correct Content-Type header.
func TestBFFs_AllReturnValidJSON(t *testing.T) {
	data := testOrders()

	tests := []struct {
		name    string
		handler http.Handler
	}{
		{"web BFF", web.Handler(data)},
		{"mobile BFF", mobile.Handler(data)},
		{"admin BFF", admin.Handler(data)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := callHandler(t, tc.handler)

			// Status must be 200.
			if rec.Code != http.StatusOK {
				t.Errorf("%s: expected status 200, got %d", tc.name, rec.Code)
			}

			// Content-Type must be application/json.
			contentType := rec.Header().Get("Content-Type")
			if contentType != "application/json" {
				t.Errorf("%s: expected Content-Type=application/json, got %q", tc.name, contentType)
			}

			// Body must be valid JSON.
			var result any
			if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
				t.Errorf("%s: response body is not valid JSON: %v\nbody: %s",
					tc.name, err, rec.Body.String())
			}

			// Body must not be empty.
			if rec.Body.Len() == 0 {
				t.Errorf("%s: response body is empty", tc.name)
			}
		})
	}
}

// TestBFFs_EmptyOrderList verifies that all BFFs handle an empty order list
// gracefully — they should return valid JSON with empty/zero values, not panic.
func TestBFFs_EmptyOrderList(t *testing.T) {
	emptyData := []orders.Order{}

	tests := []struct {
		name    string
		handler http.Handler
	}{
		{"web BFF", web.Handler(emptyData)},
		{"mobile BFF", mobile.Handler(emptyData)},
		{"admin BFF", admin.Handler(emptyData)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := callHandler(t, tc.handler)

			if rec.Code != http.StatusOK {
				t.Errorf("%s: expected status 200 on empty data, got %d", tc.name, rec.Code)
			}

			var result any
			if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
				t.Errorf("%s: invalid JSON on empty data: %v", tc.name, err)
			}
		})
	}
}
