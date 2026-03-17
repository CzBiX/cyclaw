package skill

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Loader scans a directory for skills following the agentskills.io specification.
type Loader struct {
	dir string
}

// NewLoader creates a new skill loader.
func NewLoader(dir string) *Loader {
	return &Loader{dir: dir}
}

// LoadAll scans the skills directory and loads all valid skills.
// Each skill is expected to be in a subdirectory containing a SKILL.md file.
func (l *Loader) LoadAll() ([]*Skill, error) {
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Debug("skills directory does not exist, skipping", "dir", l.dir)
			return nil, nil
		}
		return nil, fmt.Errorf("read skills dir: %w", err)
	}

	var skills []*Skill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillPath := filepath.Join(l.dir, entry.Name(), "SKILL.md")
		s, err := l.loadSkill(skillPath)
		if err != nil {
			slog.Warn("failed to load skill", "dir", entry.Name(), "error", err)
			continue
		}

		// Validate: name must match directory name
		if s.Name != entry.Name() {
			slog.Warn("skill name does not match directory name",
				"skill_name", s.Name,
				"dir_name", entry.Name(),
			)
			continue
		}

		skills = append(skills, s)
		slog.Debug("skill loaded", "name", s.Name, "description_len", len(s.Description))
	}

	return skills, nil
}

// loadSkill reads and parses a single SKILL.md file.
func (l *Loader) loadSkill(path string) (*Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read SKILL.md: %w", err)
	}

	content := string(data)

	// Parse YAML frontmatter
	frontmatter, body, err := parseFrontmatter(content)
	if err != nil {
		return nil, fmt.Errorf("parse frontmatter: %w", err)
	}

	var s Skill
	if err := yaml.Unmarshal([]byte(frontmatter), &s); err != nil {
		return nil, fmt.Errorf("unmarshal frontmatter: %w", err)
	}

	if s.Name == "" {
		return nil, fmt.Errorf("skill name is required")
	}
	if s.Description == "" {
		return nil, fmt.Errorf("skill description is required")
	}

	s.Body = strings.TrimSpace(body)
	s.Dir = filepath.Dir(path)
	s.Location = fmt.Sprintf("@skills/%s/SKILL.md", s.Name)

	return &s, nil
}

// BuildPrompt builds the skill catalog prompt following the agentskills.io
// progressive disclosure pattern (Tier 1: name + description + location).
//
// Returns an empty string if no skills are provided.
func BuildPrompt(skills []*Skill) string {
	if len(skills) == 0 {
		return ""
	}

	var b strings.Builder

	b.WriteString("# Skills\n\n")

	b.WriteString("Skills provide specialized instructions for specific tasks.\n")
	b.WriteString("When a task matches a skill's description, use your file-read tool to load\n")
	b.WriteString("the SKILL.md at the listed location before proceeding.\n")

	b.WriteString("<available_skills>\n")
	for _, s := range skills {
		b.WriteString("  <skill>\n")
		fmt.Fprintf(&b, "  <name>%s</name>\n", s.Name)
		fmt.Fprintf(&b, "  <description>%s</description>\n", s.Description)
		fmt.Fprintf(&b, "  <location>%s</location>\n", s.Location)
		b.WriteString("  </skill>\n")
	}
	b.WriteString("</available_skills>")

	return b.String()
}

// parseFrontmatter splits YAML frontmatter from Markdown body.
// Expects content starting with "---\n" and ending with "\n---\n".
func parseFrontmatter(content string) (frontmatter, body string, err error) {
	if !strings.HasPrefix(content, "---") {
		return "", "", fmt.Errorf("missing frontmatter delimiter")
	}

	// Find the closing delimiter
	rest := content[3:]
	if rest[0] == '\n' {
		rest = rest[1:]
	} else if rest[0] == '\r' && len(rest) > 1 && rest[1] == '\n' {
		rest = rest[2:]
	}

	before, after, ok := strings.Cut(rest, "\n---")
	if !ok {
		return "", "", fmt.Errorf("missing closing frontmatter delimiter")
	}

	frontmatter = before
	body = after

	// Trim the newline after closing ---
	if len(body) > 0 && body[0] == '\n' {
		body = body[1:]
	} else if len(body) > 1 && body[0] == '\r' && body[1] == '\n' {
		body = body[2:]
	}

	return frontmatter, body, nil
}
