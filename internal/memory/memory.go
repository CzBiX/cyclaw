package memory

import (
	"fmt"
	"os"
	"path/filepath"
)

// Manager handles reading and writing of memory files.
type Manager struct {
	dir string
}

// NewManager creates a new memory manager.
func NewManager(dir string) *Manager {
	return &Manager{dir: dir}
}

// Read reads a memory file by name (e.g. "MEMORY.md" or "20260314.md").
func (m *Manager) Read(name string) (string, error) {
	path := filepath.Join(m.dir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read memory %s: %w", name, err)
	}
	return string(data), nil
}

// Write writes content to a memory file.
func (m *Manager) Write(name, content string) error {
	path := filepath.Join(m.dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create memory dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write memory %s: %w", name, err)
	}
	return nil
}

// Append appends content to a memory file.
func (m *Manager) Append(name, content string) error {
	existing, err := m.Read(name)
	if err != nil {
		return err
	}

	newContent := existing
	if newContent != "" {
		newContent += "\n"
	}
	newContent += content

	return m.Write(name, newContent)
}
