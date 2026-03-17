package tool

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

type contextKey string

const agentIDKey contextKey = "agent_id"

// WithAgentID returns a new context that carries the given agent ID.
func WithAgentID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, agentIDKey, id)
}

// AgentIDFrom extracts the agent ID from the context.
func AgentIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(agentIDKey).(string)
	return id
}

// allowedPrefixes lists the known @-path prefixes.
// "agent" is special: it maps to agents/<agentID>/ at runtime.
var allowedPrefixes = map[string]bool{
	"agent":     true,
	"memory":    true,
	"skills":    true,
	"workspace": true,
}

// allowedPrefixList returns the sorted list of allowed @-prefixes for use
// in tool descriptions and error messages (e.g. "@agent, @memory, ...").
func allowedPrefixList() string {
	prefixes := make([]string, 0, len(allowedPrefixes))
	for k := range allowedPrefixes {
		prefixes = append(prefixes, "@"+k)
	}
	return strings.Join(prefixes, ", ")
}

// pathParamDesc is the shared parameter description for @-prefixed path parameters.
const pathParamDesc = `@-prefixed path, e.g. @agent/AGENT.md, @memory/MEMORY.md, @skills/name/SKILL.md, @workspace/file.txt`

// prefixDirs maps non-agent prefixes to their directory names under dataDir.
var prefixDirs = map[string]string{
	"memory":    "memory",
	"skills":    "skills",
	"workspace": "workspace",
}

// resolvePrefix validates an @-prefixed path's prefix and returns the absolute
// base directory and the remaining path after the prefix.
//
// It enforces:
//   - Only known @-prefixes are accepted (agent, memory, skills, workspace).
//   - @agent maps to agents/<agentID>/, scoped to the current agent only.
//   - Non-@ (raw) paths are rejected.
//
// Returns the absolute base directory, the prefix name, and the remaining path portion.
func resolvePrefix(dataDir, agentID, path string) (absBase string, prefix string, rest string, err error) {
	if len(path) == 0 {
		return "", "", "", fmt.Errorf("empty path")
	}

	if path[0] != '@' {
		return "", "", "", fmt.Errorf("path must use @-prefix (e.g. @memory/file.md): %s", path)
	}

	// Split "@prefix/rest"
	trimmed := path[1:]
	pfx, remainder, hasSep := strings.Cut(trimmed, "/")
	if !hasSep || remainder == "" {
		return "", "", "", fmt.Errorf("invalid @-path format, expected @<prefix>/<path>: %s", path)
	}

	if !allowedPrefixes[pfx] {
		allowed := make([]string, 0, len(allowedPrefixes))
		for k := range allowedPrefixes {
			allowed = append(allowed, "@"+k)
		}
		return "", "", "", fmt.Errorf("unknown prefix %q, allowed: %s", pfx, strings.Join(allowed, ", "))
	}

	// Determine the base directory for this prefix.
	var baseDir string
	if pfx == "agent" {
		if agentID == "" {
			return "", "", "", fmt.Errorf("@agent path requires an active agent context")
		}
		baseDir = filepath.Join(dataDir, "agents", agentID)
	} else {
		baseDir = filepath.Join(dataDir, prefixDirs[pfx])
	}

	abs, err := filepath.Abs(baseDir)
	if err != nil {
		return "", "", "", fmt.Errorf("resolve base dir: %w", err)
	}

	return abs, pfx, remainder, nil
}

// resolveAndValidate resolves an @-prefixed path to a safe absolute path under dataDir.
//
// It enforces:
//   - Only known @-prefixes are accepted (agent, memory, skills, workspace).
//   - @agent maps to agents/<agentID>/, scoped to the current agent only.
//   - No directory traversal: the resolved path must stay within the target subdirectory.
//   - Non-@ (raw) paths are rejected.
//
// Returns the resolved absolute path and the matched prefix name.
func resolveAndValidate(dataDir, agentID, path string) (resolved string, prefix string, err error) {
	absBase, pfx, rest, err := resolvePrefix(dataDir, agentID, path)
	if err != nil {
		return "", "", err
	}

	target := filepath.Join(absBase, filepath.Clean(rest))

	// Ensure the cleaned target still falls under absBase.
	if !strings.HasPrefix(target, absBase+string(filepath.Separator)) && target != absBase {
		return "", "", fmt.Errorf("path traversal detected: %s escapes %s", path, path)
	}

	return target, pfx, nil
}
