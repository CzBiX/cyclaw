package tool

import (
	"context"
	"encoding/json"
)

// EndChatTool is a special tool available only during background tasks
// (scheduled jobs, self-reflection, etc.). When the LLM calls this tool, the
// agent loop terminates immediately without feeding the result back to the LLM.
//
// This tool is NOT registered in the shared Registry. Instead, the agent loop
// appends its definition to the tool list only for background sessions, and
// detects calls to it by name.
type EndChatTool struct{}

var SingleEndChatTool = &EndChatTool{}

func (t *EndChatTool) Name() string { return "end_chat" }

func (t *EndChatTool) Description() string {
	return "Signal completion of a background/scheduled task to end the session."
}

func (t *EndChatTool) Parameters() json.RawMessage {
	return nil
}

func (t *EndChatTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	panic("This should never be called")
}
