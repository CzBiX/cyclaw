package session

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// FileStore persists sessions as individual JSON files in a directory.
// Each session is stored as <dir>/<chatID>.json.
type FileStore struct {
	mu       sync.Mutex
	dir      string
	sessions map[string]*Session
}

// NewFileStore creates a FileStore and loads any existing sessions from dir.
func NewFileStore(dir string) *FileStore {
	fs := &FileStore{
		dir:      dir,
		sessions: make(map[string]*Session),
	}
	fs.load()
	return fs
}

// Get returns the session for chatID, creating a new one if it doesn't exist.
func (fs *FileStore) Get(chatID string) *Session {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if s, ok := fs.sessions[chatID]; ok {
		return s
	}

	now := time.Now()
	s := &Session{
		ChatID:    chatID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	fs.sessions[chatID] = s
	slog.Info("session created", "chat_id", chatID)
	return s
}

// Save persists the session to disk as a JSON file.
// Background sessions are written to a separate "background" subdirectory so
// they are never scanned during startup.
func (fs *FileStore) Save(s *Session) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	fs.sessions[s.ChatID] = s

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		slog.Error("failed to marshal session", "chat_id", s.ChatID, "error", err)
		return
	}

	dir := fs.dir
	if s.Background {
		dir = filepath.Join(fs.dir, "background")
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		slog.Error("failed to create sessions dir", "dir", dir, "error", err)
		return
	}

	path := filepath.Join(dir, s.ChatID+".json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		slog.Error("failed to persist session", "path", path, "error", err)
		return
	}

	slog.Debug("session persisted", "chat_id", s.ChatID, "path", path)
}

// Delete removes the session from memory and deletes the backing file.
func (fs *FileStore) Delete(chatID string) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	delete(fs.sessions, chatID)

	path := filepath.Join(fs.dir, chatID+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		slog.Warn("failed to remove session file", "path", path, "error", err)
	}
}

// Evict removes the session from the in-memory map but keeps the backing
// file on disk. This is used for background sessions that should be preserved
// for reference but not held in memory.
func (fs *FileStore) Evict(chatID string) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	delete(fs.sessions, chatID)
	slog.Debug("session evicted from memory", "chat_id", chatID)
}

// Stale returns all sessions with non-empty history that have not been
// updated within the given threshold duration.
func (fs *FileStore) Stale(threshold time.Duration) []*Session {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	cutoff := time.Now().Add(-threshold)
	var stale []*Session
	for _, s := range fs.sessions {
		if len(s.History) > 0 && s.UpdatedAt.Before(cutoff) {
			stale = append(stale, s)
		}
	}
	return stale
}

// load reads all persisted session files from the directory.
func (fs *FileStore) load() {
	entries, err := os.ReadDir(fs.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		slog.Warn("failed to read sessions dir", "dir", fs.dir, "error", err)
		return
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		path := filepath.Join(fs.dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			slog.Warn("failed to read session file", "path", path, "error", err)
			continue
		}

		var s Session
		if err := json.Unmarshal(data, &s); err != nil {
			slog.Warn("failed to unmarshal session file", "path", path, "error", err)
			continue
		}

		if s.ChatID == "" {
			s.ChatID = strings.TrimSuffix(entry.Name(), ".json")
		}

		fs.sessions[s.ChatID] = &s
		slog.Debug("session loaded", "chat_id", s.ChatID, "history_len", len(s.History))
	}

	slog.Info("sessions loaded from disk", "dir", fs.dir, "count", len(fs.sessions))
}
