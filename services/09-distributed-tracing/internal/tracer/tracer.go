// tracer.go — Distributed tracing: spans, traces, and HTTP context propagation.
//
// This is a minimal in-process tracer. In production you'd send spans to
// Jaeger or Zipkin via OpenTelemetry. Here we store everything in memory
// so the demo runs without any external dependencies.
//
// Concepts:
//   Trace  = one end-to-end request (identified by TraceID)
//   Span   = one unit of work within a trace (has a start time and duration)
//   Context propagation = passing the TraceID through HTTP headers

package tracer

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net/http"
	"sync"
	"time"
)

// traceIDKey is the key used to store the Trace ID in a Go context.
// We use a private type so it can't collide with keys from other packages.
type traceIDKey struct{}

// spanKey is the key for the current active span in a context.
type spanKey struct{}

// TraceIDHeader is the HTTP header name for propagating the trace ID.
const TraceIDHeader = "X-Trace-ID"

// Span is a record of one unit of work.
type Span struct {
	TraceID   string            // which trace this span belongs to
	SpanID    string            // unique ID for this span
	Service   string            // which service created this span
	Operation string            // what operation was performed (e.g. "DB.Query", "HTTP.Call")
	StartTime time.Time         // when this span started
	Duration  time.Duration     // how long it took (set when the span ends)
	Error     error             // non-nil if this span represents a failure
	Tags      map[string]string // additional metadata (e.g. "http.status": "200")
}

// End marks the span as finished and records how long it took.
func (s *Span) End() {
	s.Duration = time.Since(s.StartTime)
}

// SetTag adds a key-value tag to the span.
func (s *Span) SetTag(key, value string) {
	if s.Tags == nil {
		s.Tags = make(map[string]string)
	}
	s.Tags[key] = value
}

// SetError marks the span as failed.
func (s *Span) SetError(err error) {
	s.Error = err
}

// Tracer creates spans and stores them in memory.
type Tracer struct {
	mu      sync.RWMutex
	service string
	spans   []*Span // all recorded spans (in a real system, these would be exported)
}

// New creates a tracer for the given service name.
func New(service string) *Tracer {
	return &Tracer{service: service}
}

// StartSpan begins a new span for the given operation.
// The traceID comes from the context (set by HTTP middleware or by the caller).
// If no traceID is found, a new one is generated (this is the root span).
func (t *Tracer) StartSpan(ctx context.Context, operation string) (*Span, context.Context) {
	traceID, _ := ctx.Value(traceIDKey{}).(string)
	if traceID == "" {
		traceID = newID() // this is the root span — generate a new trace ID
	}

	span := &Span{
		TraceID:   traceID,
		SpanID:    newID(),
		Service:   t.service,
		Operation: operation,
		StartTime: time.Now(),
	}

	// Store the current span in the context so child spans can reference it
	ctx = context.WithValue(ctx, traceIDKey{}, traceID)
	ctx = context.WithValue(ctx, spanKey{}, span)

	return span, ctx
}

// FinishSpan ends the span and records it in the tracer's store.
// Always call this after StartSpan, typically via defer:
//
//	span, ctx := tracer.StartSpan(ctx, "DB.Query")
//	defer tracer.FinishSpan(span)
func (t *Tracer) FinishSpan(span *Span) {
	span.End()
	t.mu.Lock()
	t.spans = append(t.spans, span)
	t.mu.Unlock()
}

// GetTrace returns all spans for a given trace ID.
// In a real system this would query Jaeger or Zipkin.
func (t *Tracer) GetTrace(traceID string) []*Span {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var result []*Span
	for _, s := range t.spans {
		if s.TraceID == traceID {
			result = append(result, s)
		}
	}
	return result
}

// AllSpans returns every recorded span (for tests/debug).
func (t *Tracer) AllSpans() []*Span {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make([]*Span, len(t.spans))
	copy(result, t.spans)
	return result
}

// ── HTTP Integration ──────────────────────────────────────────────────────────

// Middleware wraps an HTTP handler to:
//  1. Extract the trace ID from X-Trace-ID header (or generate a new one)
//  2. Inject it into the request context
//  3. Start a root span for this service's handling of the request
//  4. Add the trace ID to the response headers (for debugging)
func (t *Tracer) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract or generate trace ID
		traceID := r.Header.Get(TraceIDHeader)
		if traceID == "" {
			traceID = newID()
		}

		// Put trace ID in context
		ctx := context.WithValue(r.Context(), traceIDKey{}, traceID)

		// Start a span for this request
		span, ctx := t.StartSpan(ctx, r.Method+" "+r.URL.Path)
		span.SetTag("http.method", r.Method)
		span.SetTag("http.url", r.URL.String())
		defer t.FinishSpan(span)

		// Add trace ID to response so client can log it
		w.Header().Set(TraceIDHeader, traceID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// InjectHeader adds the current trace ID to an outgoing HTTP request.
// Call this before making HTTP calls to downstream services so the trace
// continues through the next service.
//
//	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
//	tracer.InjectHeader(ctx, req)
//	http.DefaultClient.Do(req)
func InjectHeader(ctx context.Context, req *http.Request) {
	if traceID, ok := ctx.Value(traceIDKey{}).(string); ok {
		req.Header.Set(TraceIDHeader, traceID)
	}
}

// TraceIDFromContext extracts the trace ID from a context.
// Returns empty string if no trace ID is set.
func TraceIDFromContext(ctx context.Context) string {
	traceID, _ := ctx.Value(traceIDKey{}).(string)
	return traceID
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// newID generates a short random hex ID (8 bytes = 16 hex chars).
func newID() string {
	return fmt.Sprintf("%016x", rand.Uint64())
}
