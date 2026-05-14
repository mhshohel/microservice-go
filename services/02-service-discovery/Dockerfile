# Multi-stage Dockerfile for the Service Discovery registry.
#
# Stage 1 (builder): Compile the Go binary in a full Go environment.
# Stage 2 (final):   Copy only the compiled binary into a minimal image.
#
# Why multi-stage? The Go compiler and source code are only needed for building.
# Shipping them in the final image adds ~300MB for no runtime benefit.
# The final image is just the binary on top of a bare OS layer.

# ── Stage 1: Build ────────────────────────────────────────────────────────────
FROM golang:1.26-alpine AS builder

WORKDIR /app

# Copy go.mod first so Docker caches the dependency download layer.
# If only source files change, Docker reuses the cached modules layer.
COPY go.mod ./
RUN go mod download

# Copy all source files and build the binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s" \
    -o /service-discovery \
    ./services/02-service-discovery/cmd/main.go

# ── Stage 2: Runtime ──────────────────────────────────────────────────────────
FROM alpine:3.20

# ca-certificates: needed if the registry ever makes outbound HTTPS calls
RUN apk add --no-cache ca-certificates

WORKDIR /app

# Copy only the compiled binary from the builder stage
COPY --from=builder /service-discovery .

EXPOSE 8081

ENTRYPOINT ["./service-discovery"]
