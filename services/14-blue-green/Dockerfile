# ── Stage 1: Build ───────────────────────────────────────────────────────────
FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /blue-green ./services/14-blue-green/cmd/main.go

# ── Stage 2: Runtime ─────────────────────────────────────────────────────────
FROM alpine:3.20

RUN adduser -D -u 1001 appuser
USER appuser

COPY --from=builder /blue-green /usr/local/bin/blue-green

EXPOSE 8080
ENTRYPOINT ["blue-green"]
