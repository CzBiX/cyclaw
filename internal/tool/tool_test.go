package tool

import (
	"context"
	"encoding/json"
	"testing"

	"cyclaw/internal/channel"
	"cyclaw/internal/scheduler"
)

func TestToLLMDef(t *testing.T) {
	tool := &mockTool{
		name:        "read_file",
		description: "Read a file",
		params:      json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
	}

	def := ToLLMDef(tool)

	if def.Name != "read_file" {
		t.Errorf("Function.Name = %q, want %q", def.Name, "read_file")
	}
	if def.Description != "Read a file" {
		t.Errorf("Function.Description = %q, want %q", def.Description, "Read a file")
	}
	if def.Parameters == nil {
		t.Error("Function.Parameters is nil, want non-nil")
	}
}

func TestToLLMDef_NilParams(t *testing.T) {
	tool := &mockTool{
		name:        "simple",
		description: "A simple tool",
		params:      nil,
	}

	def := ToLLMDef(tool)

	if def.Parameters != nil {
		t.Errorf("Function.Parameters = %v, want nil", def.Parameters)
	}
}

type mockSender struct{}

func (m *mockSender) ID() string { return "mock" }

func (m *mockSender) Send(_ context.Context, _ *channel.OutgoingMessage) error {
	return nil
}

func (m *mockSender) Prompt() string { return "" }

func allToolsForParametersTest(t *testing.T) []Tool {
	t.Helper()

	tmpDir := t.TempDir()
	sched := scheduler.New("", func(context.Context, *scheduler.Task) error { return nil })

	return []Tool{
		NewReadFileTool(tmpDir),
		NewWriteFileTool(tmpDir),
		NewGlobTool(tmpDir),
		NewWebFetchTool(),
		NewWebSearchTool(),
		NewExecTool(tmpDir),
		NewCronTool(sched),
		NewSendMessageTool(&mockSender{}, nil),
	}
}

func TestAllToolParametersAreValid(t *testing.T) {
	for _, tl := range allToolsForParametersTest(t) {
		t.Run(tl.Name(), func(t *testing.T) {
			raw := tl.Parameters()

			if !json.Valid(raw) {
				t.Fatalf("parameters is not valid JSON")
			}

			def := ToLLMDef(tl)

			if _, err := json.Marshal(def); err != nil {
				t.Fatalf("tool definition cannot be marshaled: %v", err)
			}
		})
	}
}
