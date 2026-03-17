package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"cyclaw/internal/scheduler"
)

type CronTool struct {
	scheduler *scheduler.Scheduler
}

func NewCronTool(sched *scheduler.Scheduler) *CronTool {
	return &CronTool{scheduler: sched}
}

func (t *CronTool) Name() string { return "cron" }

func (t *CronTool) Description() string {
	return "Manage scheduled/recurring cron tasks."
}

func (t *CronTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"operation": {
				"type": "string",
				"enum": ["add", "remove", "list"],
				"description": "Operation to perform (default: list)."
			},
			"id": {
				"type": "string",
				"description": "Task ID (auto-generated for add if omitted)."
			},
			"schedule": {
				"type": "string",
				"description": "Cron expression, e.g. '35 9 * * *'."
			},
			"action": {
				"type": "string",
				"description": "Action to perform. Example: 'Send daily report email'."
			}
		}
	}`)
}

func (t *CronTool) Execute(_ context.Context, params json.RawMessage) (string, error) {
	var p struct {
		Operation string `json:"operation"`
		ID        string `json:"id"`
		Schedule  string `json:"schedule"`
		Action    string `json:"action"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("parse params: %w", err)
	}

	if p.Operation == "" {
		p.Operation = "list"
	}

	switch p.Operation {
	case "add":
		if p.Schedule == "" || p.Action == "" {
			return "", fmt.Errorf("add requires schedule and action")
		}
		if p.ID == "" {
			p.ID = fmt.Sprintf("task-%s", generateID())
		}

		task := &scheduler.Task{
			ID:       p.ID,
			Schedule: p.Schedule,
			Action:   p.Action,
		}
		if err := t.scheduler.Add(task); err != nil {
			return "", err
		}
		return fmt.Sprintf("Task %q scheduled: %s (%s)", p.ID, p.Schedule, p.Action), nil

	case "remove":
		if p.ID == "" {
			return "", fmt.Errorf("remove requires id")
		}
		if err := t.scheduler.Remove(p.ID); err != nil {
			return "", err
		}
		return fmt.Sprintf("Task %q removed", p.ID), nil

	case "list":
		tasks := t.scheduler.List()
		if len(tasks) == 0 {
			return "No scheduled tasks.", nil
		}
		var sb strings.Builder
		sb.WriteString("Scheduled tasks:\n")
		for _, task := range tasks {
			fmt.Fprintf(&sb, "- [%s] %s → %s\n", task.ID, task.Schedule, task.Action)
		}
		return sb.String(), nil

	default:
		return "", fmt.Errorf("unknown operation: %s", p.Operation)
	}
}
