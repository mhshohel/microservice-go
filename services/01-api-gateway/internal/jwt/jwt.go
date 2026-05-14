// jwt.go — Simple JSON Web Token (JWT) helper using only the Go standard library.
//
// What is a JWT?
// A JWT is a compact, URL-safe string that proves who a client is.
// It has three parts separated by dots:
//
//   eyJhbGciOiJIUzI1NiJ9  .  eyJ1c2VyX2lkIjoiMSJ9  .  SflKxwRJSMeKKF2QT4fw
//   └─── header ─────────┘  └────── payload ───────┘  └─── signature ───────┘
//
// - Header:    which algorithm was used to sign this token
// - Payload:   the actual data (user ID, role, expiry time)
// - Signature: proof that nobody tampered with the header or payload
//
// How the signature works:
//   signature = HMAC-SHA256(header + "." + payload, secretKey)
//
// If someone changes the payload (e.g., upgrades their role to "admin"),
// the signature won't match anymore — and we reject the token.

package jwt

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// SecretKey is used to sign and verify tokens.
// In a real app this must come from an environment variable — never hardcode it.
const SecretKey = "demo-secret-key-change-this-in-production"

// Claims is the data we store inside the token.
// Anyone who has the token can read the claims (they're base64-encoded, not encrypted).
// But they cannot change them without breaking the signature.
type Claims struct {
	UserID string `json:"user_id"` // who this token belongs to
	Role   string `json:"role"`    // their role (e.g., "customer", "admin")
	Exp    int64  `json:"exp"`     // expiry time as Unix timestamp (seconds since 1970)
}

// header is the first part of every JWT — it tells receivers which algorithm to use.
type header struct {
	Algorithm string `json:"alg"` // "HS256" = HMAC + SHA-256
	Type      string `json:"typ"` // always "JWT"
}

// Generate creates a new JWT token for the given user.
// Returns the token string the client should send in the Authorization header.
func Generate(userID, role string) string {
	// Part 1: encode the header
	h := header{Algorithm: "HS256", Type: "JWT"}
	headerBytes, _ := json.Marshal(h)
	encodedHeader := base64.RawURLEncoding.EncodeToString(headerBytes)

	// Part 2: encode the payload (claims)
	claims := Claims{
		UserID: userID,
		Role:   role,
		Exp:    time.Now().Add(24 * time.Hour).Unix(), // valid for 24 hours
	}
	payloadBytes, _ := json.Marshal(claims)
	encodedPayload := base64.RawURLEncoding.EncodeToString(payloadBytes)

	// Part 3: sign "header.payload" with our secret key
	message := encodedHeader + "." + encodedPayload
	sig := computeSignature(message)

	// Combine all three parts with dots: "header.payload.signature"
	return message + "." + sig
}

// Validate checks whether a token is valid and returns the claims inside it.
// Returns an error if the token is malformed, tampered with, or expired.
func Validate(tokenString string) (*Claims, error) {
	// A valid JWT always has exactly three parts separated by dots
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format: expected 3 parts (header.payload.signature), got %d", len(parts))
	}

	encodedHeader := parts[0]
	encodedPayload := parts[1]
	receivedSig := parts[2]

	// Recompute what the signature SHOULD be
	message := encodedHeader + "." + encodedPayload
	expectedSig := computeSignature(message)

	// Compare signatures using hmac.Equal — this is a constant-time comparison.
	// A normal string comparison (==) leaks timing info that attackers can exploit.
	// hmac.Equal takes the same amount of time regardless of where strings differ.
	if !hmac.Equal([]byte(expectedSig), []byte(receivedSig)) {
		return nil, fmt.Errorf("invalid token: signature does not match (token may have been tampered with)")
	}

	// Decode the payload to get the claims
	payloadBytes, err := base64.RawURLEncoding.DecodeString(encodedPayload)
	if err != nil {
		return nil, fmt.Errorf("invalid token: cannot decode payload: %w", err)
	}

	var claims Claims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, fmt.Errorf("invalid token: cannot parse claims: %w", err)
	}

	// Check whether the token has expired
	if claims.Exp > 0 && time.Now().Unix() > claims.Exp {
		return nil, fmt.Errorf("token has expired")
	}

	return &claims, nil
}

// computeSignature creates an HMAC-SHA256 signature for the given message.
// This is a one-way operation — you can verify but not reverse it without the secret key.
func computeSignature(message string) string {
	mac := hmac.New(sha256.New, []byte(SecretKey))
	mac.Write([]byte(message))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
