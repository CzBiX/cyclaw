package memory

import (
	"strings"
	"testing"
	"time"
)

func TestManager_WriteAndRead(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	if err := m.Write("test.md", "hello world"); err != nil {
		t.Fatalf("Write() error: %v", err)
	}

	content, err := m.Read("test.md")
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if content != "hello world" {
		t.Errorf("Read() = %q, want %q", content, "hello world")
	}
}

func TestManager_ReadNonexistent(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	content, err := m.Read("nonexistent.md")
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if content != "" {
		t.Errorf("Read() = %q, want empty string", content)
	}
}

func TestManager_Append(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	if err := m.Write("log.md", "line1"); err != nil {
		t.Fatal(err)
	}
	if err := m.Append("log.md", "line2"); err != nil {
		t.Fatal(err)
	}

	content, err := m.Read("log.md")
	if err != nil {
		t.Fatal(err)
	}

	if content != "line1\nline2" {
		t.Errorf("Read() = %q, want %q", content, "line1\nline2")
	}
}

func TestManager_AppendToNew(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	if err := m.Append("new.md", "first entry"); err != nil {
		t.Fatal(err)
	}

	content, err := m.Read("new.md")
	if err != nil {
		t.Fatal(err)
	}
	if content != "first entry" {
		t.Errorf("Read() = %q, want %q", content, "first entry")
	}
}

func TestDiaryName(t *testing.T) {
	tm := time.Date(2026, 3, 14, 10, 30, 0, 0, time.UTC)
	got := DiaryName(tm)
	if got != "202603/14.md" {
		t.Errorf("DiaryName() = %q, want %q", got, "202603/14.md")
	}
}

func TestManager_AppendDiary(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	if err := m.AppendDiary("test entry"); err != nil {
		t.Fatal(err)
	}

	content, err := m.Read(TodayDiary())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, "test entry") {
		t.Errorf("diary content = %q, want to contain %q", content, "test entry")
	}
	if !strings.Contains(content, "- [") {
		t.Errorf("diary content = %q, want to contain timestamp prefix", content)
	}
}
