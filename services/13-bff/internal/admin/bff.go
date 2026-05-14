// bff.go — Admin BFF (Backend for Frontend) handler.
//
// The admin client is a management dashboard. Admins don't need a raw list of
// individual orders — they need AGGREGATED data: how many orders are there in
// total, what is the total revenue, and how are orders distributed across statuses?
//
// This BFF does not return a list at all. It aggregates all orders into a
// single summary object — a "dashboard" view.
//
// Route: GET /admin/dashboard
//
// Response shape:
//
//	{
//	  "total_orders": 5,
//	  "total_revenue_cents": 26492,
//	  "by_status": {
//	    "delivered": 1,
//	    "shipped": 2,
//	    "pending": 2
//	  }
//	}

package admin

import (
	"encoding/json"
	"net/http"

	"microservices-go/services/13-bff/internal/orders"
)

// dashboardResponse is the aggregated view that the admin BFF returns.
// Instead of a list of orders, the admin gets a summary with counts and totals.
type dashboardResponse struct {
	// TotalOrders is the total number of orders in the system.
	TotalOrders int `json:"total_orders"`

	// TotalRevenueCents is the sum of all order totals (in cents).
	// The admin dashboard uses this to show revenue figures.
	TotalRevenueCents int `json:"total_revenue_cents"`

	// ByStatus is a count of orders grouped by their status.
	// e.g. {"pending": 2, "shipped": 2, "delivered": 1}
	// This powers the status breakdown chart on the admin dashboard.
	ByStatus map[string]int `json:"by_status"`
}

// Handler returns an http.Handler for the admin BFF endpoint.
//
// The data parameter is the list of orders to aggregate.
// Accepting data as a parameter makes this handler easily testable —
// tests pass in a controlled slice of orders.
func Handler(data []orders.Order) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Aggregate the raw order list into management metrics.
		// A real admin BFF might call multiple backend services and join their
		// results — here we aggregate a single order list for clarity.
		totalRevenue := 0
		byStatus := make(map[string]int)

		for _, o := range data {
			// Sum up the revenue from all orders.
			totalRevenue += o.TotalCents

			// Count orders per status.
			// map[string]int initialises missing keys to 0, so ++ works directly.
			byStatus[o.Status]++
		}

		response := dashboardResponse{
			TotalOrders:       len(data),
			TotalRevenueCents: totalRevenue,
			ByStatus:          byStatus,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})
}
