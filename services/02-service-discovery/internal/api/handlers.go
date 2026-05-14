// handlers.go — HTTP handlers for the service registry API.
//
// Each handler is a thin wrapper around the Registry: it decodes the request,
// calls the right registry method, and writes a JSON response.
//
// The handlers use Go's standard "encoding/json" — no external dependencies.
//
// Error responses always have this shape:
//
//	{"error": "reason why it failed"}
//
// Success responses vary by endpoint but always use the structures defined here.

package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"microservices-go/services/02-service-discovery/internal/registry"
)

// Handler holds the registry and exposes HTTP methods over it.
// It's a struct so all handlers share the same registry instance.
type Handler struct {
	reg *registry.Registry
}

// New creates a Handler wired to the given registry.
func New(reg *registry.Registry) *Handler {
	return &Handler{reg: reg}
}

// RegisterRoutes attaches all handlers to the given mux.
//
// We use Go 1.22's "METHOD /path" pattern syntax so the router automatically
// rejects wrong methods (e.g., GET /register → 405 Method Not Allowed).
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", h.handleHealth)
	mux.HandleFunc("POST /register", h.handleRegister)
	mux.HandleFunc("DELETE /deregister/{id}", h.handleDeregister)
	mux.HandleFunc("PUT /heartbeat/{id}", h.handleHeartbeat)
	mux.HandleFunc("GET /services/{name}", h.handleGetByName)
	mux.HandleFunc("GET /services", h.handleGetAll)
}

// ── Request / Response types ──────────────────────────────────────────────────

// registerRequest is what the client sends in the POST /register body.
type registerRequest struct {
	Name     string            `json:"name"`
	Address  string            `json:"address"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// instanceResponse is what we send back after a successful register or query.
// We convert *registry.Instance to this type so the JSON field names are clean.
type instanceResponse struct {
	ID           string            `json:"instance_id"`
	Name         string            `json:"name"`
	Address      string            `json:"address"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	RegisteredAt time.Time         `json:"registered_at"`
}

// toResponse converts the internal Instance type to the HTTP response type.
func toResponse(inst *registry.Instance) instanceResponse {
	return instanceResponse{
		ID:           inst.ID,
		Name:         inst.Name,
		Address:      inst.Address,
		Metadata:     inst.Metadata,
		RegisteredAt: inst.RegisteredAt,
	}
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// handleHealth responds with a simple status check.
// Load balancers ping this to know if the registry process is alive.
func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"service": "service-registry",
		"count":   h.reg.Count(), // how many instances are currently registered
	})
}

// handleRegister accepts a new service instance into the registry.
//
// Request:  POST /register   Body: {"name":"user-svc","address":"localhost:9001"}
// Response: 201 Created      Body: {"instance_id":"...","name":"...","address":"..."}
func (h *Handler) handleRegister(w http.ResponseWriter, r *http.Request) {
	// Decode the JSON body into our request struct
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}

	// Ask the registry to create the instance
	inst, err := h.reg.Register(registry.RegisterRequest{
		Name:     req.Name,
		Address:  req.Address,
		Metadata: req.Metadata,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	slog.Info("registered instance", "id", inst.ID, "name", inst.Name, "address", inst.Address)

	// 201 Created is more accurate than 200 OK for a new resource
	writeJSON(w, http.StatusCreated, toResponse(inst))
}

// handleDeregister removes a specific service instance from the registry.
//
// Request:  DELETE /deregister/{id}
// Response: 200 OK   Body: {"message":"deregistered"}
//
// The {id} is extracted from the URL path using r.PathValue (Go 1.22+).
func (h *Handler) handleDeregister(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing instance id in path")
		return
	}

	if err := h.reg.Deregister(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	slog.Info("deregistered instance", "id", id)
	writeJSON(w, http.StatusOK, map[string]string{"message": "deregistered", "instance_id": id})
}

// handleHeartbeat resets the "last seen" timer for a service instance.
//
// Request:  PUT /heartbeat/{id}
// Response: 200 OK   Body: {"message":"ok"}
func (h *Handler) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing instance id in path")
		return
	}

	if err := h.reg.Heartbeat(id); err != nil {
		// 404 if the ID doesn't exist (e.g., it already timed out and was removed)
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "ok", "instance_id": id})
}

// handleGetByName returns all registered instances for the given service name.
//
// Request:  GET /services/{name}
// Response: 200 OK   Body: [{"instance_id":"...","address":"..."},...]
//
// This supports both discovery patterns:
//   - Client-side: caller gets all instances and picks one
//   - Server-side: the gateway can call PickOne on the registry
func (h *Handler) handleGetByName(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "missing service name in path")
		return
	}

	instances := h.reg.GetByName(name)

	// Build the response list
	result := make([]instanceResponse, 0, len(instances))
	for _, inst := range instances {
		result = append(result, toResponse(inst))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"name":      name,
		"count":     len(result),
		"instances": result,
	})
}

// handleGetAll returns every registered instance across all services.
//
// Request:  GET /services
// Response: 200 OK   Body: {"count":3,"instances":[...]}
func (h *Handler) handleGetAll(w http.ResponseWriter, r *http.Request) {
	instances := h.reg.GetAll()

	result := make([]instanceResponse, 0, len(instances))
	for _, inst := range instances {
		result = append(result, toResponse(inst))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"count":     len(result),
		"instances": result,
	})
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// writeJSON marshals v to JSON and writes it with the given HTTP status code.
// It always sets Content-Type: application/json.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to write JSON response", "error", err)
	}
}

// writeError writes a JSON error response: {"error": "message"}.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
