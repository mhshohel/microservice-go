// bff.go — Web BFF (Backend for Frontend) handler.
//
// The web client is a desktop browser application. It has plenty of bandwidth
// and a large screen, so it wants FULL order details — every field available.
// This BFF returns the complete order list with all fields included.
//
// Route: GET /web/orders
//
// Response shape:
//
//	{
//	  "orders": [
//	    {
//	      "id": "ord-001",
//	      "customer_name": "Alice Chen",
//	      "item": "Wireless Keyboard",
//	      "quantity": 1,
//	      "total_cents": 4999,
//	      "status": "delivered",
//	      "created_at": "2026-04-10T09:00:00Z"
//	    },
//	    ...
//	  ],
//	  "total_count": 5
//	}

package web

import (
	"encoding/json"
	"net/http"
	"time"

	"microservices-go/services/13-bff/internal/orders"
)

// webOrder is the full response shape for a web client.
// Every field is included because a desktop app can use all of them:
// the product details for a rich order list, the timestamp for sorting,
// and the status for colour-coded badges.
type webOrder struct {
	ID           string    `json:"id"`
	CustomerName string    `json:"customer_name"`
	Item         string    `json:"item"`
	Quantity     int       `json:"quantity"`
	TotalCents   int       `json:"total_cents"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
}

// webResponse is the top-level JSON envelope returned to the web client.
type webResponse struct {
	Orders     []webOrder `json:"orders"`
	TotalCount int        `json:"total_count"`
}

// Handler returns an http.Handler for the web BFF endpoint.
//
// The data parameter is the list of orders to shape and return.
// Accepting data as a parameter (rather than reading from a global) makes this
// handler trivially testable — tests simply pass in their own order slice.
func Handler(data []orders.Order) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Shape each raw Order into the web-specific response format.
		// The web client gets ALL fields — this is the richest response.
		shaped := make([]webOrder, len(data))
		for i, o := range data {
			shaped[i] = webOrder{
				ID:           o.ID,
				CustomerName: o.CustomerName,
				Item:         o.Item,
				Quantity:     o.Quantity,
				TotalCents:   o.TotalCents,
				Status:       o.Status,
				CreatedAt:    o.CreatedAt,
			}
		}

		response := webResponse{
			Orders:     shaped,
			TotalCount: len(shaped),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})
}
