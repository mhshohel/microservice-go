# Dockerfile — Multi-stage build for the "Database per Service" demo.
#
# This service uses modernc.org/sqlite, which is a pure-Go SQLite implementation.
# Because there is NO C code involved, we can set CGO_ENABLED=0 and still get
# a fully static binary — simpler and more portable than the cgo-based sqlite3 driver.
#
# STAGE SIZES (approximate):
#
#   golang:1.26-alpine (builder)   ~300 MB
#         │
#         │  go build (CGO_ENABLED=0) → /database-per-service-demo  (~8 MB)
#         │
#         ▼ only the binary is copied
#   alpine:3.20 (runtime)           ~16 MB total
#

# ──────────────────────────────────────────────────────────────────────────────
# STAGE 1 — builder
# ──────────────────────────────────────────────────────────────────────────────
FROM golang:1.26-alpine AS builder

WORKDIR /build

# Copy dependency manifests first for better layer caching.
# Docker won't re-download modules unless go.mod or go.sum changes.
COPY go.mod go.sum ./
RUN go mod download

# Copy the full source tree.
COPY . .

# Build the binary.
# CGO_ENABLED=0 is critical here: even though modernc.org/sqlite is pure Go,
# the build system still needs to know not to look for a C compiler.
# Without this flag you might get a "cgo: C compiler not found" error in CI
# or on machines without gcc installed.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s" \
    -o /database-per-service-demo \
    ./services/11-database-per-service/cmd/main.go

# ──────────────────────────────────────────────────────────────────────────────
# STAGE 2 — runtime
# ──────────────────────────────────────────────────────────────────────────────
FROM alpine:3.20

# CA certificates allow the binary to make HTTPS calls if needed.
RUN apk add --no-cache ca-certificates

WORKDIR /app

# Copy only the compiled binary from the builder stage.
COPY --from=builder /database-per-service-demo .

# The demo writes *.db files to the current directory. Use a volume mount
# if you want data to persist across container restarts:
#   docker run -v $(pwd)/data:/app database-per-service-demo
ENTRYPOINT ["./database-per-service-demo"]
