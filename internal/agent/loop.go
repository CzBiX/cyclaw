package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"cyclaw/internal/channel"
	"cyclaw/internal/llm"
	"cyclaw/internal/prompt"
	"cyclaw/internal/session"
	"cyclaw/internal/tool"
)

// loopOpts holds the parameters for a single run of the agent LLM-tool loop.
type loopOpts struct {
	session    *session.Session  // session whose History is used and mutated
	tools      *tool.Registry    // tool registry for execution
	toolDefs   []llm.FunctionDef // tool definitions sent to the LLM
	prompt     string            // system prompt
	streamCb   llm.StreamCallback
	background bool // if true, the end_chat tool can terminate the loop
}

// runLoop is the core LLM-tool loop shared by HandleMessage and HandleSubTask.
// It calls the LLM, executes any tool calls, appends results to the session
// history, and repeats until the LLM produces a final text response or
// MaxToolRounds is exceeded.
func (a *Agent) runLoop(ctx context.Context, opts loopOpts) (*llm.ChatResponse, error) {
	chatID := opts.session.ChatID

	for round := 1; round <= a.MaxToolRounds; round++ {
		req := &llm.ChatRequest{
			Model:        a.Config.Model,
			Instructions: opts.prompt,
			Items:        opts.session.History,
			Functions:    opts.toolDefs,
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
		if opts.streamCb != nil {
			resp, err = a.Provider.StreamChat(ctx, req, opts.streamCb)
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
				opts.session.AppendHistory(item)
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
			opts.session.AppendHistory(item)
		}

		// Notify the channel about function calls
		if opts.streamCb != nil {
			if err := opts.streamCb(llm.StreamDelta{
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
			if opts.background && fc.Name == tool.SingleEndChatTool.Name() {
				slog.Info("end_chat signal received, ending conversation",
					"agent", a.Id,
					"chat_id", chatID,
					"round", round,
				)
				endChat = true
				break
			}

			result, err := opts.tools.Execute(ctx, fc.Name, json.RawMessage(fc.Arguments))
			if err != nil {
				result = fmt.Sprintf("Error: %s", err.Error())
				slog.Error("tool execution failed",
					"tool", fc.Name,
					"error", err,
				)
			}

			opts.session.AppendHistory(llm.NewFunctionCallOutput(fc.CallID, result))
		}
		if endChat {
			return nil, nil
		}
	}

	return nil, fmt.Errorf("exceeded maximum tool rounds (%d)", a.MaxToolRounds)
}

// HandleMessage processes an incoming message through the agent loop.
// It builds the prompt, calls the LLM, executes any tool calls in a loop,
// and returns the final text response.
// If streamCb is non-nil, LLM responses are streamed and the callback is
// invoked for each text delta.
func (a *Agent) HandleMessage(ctx context.Context, msg *channel.IncomingMessage, streamCb llm.StreamCallback) (*llm.ChatResponse, error) {
	chatID := msg.ChatID
	userText := msg.Text

	ctx = tool.WithAgentID(ctx, a.Id)
	ctx = tool.WithChatID(ctx, chatID)
	sess := a.GetSession(chatID)
	if msg.Background {
		sess.Background = true
	}
	if msg.NoArchive {
		sess.NoArchive = true
	}
	defer func() { a.Sessions.Save(sess) }()

	slog.Info("handling message",
		"agent", a.Id,
		"chat_id", chatID,
		"text_len", len(userText),
		"history_len", len(sess.History),
	)

	// Build system prompt
	agentFiles := prompt.AgentFiles{
		Id: a.Config.Id,
	}
	systemPrompt := a.Builder.BuildSystemPrompt(agentFiles, a.Skills, msg)

	// Add user message to history
	sess.AppendHistory(llm.NewMessage(llm.RoleUser, userText))

	// Get tool definitions
	toolDefs := a.Tools.LLMDefs()

	// For background tasks, include end_chat so the LLM can signal that
	// it has finished and no further rounds are needed.
	if msg.Background {
		toolDefs = append(toolDefs, tool.ToLLMDef(tool.SingleEndChatTool))
	}

	// Compress context if history exceeds token limit
	if a.MaxTokens > 0 {
		estimated := estimateTokens(sess.History)
		if estimated > compressionThreshold(a.MaxTokens, a.CompressionRatio) {
			compressed, err := compressHistory(ctx, a.Provider, a.Config.Model, sess.History, a.MaxTokens, a.CompressionRatio)
			if err != nil {
				slog.Error("context compression error", "error", err)
			} else {
				sess.History = compressed
			}
		}
	}

	return a.runLoop(ctx, loopOpts{
		session:    sess,
		tools:      a.Tools,
		toolDefs:   toolDefs,
		prompt:     systemPrompt,
		streamCb:   streamCb,
		background: msg.Background,
	})
}
