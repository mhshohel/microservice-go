// bff.go — Mobile BFF (Backend for Frontend) handler.
//
// The mobile client is a phone app. Mobile devices have limited bandwidth
// and small screens, so we return the MINIMUM data needed to render the
// orders list: just the order ID, status, and total price.
//
// Contrast with the Web BFF: this response is intentionally stripped down.
// We omit customer name, item description, quantity, timestamps — all things
// the mobile order list doesn't display. Less data = faster load = less battery.
//
// Route: GET /mobile/orders
//
// Response shape:
//
//	{
//	  "orders": [
//	    {
//	      "id": "ord-001",
//	      "status": "delivered",
//	      "total_cents": 4999
//	    },
//	    ...
//	  ]
//	}

package mobile

import (
	"encoding/json"
	"net/http"

	"microservices-go/services/13-bff/internal/orders"
)

// mobileOrder is the minimal response shape for a mobile client.
// Only three fields — the absolute minimum to show an order summary on a phone.
// The mobile app doesn't need the customer name (it's the logged-in user),
// item details (shown on a separate detail screen), or timestamps.
type mobileOrder struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	TotalCents int    `json:"total_cents"`
}

// mobileResponse is the top-level JSON envelope returned to the mobile client.
type mobileResponse struct {
	Orders []mobileOrder `json:"orders"`
}

// Handler returns an http.Handler for the mobile BFF endpoint.
//
// The data parameter is the list of orders to shape and return.
// Accepting data as a parameter makes this handler easily testable.
func Handler(data []orders.Order) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Shape each raw Order into the mobile-specific minimal format.
		// We deliberately drop most fields to keep the payload small.
		shaped := make([]mobileOrder, len(data))
		for i, o := range data {
			shaped[i] = mobileOrder{
				ID:         o.ID,
				Status:     o.Status,
				TotalCents: o.TotalCents,
			}
		}

		response := mobileResponse{
			Orders: shaped,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})
}
