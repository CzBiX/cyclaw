package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"cyclaw/internal/channel"
	"cyclaw/internal/llm"
	"cyclaw/internal/prompt"
	"cyclaw/internal/tool"
)

// HandleMessage processes an incoming message through the agent loop.
// It builds the prompt, calls the LLM, executes any tool calls in a loop,
// and returns the final text response.
// If streamCb is non-nil, LLM responses are streamed and the callback is
// invoked for each text delta.
func (a *Agent) HandleMessage(ctx context.Context, msg *channel.IncomingMessage, streamCb llm.StreamCallback) (*llm.ChatResponse, error) {
	chatID := msg.ChatID
	userText := msg.Text

	ctx = tool.WithAgentID(ctx, a.Id)
	session := a.GetSession(chatID)
	if msg.Background {
		session.Background = true
	}
	if msg.NoArchive {
		session.NoArchive = true
	}
	defer func() { a.Sessions.Save(session) }()

	slog.Info("handling message",
		"agent", a.Id,
		"chat_id", chatID,
		"text_len", len(userText),
		"history_len", len(session.History),
	)

	// Build system prompt
	agentFiles := prompt.AgentFiles{
		Id: a.Config.Id,
	}
	systemPrompt := a.Builder.BuildSystemPrompt(agentFiles, a.Skills, msg)

	// Add user message to history
	session.AppendHistory(llm.NewMessage(llm.RoleUser, userText))

	// Get tool definitions
	toolDefs := a.Tools.LLMDefs()

	// For background tasks, include end_chat so the LLM can signal that
	// it has finished and no further rounds are needed.
	if msg.Background {
		toolDefs = append(toolDefs, tool.ToLLMDef(tool.SingleEndChatTool))
	}

	// Compress context if history exceeds token limit
	if a.MaxTokens > 0 {
		// Estimate tokens for history
		estimated := estimateTokens(session.History)
		if estimated > compressionThreshold(a.MaxTokens, a.CompressionRatio) {
			compressed, err := compressHistory(ctx, a.Provider, a.Config.Model, session.History, a.MaxTokens, a.CompressionRatio)
			if err != nil {
				slog.Error("context compression error", "error", err)
				// Continue with original history on error
			} else {
				session.History = compressed
			}
		}
	}

	// Main loop: call LLM, execute tools, repeat until no more tool calls
	for round := 1; round < a.MaxToolRounds; round++ {
		req := &llm.ChatRequest{
			Model:        a.Config.Model,
			Instructions: systemPrompt,
			Items:        session.History,
			Functions:    toolDefs,
		}

		slog.Debug("calling LLM",
			"agent", a.Id,
			"chat", chatID,
			"round", round,
			"items", len(req.Items),
		)

		var (
			resp *llm.ChatResponse
			err  error
		)
		if streamCb != nil {
			resp, err = a.Provider.StreamChat(ctx, req, streamCb)
		} else {
			resp, err = a.Provider.Chat(ctx, req)
		}
		if err != nil {
			return nil, fmt.Errorf("llm chat (round %d): %w", round, err)
		}

		slog.Debug("LLM response",
			"agent", a.Id,
			"chat_id", chatID,
			"content_len", len(resp.Content),
			"function_calls", len(resp.FunctionCalls),
		)

		// If no function calls, we're done — append all output items
		// (including reasoning, text, etc.) to preserve full context.
		if len(resp.FunctionCalls) == 0 {
			for _, item := range resp.Items {
				session.AppendHistory(item)
			}
			slog.Info("message complete",
				"agent", a.Id,
				"chat_id", chatID,
				"rounds", round,
				"response_len", len(resp.Content),
			)
			return resp, nil
		}

		// Record all output items (reasoning + text + tool calls).
		for _, item := range resp.Items {
			session.AppendHistory(item)
		}

		// Notify the channel about function calls
		if streamCb != nil {
			if err := streamCb(llm.StreamDelta{
				FunctionCalls: resp.FunctionCalls,
			}); err != nil {
				slog.Warn("stream callback error for function calls", "error", err)
			}
		}

		// Execute each function call and append results to history
		endChat := false
		for _, fc := range resp.FunctionCalls {
			slog.Debug("executing tool",
				"tool", fc.Name,
				"call_id", fc.CallID,
			)

			// Handle end_chat directly — it is not in the registry and
			// signals that the background task is done.
			if fc.Name == tool.SingleEndChatTool.Name() {
				slog.Info("end_chat signal received, ending conversation",
					"agent", a.Id,
					"chat_id", chatID,
					"round", round,
				)
				endChat = true
				break
			}

			result, err := a.Tools.Execute(ctx, fc.Name, json.RawMessage(fc.Arguments))
			if err != nil {
				result = fmt.Sprintf("Error: %s", err.Error())
				slog.Error("tool execution failed",
					"tool", fc.Name,
					"error", err,
				)
			}

			session.AppendHistory(llm.NewFunctionCallOutput(fc.CallID, result))
		}
		if endChat {
			return nil, nil
		}
	}

	return nil, fmt.Errorf("exceeded maximum tool rounds (%d)", a.MaxToolRounds)
}
