package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"cyclaw/internal/llm"
)

func TestSession_AppendHistory(t *testing.T) {
	s := &Session{}

	for i := 0; i < 5; i++ {
		s.AppendHistory(llm.NewMessage(llm.RoleUser, "msg"))
	}

	if len(s.History) != 5 {
		t.Errorf("History length = %d, want 5", len(s.History))
	}
}

func TestSession_AppendHistory_Trims(t *testing.T) {
	s := &Session{}

	for i := 0; i < 60; i++ {
		s.AppendHistory(llm.NewMessage(llm.RoleUser, "msg"))
	}

	if len(s.History) != MaxHistoryMessages {
		t.Errorf("History length = %d, want %d", len(s.History), MaxHistoryMessages)
	}
}

func TestSession_AppendHistory_UpdatesTimestamp(t *testing.T) {
	s := &Session{}

	s.AppendHistory(llm.NewMessage(llm.RoleUser, "hello"))

	if s.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should not be zero after AppendHistory")
	}
}

func TestFileStore_GetCreatesSession(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	s := store.Get("chat1")
	if s == nil {
		t.Fatal("Get returned nil")
	}
	if s.ChatID != "chat1" {
		t.Errorf("ChatID = %q, want %q", s.ChatID, "chat1")
	}
	if s.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}

	// Same chat ID should return the same session
	s2 := store.Get("chat1")
	if s != s2 {
		t.Error("Get returned different sessions for same chat ID")
	}
}

func TestFileStore_Save(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	s := store.Get("chat1")
	s.AppendHistory(llm.NewMessage(llm.RoleUser, "hello"))
	s.AppendHistory(llm.NewMessage(llm.RoleAssistant, "hi there"))

	store.Save(s)

	// Verify file was created with correct content
	path := filepath.Join(dir, "chat1.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read persisted session: %v", err)
	}

	var loaded Session
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("failed to unmarshal session: %v", err)
	}

	if loaded.ChatID != "chat1" {
		t.Errorf("ChatID = %q, want %q", loaded.ChatID, "chat1")
	}
	if len(loaded.History) != 2 {
		t.Errorf("History length = %d, want 2", len(loaded.History))
	}
	it := loaded.History[0]
	if it.Type != llm.ItemTypeMessage {
		t.Fatalf("History[0].Type = %q, want %q", it.Type, llm.ItemTypeMessage)
	}
	if it.Message.Content != "hello" {
		t.Errorf("History[0].Message.Content = %q, want %q", it.Message.Content, "hello")
	}
}

func TestFileStore_LoadOnCreate(t *testing.T) {
	dir := t.TempDir()

	// Create and persist a session
	store1 := NewFileStore(dir)
	s := store1.Get("chat42")
	s.AppendHistory(llm.NewMessage(llm.RoleUser, "persisted message"))
	s.AppendHistory(llm.NewMessage(llm.RoleAssistant, "persisted reply"))
	store1.Save(s)

	// Create a new store pointing to the same directory — simulates restart
	store2 := NewFileStore(dir)

	s2 := store2.Get("chat42")
	if len(s2.History) != 2 {
		t.Fatalf("loaded History length = %d, want 2", len(s2.History))
	}
	it := s2.History[0]
	if it.Type != llm.ItemTypeMessage {
		t.Fatalf("History[0].Type = %q, want %q", it.Type, llm.ItemTypeMessage)
	}
	if it.Message.Content != "persisted message" {
		t.Errorf("History[0].Message.Content = %q, want %q", it.Message.Content, "persisted message")
	}
	if s2.ChatID != "chat42" {
		t.Errorf("ChatID = %q, want %q", s2.ChatID, "chat42")
	}
}

func TestFileStore_Delete(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	s := store.Get("chat1")
	s.AppendHistory(llm.NewMessage(llm.RoleUser, "hello"))
	store.Save(s)

	store.Delete("chat1")

	// File should be removed
	path := filepath.Join(dir, "chat1.json")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("session file should have been deleted")
	}

	// Getting the same chat ID should return a fresh session
	s2 := store.Get("chat1")
	if len(s2.History) != 0 {
		t.Errorf("History length after delete = %d, want 0", len(s2.History))
	}
}

func TestFileStore_Evict(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	s := store.Get("task-123")
	s.Background = true
	s.AppendHistory(llm.NewMessage(llm.RoleUser, "task output"))
	store.Save(s)

	store.Evict("task-123")

	// File should still exist on disk in the background subdirectory
	path := filepath.Join(dir, "background", "task-123.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("session file should still exist after evict: %v", err)
	}

	var loaded Session
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("failed to unmarshal session: %v", err)
	}
	if len(loaded.History) != 1 {
		t.Errorf("History length on disk = %d, want 1", len(loaded.History))
	}

	// Getting the same chat ID should return a fresh session (not the old one)
	s2 := store.Get("task-123")
	if len(s2.History) != 0 {
		t.Errorf("History length after evict+get = %d, want 0", len(s2.History))
	}
}

func TestFileStore_BackgroundNotLoadedOnRestart(t *testing.T) {
	dir := t.TempDir()

	// Create and persist a background session
	store1 := NewFileStore(dir)
	s := store1.Get("task-456")
	s.Background = true
	s.AppendHistory(llm.NewMessage(llm.RoleUser, "task data"))
	store1.Save(s)

	// Background file should be in the background subdirectory
	bgPath := filepath.Join(dir, "background", "task-456.json")
	if _, err := os.Stat(bgPath); err != nil {
		t.Fatalf("background session file should exist at %s: %v", bgPath, err)
	}

	// Regular directory should NOT contain the file
	mainPath := filepath.Join(dir, "task-456.json")
	if _, err := os.Stat(mainPath); !os.IsNotExist(err) {
		t.Fatalf("background session should not be in main dir")
	}

	// Create a new store pointing to the same directory — simulates restart
	store2 := NewFileStore(dir)

	// The background session should NOT be loaded into memory
	s2 := store2.Get("task-456")
	if len(s2.History) != 0 {
		t.Errorf("background session should not be loaded, got History length = %d", len(s2.History))
	}

	// But the file should still be on disk for reference
	data, err := os.ReadFile(bgPath)
	if err != nil {
		t.Fatalf("background session file should still exist: %v", err)
	}

	var loaded Session
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("failed to unmarshal session: %v", err)
	}
	if !loaded.Background {
		t.Error("session on disk should have Background=true")
	}
	if len(loaded.History) != 1 {
		t.Errorf("session on disk History length = %d, want 1", len(loaded.History))
	}
}

func TestFileStore_NoArchivePersisted(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	s := store.Get("reflect-1")
	s.Background = true
	s.NoArchive = true
	s.AppendHistory(llm.NewMessage(llm.RoleUser, "self-reflection output"))
	store.Save(s)

	// Read back from disk and verify NoArchive is persisted
	bgPath := filepath.Join(dir, "background", "reflect-1.json")
	data, err := os.ReadFile(bgPath)
	if err != nil {
		t.Fatalf("session file should exist at %s: %v", bgPath, err)
	}

	var loaded Session
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("failed to unmarshal session: %v", err)
	}

	if !loaded.NoArchive {
		t.Error("session on disk should have NoArchive=true")
	}
	if !loaded.Background {
		t.Error("session on disk should have Background=true")
	}
}

func TestFileStore_SaveAfterClear(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	s := store.Get("chat1")
	s.AppendHistory(llm.NewMessage(llm.RoleUser, "hello"))
	store.Save(s)

	// Clear history and save again
	s.History = nil
	store.Save(s)

	// Load from disk and verify history is empty
	data, err := os.ReadFile(filepath.Join(dir, "chat1.json"))
	if err != nil {
		t.Fatalf("failed to read persisted session: %v", err)
	}

	var loaded Session
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("failed to unmarshal session: %v", err)
	}

	if len(loaded.History) != 0 {
		t.Errorf("History length after clear = %d, want 0", len(loaded.History))
	}
}
