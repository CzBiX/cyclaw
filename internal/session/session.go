package session

import (
	"time"

	"cyclaw/internal/llm"
)

const MaxHistoryMessages = 50

// Session holds the conversation state for a single chat.
type Session struct {
	ChatID    string     `json:"chat_id"`
	History   []llm.Item `json:"history,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	// Background marks sessions created by background tasks (e.g. scheduled
	// jobs, self-reflection). When archived, background sessions keep their
	// history on disk for reference but are evicted from memory and not
	// reloaded on startup.
	Background bool `json:"background,omitempty"`
	// NoArchive prevents the session from being summarized into a diary
	// entry when rotated. The session is still rotated (history cleared or
	// evicted) but no LLM summarization or diary write occurs. This is
	// used for self-reflection sessions whose output should not feed back
	// into the diary.
	NoArchive bool `json:"no_archive,omitempty"`
}

// AppendHistory adds an item to the session history and trims if needed.
func (s *Session) AppendHistory(item llm.Item) {
	s.History = append(s.History, item)
	if len(s.History) > MaxHistoryMessages {
		s.History = s.History[len(s.History)-MaxHistoryMessages:]
	}
	s.UpdatedAt = time.Now()
}

// Store defines the interface for session persistence.
type Store interface {
	// Get returns the session for the given chat ID.
	// If no session exists, a new empty one is created and returned.
	Get(chatID string) *Session
	// Save persists the session to the backing store.
	Save(s *Session)
	// Delete removes the session for the given chat ID from the backing store.
	Delete(chatID string)
	// Evict removes the session from the in-memory map but keeps the
	// backing file on disk. This is used for background sessions that should
	// be preserved for reference but not kept in memory.
	Evict(chatID string)
	// Stale returns all sessions with non-empty history whose UpdatedAt
	// is older than the given threshold duration.
	Stale(threshold time.Duration) []*Session
}
