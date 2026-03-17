package tool

import (
	"context"
	"encoding/json"
	"log/slog"
	"math/rand"

	"cyclaw/internal/llm"
)

// Tool defines the interface that all tools must implement.
type Tool interface {
	// Name returns the unique name of the tool.
	Name() string

	// Description returns a human-readable description of what the tool does.
	Description() string

	// Parameters returns the JSON Schema for the tool's parameters.
	Parameters() json.RawMessage

	// Execute runs the tool with the given parameters and returns the result.
	Execute(ctx context.Context, params json.RawMessage) (string, error)
}

// ToLLMDef converts a Tool to an LLM FunctionDef for use in chat requests.
func ToLLMDef(t Tool) llm.FunctionDef {
	var params map[string]any
	raw := t.Parameters()
	if raw != nil {
		if err := json.Unmarshal(raw, &params); err != nil {
			slog.Error("failed to unmarshal tool parameters", "tool", t.Name(), "error", err)
		}
	}

	return llm.FunctionDef{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters:  params,
	}
}

func generateID() string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 6)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}
