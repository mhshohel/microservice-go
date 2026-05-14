// main.go — Blue-Green Deployment demo.
//
// Shows the full deployment workflow:
//   1. Blue is live (v1)
//   2. Deploy v2 to Green (idle)
//   3. Smoke test Green
//   4. Switch traffic to Green
//   5. Simulate a problem → rollback to Blue
//
// HOW TO RUN:
//   go run ./services/14-blue-green/cmd/main.go

package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"microservices-go/services/14-blue-green/internal/env"
	"microservices-go/services/14-blue-green/internal/router"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// ── Set up two environments ───────────────────────────────────────────────
	blue := env.New(env.Blue, "v1")
	green := env.New(env.Green, "v1-idle") // green starts idle
	defer blue.Close()
	defer green.Close()

	rt := router.New(blue, green)

	// ── Step 1: Show current state ────────────────────────────────────────────
	fmt.Println("\n══ Initial State ══")
	showState(rt, blue, green)
	callLive(rt)

	// ── Step 2: Deploy v2 to idle (green) ─────────────────────────────────────
	fmt.Println("\n══ Deploy v2 to Green (idle) ══")
	green.Deploy("v2")
	slog.Info("deployed v2 to green", "green_version", green.Version(), "live", rt.LiveName())

	// ── Step 3: Switch traffic to green ───────────────────────────────────────
	fmt.Println("\n══ Switch Traffic (Blue → Green) ══")
	result, err := rt.Switch()
	if err != nil {
		slog.Error("switch failed", "error", err)
		os.Exit(1)
	}
	slog.Info("traffic switched",
		"from", result.From,
		"to", result.To,
		"smoke_tests", result.SmokeTests,
	)
	callLive(rt)

	// ── Step 4: Simulate a problem → rollback ─────────────────────────────────
	fmt.Println("\n══ Problem Detected → Rollback to Blue ══")
	rollbackResult := rt.Rollback()
	slog.Info("rolled back",
		"from", rollbackResult.From,
		"to", rollbackResult.To,
	)
	callLive(rt)

	// ── Final state ────────────────────────────────────────────────────────────
	fmt.Println("\n══ Final State ══")
	showState(rt, blue, green)
}

// callLive makes a request to the current live environment and prints the response.
func callLive(rt *router.Router) {
	resp, err := http.Get(rt.LiveURL() + "/api/hello")
	if err != nil {
		slog.Error("call failed", "error", err)
		return
	}
	defer resp.Body.Close()

	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	fmt.Printf("  Response: env=%s version=%s\n", body["environment"], body["version"])
}

func showState(rt *router.Router, blue, green *env.Environment) {
	fmt.Printf("  Blue:  version=%-5s live=%v\n", blue.Version(), blue.IsLive())
	fmt.Printf("  Green: version=%-5s live=%v\n", green.Version(), green.IsLive())
	fmt.Printf("  Traffic → %s (%s)\n", rt.LiveName(), rt.LiveVersion())
}
