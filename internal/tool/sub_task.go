package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
)

const maxSubTaskDepth = 2

// SubTaskExecutor is satisfied by any type that can run a sub-task on a
// named agent. In practice this is the agent.Router (adapted via a thin
// wrapper registered at startup).
type SubTaskExecutor interface {
	// ExecuteSubTask runs a sub-task on the named agent (empty string = caller's
	// own agent). Returns the final text response.
	ExecuteSubTask(ctx context.Context, agentName string, task string, instructions string, tools *Registry) (string, error)
}

// SubTaskTool delegates a task to another agent and returns the result.
type SubTaskTool struct {
	executor SubTaskExecutor
	registry *Registry // parent registry, used to build filtered registries
}

// NewSubTaskTool creates a new sub_task tool.
// executor provides the ability to invoke an agent's sub-task loop.
// registry is the parent tool registry used when filtering tools.
func NewSubTaskTool(executor SubTaskExecutor, registry *Registry) *SubTaskTool {
	return &SubTaskTool{executor: executor, registry: registry}
}

func (t *SubTaskTool) Name() string { return "sub_task" }

func (t *SubTaskTool) Description() string {
	return "Delegate a task to another agent (or yourself). " +
		"The sub-agent runs a full LLM-tool loop in a fresh session and returns the final text response. " +
		"Use this for complex multi-step tasks that benefit from focused, isolated execution."
}

func (t *SubTaskTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"task": {
				"type": "string",
				"description": "The instruction/prompt to send to the sub-agent."
			},
			"agent": {
				"type": "string",
				"description": "Target agent name. defaults to current agent."
			},
			"instructions": {
				"type": "string",
				"description": "Custom system prompt for the sub-agent. If omitted, the target agent's default prompt is used."
			},
			"tools": {
				"type": "array",
				"items": { "type": "string" },
				"description": "Subset of tool names to make available to the sub-agent. If empty, all tools are available."
			}
		},
		"required": ["task"]
	}`)
}

func (t *SubTaskTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p struct {
		Task         string   `json:"task"`
		Agent        string   `json:"agent"`
		Instructions string   `json:"instructions"`
		Tools        []string `json:"tools"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("parse params: %w", err)
	}

	if p.Task == "" {
		return "", fmt.Errorf("task parameter is required")
	}

	// Check recursion depth
	depth := SubTaskDepthFrom(ctx)
	if depth >= maxSubTaskDepth {
		return "", fmt.Errorf("maximum sub_task depth (%d) exceeded", maxSubTaskDepth)
	}

	// Increment depth for the sub-agent's context
	ctx = WithSubTaskDepth(ctx, depth+1)

	// Default to current agent
	agentName := p.Agent
	if agentName == "" {
		agentName = AgentIDFrom(ctx)
	}

	slog.Info("sub_task invoked",
		"target_agent", agentName,
		"task_len", len(p.Task),
		"depth", depth+1,
		"has_instructions", p.Instructions != "",
		"tool_filter", p.Tools,
	)

	// Build filtered tool registry if tools param is specified
	var filteredRegistry *Registry
	if len(p.Tools) > 0 {
		filteredRegistry = NewRegistry()

		// Validate all requested tool names first
		var unknown []string
		for _, name := range p.Tools {
			if _, ok := t.registry.Get(name); !ok {
				// Also check if it's "sub_task" itself (which is this tool)
				if name != t.Name() {
					unknown = append(unknown, name)
				}
			}
		}
		if len(unknown) > 0 {
			return "", fmt.Errorf("unknown tool(s): %s", strings.Join(unknown, ", "))
		}

		// Register the requested tools
		for _, name := range p.Tools {
			if name == t.Name() {
				// Register this sub_task tool itself for recursion
				filteredRegistry.Register(t)
				continue
			}
			if tool, ok := t.registry.Get(name); ok {
				filteredRegistry.Register(tool)
			}
		}
	}

	result, err := t.executor.ExecuteSubTask(ctx, agentName, p.Task, p.Instructions, filteredRegistry)
	if err != nil {
		return "", fmt.Errorf("sub_task failed: %w", err)
	}

	return result, nil
}
