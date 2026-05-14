# Dockerfile — Multi-stage build for the Containerization demo service.
#
# WHY MULTI-STAGE?
# ────────────────
# A single-stage build would produce an image containing the entire Go toolchain
# (~300 MB). We only need the compiled binary at runtime (~6 MB).
# Multi-stage builds solve this: Stage 1 compiles the code; Stage 2 copies only
# the binary into a tiny base image. The build tools never make it to production.
#
# STAGE SIZES (approximate):
#
#   golang:1.26-alpine (builder)  ~300 MB  ← includes Go compiler, stdlib, caches
#         │
#         │  go build  →  /app-demo  (~6 MB stripped binary)
#         │
#         ▼ only /app-demo is copied ↓
#   alpine:3.20 (runtime)          ~10 MB  ← tiny Linux + our binary only
#
# Final image size: ~16 MB  vs  ~306 MB for a single-stage build
#

# ──────────────────────────────────────────────────────────────────────────────
# STAGE 1 — builder
#
# We use golang:1.26-alpine (not the full Debian-based image) to keep the
# builder smaller and reduce CI pull times. "AS builder" gives this stage a
# name so we can reference it in the COPY --from directive below.
# ──────────────────────────────────────────────────────────────────────────────
FROM golang:1.26-alpine AS builder

# Set the working directory inside the container. All subsequent commands run
# relative to this path. Using /build keeps things separate from the system.
WORKDIR /build

# Copy dependency manifests FIRST, before copying source code.
# Docker caches each layer separately. If only source code changes (not go.mod),
# Docker reuses the cached "go mod download" layer — much faster rebuilds.
COPY go.mod go.sum ./

# Download all dependencies declared in go.mod.
# These are cached as a layer. Subsequent builds skip this step if go.mod unchanged.
RUN go mod download

# Now copy the full source tree. This layer is invalidated whenever any source
# file changes, but the dependency download layer above is still reused.
COPY . .

# Build the binary. Key flags explained:
#   CGO_ENABLED=0  — pure Go build, no C library linkage. The resulting binary
#                    is fully self-contained and works in a minimal image like
#                    Alpine that may not have glibc installed.
#   GOOS=linux     — cross-compile for Linux (the container OS), even if you're
#                    building on macOS or Windows.
#   -ldflags="-w -s" — strip the DWARF debug info (-w) and symbol table (-s).
#                    This alone can cut binary size by 30–40%.
#   -o /app-demo   — output path for the compiled binary.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s" \
    -o /app-demo \
    ./services/10-containerization/cmd/main.go

# ──────────────────────────────────────────────────────────────────────────────
# STAGE 2 — runtime
#
# We start fresh from a minimal Alpine Linux image. Nothing from the builder
# stage exists here unless we explicitly COPY it. The Go toolchain, build cache,
# intermediate object files — all gone.
# ──────────────────────────────────────────────────────────────────────────────
FROM alpine:3.20

# Add CA certificates so the binary can make HTTPS calls to external services.
# Alpine ships without them by default. This is a common gotcha when moving
# from a full base image (which includes certs) to a minimal one.
RUN apk add --no-cache ca-certificates

# Set the working directory inside the runtime container.
WORKDIR /app

# Copy ONLY the compiled binary from the builder stage.
# --from=builder references the named stage above.
# Everything else (source code, Go toolchain, module cache) stays behind.
COPY --from=builder /app-demo .

# Tell Docker which port this service listens on.
# This is documentation — it does NOT actually open the port. Use
# `docker run -p 8090:8090` or `ports:` in docker-compose.yml to expose it.
EXPOSE 8090

# ENTRYPOINT defines the command that runs when the container starts.
# Using the exec form (JSON array) avoids spawning a shell, which means
# signals like SIGTERM are delivered directly to our binary — important
# for graceful shutdown in Kubernetes or docker stop.
ENTRYPOINT ["./app-demo"]
