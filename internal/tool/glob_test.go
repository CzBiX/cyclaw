package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupGlobTestDir creates a test directory tree under dataDir for glob tests.
//
//	memory/
//	  MEMORY.md
//	  notes.txt
//	  sub/
//	    deep.md
//	    deep.txt
//	workspace/
//	  project/
//	    main.go
//	    util.go
//	    pkg/
//	      lib.go
//	agents/
//	  myagent/
//	    AGENT.md
//	    config.yml
func setupGlobTestDir(t *testing.T) string {
	t.Helper()
	dataDir := t.TempDir()

	dirs := []string{
		filepath.Join(dataDir, "memory", "sub"),
		filepath.Join(dataDir, "workspace", "project", "pkg"),
		filepath.Join(dataDir, "agents", "myagent"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	files := map[string]string{
		filepath.Join(dataDir, "memory", "MEMORY.md"):                   "# Memory",
		filepath.Join(dataDir, "memory", "notes.txt"):                   "notes",
		filepath.Join(dataDir, "memory", "sub", "deep.md"):              "# Deep",
		filepath.Join(dataDir, "memory", "sub", "deep.txt"):             "deep text",
		filepath.Join(dataDir, "workspace", "project", "main.go"):       "package main",
		filepath.Join(dataDir, "workspace", "project", "util.go"):       "package main",
		filepath.Join(dataDir, "workspace", "project", "pkg", "lib.go"): "package pkg",
		filepath.Join(dataDir, "agents", "myagent", "AGENT.md"):         "# Agent",
		filepath.Join(dataDir, "agents", "myagent", "config.yml"):       "name: myagent",
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	return dataDir
}

func TestGlobTool_Execute_SimpleWildcard(t *testing.T) {
	dataDir := setupGlobTestDir(t)
	tool := NewGlobTool(dataDir)
	ctx := WithAgentID(t.Context(), "myagent")

	params, _ := json.Marshal(map[string]string{"pattern": "@memory/*.md"})
	result, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if !strings.Contains(result, "@memory/MEMORY.md") {
		t.Errorf("result should contain @memory/MEMORY.md, got:\n%s", result)
	}
	// Should not match deep.md in sub/
	if strings.Contains(result, "deep.md") {
		t.Errorf("result should not contain deep.md (not recursive), got:\n%s", result)
	}
	// Should not match .txt files
	if strings.Contains(result, "notes.txt") {
		t.Errorf("result should not contain notes.txt, got:\n%s", result)
	}
}

func TestGlobTool_Execute_AllFilesInDir(t *testing.T) {
	dataDir := setupGlobTestDir(t)
	tool := NewGlobTool(dataDir)
	ctx := WithAgentID(t.Context(), "myagent")

	params, _ := json.Marshal(map[string]string{"pattern": "@memory/*"})
	result, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if !strings.Contains(result, "MEMORY.md") {
		t.Error("result should contain MEMORY.md")
	}
	if !strings.Contains(result, "notes.txt") {
		t.Error("result should contain notes.txt")
	}
	if !strings.Contains(result, "sub/") {
		t.Error("result should contain sub/ (directory)")
	}
}

func TestGlobTool_Execute_RecursiveDoublestar(t *testing.T) {
	dataDir := setupGlobTestDir(t)
	tool := NewGlobTool(dataDir)
	ctx := WithAgentID(t.Context(), "myagent")

	params, _ := json.Marshal(map[string]string{"pattern": "@memory/**/*.md"})
	result, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if !strings.Contains(result, "MEMORY.md") {
		t.Errorf("result should contain MEMORY.md, got:\n%s", result)
	}
	if !strings.Contains(result, "sub/deep.md") {
		t.Errorf("result should contain sub/deep.md, got:\n%s", result)
	}
	// Should not match .txt
	if strings.Contains(result, ".txt") {
		t.Errorf("result should not contain .txt files, got:\n%s", result)
	}
}

func TestGlobTool_Execute_WorkspacePath(t *testing.T) {
	dataDir := setupGlobTestDir(t)
	tool := NewGlobTool(dataDir)
	ctx := WithAgentID(t.Context(), "myagent")

	params, _ := json.Marshal(map[string]string{"pattern": "@workspace/project/*.go"})
	result, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if !strings.Contains(result, "main.go") {
		t.Error("result should contain main.go")
	}
	if !strings.Contains(result, "util.go") {
		t.Error("result should contain util.go")
	}
	// Should not match lib.go in pkg/
	if strings.Contains(result, "lib.go") {
		t.Error("result should not contain lib.go (in subdirectory)")
	}
}

func TestGlobTool_Execute_RecursiveWorkspace(t *testing.T) {
	dataDir := setupGlobTestDir(t)
	tool := NewGlobTool(dataDir)
	ctx := WithAgentID(t.Context(), "myagent")

	params, _ := json.Marshal(map[string]string{"pattern": "@workspace/**/*.go"})
	result, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if !strings.Contains(result, "main.go") {
		t.Errorf("result should contain main.go, got:\n%s", result)
	}
	if !strings.Contains(result, "lib.go") {
		t.Errorf("result should contain lib.go (recursive), got:\n%s", result)
	}
}

func TestGlobTool_Execute_AgentPath(t *testing.T) {
	dataDir := setupGlobTestDir(t)
	tool := NewGlobTool(dataDir)
	ctx := WithAgentID(t.Context(), "myagent")

	params, _ := json.Marshal(map[string]string{"pattern": "@agent/*.md"})
	result, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if !strings.Contains(result, "@agent/AGENT.md") {
		t.Errorf("result should contain @agent/AGENT.md, got:\n%s", result)
	}
	// Should not match yml files
	if strings.Contains(result, "config.yml") {
		t.Error("result should not contain config.yml")
	}
}

func TestGlobTool_Execute_NoMatches(t *testing.T) {
	dataDir := setupGlobTestDir(t)
	tool := NewGlobTool(dataDir)
	ctx := WithAgentID(t.Context(), "myagent")

	params, _ := json.Marshal(map[string]string{"pattern": "@memory/*.json"})
	result, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if !strings.Contains(result, "No files matched") {
		t.Errorf("result should indicate no matches, got:\n%s", result)
	}
}

func TestGlobTool_Execute_TraversalBlocked(t *testing.T) {
	dataDir := setupGlobTestDir(t)
	tool := NewGlobTool(dataDir)
	ctx := WithAgentID(t.Context(), "myagent")

	params, _ := json.Marshal(map[string]string{"pattern": "@memory/../../etc/*"})
	_, err := tool.Execute(ctx, params)
	if err == nil {
		t.Fatal("expected error for traversal attempt")
	}
}

func TestGlobTool_Execute_InvalidPrefix(t *testing.T) {
	tool := NewGlobTool(t.TempDir())
	ctx := WithAgentID(t.Context(), "myagent")

	params, _ := json.Marshal(map[string]string{"pattern": "@unknown/*.md"})
	_, err := tool.Execute(ctx, params)
	if err == nil {
		t.Fatal("expected error for unknown prefix")
	}
}

func TestGlobTool_Execute_NoAtPrefix(t *testing.T) {
	tool := NewGlobTool(t.TempDir())
	ctx := WithAgentID(t.Context(), "myagent")

	params, _ := json.Marshal(map[string]string{"pattern": "memory/*.md"})
	_, err := tool.Execute(ctx, params)
	if err == nil {
		t.Fatal("expected error for path without @ prefix")
	}
}

func TestGlobTool_Execute_AgentWithoutContext(t *testing.T) {
	tool := NewGlobTool(t.TempDir())
	ctx := t.Context()

	params, _ := json.Marshal(map[string]string{"pattern": "@agent/*.md"})
	_, err := tool.Execute(ctx, params)
	if err == nil {
		t.Fatal("expected error when @agent used without agent context")
	}
}

func TestGlobTool_Execute_InvalidJSON(t *testing.T) {
	tool := NewGlobTool(t.TempDir())
	ctx := WithAgentID(t.Context(), "myagent")

	_, err := tool.Execute(ctx, json.RawMessage(`{bad json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON params")
	}
}

func TestGlobTool_Execute_EmptyPattern(t *testing.T) {
	tool := NewGlobTool(t.TempDir())
	ctx := WithAgentID(t.Context(), "myagent")

	params, _ := json.Marshal(map[string]string{"pattern": ""})
	_, err := tool.Execute(ctx, params)
	if err == nil {
		t.Fatal("expected error for empty pattern")
	}
}

func TestGlobTool_Execute_PrefixOnly(t *testing.T) {
	tool := NewGlobTool(t.TempDir())
	ctx := WithAgentID(t.Context(), "myagent")

	params, _ := json.Marshal(map[string]string{"pattern": "@memory"})
	_, err := tool.Execute(ctx, params)
	if err == nil {
		t.Fatal("expected error for prefix-only pattern")
	}
}

func TestGlobTool_Name(t *testing.T) {
	tool := NewGlobTool("/tmp")
	if tool.Name() != "glob" {
		t.Errorf("Name() = %q, want %q", tool.Name(), "glob")
	}
}

func TestGlobTool_Description(t *testing.T) {
	tool := NewGlobTool("/tmp")
	desc := tool.Description()
	if desc == "" {
		t.Error("Description() should not be empty")
	}
}

func TestGlobTool_Parameters(t *testing.T) {
	tool := NewGlobTool("/tmp")
	params := tool.Parameters()
	if params == nil {
		t.Fatal("Parameters() should not be nil")
	}

	var schema map[string]any
	if err := json.Unmarshal(params, &schema); err != nil {
		t.Fatalf("Parameters() should be valid JSON: %v", err)
	}

	if schema["type"] != "object" {
		t.Error("schema type should be 'object'")
	}
}

func TestMatchDoublestar(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		// Simple patterns (no **)
		{"*.md", "MEMORY.md", true},
		{"*.md", "notes.txt", false},
		{"*.md", "sub/deep.md", false},

		// ** at beginning
		{"**/*.md", "MEMORY.md", true},
		{"**/*.md", "sub/deep.md", true},
		{"**/*.md", "a/b/c.md", true},
		{"**/*.md", "notes.txt", false},

		// ** in middle
		{"project/**/*.go", "project/main.go", true},
		{"project/**/*.go", "project/pkg/lib.go", true},
		{"project/**/*.go", "other/main.go", false},

		// ** at end
		{"sub/**", "sub/deep.md", true},
		{"sub/**", "sub/a/b/c.txt", true},

		// Just **
		{"**", "anything", true},
		{"**", "a/b/c", true},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.path, func(t *testing.T) {
			got := matchDoublestar(tt.pattern, tt.path)
			if got != tt.want {
				t.Errorf("matchDoublestar(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
			}
		})
	}
}
