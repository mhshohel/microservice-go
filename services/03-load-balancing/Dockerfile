# Multi-stage Dockerfile for the Load Balancing demo.

FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s" \
    -o /load-balancer \
    ./services/03-load-balancing/cmd/main.go

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /load-balancer .
EXPOSE 8082
ENTRYPOINT ["./load-balancer"]
