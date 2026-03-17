package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"cyclaw/internal/memory"
)

// DiaryReader defines the interface for reading diary entries.
type DiaryReader interface {
	ReadDiary(t time.Time) (string, error)
}

type ReadDiaryTool struct {
	mem *memory.Manager
}

func NewReadDiaryTool(mem *memory.Manager) *ReadDiaryTool {
	return &ReadDiaryTool{mem: mem}
}

func (t *ReadDiaryTool) Name() string { return "read_diary" }

func (t *ReadDiaryTool) Description() string {
	return "Read recent diary entries from the last N days."
}

func (t *ReadDiaryTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"days": {
				"type": "integer",
				"description": "Number of days to include, counting from today (default 1)."
			}
		}
	}`)
}

func (t *ReadDiaryTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p struct {
		Days int `json:"days"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("parse params: %w", err)
	}

	diaryEntries, err := t.mem.ReadDiaryRange(p.Days)
	if err != nil {
		return "", fmt.Errorf("read diary: %w", err)
	}
	if len(diaryEntries) == 0 {
		return "No diary entries found for the specified range.", nil
	}

	return strings.Join(diaryEntries, "\n\n---\n\n"), nil
}
