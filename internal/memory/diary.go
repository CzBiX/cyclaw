package memory

import (
	"fmt"
	"strings"
	"time"
)

// DiaryName returns the diary file path for the given time, e.g. "202603/15.md".
func DiaryName(t time.Time) string {
	return fmt.Sprintf("%s/%s.md", t.Format("200601"), t.Format("02"))
}

// TodayDiary returns the diary file path for today.
func TodayDiary() string {
	return DiaryName(time.Now())
}

// ReadDiary reads the diary for a given date.
func (m *Manager) ReadDiary(t time.Time) (string, error) {
	return m.Read(DiaryName(t))
}

// WriteDiary writes the diary for a given date.
func (m *Manager) WriteDiary(t time.Time, content string) error {
	return m.Write(DiaryName(t), content)
}

// AppendDiary appends an entry to today's diary with a timestamp.
func (m *Manager) AppendDiary(entry string) error {
	timestamp := time.Now().Format("15:04:05")
	line := fmt.Sprintf("- [%s] %s\n", timestamp, entry)
	return m.Append(TodayDiary(), line)
}

// ReadDiaryRange returns diary entries for the last N days (including today).
// Each day's entry is preceded by a heading like "## 2026-03-15 (Sunday)".
// Days are separated by "\n\n---\n\n".
func (m *Manager) ReadDiaryRange(days int) ([]string, error) {
	if days <= 0 {
		days = 1
	}

	now := time.Now()
	var parts []string

	for i := 0; i < days; i++ {
		day := now.AddDate(0, 0, -i)
		content, err := m.ReadDiary(day)
		if err != nil {
			return nil, fmt.Errorf("read diary for %s: %w", day.Format("2006-01-02"), err)
		}
		if content == "" {
			continue
		}
		head := fmt.Sprintf("## %s", day.Format("2006-01-02 (Monday)"))
		parts = append(parts, head+"\n\n"+strings.TrimSpace(content))
	}

	return parts, nil
}
