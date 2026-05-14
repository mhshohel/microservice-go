// proxy.go — Reverse proxy that forwards requests to backend services.
//
// A reverse proxy sits between the client and the backend.
// The client sends a request to the gateway, and the proxy:
//   1. Forwards the request to the right backend service
//   2. Waits for the backend's response
//   3. Sends that response back to the client
//
// The client never communicates directly with the backend — it only talks to the gateway.
//
// We use Go's built-in httputil.ReverseProxy rather than writing our own
// request-copying logic — it handles edge cases like streaming responses,
// hop-by-hop headers, and connection reuse.
//
// Note on Director vs Rewrite:
// Go 1.20 added the Rewrite hook as the recommended replacement for Director.
// ReverseProxy requires exactly one of them to be set — never both.
// We create the proxy directly (without NewSingleHostReverseProxy) so we can
// use Rewrite without conflicts.

package proxy

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

// Forward sends the incoming HTTP request to targetURL and writes the backend's
// response back to the client via w.
//
// targetURL is the base URL of the backend service, e.g. "http://localhost:9001".
// The request's path and query string are appended automatically by pr.SetURL.
func Forward(w http.ResponseWriter, r *http.Request, targetURL string) {
	// Parse the target URL so Go knows the scheme and host to forward to
	target, err := url.Parse(targetURL)
	if err != nil {
		slog.Error("proxy: invalid backend URL", "url", targetURL, "error", err)
		http.Error(w, "gateway configuration error", http.StatusInternalServerError)
		return
	}

	// Build the reverse proxy directly so we can use only Rewrite (not Director).
	// httputil.NewSingleHostReverseProxy would set Director automatically,
	// and having both Director and Rewrite set causes a runtime panic.
	reverseProxy := &httputil.ReverseProxy{

		// Rewrite is called just before the request is forwarded to the backend.
		// pr.In  = the original incoming request (read-only)
		// pr.Out = the outgoing copy we can modify
		Rewrite: func(pr *httputil.ProxyRequest) {
			// SetURL points the outgoing request at our backend (sets Host + Scheme).
			// It also preserves the path and query string from the incoming request.
			pr.SetURL(target)

			// SetXForwarded adds standard forwarding headers:
			//   X-Forwarded-For:  the client's IP address
			//   X-Forwarded-Host: the original host the client requested
			//   X-Forwarded-Proto: http or https
			pr.SetXForwarded()

			// Mark that this request passed through our gateway
			pr.Out.Header.Set("X-Forwarded-By", "api-gateway")

			slog.Info("proxy: forwarding",
				"method", pr.Out.Method,
				"path", pr.Out.URL.Path,
				"backend", targetURL,
			)
		},

		// ErrorHandler is called when the backend is unreachable or returns an error.
		// Without this, Go's default behaviour is to return a plain 502 with no body.
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			slog.Error("proxy: backend failed", "backend", targetURL, "error", err)
			http.Error(w,
				fmt.Sprintf("backend service unavailable: %s", err.Error()),
				http.StatusBadGateway, // 502 = Bad Gateway
			)
		},

		// Transport controls how the proxy makes outgoing connections.
		// ResponseHeaderTimeout prevents a slow backend from holding the gateway indefinitely.
		Transport: &http.Transport{
			ResponseHeaderTimeout: 10 * time.Second,
		},
	}

	reverseProxy.ServeHTTP(w, r)
}
