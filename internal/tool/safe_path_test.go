package tool

import (
	"path/filepath"
	"testing"
)

func TestResolveAndValidate_ValidPaths(t *testing.T) {
	dataDir := "/tmp/testdata"

	tests := []struct {
		name       string
		agentID    string
		path       string
		wantPfx    string
		wantSuffix string
	}{
		{
			name:       "memory path",
			agentID:    "main",
			path:       "@memory/MEMORY.md",
			wantPfx:    "memory",
			wantSuffix: filepath.Join("memory", "MEMORY.md"),
		},
		{
			name:       "workspace path",
			agentID:    "main",
			path:       "@workspace/notes.txt",
			wantPfx:    "workspace",
			wantSuffix: filepath.Join("workspace", "notes.txt"),
		},
		{
			name:       "skills path",
			agentID:    "main",
			path:       "@skills/coding/SKILL.md",
			wantPfx:    "skills",
			wantSuffix: filepath.Join("skills", "coding", "SKILL.md"),
		},
		{
			name:       "agent path",
			agentID:    "main",
			path:       "@agent/AGENT.md",
			wantPfx:    "agent",
			wantSuffix: filepath.Join("agents", "main", "AGENT.md"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved, pfx, err := resolveAndValidate(dataDir, tt.agentID, tt.path)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pfx != tt.wantPfx {
				t.Errorf("prefix = %q, want %q", pfx, tt.wantPfx)
			}
			wantEnd := filepath.Join(dataDir, tt.wantSuffix)
			absWant, _ := filepath.Abs(wantEnd)
			if resolved != absWant {
				t.Errorf("resolved = %q, want suffix %q", resolved, absWant)
			}
		})
	}
}

func TestResolveAndValidate_Errors(t *testing.T) {
	dataDir := "/tmp/testdata"

	tests := []struct {
		name    string
		agentID string
		path    string
	}{
		{name: "empty path", agentID: "main", path: ""},
		{name: "no @ prefix", agentID: "main", path: "memory/file.md"},
		{name: "unknown prefix", agentID: "main", path: "@unknown/file.md"},
		{name: "missing file part", agentID: "main", path: "@memory/"},
		{name: "prefix only", agentID: "main", path: "@memory"},
		{name: "agent without agent ID", agentID: "", path: "@agent/AGENT.md"},
		{name: "traversal attempt", agentID: "main", path: "@memory/../../etc/passwd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := resolveAndValidate(dataDir, tt.agentID, tt.path)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestWithAgentID_And_AgentIDFrom(t *testing.T) {
	ctx := WithAgentID(t.Context(), "test-agent")
	got := AgentIDFrom(ctx)
	if got != "test-agent" {
		t.Errorf("AgentIDFrom = %q, want %q", got, "test-agent")
	}
}
