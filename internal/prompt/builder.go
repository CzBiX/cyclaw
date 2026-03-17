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

// AgentInfo describes an agent for inclusion in the system prompt.
type AgentInfo struct {
	Id      string
	Current bool // true if this is the agent receiving the prompt
}

// AgentFiles holds the paths for an agent's core definition files.
type AgentFiles struct {
	Id     string
	Agents []AgentInfo // all registered agents
}

// BuildSystemPrompt assembles the complete system prompt for an agent.
// msg provides the incoming message context including channel and chat information.
func (b *Builder) BuildSystemPrompt(agentFiles AgentFiles, skills []*skill.Skill, msg *channel.IncomingMessage) string {
	parts := make([]string, 0)

	parts = append(parts, b.loadAgentCoreFiles(agentFiles))
	parts = append(parts, b.loadSkillsSummary(skills))
	if agentsList := buildAgentsList(agentFiles.Agents); agentsList != "" {
		parts = append(parts, agentsList)
	}
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

// buildAgentsList renders the available agents section for the system prompt.
// This lets the LLM know which agents can be targeted with the sub_task tool.
func buildAgentsList(agents []AgentInfo) string {
	if len(agents) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("# Agents\n\n")
	sb.WriteString("Available agents for delegation via the `sub_task` tool:\n\n")
	for _, a := range agents {
		marker := ""
		if a.Current {
			marker = " (you)"
		}
		sb.WriteString(fmt.Sprintf("- **%s**%s\n", a.Id, marker))
	}
	return sb.String()
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
