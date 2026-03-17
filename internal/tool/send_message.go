package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"cyclaw/internal/channel"
	"cyclaw/internal/llm"
	"cyclaw/internal/session"
)

// SessionResolver resolves a session store for a given chat ID.
// This is typically implemented by the agent router so that send_msg
// can record outgoing messages in the target chat's normal session.
type SessionResolver interface {
	// ResolveSession returns the session store and session for the given
	// chat ID, or nil if no agent handles that chat.
	ResolveSession(chatID string) (session.Store, *session.Session)
}

type SendMessageTool struct {
	sender   channel.Sender
	sessions SessionResolver
}

func NewSendMessageTool(sender channel.Sender, sessions SessionResolver) *SendMessageTool {
	return &SendMessageTool{sender: sender, sessions: sessions}
}

func (t *SendMessageTool) Name() string { return "send_msg" }

func (t *SendMessageTool) Description() string {
	return "Proactively send a message to a chat/group."
}

func (t *SendMessageTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"chat_id": {
				"type": "string",
				"description": "Target chat/group ID."
			},
			"text": {
				"type": "string",
				"description": "Message text. Supports Markdown/HTML when parse_mode is set."
			},
			"parse_mode": {
				"type": "string",
				"enum": ["markdown", "html", ""],
				"description": "Formatting mode, default is plain text."
			},
			"disable_preview": {
				"type": "boolean",
				"description": "Disable link previews."
			}
		},
		"required": ["chat_id", "text"]
	}`)
}

func (t *SendMessageTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p struct {
		ChatID         string `json:"chat_id"`
		Text           string `json:"text"`
		ParseMode      string `json:"parse_mode"`
		DisablePreview bool   `json:"disable_preview"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("parse params: %w", err)
	}

	msg := &channel.OutgoingMessage{
		ChannelID:      "telegram",
		GroupID:        p.ChatID,
		Text:           p.Text,
		ParseMode:      p.ParseMode,
		DisablePreview: p.DisablePreview,
	}

	if err := t.sender.Send(ctx, msg); err != nil {
		return "", fmt.Errorf("send message: %w", err)
	}

	// Record the sent message in the target chat's normal session so the
	// agent has context about previously sent proactive messages when the
	// user replies in that chat.
	if t.sessions != nil {
		if store, sess := t.sessions.ResolveSession(p.ChatID); store != nil {
			sess.AppendHistory(llm.NewMessage(llm.RoleAssistant, p.Text))
			store.Save(sess)
			slog.Debug("proactive message recorded in session", "chat_id", p.ChatID)
		}
	}

	slog.Info("proactive message sent", "chat_id", p.ChatID, "text_len", len(p.Text))
	return fmt.Sprintf("Message sent to %s", p.ChatID), nil
}
