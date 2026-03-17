package skill

import (
	"os"
	"path/filepath"
)

// Skill represents a loaded skill definition following the agentskills.io specification.
type Skill struct {
	// Name is the skill identifier (from frontmatter).
	Name string `yaml:"name"`

	// Description describes what the skill does and when to use it (from frontmatter).
	Description string `yaml:"description"`

	// Metadata holds arbitrary key-value pairs.
	Metadata map[string]string `yaml:"metadata,omitempty"`

	// Body is the Markdown content after the frontmatter (the actual instructions).
	Body string `yaml:"-"`

	// Dir is the directory path where this skill was loaded from.
	Dir string `yaml:"-"`

	// Location is the absolute path to the SKILL.md file.
	Location string `yaml:"-"`
}

// Resources enumerates bundled resource files in the skill directory.
// It returns paths relative to the skill directory, excluding SKILL.md itself.
func (s *Skill) Resources() []string {
	var resources []string
	_ = filepath.WalkDir(s.Dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(s.Dir, path)
		if err != nil {
			return nil
		}
		if rel == "SKILL.md" {
			return nil
		}
		resources = append(resources, rel)
		return nil
	})
	return resources
}
