---
name: go
description: >
  Go 1.26.2 language guidance for this microservices project. Use this skill whenever
  writing, reviewing, or explaining any Go code in this repository — including service
  implementation, HTTP handlers, tests, config loading, error handling, concurrency, or
  any stdlib usage. Also trigger this when the user asks how to do something in Go, asks
  why Go code works a certain way, or wants to know the idiomatic/modern approach for a
  Go pattern. If the user is writing Go and you are unsure whether to consult this skill,
  consult it.
---

# Go 1.26.2 — Project Skill

This project uses Go 1.26.2 (`go 1.26` in `go.mod`). All code must be idiomatic,
beginner-readable, and use the latest stdlib features — no third-party libraries when the
standard library covers the need.

---

## Language Features by Version (all apply at 1.26)

### Go 1.21 — New builtins and packages
```go
// min, max, clear are now built-in — no need to write helpers
smallest := min(a, b)
largest  := max(a, b, c)
clear(mySlice) // zeroes all elements

// slices package replaces hand-rolled slice helpers
import "slices"
slices.Sort(items)
slices.Contains(items, target)
slices.Reverse(items)

// maps package replaces hand-rolled map helpers
import "maps"
maps.Keys(m)        // returns all keys as a slice
maps.Values(m)      // returns all values as a slice
maps.Clone(m)       // shallow copy

// cmp package for ordered comparisons
import "cmp"
cmp.Compare(a, b)   // -1, 0, or 1

// log/slog for structured logging (replaces log.Printf everywhere)
import "log/slog"
slog.Info("user created", "userID", id, "email", email)
```

### Go 1.22 — Range and routing improvements
```go
// Range over an integer — no more for i := 0; i < n; i++
for i := range 10 {
    fmt.Println(i) // 0, 1, 2 … 9
}

// Loop variables are now scoped per-iteration — the old closure bug is fixed
// This works correctly now (no need for v := v inside the loop):
for _, v := range items {
    go func() { process(v) }()
}

// net/http.ServeMux now supports method + path pattern routing
// No third-party router needed for basic REST APIs
mux := http.NewServeMux()
mux.HandleFunc("GET /users/{id}", getUserHandler)   // GET only, {id} is a path variable
mux.HandleFunc("POST /users", createUserHandler)    // POST only
mux.HandleFunc("DELETE /users/{id}", deleteHandler) // DELETE only

// Read the path variable from the request
func getUserHandler(w http.ResponseWriter, r *http.Request) {
    userID := r.PathValue("id") // built-in — no external router needed
}

// math/rand/v2 — use instead of math/rand (no global seed needed)
import "math/rand/v2"
n := rand.IntN(100) // random int in [0, 100)
```

### Go 1.23 — Iterators
```go
// Range over custom iterators using the iter package
// Your own types can now be ranged over with a for-range loop
import "iter"

// Define a type that yields values one at a time
func (c *Collection) All() iter.Seq[Item] {
    return func(yield func(Item) bool) {
        for _, item := range c.items {
            if !yield(item) {
                return // caller broke out of the loop
            }
        }
    }
}

// Now users can range over it naturally
for item := range collection.All() {
    fmt.Println(item)
}

// slices.Collect and maps.Collect convert iterators to slices/maps
all := slices.Collect(collection.All())
```

### Go 1.24 — Generic aliases and weak references
```go
// Generic type aliases are fully supported
type StringMap[V any] = map[string]V   // alias, not a new type
type Pair[A, B any] = struct{ First A; Second B }

// weak package for cache-friendly data structures
// (advanced — only use when you need GC-friendly caching)
import "weak"
ref := weak.Make(&expensiveObject)
if ptr := ref.Value(); ptr != nil {
    // object is still alive, use it
}
```

---

## Microservice Patterns

### HTTP Server Setup
```go
// main.go — straightforward server setup, no magic
func main() {
    // Load config from environment variables
    cfg := config.Load()

    // Set up structured logging first so all logs are JSON
    logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
        Level: slog.LevelInfo,
    }))
    slog.SetDefault(logger)

    // Register routes — method + path, readable at a glance
    mux := http.NewServeMux()
    mux.HandleFunc("GET /health", healthHandler)
    mux.HandleFunc("GET /api/v1/users/{id}", getUserHandler)
    mux.HandleFunc("POST /api/v1/users", createUserHandler)

    // Set timeouts — always do this to prevent slow clients from blocking the server
    server := &http.Server{
        Addr:         ":" + cfg.Port,
        Handler:      mux,
        ReadTimeout:  5 * time.Second,  // max time to read the full request
        WriteTimeout: 10 * time.Second, // max time to write the full response
        IdleTimeout:  60 * time.Second, // max time between requests on a keep-alive connection
    }

    slog.Info("server starting", "port", cfg.Port)

    // Start the server and handle graceful shutdown
    if err := runServer(server); err != nil {
        slog.Error("server stopped", "error", err)
        os.Exit(1)
    }
}

// runServer starts the server and waits for an OS signal to shut down cleanly.
// "Graceful shutdown" means: stop accepting new requests, but finish the ones already running.
func runServer(server *http.Server) error {
    // signal.NotifyContext returns a context that is cancelled when the program receives
    // SIGINT (Ctrl+C) or SIGTERM (what Docker/Kubernetes sends when stopping a container)
    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    // Run the server in a goroutine so we can wait for the shutdown signal below
    serverError := make(chan error, 1)
    go func() {
        if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
            serverError <- err
        }
    }()

    // Block here until either the server crashes or we receive a shutdown signal
    select {
    case err := <-serverError:
        return err
    case <-ctx.Done():
        slog.Info("shutting down...")
    }

    // Give in-flight requests up to 10 seconds to finish before we force-quit
    shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    return server.Shutdown(shutdownCtx)
}
```

### Structured Logging with slog
```go
// Always use slog — never fmt.Println or log.Printf in service code.
// Key-value pairs make logs searchable in log aggregators (Grafana, Datadog, etc.)

// Info: normal operations
slog.Info("request received", "method", r.Method, "path", r.URL.Path)

// Warn: something unexpected but recoverable
slog.Warn("slow query detected", "duration_ms", elapsed.Milliseconds(), "query", sql)

// Error: something failed — always include the error
slog.Error("database query failed", "error", err, "userID", userID)

// Add a trace ID to every log so you can follow a request across services
slog.Info("user fetched",
    "traceID", r.Header.Get("X-Trace-ID"),
    "userID", userID,
    "duration_ms", elapsed.Milliseconds(),
)
```

### Error Wrapping
```go
// Always wrap errors with %w so the caller can inspect the error chain.
// The format is: "what you were doing: original error"
func getUserFromDB(ctx context.Context, id string) (*User, error) {
    user, err := db.QueryUser(ctx, id)
    if err != nil {
        // %w wraps the error — the caller can use errors.Is() or errors.As() to check it
        return nil, fmt.Errorf("getUserFromDB id=%s: %w", id, err)
    }
    return user, nil
}

// Checking for specific error types at the handler boundary
user, err := service.GetUser(ctx, userID)
if err != nil {
    // errors.Is walks the full error chain — works even if the error was wrapped multiple times
    if errors.Is(err, ErrNotFound) {
        http.Error(w, "user not found", http.StatusNotFound)
        return
    }
    // errors.As extracts a specific error type from the chain
    var validationErr *ValidationError
    if errors.As(err, &validationErr) {
        http.Error(w, validationErr.Message, http.StatusBadRequest)
        return
    }
    slog.Error("unexpected error fetching user", "error", err, "userID", userID)
    http.Error(w, "internal server error", http.StatusInternalServerError)
}
```

### Context Propagation
```go
// Context carries deadlines, cancellation signals, and request-scoped values.
// Rule: context.Context is ALWAYS the first parameter. Never store it in a struct.

// Service layer: accept context, pass it down to every I/O call
func (s *UserService) GetUser(ctx context.Context, userID string) (*User, error) {
    // Pass ctx to the repository — if the HTTP request is cancelled, the DB query cancels too
    return s.repo.FindByID(ctx, userID)
}

// Repository layer: pass ctx to every database call
func (r *UserRepository) FindByID(ctx context.Context, id string) (*User, error) {
    row := r.db.QueryRowContext(ctx, "SELECT id, name, email FROM users WHERE id = $1", id)
    // ...
}

// Handler layer: use r.Context() to get the request context
func getUserHandler(w http.ResponseWriter, r *http.Request) {
    userID := r.PathValue("id")
    // r.Context() is automatically cancelled if the client disconnects
    user, err := userService.GetUser(r.Context(), userID)
    // ...
}
```

### Config from Environment Variables
```go
// config/config.go — load all config at startup, fail fast if required vars are missing

// Config holds all settings for this service.
// Using a struct makes it easy to see all config in one place and pass it around.
type Config struct {
    Port        string
    DatabaseURL string
    LogLevel    slog.Level
}

// Load reads config from environment variables.
// Call this once at startup — if required vars are missing, the program exits immediately.
func Load() Config {
    return Config{
        Port:        getEnv("PORT", "8080"),       // optional — has a sensible default
        DatabaseURL: mustEnv("DATABASE_URL"),       // required — crash if missing
        LogLevel:    parseLogLevel(getEnv("LOG_LEVEL", "info")),
    }
}

// mustEnv returns the value of an env var, or panics with a clear message if it's not set.
// We panic at startup rather than silently using a zero value — misconfigured services
// should fail loudly before they do any real work.
func mustEnv(key string) string {
    value := os.Getenv(key)
    if value == "" {
        panic("required environment variable not set: " + key)
    }
    return value
}

// getEnv returns the value of an env var, or the fallback if it's not set.
func getEnv(key, fallback string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return fallback
}
```

---

## Stdlib-First: slices and maps

Always prefer the standard library over manual loops or third-party packages:

```go
// Sorting
slices.Sort(names)                                             // sort strings/ints in place
slices.SortFunc(users, func(a, b User) int {                   // sort by custom field
    return cmp.Compare(a.Name, b.Name)
})
slices.SortStableFunc(items, compareFn) // stable sort (preserves order of equal elements)

// Searching
found := slices.Contains(names, "Alice")                       // true/false
idx, found := slices.BinarySearch(sortedNames, "Alice")        // fast search in sorted slice
idx := slices.Index(names, "Alice")                            // first index, or -1

// Transforming
filtered := slices.DeleteFunc(users, func(u User) bool {       // remove elements matching a condition
    return u.Age < 18
})
slices.Reverse(items) // reverse in place

// Maps
allKeys := maps.Keys(userMap)     // []string of all keys
allValues := maps.Values(userMap) // []User of all values
copy := maps.Clone(userMap)       // shallow copy of the map
maps.DeleteFunc(userMap, func(k string, v User) bool { // delete matching entries
    return v.Inactive
})
```

---

## Testing

Every service must be tested. The pattern is: **Arrange → Act → Assert**.

```go
// user_service_test.go — testing the circuit breaker state machine

func TestCircuitBreaker_OpensAfterFailures(t *testing.T) {
    // Arrange — set up the thing we're testing with a low threshold so the test runs fast.
    // We don't want to wait for 100 real failures; 3 is enough to prove the behaviour.
    breaker := NewCircuitBreaker(Config{
        FailureThreshold: 3,
        Timeout:          100 * time.Millisecond,
    })

    // Act — trigger the behaviour we want to test.
    // Simulate 3 consecutive failures to trip the breaker.
    for range 3 {
        breaker.RecordFailure()
    }

    // Assert — check that the system ended up in the state we expected.
    if breaker.State() != StateOpen {
        t.Fatalf("expected circuit breaker to be OPEN after 3 failures, got %s", breaker.State())
    }
}

// Table-driven tests are great when you want to test many inputs with the same logic.
// Each row in the table is one test case — you add new cases without writing new functions.
func TestParseUserID(t *testing.T) {
    tests := []struct {
        name    string // what this case is testing
        input   string
        wantID  int
        wantErr bool  // true if we expect an error
    }{
        {name: "valid numeric ID",    input: "42",   wantID: 42,  wantErr: false},
        {name: "empty string",        input: "",     wantID: 0,   wantErr: true},
        {name: "non-numeric string",  input: "abc",  wantID: 0,   wantErr: true},
        {name: "negative number",     input: "-1",   wantID: 0,   wantErr: true},
    }

    for _, tc := range tests {
        // t.Run creates a sub-test — you can run one case with: go test -run TestParseUserID/valid
        t.Run(tc.name, func(t *testing.T) {
            t.Parallel() // run cases concurrently — makes the test suite faster

            gotID, err := parseUserID(tc.input)

            // Check error expectation
            if tc.wantErr && err == nil {
                t.Fatalf("expected an error for input %q, but got none", tc.input)
            }
            if !tc.wantErr && err != nil {
                t.Fatalf("did not expect an error for input %q, but got: %v", tc.input, err)
            }

            // Check the returned value (only when no error expected)
            if !tc.wantErr && gotID != tc.wantID {
                t.Errorf("parseUserID(%q) = %d, want %d", tc.input, gotID, tc.wantID)
            }
        })
    }
}
```

Run tests with the race detector — catches data races in concurrent code:
```bash
go test ./services/XX-name/... -v -race
```

---

## Do / Don't

| Do | Don't |
|----|-------|
| `slog.Info("msg", "key", val)` | `fmt.Println(...)` or `log.Printf(...)` in service code |
| `mux.HandleFunc("GET /users/{id}", h)` | Import `gorilla/mux` or `chi` for basic routing |
| `slices.Sort(items)` | Write a manual sort loop |
| `for i := range 10 { }` | `for i := 0; i < 10; i++ { }` |
| `fmt.Errorf("op %s: %w", id, err)` | `errors.New(err.Error())` — this loses the error chain |
| `ctx context.Context` as first parameter | Store context in a struct field |
| `mustEnv("KEY")` — panic at startup if missing | Silent zero values for required config |
| `signal.NotifyContext(...)` for shutdown | `os.Exit(1)` from inside a handler |
| Comment WHY concurrency is needed | Leave mutex/goroutine usage unexplained |
