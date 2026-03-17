package tool

import (
	"context"
	"encoding/json"
	"testing"
)

// mockTool is a simple Tool implementation for testing.
type mockTool struct {
	name        string
	description string
	params      json.RawMessage
	execResult  string
	execErr     error
}

func (m *mockTool) Name() string                { return m.name }
func (m *mockTool) Description() string         { return m.description }
func (m *mockTool) Parameters() json.RawMessage { return m.params }
func (m *mockTool) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	return m.execResult, m.execErr
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	tool := &mockTool{name: "test_tool", description: "A test tool"}

	r.Register(tool)

	got, ok := r.Get("test_tool")
	if !ok {
		t.Fatal("Get() returned false for registered tool")
	}
	if got.Name() != "test_tool" {
		t.Errorf("Get().Name() = %q, want %q", got.Name(), "test_tool")
	}
}

func TestRegistry_GetNotFound(t *testing.T) {
	r := NewRegistry()

	_, ok := r.Get("nonexistent")
	if ok {
		t.Fatal("Get() returned true for non-registered tool")
	}
}

func TestRegistry_Execute(t *testing.T) {
	r := NewRegistry()
	tool := &mockTool{
		name:       "echo",
		execResult: "hello world",
	}
	r.Register(tool)

	result, err := r.Execute(context.Background(), "echo", nil)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if result != "hello world" {
		t.Errorf("Execute() = %q, want %q", result, "hello world")
	}
}

func TestRegistry_ExecuteNotFound(t *testing.T) {
	r := NewRegistry()

	_, err := r.Execute(context.Background(), "nonexistent", nil)
	if err == nil {
		t.Fatal("Execute() expected error for non-registered tool")
	}
}

func TestRegistry_Names(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTool{name: "alpha"})
	r.Register(&mockTool{name: "beta"})

	names := r.Names()
	if len(names) != 2 {
		t.Fatalf("Names() returned %d names, want 2", len(names))
	}

	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}
	if !nameSet["alpha"] || !nameSet["beta"] {
		t.Errorf("Names() = %v, want [alpha, beta]", names)
	}
}

func TestRegistry_LLMDefs(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTool{
		name:        "test",
		description: "A test tool",
		params:      json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
	})

	defs := r.LLMDefs()
	if len(defs) != 1 {
		t.Fatalf("LLMDefs() returned %d defs, want 1", len(defs))
	}

	def := defs[0]
	if def.Name != "test" {
		t.Errorf("def.Function.Name = %q, want %q", def.Name, "test")
	}
	if def.Description != "A test tool" {
		t.Errorf("def.Function.Description = %q, want %q", def.Description, "A test tool")
	}
}
