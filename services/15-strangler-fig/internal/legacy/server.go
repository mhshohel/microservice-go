// server.go — Fake legacy monolith handler.
//
// In a real migration, this would be the existing monolith application.
// Here it's a simple HTTP handler that always appends "[legacy]" to responses
// so tests can verify which system handled the request.

package legacy

import (
	"encoding/json"
	"net/http"
)

// Handler returns an http.Handler for the fake legacy monolith.
// It handles any path and always identifies itself as "legacy".
func Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"source":  "legacy",
			"path":    r.URL.Path,
			"message": "handled by legacy monolith",
		})
	})

	return mux
}
