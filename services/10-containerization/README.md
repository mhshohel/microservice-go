# Service 10 — Containerization

## What is Containerization?

Containerization packages an application and all its dependencies (libraries, config files, runtime) into a single, portable unit called a **container image**. The container runs identically on a developer's laptop, a CI runner, and a production server — no more "it works on my machine".

For Go microservices, containerization means:
- **Reproducible builds** — the same `docker build` command always produces the same binary
- **Minimal images** — Go compiles to a single static binary; no interpreter or VM needed
- **Isolation** — each service runs in its own container with its own filesystem and network namespace

---

## Why Multi-Stage Builds?

A naive single-stage Dockerfile copies your source code into a `golang` image and compiles it there. The resulting image includes:

- The entire Go toolchain (~250 MB)
- The Go standard library source
- Your build cache and intermediate object files
- Your actual binary (~6 MB)

You ship 256 MB when you only need 6 MB.

**Multi-stage builds** solve this by splitting the process into two stages:

```
Stage 1 (builder)                    Stage 2 (runtime)
─────────────────────────────────    ──────────────────────────────
golang:1.26-alpine (~300 MB)         alpine:3.20 (~8 MB)
  + go mod download                    + ca-certificates (~2 MB)
  + go build -ldflags="-w -s"         + your binary (~6 MB)
  → /app-demo (binary only)         ─────────────────────────────
                                     Total: ~16 MB  ✓
```

Only the final binary crosses from Stage 1 to Stage 2. The compiler, source code, and build cache never reach the production image.

---

## Layer Size Diagram

```
 ┌─────────────────────────────────────────────┐
 │  golang:1.26-alpine  (builder stage)        │
 │  ┌─────────────────────────────────────┐    │
 │  │  go.mod / go.sum  (deps layer)  ~5MB│    │
 │  ├─────────────────────────────────────┤    │
 │  │  source code      (src layer)   ~1MB│    │
 │  ├─────────────────────────────────────┤    │
 │  │  go build         (build layer)     │    │
 │  │  → /app-demo      (binary)      ~6MB│    │
 │  └─────────────────────────────────────┘    │
 │  TOTAL builder image: ~300 MB               │
 └──────────────────────┬──────────────────────┘
                        │  COPY --from=builder /app-demo
                        ▼  (only the binary crosses stages)
 ┌─────────────────────────────────────────────┐
 │  alpine:3.20  (runtime stage)               │
 │  ┌─────────────────────────────────────┐    │
 │  │  alpine base             ~8 MB      │    │
 │  ├─────────────────────────────────────┤    │
 │  │  ca-certificates         ~2 MB      │    │
 │  ├─────────────────────────────────────┤    │
 │  │  /app/app-demo (binary)  ~6 MB      │    │
 │  └─────────────────────────────────────┘    │
 │  TOTAL runtime image: ~16 MB   ✓            │
 └─────────────────────────────────────────────┘
```

---

## Key Dockerfile Techniques

| Technique | Why it matters |
|-----------|---------------|
| `COPY go.mod go.sum ./` before `COPY . .` | Docker caches the dependency download layer separately; only invalidated when go.mod changes |
| `CGO_ENABLED=0` | Produces a fully static binary with no C library dependency — works in Alpine |
| `GOOS=linux` | Cross-compiles for Linux even when building on macOS/Windows |
| `-ldflags="-w -s"` | Strips debug info and symbol table — reduces binary size ~30% |
| `ENTRYPOINT ["./app-demo"]` | Exec form (not shell form) — signals like SIGTERM reach the binary directly |
| `EXPOSE 8090` | Documents the port; use `-p 8090:8090` at `docker run` to actually open it |

---

## Endpoints

| Method | Path | Response |
|--------|------|----------|
| GET | `/health` | `{"status":"ok","service":"containerization-demo"}` |
| GET | `/info` | `{"go_version":"go1.26","binary_size":"small","os":"linux","arch":"amd64"}` |

---

## How to Build and Run

### Without Docker (local development)

```bash
# From the repo root
go run ./services/10-containerization/cmd/main.go

# Test
curl http://localhost:8090/health
curl http://localhost:8090/info
```

### With Docker (the full multi-stage demo)

```bash
# Build the image — run from the repo root so Docker can COPY the whole module
docker build \
  -t containerization-demo \
  -f services/10-containerization/Dockerfile \
  .

# Check the image size (compare to ~300 MB for a single-stage build)
docker images containerization-demo

# Run the container
docker run -p 8090:8090 containerization-demo

# Test
curl http://localhost:8090/health
curl http://localhost:8090/info
```

### Run tests

```bash
go test -v -race ./services/10-containerization/...
```
