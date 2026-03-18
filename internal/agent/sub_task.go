package agent

import (
	"context"
	"fmt"
	"log/slog"

	"cyclaw/internal/channel"
	"cyclaw/internal/llm"
	"cyclaw/internal/prompt"
	"cyclaw/internal/tool"
)

// HandleSubTask runs a delegated sub-task through the shared agent loop.
// A persisted session is created for the sub-task so its history is retained.
// The session is marked as background + no-archive so it doesn't pollute
// the agent's diary but remains on disk for inspection.
//
// Parameters:
//   - ctx: context (should carry sub-task depth via tool.WithSubTaskDepth)
//   - task: the user-facing prompt/instruction for the sub-agent
//   - instructions: optional system prompt override; if empty, the agent's
//     default system prompt is used
//   - tools: optional tool registry override; if nil, the agent's own registry
//     is used
func (a *Agent) HandleSubTask(ctx context.Context, task string, instructions string, tools *tool.Registry) (string, error) {
	ctx = tool.WithAgentID(ctx, a.Id)

	chatID := fmt.Sprintf("sub_task_%s", tool.GenerateID())
	ctx = tool.WithChatID(ctx, chatID)

	slog.Info("handling sub_task",
		"agent", a.Id,
		"chat_id", chatID,
		"task_len", len(task),
		"has_custom_instructions", instructions != "",
		"has_custom_tools", tools != nil,
		"depth", tool.SubTaskDepthFrom(ctx),
	)

	// Create a persisted session for the sub-task
	sess := a.GetSession(chatID)
	sess.Background = true
	sess.NoArchive = true
	defer func() { a.Sessions.Save(sess) }()

	// Build system prompt
	var systemPrompt string
	if instructions != "" {
		systemPrompt = instructions
	} else {
		agentFiles := prompt.AgentFiles{Id: a.Config.Id}
		msg := &channel.IncomingMessage{
			ChannelID: "sub_task",
			ChatID:    chatID,
		}
		systemPrompt = a.Builder.BuildSystemPrompt(agentFiles, a.Skills, msg)
	}

	// Use provided tools or fall back to the agent's own registry
	toolRegistry := a.Tools
	if tools != nil {
		toolRegistry = tools
	}

	// Add user message to session history
	sess.AppendHistory(llm.NewMessage(llm.RoleUser, task))

	// Get tool definitions
	toolDefs := toolRegistry.LLMDefs()

	resp, err := a.runLoop(ctx, loopOpts{
		session:  sess,
		tools:    toolRegistry,
		toolDefs: toolDefs,
		prompt:   systemPrompt,
	})
	if err != nil {
		return "", err
	}

	if resp == nil || resp.Content == "" {
		return "(sub_task returned no output)", nil
	}

	return resp.Content, nil
}
