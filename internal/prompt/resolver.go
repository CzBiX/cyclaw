package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Resolver resolves @-prefixed paths to actual file paths and reads their content.
type Resolver struct {
	dataDir string
}

// NewResolver creates a new path resolver.
func NewResolver(dataDir string) *Resolver {
	return &Resolver{dataDir: dataDir}
}

// prefixDirs maps @-path prefixes to their directory names under dataDir.
var prefixDirs = map[string]string{
	"agent":  "agents",
	"memory": "memory",
	"skill":  "skills",
}

// Resolve converts an @-path reference to an absolute file path.
//
// Supported formats:
//
//	@agent/<name>/<FILE>.md  → <dataDir>/agents/<name>/<FILE>.md
//	@memory/<FILE>.md        → <dataDir>/memory/<FILE>.md
//	@skill/<name>/SKILL.md   → <dataDir>/skills/<name>/SKILL.md
//
// Directory traversal (e.g. @memory/../../etc/passwd) is rejected.
func (r *Resolver) Resolve(ref string) (string, error) {
	if !strings.HasPrefix(ref, "@") {
		return "", fmt.Errorf("invalid reference: must start with @: %s", ref)
	}

	path := strings.TrimPrefix(ref, "@")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 || parts[1] == "" {
		return "", fmt.Errorf("invalid reference format: %s", ref)
	}

	prefix := parts[0]
	rest := parts[1]

	dir, ok := prefixDirs[prefix]
	if !ok {
		return "", fmt.Errorf("unknown reference prefix: %s", prefix)
	}

	baseDir := filepath.Join(r.dataDir, dir)
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("resolve base dir: %w", err)
	}

	target := filepath.Join(absBase, filepath.Clean(rest))

	// Ensure resolved path stays within the target subdirectory.
	if !strings.HasPrefix(target, absBase+string(filepath.Separator)) && target != absBase {
		return "", fmt.Errorf("path traversal detected: %s escapes %s", ref, dir)
	}

	return target, nil
}

// ReadRef resolves an @-path and reads the file content.
func (r *Resolver) ReadRef(ref string) (string, error) {
	path, err := r.Resolve(ref)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s (%s): %w", ref, path, err)
	}

	return string(data), nil
}

// ReadFile reads a file by its path relative to the data directory.
// The path must not escape the data directory.
func (r *Resolver) ReadFile(relPath string) (string, error) {
	absBase, err := filepath.Abs(r.dataDir)
	if err != nil {
		return "", fmt.Errorf("resolve data dir: %w", err)
	}

	target := filepath.Join(absBase, filepath.Clean(relPath))

	if !strings.HasPrefix(target, absBase+string(filepath.Separator)) && target != absBase {
		return "", fmt.Errorf("path traversal detected: %s escapes data directory", relPath)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", target, err)
	}
	return string(data), nil
}
