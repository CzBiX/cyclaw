package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"cyclaw/internal/llm"
	"cyclaw/internal/session"
)

// archivePrompt instructs the LLM to produce a session summary for archival.
const archivePrompt = `You are a diary writer. Produce a brief summary (1-3 sentences) of the conversation below, capturing the key topics discussed, decisions made, and any important outcomes. Output ONLY the summary.`

// NewSession archives the current session in the background and immediately
// clears the history so the user can start a new conversation without waiting.
// Returns true if there was a conversation to archive, false if the session was
// already empty.
func (a *Agent) NewSession(ctx context.Context, chatID string) bool {
	s := a.Sessions.Get(chatID)
	if len(s.History) == 0 {
		return false
	}

	a.rotateSession(ctx, s)
	slog.Info("new session started", "agent", a.Id, "chat_id", chatID)

	return true
}

// rotateSession snapshots the session history, archives it in the background,
// and resets the session for reuse. The caller must ensure s.History is
// non-empty before calling.
//
// For background sessions (scheduled tasks, self-reflection), the history is
// preserved on disk for reference but the session is evicted from memory.
// For regular sessions, the history is cleared so a new conversation can begin.
func (a *Agent) rotateSession(ctx context.Context, s *session.Session) {
	snapshot := s.History

	if s.Background {
		// Keep history on disk for reference, but remove from memory.
		a.Sessions.Save(s)
		a.Sessions.Evict(s.ChatID)
	} else {
		s.History = nil
		a.Sessions.Save(s)
	}

	slog.Info("session rotated", "agent", a.Id, "chat_id", s.ChatID,
		"archived_messages", len(snapshot), "background", s.Background,
		"no_archive", s.NoArchive)
	if !s.NoArchive {
		go a.archiveHistory(context.WithoutCancel(ctx), snapshot)
	}
}

func messagesToText(history []llm.Item) string {
	var sb strings.Builder
	for _, it := range history {
		switch it.Type {
		case llm.ItemTypeMessage:
			v := it.Message
			fmt.Fprintf(&sb, "[%s]: %s\n", v.Role, v.Content)
		case llm.ItemTypeFunctionCall:
			v := it.FunctionCall
			fmt.Fprintf(&sb, "[tool_call %s]: %s(%s)\n", v.CallID, v.Name, v.Arguments)
		case llm.ItemTypeFunctionCallOutput:
			v := it.FunctionCallOutput
			fmt.Fprintf(&sb, "[tool_result %s]: %s\n", v.CallID, v.Output)
		case llm.ItemTypeOther:
			// Skip opaque items (e.g. reasoning) in text representation.
		}
	}
	return sb.String()
}

// archiveHistory summarizes the given history and saves it as a diary entry.
// It operates only on the provided snapshot, not on any live session state,
// so it is safe to call from a background goroutine.
func (a *Agent) archiveHistory(ctx context.Context, history []llm.Item) {
	conversationText := messagesToText(history)

	// Summarize using the LLM
	summaryReq := &llm.ChatRequest{
		Model:        a.Config.Model,
		Instructions: archivePrompt,
		Items:        []llm.Item{llm.NewMessage(llm.RoleUser, conversationText)},
	}

	resp, err := a.Provider.Chat(ctx, summaryReq)
	if err != nil {
		slog.Error("archive summarization failed", "error", err)
		return
	}

	summary := resp.Content
	if a.Diary != nil && summary != "" {
		if err := a.Diary.AppendDiary(summary); err != nil {
			slog.Error("failed to save archive to diary", "error", err)
		}
	}

	slog.Info("session archived", "agent", a.Id, "summary_len", len(summary))
}

// StartAutoArchive launches a background goroutine that periodically checks for
// idle sessions and archives them. It returns immediately. The goroutine stops
// when the provided context is cancelled. If SessionIdleTimeout is zero or
// negative, auto-archiving is disabled and this method is a no-op.
func (a *Agent) StartAutoArchive(ctx context.Context) {
	if a.SessionIdleTimeout <= 0 {
		return
	}

	// Check every minute or at half the idle timeout, whichever is shorter.
	interval := min(a.SessionIdleTimeout/2, time.Minute)

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				a.archiveIdleSessions(ctx)
			}
		}
	}()

	slog.Info("auto-archive enabled", "agent", a.Id, "idle_timeout", a.SessionIdleTimeout, "check_interval", interval)
}

// archiveIdleSessions finds sessions that have been idle longer than the
// configured threshold, archives them and clears their history.
func (a *Agent) archiveIdleSessions(ctx context.Context) {
	for _, s := range a.Sessions.Stale(a.SessionIdleTimeout) {
		slog.Info("auto-archiving idle session", "agent", a.Id, "chat_id", s.ChatID,
			"last_activity", s.UpdatedAt, "idle_for", time.Since(s.UpdatedAt).Round(time.Second))
		a.rotateSession(ctx, s)
	}
}
