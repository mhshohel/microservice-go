// tracing_test.go — Tests for distributed tracing.

package tracing_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"microservices-go/services/09-distributed-tracing/internal/tracer"
)

// ── Span Tests ────────────────────────────────────────────────────────────────

func TestSpan_StartAndEnd(t *testing.T) {
	tr := tracer.New("test-svc")

	span, _ := tr.StartSpan(context.Background(), "do.work")
	time.Sleep(2 * time.Millisecond) // ensure some duration
	tr.FinishSpan(span)

	if span.Duration == 0 {
		t.Error("expected non-zero duration after FinishSpan")
	}
	if span.Operation != "do.work" {
		t.Errorf("expected operation 'do.work', got %q", span.Operation)
	}
	if span.Service != "test-svc" {
		t.Errorf("expected service 'test-svc', got %q", span.Service)
	}
}

func TestSpan_NewTraceIDGenerated_WhenNoneInContext(t *testing.T) {
	tr := tracer.New("test-svc")

	span, _ := tr.StartSpan(context.Background(), "root.op")
	tr.FinishSpan(span)

	if span.TraceID == "" {
		t.Error("expected a trace ID to be generated for root span")
	}
}

func TestSpan_TraceIDPropagatedThroughContext(t *testing.T) {
	tr := tracer.New("test-svc")

	// Start a root span — generates a trace ID
	rootSpan, ctx := tr.StartSpan(context.Background(), "root.op")
	tr.FinishSpan(rootSpan)

	// Start a child span in the same context — should inherit the trace ID
	childSpan, _ := tr.StartSpan(ctx, "child.op")
	tr.FinishSpan(childSpan)

	if rootSpan.TraceID == "" {
		t.Error("root span should have a trace ID")
	}
	if childSpan.TraceID != rootSpan.TraceID {
		t.Errorf("child span should share root span's trace ID: got %q, want %q",
			childSpan.TraceID, rootSpan.TraceID)
	}
}

func TestSpan_Tags(t *testing.T) {
	tr := tracer.New("test-svc")
	span, _ := tr.StartSpan(context.Background(), "op")
	span.SetTag("http.status", "200")
	span.SetTag("user.id", "alice")
	tr.FinishSpan(span)

	if span.Tags["http.status"] != "200" {
		t.Errorf("expected tag http.status=200, got %q", span.Tags["http.status"])
	}
	if span.Tags["user.id"] != "alice" {
		t.Errorf("expected tag user.id=alice, got %q", span.Tags["user.id"])
	}
}

func TestSpan_Error(t *testing.T) {
	tr := tracer.New("test-svc")
	span, _ := tr.StartSpan(context.Background(), "op")
	span.SetError(http.ErrServerClosed)
	tr.FinishSpan(span)

	if span.Error == nil {
		t.Error("expected span to have an error")
	}
}

// ── Tracer Store Tests ────────────────────────────────────────────────────────

func TestTracer_GetTrace_ReturnsSpansForTraceID(t *testing.T) {
	tr := tracer.New("svc")

	span1, ctx1 := tr.StartSpan(context.Background(), "op1")
	span2, _ := tr.StartSpan(ctx1, "op2")
	tr.FinishSpan(span1)
	tr.FinishSpan(span2)

	// Different trace — should not be returned
	span3, _ := tr.StartSpan(context.Background(), "other-trace-op")
	tr.FinishSpan(span3)

	spans := tr.GetTrace(span1.TraceID)
	if len(spans) != 2 {
		t.Errorf("expected 2 spans for trace %s, got %d", span1.TraceID, len(spans))
	}

	for _, s := range spans {
		if s.TraceID != span1.TraceID {
			t.Errorf("GetTrace returned span from wrong trace: %s", s.TraceID)
		}
	}
}

func TestTracer_AllSpans_ReturnsEverything(t *testing.T) {
	tr := tracer.New("svc")

	for range 5 {
		span, _ := tr.StartSpan(context.Background(), "op")
		tr.FinishSpan(span)
	}

	if len(tr.AllSpans()) != 5 {
		t.Errorf("expected 5 total spans, got %d", len(tr.AllSpans()))
	}
}

// ── HTTP Middleware Tests ─────────────────────────────────────────────────────

func TestMiddleware_GeneratesTraceID(t *testing.T) {
	tr := tracer.New("test-svc")

	srv := httptest.NewServer(tr.Middleware(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("ok"))
		}),
	))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	traceID := resp.Header.Get(tracer.TraceIDHeader)
	if traceID == "" {
		t.Error("expected X-Trace-ID header in response, got empty")
	}
}

func TestMiddleware_PropagatesExistingTraceID(t *testing.T) {
	tr := tracer.New("downstream-svc")

	srv := httptest.NewServer(tr.Middleware(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// The trace ID should come from the request, not be newly generated
			traceID := tracer.TraceIDFromContext(r.Context())
			w.Header().Set("X-Got-TraceID", traceID)
			w.Write([]byte("ok"))
		}),
	))
	defer srv.Close()

	// Send a request with a pre-existing trace ID
	req, _ := http.NewRequest("GET", srv.URL, nil)
	req.Header.Set(tracer.TraceIDHeader, "existing-trace-123")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	gotTraceID := resp.Header.Get("X-Got-TraceID")
	if gotTraceID != "existing-trace-123" {
		t.Errorf("expected trace ID to be propagated as 'existing-trace-123', got %q", gotTraceID)
	}
}

func TestMiddleware_RecordsSpanForRequest(t *testing.T) {
	tr := tracer.New("test-svc")

	srv := httptest.NewServer(tr.Middleware(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("ok"))
		}),
	))
	defer srv.Close()

	resp, _ := http.Get(srv.URL + "/some/path")
	resp.Body.Close()

	spans := tr.AllSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span recorded, got %d", len(spans))
	}
	if spans[0].Service != "test-svc" {
		t.Errorf("expected service 'test-svc', got %q", spans[0].Service)
	}
}

// ── Header Injection Tests ────────────────────────────────────────────────────

func TestInjectHeader_AddsTraceIDToRequest(t *testing.T) {
	tr := tracer.New("caller-svc")
	_, ctx := tr.StartSpan(context.Background(), "root")

	req, _ := http.NewRequest("GET", "http://example.com", nil)
	tracer.InjectHeader(ctx, req)

	if req.Header.Get(tracer.TraceIDHeader) == "" {
		t.Error("expected X-Trace-ID to be injected into outgoing request")
	}
}

func TestInjectHeader_EmptyContext_NoHeader(t *testing.T) {
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	tracer.InjectHeader(context.Background(), req) // no trace ID in context

	if req.Header.Get(tracer.TraceIDHeader) != "" {
		t.Error("expected no X-Trace-ID for context without trace ID")
	}
}

// ── End-to-End Propagation Test ───────────────────────────────────────────────

func TestTracing_EndToEnd_SameTraceIDAcrossServices(t *testing.T) {
	// Two "services" each with their own tracer
	upstreamTracer := tracer.New("upstream-svc")
	downstreamTracer := tracer.New("downstream-svc")

	// Downstream service records the trace ID it received
	var receivedTraceID string
	downstream := httptest.NewServer(downstreamTracer.Middleware(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedTraceID = tracer.TraceIDFromContext(r.Context())
			w.Write([]byte("ok"))
		}),
	))
	defer downstream.Close()

	// Upstream service calls downstream, propagating trace ID
	upstream := httptest.NewServer(upstreamTracer.Middleware(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			span, ctx := upstreamTracer.StartSpan(r.Context(), "call.downstream")
			defer upstreamTracer.FinishSpan(span)

			req, _ := http.NewRequestWithContext(ctx, "GET", downstream.URL, nil)
			tracer.InjectHeader(ctx, req)
			resp, _ := http.DefaultClient.Do(req)
			if resp != nil {
				resp.Body.Close()
			}
			w.Write([]byte("ok"))
		}),
	))
	defer upstream.Close()

	// Client sends request to upstream
	resp, err := http.Get(upstream.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	sentTraceID := resp.Header.Get(tracer.TraceIDHeader)
	resp.Body.Close()

	// Allow time for async handling
	time.Sleep(10 * time.Millisecond)

	if sentTraceID == "" {
		t.Fatal("expected trace ID in upstream response")
	}
	if receivedTraceID != sentTraceID {
		t.Errorf("downstream received trace ID %q, but upstream sent %q — propagation failed",
			receivedTraceID, sentTraceID)
	}
}
