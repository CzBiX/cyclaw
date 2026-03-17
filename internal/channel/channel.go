package channel

import "context"

// IncomingMessage represents a message received from a channel.
type IncomingMessage struct {
	ChannelID     string         // Channel identifier (e.g. "telegram")
	ChatID        string         // Group/chat ID for agent routing
	UserID        string         // Sender ID
	UserName      string         // Sender display name
	Text          string         // Message text
	ChannelPrompt string         // Channel-specific system prompt instructions
	Metadata      map[string]any // Channel-specific data
	Background    bool           // If true, this is a background task (scheduled job, self-reflection, etc.). Session history is preserved on disk but not loaded into memory on restart.
	NoArchive     bool           // If true, the session will not be summarized into a diary entry when rotated.
}

// OutgoingMessage represents a message to be sent to a channel.
type OutgoingMessage struct {
	ChannelID      string // Target channel
	GroupID        string // Target group/chat
	Text           string // Message text
	ParseMode      string // "Markdown", "HTML", etc.
	DisablePreview bool   // If true, link previews are disabled
}

// Sender provides the ability to send messages through a channel.
type Sender interface {
	ID() string
	Send(ctx context.Context, msg *OutgoingMessage) error
	// Prompt returns channel-specific system prompt instructions.
	// These are appended to the agent's system prompt to guide output
	// formatting and other channel-specific behaviors.
	Prompt() string
}

// MessageHandler is called when a message is received.
type MessageHandler func(ctx context.Context, msg *IncomingMessage) error
