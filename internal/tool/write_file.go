package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

type WriteFileTool struct {
	dataDir string
}

func NewWriteFileTool(dataDir string) *WriteFileTool {
	return &WriteFileTool{dataDir: dataDir}
}

func (t *WriteFileTool) Name() string { return "write_file" }

func (t *WriteFileTool) Description() string {
	return "Write content to a file."
}

func (t *WriteFileTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "` + pathParamDesc + `"
			},
			"content": {
				"type": "string",
				"description": "Content to write, append, or unified diff to patch."
			},
			"mode": {
				"type": "string",
				"enum": ["overwrite", "append", "patch"],
				"description": "overwrite (default): replace file. append: add to end. patch: apply unified diff."
			}
		},
		"required": ["path", "content"]
	}`)
}

func (t *WriteFileTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p struct {
		Path    string `json:"path"`
		Content string `json:"content"`
		Mode    string `json:"mode"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("parse params: %w", err)
	}

	if p.Mode == "" {
		p.Mode = "overwrite"
	}

	resolved, _, err := resolveAndValidate(t.dataDir, AgentIDFrom(ctx), p.Path)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return "", fmt.Errorf("create directory: %w", err)
	}

	switch p.Mode {
	case "append":
		f, err := os.OpenFile(resolved, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return "", fmt.Errorf("open file for append: %w", err)
		}
		defer f.Close()

		if _, err := f.WriteString(p.Content); err != nil {
			return "", fmt.Errorf("append to file: %w", err)
		}
		slog.Info("file appended", "path", p.Path, "content_len", len(p.Content))
		return fmt.Sprintf("Appended to %s", p.Path), nil

	case "patch":
		original, err := os.ReadFile(resolved)
		if err != nil {
			return "", fmt.Errorf("read file for patching: %w", err)
		}

		patched, err := applyUnifiedDiff(string(original), p.Content)
		if err != nil {
			return "", fmt.Errorf("apply patch: %w", err)
		}

		if err := os.WriteFile(resolved, []byte(patched), 0o644); err != nil {
			return "", fmt.Errorf("write patched file: %w", err)
		}
		slog.Info("file patched", "path", p.Path, "diff_len", len(p.Content))
		return fmt.Sprintf("Patched %s", p.Path), nil

	case "overwrite":
		if err := os.WriteFile(resolved, []byte(p.Content), 0o644); err != nil {
			return "", fmt.Errorf("write file: %w", err)
		}
		slog.Info("file written", "path", p.Path, "content_len", len(p.Content))
		return fmt.Sprintf("Written to %s", p.Path), nil

	default:
		return "", fmt.Errorf("unknown mode %q: must be one of overwrite, append, patch", p.Mode)
	}
}
