package prompt

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolve_ValidPaths(t *testing.T) {
	dataDir := "/tmp/testdata"
	r := NewResolver(dataDir)

	tests := []struct {
		name       string
		ref        string
		wantSuffix string
	}{
		{"agent path", "@agent/main/AGENT.md", filepath.Join("agents", "main", "AGENT.md")},
		{"memory path", "@memory/MEMORY.md", filepath.Join("memory", "MEMORY.md")},
		{"skill path", "@skill/coding/SKILL.md", filepath.Join("skills", "coding", "SKILL.md")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := r.Resolve(tt.ref)
			if err != nil {
				t.Fatalf("Resolve(%q) error: %v", tt.ref, err)
			}
			wantEnd := filepath.Join(dataDir, tt.wantSuffix)
			absWant, _ := filepath.Abs(wantEnd)
			if got != absWant {
				t.Errorf("Resolve(%q) = %q, want %q", tt.ref, got, absWant)
			}
		})
	}
}

func TestResolve_Errors(t *testing.T) {
	r := NewResolver("/tmp/testdata")

	tests := []struct {
		name string
		ref  string
	}{
		{"no @ prefix", "memory/file.md"},
		{"unknown prefix", "@unknown/file.md"},
		{"missing file part", "@memory"},
		{"empty after prefix", "@memory/"},
		{"traversal attempt", "@memory/../../etc/passwd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := r.Resolve(tt.ref)
			if err == nil {
				t.Errorf("Resolve(%q) expected error, got nil", tt.ref)
			}
		})
	}
}

func TestReadRef(t *testing.T) {
	tmpDir := t.TempDir()
	r := NewResolver(tmpDir)

	// Create the memory directory and file
	memDir := filepath.Join(tmpDir, "memory")
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(memDir, "test.md"), []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	content, err := r.ReadRef("@memory/test.md")
	if err != nil {
		t.Fatalf("ReadRef() error: %v", err)
	}
	if content != "content" {
		t.Errorf("ReadRef() = %q, want %q", content, "content")
	}
}

func TestReadRef_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	r := NewResolver(tmpDir)

	_, err := r.ReadRef("@memory/nonexistent.md")
	if err == nil {
		t.Fatal("ReadRef() expected error for nonexistent file")
	}
}

func TestReadFile(t *testing.T) {
	tmpDir := t.TempDir()
	r := NewResolver(tmpDir)

	// Create a test file in the data dir
	if err := os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	content, err := r.ReadFile("test.txt")
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	if content != "data" {
		t.Errorf("ReadFile() = %q, want %q", content, "data")
	}
}

func TestReadFile_TraversalBlocked(t *testing.T) {
	tmpDir := t.TempDir()
	r := NewResolver(tmpDir)

	_, err := r.ReadFile("../../etc/passwd")
	if err == nil {
		t.Fatal("ReadFile() expected error for path traversal")
	}
}
