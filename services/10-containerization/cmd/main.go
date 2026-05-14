// main.go — Entry point for the Containerization demo service.
//
// This file is intentionally tiny. Its only job is to:
//  1. Configure structured logging
//  2. Build the HTTP mux (defined in internal/handlers)
//  3. Start the server with sensible timeouts
//
// The real teaching moment is in the Dockerfile alongside this service —
// it shows how a multi-stage Docker build compiles this program in one
// "builder" stage, then copies only the finished binary into a tiny
// Alpine image for the final "runtime" stage.
//
// HOW TO RUN (without Docker):
//
//	go run ./services/10-containerization/cmd/main.go
//
// HOW TO RUN (with Docker — the full demo):
//
//	docker build -t containerization-demo -f services/10-containerization/Dockerfile .
//	docker run -p 8090:8090 containerization-demo
package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"microservices-go/services/10-containerization/internal/handlers"
)

func main() {
	// Configure structured logging. slog is Go's built-in structured logger
	// (introduced in Go 1.21). Text format is readable in Docker logs.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// Build the HTTP mux with all routes registered.
	// The mux lives in internal/handlers so it can be tested independently.
	mux := handlers.BuildMux()

	// Always set timeouts on HTTP servers. Without them, a slow or malicious
	// client can hold a connection open forever, eventually exhausting all
	// goroutines. Inside a container these limits still matter — containers
	// do not automatically protect against slow clients.
	server := &http.Server{
		Addr:         ":8090",
		Handler:      mux,
		ReadTimeout:  5 * time.Second,  // max time to read the full request
		WriteTimeout: 10 * time.Second, // max time to write the full response
		IdleTimeout:  30 * time.Second, // max time to keep an idle Keep-Alive connection
	}

	slog.Info("containerization-demo service starting", "addr", server.Addr)

	// ListenAndServe blocks until the server is shut down.
	// It only returns when something goes wrong, so we treat any return as fatal.
	if err := server.ListenAndServe(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
