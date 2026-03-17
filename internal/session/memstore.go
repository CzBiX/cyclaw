package session

import (
	"sync"
	"time"
)

// MemStore is an in-memory session store for testing.
type MemStore struct {
	mu       sync.Mutex
	sessions map[string]*Session
}

// NewMemStore creates a new in-memory session store.
func NewMemStore() *MemStore {
	return &MemStore{sessions: make(map[string]*Session)}
}

func (m *MemStore) Get(chatID string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	if s, ok := m.sessions[chatID]; ok {
		return s
	}

	now := time.Now()
	s := &Session{
		ChatID:    chatID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	m.sessions[chatID] = s
	return s
}

func (m *MemStore) Save(s *Session) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[s.ChatID] = s
}

func (m *MemStore) Delete(chatID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, chatID)
}

func (m *MemStore) Evict(chatID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, chatID)
}

// Stale returns all sessions with non-empty history that have not been
// updated within the given threshold duration.
func (m *MemStore) Stale(threshold time.Duration) []*Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	cutoff := time.Now().Add(-threshold)
	var stale []*Session
	for _, s := range m.sessions {
		if len(s.History) > 0 && s.UpdatedAt.Before(cutoff) {
			stale = append(stale, s)
		}
	}
	return stale
}
