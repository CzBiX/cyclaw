package agent

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"cyclaw/internal/config"
	"cyclaw/internal/llm"
	"cyclaw/internal/memory"
	"cyclaw/internal/prompt"
	"cyclaw/internal/session"
	"cyclaw/internal/skill"
	"cyclaw/internal/tool"
)

// DiaryAppender is the interface for appending diary entries.
type DiaryAppender interface {
	AppendDiary(entry string) error
}

// Agent represents an individual agent with its own personality, skills and conversation state.
type Agent struct {
	Id                 string
	Config             config.AgentConfig
	Provider           llm.Provider
	Tools              *tool.Registry
	Builder            *prompt.Builder
	Skills             []*skill.Skill
	AllAgents          []prompt.AgentInfo // all registered agents, for system prompt
	MaxTokens          int                // context window size for compression
	MaxToolRounds      int                // max LLM-tool loop iterations per message
	CompressionRatio   float64            // compression threshold as ratio of maxTokens
	SessionIdleTimeout time.Duration      // auto-archive sessions after this idle duration
	Diary              DiaryAppender
	Sessions           session.Store
}

// NewAgent creates a new agent instance.
func NewAgent(agentCfg config.AgentConfig, globalCfg *config.Config, provider llm.Provider, tools *tool.Registry, builder *prompt.Builder, skills []*skill.Skill, diary DiaryAppender, sessions session.Store) *Agent {
	// Build the list of all agents for the system prompt.
	allAgents := make([]prompt.AgentInfo, len(globalCfg.Agents))
	for i, ac := range globalCfg.Agents {
		allAgents[i] = prompt.AgentInfo{
			Id:      ac.Id,
			Current: ac.Id == agentCfg.Id,
		}
	}

	return &Agent{
		Id:                 agentCfg.Id,
		Config:             agentCfg,
		Provider:           provider,
		Tools:              tools,
		Builder:            builder,
		Skills:             skills,
		AllAgents:          allAgents,
		MaxTokens:          globalCfg.LLM.MaxTokens,
		MaxToolRounds:      globalCfg.MaxToolRounds,
		CompressionRatio:   globalCfg.CompressionRatio,
		SessionIdleTimeout: globalCfg.SessionIdleTimeout,
		Diary:              diary,
		Sessions:           sessions,
	}
}

// GetSession returns (or creates) the session for a given chat ID.
func (a *Agent) GetSession(chatID string) *session.Session {
	return a.Sessions.Get(chatID)
}

// ClearSession resets the session history for a given chat ID.
func (a *Agent) ClearSession(chatID string) {
	s := a.Sessions.Get(chatID)
	s.History = nil
	a.Sessions.Save(s)
	slog.Info("session cleared", "agent", a.Id, "chat_id", chatID)
}

// RunSelfReflection triggers a self-reflection cycle for the default agent.
// It routes the message through the normal agent loop so the agent has full
// tool access (read_diary, read_file, write_file).
func RunSelfReflection(ctx context.Context, mem *memory.Manager, router *Router) error {
	msg, err := prompt.SelfReflectionMessage(mem)
	if err != nil {
		return fmt.Errorf("create self-reflection message: %w", err)
	}

	slog.Info("starting daily self-reflection")

	resp, err := router.Route(ctx, msg, nil)
	if err != nil {
		return fmt.Errorf("self-reflection failed: %w", err)
	}

	slog.Info("self-reflection completed", "resp", resp.Content)

	return nil
}
