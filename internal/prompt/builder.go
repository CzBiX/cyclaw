package prompt

import (
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"time"

	"cyclaw/internal/channel"
	"cyclaw/internal/skill"
)

// Builder assembles the system prompt from agent definitions, memory, and skills.
type Builder struct {
	resolver *Resolver
}

// NewBuilder creates a new prompt builder.
func NewBuilder(resolver *Resolver) *Builder {
	return &Builder{resolver: resolver}
}

// AgentFiles holds the paths for an agent's core definition files.
type AgentFiles struct {
	Id string
}

// BuildSystemPrompt assembles the complete system prompt for an agent.
// msg provides the incoming message context including channel and chat information.
func (b *Builder) BuildSystemPrompt(agentFiles AgentFiles, skills []*skill.Skill, msg *channel.IncomingMessage) string {
	parts := make([]string, 0)

	parts = append(parts, b.loadAgentCoreFiles(agentFiles))
	parts = append(parts, b.loadSkillsSummary(skills))
	parts = append(parts, b.loadLongTermMemory())
	if msg.ChannelPrompt != "" {
		parts = append(parts, msg.ChannelPrompt)
	}
	parts = append(parts, b.buildContext(msg))

	return strings.Join(parts, "\n\n---\n\n")
}

func buildFileSection(ref string, content string) string {
	return fmt.Sprintf(`<file path="%s">
%s
</file>`, ref, content)
}

func (b *Builder) loadAgentCoreFiles(agentFiles AgentFiles) string {
	var parts []string
	for _, file := range []string{"AGENT.md", "SOUL.md", "USER.md", "TOOLS.md"} {
		ref := fmt.Sprintf("@agent/%s/%s", agentFiles.Id, file)
		content, err := b.resolver.ReadRef(ref)
		if err != nil {
			slog.Warn("skipping agent file", "ref", ref, "error", err)
			continue
		}
		parts = append(parts, buildFileSection(fmt.Sprintf("@agent/%s", file), content))
	}

	return strings.Join(parts, "\n")
}

func (b *Builder) loadLongTermMemory() string {
	content, err := b.resolver.ReadRef("@memory/MEMORY.md")
	if err != nil || content == "" {
		return ""
	}
	return buildFileSection("@memory/MEMORY.md", content)
}

func (b *Builder) loadSkillsSummary(skills []*skill.Skill) string {
	return skill.BuildPrompt(skills)
}

func (b *Builder) buildContext(msg *channel.IncomingMessage) string {
	now := time.Now().Format("2006-01-02 15:04 (Monday)")
	osLine := fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)

	return fmt.Sprintf(`# Context

Current time: %s
OS: %s
Channel: %s
Chat ID: %s
`, now, osLine, msg.ChannelID, msg.ChatID)
}
