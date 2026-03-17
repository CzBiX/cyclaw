package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadFileTool_Execute_ReadFile(t *testing.T) {
	dataDir := t.TempDir()

	memDir := filepath.Join(dataDir, "memory")
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(memDir, "test.txt"), []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := NewReadFileTool(dataDir)
	ctx := WithAgentID(t.Context(), "main")

	params, _ := json.Marshal(map[string]string{"path": "@memory/test.txt"})
	result, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if result != "hello world" {
		t.Errorf("Execute() = %q, want %q", result, "hello world")
	}
}

func TestReadFileTool_Execute_DirectoryRejected(t *testing.T) {
	dataDir := t.TempDir()

	wsDir := filepath.Join(dataDir, "workspace", "project")
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	tool := NewReadFileTool(dataDir)
	ctx := WithAgentID(t.Context(), "main")

	// Without trailing slash.
	params, _ := json.Marshal(map[string]string{"path": "@workspace/project"})
	_, err := tool.Execute(ctx, params)
	if err == nil {
		t.Fatal("expected error for directory path")
	}
	if !strings.Contains(err.Error(), "is a directory") {
		t.Errorf("error = %q, want to contain 'is a directory'", err.Error())
	}

	// With trailing slash.
	params, _ = json.Marshal(map[string]string{"path": "@workspace/project/"})
	_, err = tool.Execute(ctx, params)
	if err == nil {
		t.Fatal("expected error for directory path with trailing slash")
	}
	if !strings.Contains(err.Error(), "is a directory") {
		t.Errorf("error = %q, want to contain 'is a directory'", err.Error())
	}
}

func TestReadFileTool_Execute_AgentPath(t *testing.T) {
	dataDir := t.TempDir()

	agentDir := filepath.Join(dataDir, "agents", "myagent")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "AGENT.md"), []byte("# Agent"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := NewReadFileTool(dataDir)
	ctx := WithAgentID(t.Context(), "myagent")

	params, _ := json.Marshal(map[string]string{"path": "@agent/AGENT.md"})
	result, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if result != "# Agent" {
		t.Errorf("Execute() = %q, want %q", result, "# Agent")
	}
}

func TestReadFileTool_Execute_InvalidJSON(t *testing.T) {
	tool := NewReadFileTool(t.TempDir())
	ctx := WithAgentID(t.Context(), "main")

	_, err := tool.Execute(ctx, json.RawMessage(`{bad json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON params")
	}
	if !strings.Contains(err.Error(), "parse params") {
		t.Errorf("error = %q, want to contain 'parse params'", err.Error())
	}
}

func TestReadFileTool_Execute_InvalidPath(t *testing.T) {
	tool := NewReadFileTool(t.TempDir())
	ctx := WithAgentID(t.Context(), "main")

	tests := []struct {
		name string
		path string
	}{
		{"no @ prefix", "memory/file.md"},
		{"unknown prefix", "@unknown/file.md"},
		{"empty path", ""},
		{"prefix only", "@memory"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params, _ := json.Marshal(map[string]string{"path": tt.path})
			_, err := tool.Execute(ctx, params)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), "invalid path") {
				t.Errorf("error = %q, want to contain 'invalid path'", err.Error())
			}
		})
	}
}

func TestReadFileTool_Execute_NonexistentFile(t *testing.T) {
	dataDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dataDir, "memory"), 0o755); err != nil {
		t.Fatal(err)
	}

	tool := NewReadFileTool(dataDir)
	ctx := WithAgentID(t.Context(), "main")

	params, _ := json.Marshal(map[string]string{"path": "@memory/nonexistent.md"})
	_, err := tool.Execute(ctx, params)
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
	if !strings.Contains(err.Error(), "stat") {
		t.Errorf("error = %q, want to contain 'stat'", err.Error())
	}
}

func TestReadFileTool_Execute_TraversalBlocked(t *testing.T) {
	tool := NewReadFileTool(t.TempDir())
	ctx := WithAgentID(t.Context(), "main")

	params, _ := json.Marshal(map[string]string{"path": "@memory/../../etc/passwd"})
	_, err := tool.Execute(ctx, params)
	if err == nil {
		t.Fatal("expected error for path traversal attempt")
	}
}

func TestReadFileTool_Execute_AgentWithoutContext(t *testing.T) {
	tool := NewReadFileTool(t.TempDir())
	ctx := t.Context()

	params, _ := json.Marshal(map[string]string{"path": "@agent/AGENT.md"})
	_, err := tool.Execute(ctx, params)
	if err == nil {
		t.Fatal("expected error when @agent used without agent context")
	}
}
