package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type ReadFileTool struct {
	dataDir string
}

func NewReadFileTool(dataDir string) *ReadFileTool {
	return &ReadFileTool{dataDir: dataDir}
}

func (t *ReadFileTool) Name() string { return "read_file" }

func (t *ReadFileTool) Description() string {
	return "Read a file's contents."
}

func (t *ReadFileTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "` + pathParamDesc + `"
			}
		},
		"required": ["path"]
	}`)
}

func (t *ReadFileTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("parse params: %w", err)
	}

	resolved, _, err := resolveAndValidate(t.dataDir, AgentIDFrom(ctx), strings.TrimRight(p.Path, "/"))
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", p.Path, err)
	}

	if info.IsDir() {
		return "", fmt.Errorf("path %s is a directory, use the glob tool to list files", p.Path)
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		return "", fmt.Errorf("read file %s: %w", p.Path, err)
	}

	return string(data), nil
}
