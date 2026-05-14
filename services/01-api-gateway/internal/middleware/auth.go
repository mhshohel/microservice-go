// auth.go — JWT authentication middleware.
//
// "Middleware" is a function that wraps an HTTP handler to add extra behaviour.
// This one sits in front of every protected route and checks the JWT token.
//
// Request flow:
//   1. Read the "Authorization: Bearer <token>" header
//   2. If missing or wrong format → return 401 Unauthorized (stop here)
//   3. Validate the JWT token
//   4. If invalid or expired → return 401 Unauthorized (stop here)
//   5. If valid → add the user's claims to the request context and continue

package middleware

import (
	"context"
	"net/http"
	"strings"

	"microservices-go/services/01-api-gateway/internal/jwt"
)

// contextKey is a private type for context keys in this package.
// Using a dedicated type prevents accidental key collisions with other packages
// that might also store things in the context.
type contextKey string

// ClaimsKey is the key used to store/retrieve JWT claims from the request context.
const ClaimsKey contextKey = "jwt_claims"

// Auth returns an HTTP middleware that validates JWT tokens.
// It wraps the provided handler and only calls it if authentication passes.
//
// Usage:
//
//	protected := middleware.Auth(myHandler)
//	mux.Handle("/api/", protected)
func Auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The /health endpoint is public — no token needed.
		// Load balancers and monitoring tools call /health without tokens.
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		// Read the Authorization header.
		// Expected format: "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w,
				"missing Authorization header — send your token as: Authorization: Bearer <token>",
				http.StatusUnauthorized,
			)
			return
		}

		// The value must start with "Bearer " (capital B, space after)
		if !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w,
				"invalid Authorization header format — expected: Bearer <token>",
				http.StatusUnauthorized,
			)
			return
		}

		// Extract just the token string (everything after "Bearer ")
		tokenString := strings.TrimPrefix(authHeader, "Bearer ")

		// Validate the token — this checks the signature and expiry
		claims, err := jwt.Validate(tokenString)
		if err != nil {
			http.Error(w, "unauthorized: "+err.Error(), http.StatusUnauthorized)
			return
		}

		// Token is valid. Store the claims in the request context so downstream
		// handlers can use them (e.g., to know which user made the request).
		ctx := context.WithValue(r.Context(), ClaimsKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetClaims retrieves the JWT claims from a request context.
// Returns nil if the request was not authenticated.
//
// Example usage in a handler:
//
//	claims := middleware.GetClaims(r)
//	if claims != nil {
//	    fmt.Println("Request from user:", claims.UserID)
//	}
func GetClaims(r *http.Request) *jwt.Claims {
	claims, _ := r.Context().Value(ClaimsKey).(*jwt.Claims)
	return claims
}
