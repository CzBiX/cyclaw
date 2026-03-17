package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"
)

const (
	maxExecOutput  = 64 * 1024 // 64KB
	defaultTimeout = 60 * time.Second
)

type ExecTool struct {
	workDir string
}

func NewExecTool(workDir string) *ExecTool {
	return &ExecTool{workDir: workDir}
}

func (t *ExecTool) Name() string { return "exec" }

func (t *ExecTool) Description() string {
	return "Execute a bash command and return its output."
}

func (t *ExecTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": {
				"type": "string",
				"description": "The command to execute."
			},
			"timeout_seconds": {
				"type": "integer",
				"description": "Timeout in seconds (default 60)."
			},
			"workdir": {
				"type": "string",
				"description": "Working directory (default: workspace directory)."
			}
		},
		"required": ["command"]
	}`)
}

func (t *ExecTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p struct {
		Command        string `json:"command"`
		TimeoutSeconds int    `json:"timeout_seconds"`
		Workdir        string `json:"workdir"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("parse params: %w", err)
	}

	timeout := defaultTimeout
	if p.TimeoutSeconds > 0 {
		timeout = time.Duration(p.TimeoutSeconds) * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", p.Command)

	workDir := t.workDir
	if p.Workdir != "" {
		workDir = p.Workdir
	}
	cmd.Dir = workDir

	slog.Info("executing shell command",
		"command", p.Command,
		"workdir", workDir,
		"timeout", timeout.String(),
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	slog.Info("shell command completed",
		"command", p.Command,
		"duration_ms", duration.Milliseconds(),
		"exit_error", err != nil,
		"stdout_len", stdout.Len(),
		"stderr_len", stderr.Len(),
	)

	var result strings.Builder
	if stdout.Len() > 0 {
		out := stdout.String()
		if len(out) > maxExecOutput {
			out = out[:maxExecOutput] + "\n... (output truncated)"
		}
		result.WriteString(out)
	}
	if stderr.Len() > 0 {
		errOut := stderr.String()
		if len(errOut) > maxExecOutput {
			errOut = errOut[:maxExecOutput] + "\n... (stderr truncated)"
		}
		if result.Len() > 0 {
			result.WriteString("\n")
		}
		result.WriteString("STDERR:\n")
		result.WriteString(errOut)
	}

	if err != nil {
		if result.Len() > 0 {
			result.WriteString("\n")
		}
		fmt.Fprintf(&result, "Exit error: %s", err.Error())
	}

	if result.Len() == 0 {
		return "(no output)", nil
	}

	return result.String(), nil
}
