// main.go — Distributed Tracing demo.
//
// Three services, each with its own tracer.
// A request goes: client → API Gateway → Order Service → Payment Service
// All three share the same Trace ID via X-Trace-ID header.
//
// HOW TO RUN:
//   go run ./services/09-distributed-tracing/cmd/main.go

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	"microservices-go/services/09-distributed-tracing/internal/tracer"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// Each service has its own tracer (in production each would run in its own process)
	gatewayTracer := tracer.New("api-gateway")
	orderTracer := tracer.New("order-svc")
	paymentTracer := tracer.New("payment-svc")

	// ── Payment Service ───────────────────────────────────────────────────────
	paymentSvc := httptest.NewServer(paymentTracer.Middleware(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			span, ctx := paymentTracer.StartSpan(r.Context(), "charge.card")
			defer paymentTracer.FinishSpan(span)

			time.Sleep(5 * time.Millisecond) // simulate DB work
			span.SetTag("amount", "$50.00")

			traceID := tracer.TraceIDFromContext(ctx)
			slog.Info("[payment-svc] charged card", "trace_id", traceID)

			json.NewEncoder(w).Encode(map[string]string{
				"status":   "charged",
				"trace_id": traceID,
			})
		}),
	))
	defer paymentSvc.Close()

	// ── Order Service ─────────────────────────────────────────────────────────
	orderSvc := httptest.NewServer(orderTracer.Middleware(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			span, ctx := orderTracer.StartSpan(r.Context(), "order.create")
			defer orderTracer.FinishSpan(span)

			time.Sleep(3 * time.Millisecond)

			// Call payment service, propagating the trace ID
			req, _ := http.NewRequestWithContext(ctx, "GET", paymentSvc.URL+"/charge", nil)
			tracer.InjectHeader(ctx, req) // inject X-Trace-ID header
			http.DefaultClient.Do(req)    //nolint

			traceID := tracer.TraceIDFromContext(ctx)
			slog.Info("[order-svc] created order", "trace_id", traceID)

			json.NewEncoder(w).Encode(map[string]string{
				"order_id": "ord-001",
				"trace_id": traceID,
			})
		}),
	))
	defer orderSvc.Close()

	// ── API Gateway ───────────────────────────────────────────────────────────
	gatewaySvc := httptest.NewServer(gatewayTracer.Middleware(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			span, ctx := gatewayTracer.StartSpan(r.Context(), "gateway.route")
			defer gatewayTracer.FinishSpan(span)

			// Forward to order service, propagating trace ID
			req, _ := http.NewRequestWithContext(ctx, "GET", orderSvc.URL+"/order", nil)
			tracer.InjectHeader(ctx, req)
			resp, _ := http.DefaultClient.Do(req)
			if resp != nil {
				resp.Body.Close()
			}

			traceID := tracer.TraceIDFromContext(ctx)
			slog.Info("[api-gateway] request complete", "trace_id", traceID)

			json.NewEncoder(w).Encode(map[string]string{
				"trace_id": traceID,
				"status":   "ok",
			})
		}),
	))
	defer gatewaySvc.Close()

	// ── Send a request ────────────────────────────────────────────────────────
	fmt.Println("\n═══ Sending request through 3 services ═══")

	resp, err := http.Get(gatewaySvc.URL + "/api/orders")
	if err != nil {
		slog.Error("request failed", "error", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	// Extract the trace ID from the response header
	traceID := resp.Header.Get(tracer.TraceIDHeader)
	fmt.Printf("\nTrace ID: %s\n", traceID)

	// ── Show all recorded spans ───────────────────────────────────────────────
	fmt.Println("\n═══ Recorded Spans ═══")
	allTracers := map[string]*tracer.Tracer{
		"api-gateway": gatewayTracer,
		"order-svc":   orderTracer,
		"payment-svc": paymentTracer,
	}

	for svcName, t := range allTracers {
		spans := t.GetTrace(traceID)
		for _, s := range spans {
			fmt.Printf("  [%s] %-20s  duration: %v\n",
				svcName, s.Operation, s.Duration.Round(time.Millisecond))
		}
	}

	// Show what a full trace view would look like
	fmt.Println("\n═══ Trace Timeline (simulated) ═══")
	visualizeTrace(traceID, gatewayTracer, orderTracer, paymentTracer)
}

func visualizeTrace(traceID string, tracers ...*tracer.Tracer) {
	var allSpans []*tracer.Span
	for _, t := range tracers {
		allSpans = append(allSpans, t.GetTrace(traceID)...)
	}

	if len(allSpans) == 0 {
		fmt.Println("  (no spans found — trace may be empty)")
		return
	}

	// Find the earliest start time
	earliest := allSpans[0].StartTime
	for _, s := range allSpans[1:] {
		if s.StartTime.Before(earliest) {
			earliest = s.StartTime
		}
	}

	for _, s := range allSpans {
		offset := s.StartTime.Sub(earliest).Round(time.Millisecond)
		errStr := ""
		if s.Error != nil {
			errStr = " [ERROR: " + s.Error.Error() + "]"
		}
		fmt.Printf("  %-12s  +%-6v  %-25s  %v%s\n",
			s.Service, offset, s.Operation, s.Duration.Round(time.Millisecond), errStr)
	}
}

// Using context background just for type-checking — no context needed here
var _ = context.Background
