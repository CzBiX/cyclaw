package agent

import (
	"testing"

	"cyclaw/internal/config"
	"cyclaw/internal/llm"
	"cyclaw/internal/session"
)

func TestAgent_GetSession(t *testing.T) {
	a := &Agent{
		Id:       "test",
		Sessions: session.NewMemStore(),
	}

	s1 := a.GetSession("chat1")
	if s1 == nil {
		t.Fatal("GetSession returned nil")
	}

	// Same chat ID should return the same session
	s2 := a.GetSession("chat1")
	if s1 != s2 {
		t.Error("GetSession returned different sessions for same chat ID")
	}

	// Different chat ID should return a new session
	s3 := a.GetSession("chat2")
	if s1 == s3 {
		t.Error("GetSession returned same session for different chat IDs")
	}
}

func TestAgent_ClearSession(t *testing.T) {
	a := &Agent{
		Id:       "test",
		Sessions: session.NewMemStore(),
	}

	s := a.GetSession("chat1")
	s.AppendHistory(llm.NewMessage(llm.RoleUser, "hello"))

	a.ClearSession("chat1")

	s = a.GetSession("chat1")
	if len(s.History) != 0 {
		t.Errorf("History length after clear = %d, want 0", len(s.History))
	}
}

func TestRouter_RegisterAndResolve(t *testing.T) {
	r := NewRouter()

	a := &Agent{
		Id:       "main",
		Config:   config.AgentConfig{Id: "main", Default: true},
		Sessions: session.NewMemStore(),
	}
	r.Register(a)

	// Default agent should be resolved for any chat
	got := r.Resolve("any-chat")
	if got == nil {
		t.Fatal("Resolve returned nil for default agent")
	}
	if got.Id != "main" {
		t.Errorf("Resolve().Id = %q, want %q", got.Id, "main")
	}
}

func TestRouter_GroupMapping(t *testing.T) {
	r := NewRouter()

	defaultAgent := &Agent{
		Id:       "default",
		Config:   config.AgentConfig{Id: "default", Default: true},
		Sessions: session.NewMemStore(),
	}
	groupAgent := &Agent{
		Id:       "group-agent",
		Config:   config.AgentConfig{Id: "group-agent", Groups: []string{"group-123"}},
		Sessions: session.NewMemStore(),
	}

	r.Register(defaultAgent)
	r.Register(groupAgent)

	// Group chat should map to the group agent
	got := r.Resolve("group-123")
	if got == nil {
		t.Fatal("Resolve returned nil for group chat")
	}
	if got.Id != "group-agent" {
		t.Errorf("Resolve().Id = %q, want %q", got.Id, "group-agent")
	}

	// Non-group chat should fall back to default
	got = r.Resolve("other-chat")
	if got == nil {
		t.Fatal("Resolve returned nil for non-group chat")
	}
	if got.Id != "default" {
		t.Errorf("Resolve().Id = %q, want %q", got.Id, "default")
	}
}

func TestRouter_GetAgent(t *testing.T) {
	r := NewRouter()

	a := &Agent{
		Id:       "main",
		Config:   config.AgentConfig{Id: "main", Default: true},
		Sessions: session.NewMemStore(),
	}
	r.Register(a)

	got, ok := r.GetAgent("main")
	if !ok {
		t.Fatal("GetAgent returned false")
	}
	if got.Id != "main" {
		t.Errorf("GetAgent().Id = %q, want %q", got.Id, "main")
	}

	_, ok = r.GetAgent("nonexistent")
	if ok {
		t.Fatal("GetAgent returned true for nonexistent agent")
	}
}

func TestRouter_ResolveNoAgents(t *testing.T) {
	r := NewRouter()
	got := r.Resolve("any")
	if got != nil {
		t.Error("Resolve should return nil when no agents are registered")
	}
}
