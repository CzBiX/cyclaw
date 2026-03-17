package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"cyclaw/internal/llm"
)

// Registry manages the registration and execution of tools.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry creates a new tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool to the registry.
func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name()] = t
	slog.Debug("tool registered", "name", t.Name())
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// Execute runs a tool by name with the given parameters.
func (r *Registry) Execute(ctx context.Context, name string, params json.RawMessage) (string, error) {
	t, ok := r.Get(name)
	if !ok {
		return "", fmt.Errorf("tool not found: %s", name)
	}
	return t.Execute(ctx, params)
}

// LLMDefs returns all registered tools as LLM tool definitions.
func (r *Registry) LLMDefs() []llm.FunctionDef {
	r.mu.RLock()
	defer r.mu.RUnlock()

	defs := make([]llm.FunctionDef, 0, len(r.tools))
	for _, t := range r.tools {
		defs = append(defs, ToLLMDef(t))
	}
	return defs
}

// Names returns the names of all registered tools.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}
