package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"cyclaw/internal/channel"
	"cyclaw/internal/llm"
	"cyclaw/internal/session"
)

// Router routes incoming messages to the appropriate agent based on group/chat ID mapping.
type Router struct {
	mu           sync.RWMutex
	agents       map[string]*Agent // name → agent
	groupMap     map[string]string // groupID → agent name
	defaultAgent string
}

// NewRouter creates a new agent router.
func NewRouter() *Router {
	return &Router{
		agents:   make(map[string]*Agent),
		groupMap: make(map[string]string),
	}
}

// Register adds an agent to the router.
func (r *Router) Register(a *Agent) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.agents[a.Id] = a

	if a.Config.Default {
		r.defaultAgent = a.Id
		slog.Debug("default agent set", "id", a.Id)
	}

	// Register group mappings
	for _, groupID := range a.Config.Groups {
		r.groupMap[groupID] = a.Id
		slog.Debug("group mapped to agent", "group", groupID, "agent", a.Id)
	}
}

// Route determines which agent should handle a message and processes it.
// If streamCb is non-nil, LLM responses are streamed via the callback.
func (r *Router) Route(ctx context.Context, msg *channel.IncomingMessage, streamCb llm.StreamCallback) (*llm.ChatResponse, error) {
	agent := r.resolve(msg.ChatID)
	if agent == nil {
		return nil, fmt.Errorf("no agent found for chat %s", msg.ChatID)
	}

	slog.Debug("routing message", "chat", msg.ChatID, "agent", agent.Id)
	return agent.HandleMessage(ctx, msg, streamCb)
}

// resolve finds the appropriate agent for a given chat ID.
func (r *Router) resolve(chatID string) *Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Check group mapping first
	if name, ok := r.groupMap[chatID]; ok {
		if agent, ok := r.agents[name]; ok {
			return agent
		}
	}

	// Fall back to default agent
	if agent, ok := r.agents[r.defaultAgent]; ok {
		return agent
	}

	return nil
}

// Resolve returns the agent responsible for a given chat ID.
// It is a public wrapper around the private resolve method for use by
// command handlers that need direct access to the agent (e.g. session management).
func (r *Router) Resolve(chatID string) *Agent {
	return r.resolve(chatID)
}

// GetAgent returns an agent by name.
func (r *Router) GetAgent(name string) (*Agent, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.agents[name]
	return a, ok
}

// ResolveSession returns the session store and session for a given chat ID.
// It resolves the responsible agent and returns its session store together
// with the (possibly newly created) session. Returns (nil, nil) when no
// agent handles the chat.
func (r *Router) ResolveSession(chatID string) (session.Store, *session.Session) {
	agent := r.resolve(chatID)
	if agent == nil {
		return nil, nil
	}
	return agent.Sessions, agent.Sessions.Get(chatID)
}
