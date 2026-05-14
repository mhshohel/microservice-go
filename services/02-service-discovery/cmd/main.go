// main.go — Entry point for the Service Discovery demo.
//
// This program starts a service registry on :8081.
// It also spawns three "fake" microservices that register themselves,
// so you can see the registry in action without setting up real services.
//
// HOW TO RUN (from the project root):
//   go run ./services/02-service-discovery/cmd/main.go
//
// HOW TO TEST (in a second terminal):
//
//   # See all registered services (the fake ones register automatically)
//   curl http://localhost:8081/services
//
//   # See all instances of a specific service
//   curl http://localhost:8081/services/user-svc
//
//   # Register your own service instance
//   curl -X POST http://localhost:8081/register \
//     -H "Content-Type: application/json" \
//     -d '{"name":"my-svc","address":"localhost:9999"}'
//
//   # Deregister (replace INSTANCE_ID with the id from register response)
//   curl -X DELETE http://localhost:8081/deregister/INSTANCE_ID
//
//   # Heartbeat
//   curl -X PUT http://localhost:8081/heartbeat/INSTANCE_ID
//
//   # Registry health
//   curl http://localhost:8081/health

package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"

	"microservices-go/services/02-service-discovery/internal/api"
	"microservices-go/services/02-service-discovery/internal/registry"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// ── Step 1: Create the registry ───────────────────────────────────────────
	// 30-second heartbeat timeout: if a service doesn't ping us in 30s, we drop it.
	reg := registry.New(registry.DefaultHeartbeatTimeout)

	// ── Step 2: Start the background cleanup loop ─────────────────────────────
	// Every 10 seconds, scan for instances whose heartbeat has timed out and remove them.
	// This runs in a goroutine so it doesn't block the main thread.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // stop the cleanup loop when main exits

	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				removed := reg.RemoveExpired()
				for _, id := range removed {
					slog.Warn("removed expired instance (heartbeat timeout)", "instance_id", id)
				}
			case <-ctx.Done():
				// context cancelled → stop the loop
				return
			}
		}
	}()

	// ── Step 3: Register fake services for the demo ───────────────────────────
	// In a real system these services would register themselves.
	// Here we do it manually so the demo shows a populated registry.
	registerFakeServices(reg)

	// ── Step 4: Build the HTTP router ─────────────────────────────────────────
	mux := http.NewServeMux()
	handler := api.New(reg)
	handler.RegisterRoutes(mux)

	// ── Step 5: Start the server ──────────────────────────────────────────────
	addr := ":8081"
	slog.Info("──────────────────────────────────────────────")
	slog.Info("Service Registry running", "url", "http://localhost"+addr)
	slog.Info("See all services → curl http://localhost:8081/services")
	slog.Info("──────────────────────────────────────────────")

	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil {
		slog.Error("registry stopped", "error", err)
		os.Exit(1)
	}
}

// registerFakeServices populates the registry with demo instances so there's
// something to look at when you hit /services right after starting the server.
func registerFakeServices(reg *registry.Registry) {
	fakeServices := []registry.RegisterRequest{
		{
			Name:    "user-svc",
			Address: "localhost:9001",
			Metadata: map[string]string{
				"version": "1.2.0",
				"region":  "us-east-1",
			},
		},
		{
			Name:    "user-svc",
			Address: "localhost:9011",
			Metadata: map[string]string{
				"version": "1.2.0",
				"region":  "us-east-1",
			},
		},
		{
			Name:    "order-svc",
			Address: "localhost:9002",
			Metadata: map[string]string{
				"version": "2.0.1",
				"region":  "us-east-1",
			},
		},
		{
			Name:    "product-svc",
			Address: "localhost:9003",
			Metadata: map[string]string{
				"version": "3.1.0",
				"region":  "eu-west-1",
			},
		},
	}

	for _, req := range fakeServices {
		inst, err := reg.Register(req)
		if err != nil {
			slog.Error("failed to register fake service", "name", req.Name, "error", err)
			continue
		}
		slog.Info("registered demo instance", "id", inst.ID, "name", inst.Name, "address", inst.Address)
	}
}

// httpClientDiscovery shows client-side discovery: fetch all instances,
// pick one, and make a call. This is shown in logs when the server starts.
// In a real app, a service client would call this before each upstream request.
func httpClientDiscovery(registryAddr, serviceName string) {
	resp, err := http.Get("http://" + registryAddr + "/services/" + serviceName)
	if err != nil || resp.StatusCode != http.StatusOK {
		slog.Error("discovery lookup failed", "service", serviceName)
		return
	}
	defer resp.Body.Close()

	var result struct {
		Count     int `json:"count"`
		Instances []struct {
			Address string `json:"address"`
		} `json:"instances"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return
	}

	if result.Count == 0 {
		slog.Warn("no instances found for service", "service", serviceName)
		return
	}

	// Pick the first one (a real client would round-robin or use least-connections)
	chosen := result.Instances[0].Address
	slog.Info("client-side discovery: chose instance", "service", serviceName, "address", chosen)
}
