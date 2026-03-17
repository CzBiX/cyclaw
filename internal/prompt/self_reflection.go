package prompt

import (
	"fmt"
	"strings"
	"time"

	"cyclaw/internal/channel"
	"cyclaw/internal/memory"
)

// selfReflectionSchedule is the cron expression for daily self-reflection.
const selfReflectionSchedule = "0 2 * * *"

// selfReflectionTaskID is the fixed task ID for the system self-reflection job.
const selfReflectionTaskID = "self_reflection"

// SelfReflectionMessage builds the message that triggers the agent to
// review recent diary entries and consider updates to memory and soul.
func SelfReflectionMessage(mem *memory.Manager) (*channel.IncomingMessage, error) {
	diaryEntries, err := mem.ReadDiaryRange(3)
	if err != nil {
		return nil, fmt.Errorf("read diary for self-reflection: %w", err)
	}

	blocks := append([]string{
		"# Diary",
	}, diaryEntries...)

	return &channel.IncomingMessage{
		ChannelID:     selfReflectionTaskID,
		ChatID:        fmt.Sprintf("%s-%s", selfReflectionTaskID, time.Now().Format("20060102")),
		ChannelPrompt: selfReflectionPrompt,
		Text:          strings.Join(blocks, "\n\n"),
		Background:    true,
		NoArchive:     true,
	}, nil
}

// SelfReflectionSchedule returns the cron expression for the daily self-reflection job.
func SelfReflectionSchedule() string {
	return selfReflectionSchedule
}

// SelfReflectionTaskID returns the fixed task ID for the self-reflection job.
func SelfReflectionTaskID() string {
	return selfReflectionTaskID
}

// selfReflectionPrompt is injected into the system prompt via ChannelPrompt.
const selfReflectionPrompt = `# Self-Reflection

You are now in daily self-reflection mode.
Review recent diary entries and evaluate if any updates are needed to these files:

- MEMORY.md 
- SOUL.md
- TOOLS.md
- USER.md

## Details

**Memory (MEMORY.md):**
- Is there new important information to record? (user preferences, significant events, recurring needs, etc.)
- Is any existing information outdated or inaccurate and should be updated or removed?
- Does the organizational structure need adjustment?

**Soul (SOUL.md):**
- Do recent interactions suggest your communication style needs fine-tuning?
- Does user feedback imply certain personality traits should be strengthened or softened?
- Should any values or behavioral guidelines be added?

If adjustments are needed, use write_file to update the relevant files. If everything looks good, make no changes.

Guidelines:
- Be conservative — only modify when there is clear, evidence-based need.
- Changes to SOUL.md must be gradual and incremental — never overhaul your personality.
- Changes to MEMORY.md must be grounded in concrete facts, not speculation.
`
