// server.go — Fake modern microservice handler.
//
// In a real migration, this would be the new microservice being built.
// It identifies itself as "modern" so tests can verify routing works.

package modern

import (
	"encoding/json"
	"net/http"
)

// Handler returns an http.Handler for the fake new microservice.
func Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"source":  "modern",
			"path":    r.URL.Path,
			"message": "handled by new microservice",
		})
	})

	return mux
}
