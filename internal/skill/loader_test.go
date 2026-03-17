package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFrontmatter(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		wantFM       string
		wantBodyPart string
		wantErr      bool
	}{
		{
			name: "valid frontmatter",
			content: `---
name: coding
description: Help with coding tasks
---
This is the body content.`,
			wantFM:       "name: coding\ndescription: Help with coding tasks",
			wantBodyPart: "This is the body content.",
		},
		{
			name:    "missing opening delimiter",
			content: "no frontmatter here",
			wantErr: true,
		},
		{
			name: "missing closing delimiter",
			content: `---
name: coding
description: no closing`,
			wantErr: true,
		},
		{
			name: "empty body",
			content: `---
name: test
description: test skill
---
`,
			wantFM:       "name: test\ndescription: test skill",
			wantBodyPart: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm, body, err := parseFrontmatter(tt.content)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if strings.TrimSpace(fm) != strings.TrimSpace(tt.wantFM) {
				t.Errorf("frontmatter = %q, want %q", fm, tt.wantFM)
			}
			if tt.wantBodyPart != "" && !strings.Contains(body, tt.wantBodyPart) {
				t.Errorf("body = %q, want to contain %q", body, tt.wantBodyPart)
			}
		})
	}
}

func TestBuildPrompt(t *testing.T) {
	t.Run("empty skills", func(t *testing.T) {
		got := BuildPrompt(nil)
		if got != "" {
			t.Errorf("BuildPrompt(nil) = %q, want empty", got)
		}
	})

	t.Run("single skill", func(t *testing.T) {
		skills := []*Skill{
			{
				Name:        "coding",
				Description: "Help with coding tasks",
				Location:    "@skills/coding/SKILL.md",
			},
		}
		got := BuildPrompt(skills)

		if !strings.Contains(got, "<name>coding</name>") {
			t.Error("missing skill name in prompt")
		}
		if !strings.Contains(got, "<description>Help with coding tasks</description>") {
			t.Error("missing skill description in prompt")
		}
		if !strings.Contains(got, "<location>@skills/coding/SKILL.md</location>") {
			t.Error("missing skill location in prompt")
		}
		if !strings.Contains(got, "<available_skills>") {
			t.Error("missing available_skills tag")
		}
	})

	t.Run("multiple skills", func(t *testing.T) {
		skills := []*Skill{
			{Name: "coding", Description: "Coding help", Location: "@skills/coding/SKILL.md"},
			{Name: "writing", Description: "Writing help", Location: "@skills/writing/SKILL.md"},
		}
		got := BuildPrompt(skills)

		if !strings.Contains(got, "<name>coding</name>") {
			t.Error("missing first skill")
		}
		if !strings.Contains(got, "<name>writing</name>") {
			t.Error("missing second skill")
		}
	})
}

func TestLoadAll(t *testing.T) {
	// Create a temp directory with a valid skill
	tmpDir := t.TempDir()

	skillDir := filepath.Join(tmpDir, "coding")
	if err := os.Mkdir(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}

	skillContent := `---
name: coding
description: Help with coding tasks
---
# Coding Skill

Instructions for coding assistance.
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0o644); err != nil {
		t.Fatal(err)
	}

	loader := NewLoader(tmpDir)
	skills, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll() error: %v", err)
	}

	if len(skills) != 1 {
		t.Fatalf("LoadAll() returned %d skills, want 1", len(skills))
	}

	s := skills[0]
	if s.Name != "coding" {
		t.Errorf("skill name = %q, want %q", s.Name, "coding")
	}
	if s.Description != "Help with coding tasks" {
		t.Errorf("skill description = %q, want %q", s.Description, "Help with coding tasks")
	}
	if !strings.Contains(s.Body, "Coding Skill") {
		t.Errorf("skill body = %q, want to contain %q", s.Body, "Coding Skill")
	}
}

func TestLoadAll_NonexistentDir(t *testing.T) {
	loader := NewLoader("/nonexistent/path/that/does/not/exist")
	skills, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll() should not error for nonexistent dir: %v", err)
	}
	if skills != nil {
		t.Errorf("LoadAll() should return nil for nonexistent dir, got %v", skills)
	}
}
