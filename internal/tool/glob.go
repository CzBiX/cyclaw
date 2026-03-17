package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// GlobTool lists files matching a glob pattern within the sandboxed @-path directories.
type GlobTool struct {
	dataDir string
}

func NewGlobTool(dataDir string) *GlobTool {
	return &GlobTool{dataDir: dataDir}
}

func (t *GlobTool) Name() string { return "glob" }

func (t *GlobTool) Description() string {
	return "Find files matching a glob pattern (supports *, ?, **)."
}

func (t *GlobTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {
				"type": "string",
				"description": "` + pathParamDesc + ` Glob examples: @memory/*.md, @workspace/**/*.go"
			}
		},
		"required": ["pattern"]
	}`)
}

func (t *GlobTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p struct {
		Pattern string `json:"pattern"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("parse params: %w", err)
	}

	absBase, pfx, rest, err := resolvePrefix(t.dataDir, AgentIDFrom(ctx), p.Pattern)
	if err != nil {
		return "", fmt.Errorf("invalid pattern: %w", err)
	}

	// Check that the non-glob prefix of the pattern doesn't escape the base directory.
	// We strip glob metacharacters to get the "static" prefix portion and validate it.
	staticPrefix := globStaticPrefix(rest)
	if staticPrefix != "" {
		cleaned := filepath.Clean(staticPrefix)
		target := filepath.Join(absBase, cleaned)
		if !strings.HasPrefix(target, absBase+string(filepath.Separator)) && target != absBase {
			return "", fmt.Errorf("pattern traversal detected: %s escapes allowed directory", p.Pattern)
		}
	}

	var matches []string

	if strings.Contains(rest, "**") {
		// Use WalkDir for recursive ** patterns.
		matches, err = globRecursive(absBase, rest)
	} else {
		// Use filepath.Glob for simple patterns.
		fullPattern := filepath.Join(absBase, rest)
		matches, err = filepath.Glob(fullPattern)
	}
	if err != nil {
		return "", fmt.Errorf("glob %s: %w", p.Pattern, err)
	}

	// Resolve absBase through EvalSymlinks so we can compare against resolved match paths.
	realBase, err := filepath.EvalSymlinks(absBase)
	if err != nil {
		// Base directory doesn't exist; no matches possible.
		return "No files matched the pattern.", nil
	}

	// Filter results: each match must stay within absBase (protect against symlink escapes).
	var safe []string
	for _, m := range matches {
		real, err := filepath.EvalSymlinks(m)
		if err != nil {
			continue // skip unresolvable entries
		}
		if !strings.HasPrefix(real, realBase+string(filepath.Separator)) && real != realBase {
			continue // symlink escapes sandbox
		}
		safe = append(safe, m)
	}

	if len(safe) == 0 {
		return "No files matched the pattern.", nil
	}

	// Format results as @-prefixed paths relative to the base.
	var b strings.Builder
	for _, m := range safe {
		rel, err := filepath.Rel(absBase, m)
		if err != nil {
			continue
		}
		// Reconstruct the @-path.
		switch pfx {
		case "agent":
			b.WriteString("@agent/")
		default:
			b.WriteString("@" + pfx + "/")
		}
		b.WriteString(rel)

		// Mark directories with trailing slash.
		if info, err := os.Stat(m); err == nil && info.IsDir() {
			b.WriteByte('/')
		}
		b.WriteByte('\n')
	}
	return b.String(), nil
}

// globStaticPrefix returns the leading portion of a glob pattern that contains
// no metacharacters (* ? [ \). This is used to validate that the static path
// prefix doesn't escape the sandbox.
func globStaticPrefix(pattern string) string {
	for i, c := range pattern {
		if c == '*' || c == '?' || c == '[' || c == '\\' {
			// Return everything up to the last separator before this metachar.
			return pattern[:i]
		}
	}
	return pattern
}

// globRecursive implements ** (doublestar) glob matching using filepath.WalkDir.
// The pattern is split on "**" segments and matched against each walked path.
func globRecursive(baseDir, pattern string) ([]string, error) {
	// Verify the base directory exists.
	if _, err := os.Stat(baseDir); err != nil {
		return nil, nil
	}

	var matches []string
	err := filepath.WalkDir(baseDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip entries we can't access
		}

		rel, err := filepath.Rel(baseDir, path)
		if err != nil || rel == "." {
			return nil
		}

		if matchDoublestar(pattern, rel) {
			matches = append(matches, path)
		}
		return nil
	})
	return matches, err
}

// matchDoublestar matches a path against a pattern that may contain ** segments.
// ** matches zero or more path segments (directories).
// Other glob metacharacters are handled by filepath.Match.
func matchDoublestar(pattern, path string) bool {
	// Split the pattern on "**" to get segments.
	parts := strings.Split(pattern, "**")

	if len(parts) == 1 {
		// No ** in pattern, fall back to filepath.Match.
		ok, _ := filepath.Match(pattern, path)
		return ok
	}

	return matchParts(parts, path)
}

// matchParts recursively matches the path against the segments between ** markers.
func matchParts(parts []string, path string) bool {
	if len(parts) == 0 {
		return path == ""
	}

	head := parts[0]
	rest := parts[1:]

	// Remove leading/trailing separators from the head segment.
	head = strings.TrimRight(head, "/")

	if len(rest) == 0 {
		// Last segment: the remaining head (after last **) must match the end of path.
		tail := strings.TrimLeft(parts[len(parts)-1], "/")
		if tail == "" {
			return true // pattern ends with **
		}
		// The tail pattern must match the end of the path.
		// Try matching from every possible suffix.
		pathParts := strings.Split(path, string(filepath.Separator))
		for i := range pathParts {
			suffix := strings.Join(pathParts[i:], string(filepath.Separator))
			ok, _ := filepath.Match(tail, suffix)
			if ok {
				return true
			}
		}
		return false
	}

	if head == "" {
		// Pattern starts with ** (head is empty).
		// Try matching the remaining pattern parts against every possible sub-path.
		pathParts := strings.Split(path, string(filepath.Separator))
		for i := range pathParts {
			sub := strings.Join(pathParts[i:], string(filepath.Separator))
			if matchParts(rest, sub) {
				return true
			}
		}
		return false
	}

	// Head is non-empty: it must match the beginning of the path.
	headParts := strings.Split(head, "/")
	pathParts := strings.Split(path, string(filepath.Separator))

	if len(pathParts) < len(headParts) {
		return false
	}

	// Match head segments one-by-one against the path prefix.
	for i, hp := range headParts {
		ok, _ := filepath.Match(hp, pathParts[i])
		if !ok {
			return false
		}
	}

	// Continue matching the remaining path against remaining parts.
	remaining := strings.Join(pathParts[len(headParts):], string(filepath.Separator))
	// The rest of the parts still starts with a ** boundary, so try every suffix.
	remainParts := strings.Split(remaining, string(filepath.Separator))
	for i := range remainParts {
		sub := strings.Join(remainParts[i:], string(filepath.Separator))
		if matchParts(rest, sub) {
			return true
		}
	}
	// Also try with the full remaining.
	return matchParts(rest, remaining)
}
