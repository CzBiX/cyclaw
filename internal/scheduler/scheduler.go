package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// TaskAction is called when a scheduled task fires.
type TaskAction func(ctx context.Context, task *Task) error

// Task represents a scheduled task.
type Task struct {
	ID       string     `json:"id"`
	Schedule string     `json:"schedule"`           // cron expression
	Action   string     `json:"action"`             // description of what to do
	AgentID  string     `json:"agent_id,omitempty"` // which agent should handle it
	LastRun  *time.Time `json:"last_run,omitempty"` // last successful execution time
	system   bool
}

// Scheduler manages periodic tasks using robfig/cron.
type Scheduler struct {
	cron        *cron.Cron
	mu          sync.Mutex
	tasks       map[string]*Task
	entryIDs    map[string]cron.EntryID
	persistFile string
	stateFile   string // stores LastRun times for all tasks (including system)
	action      TaskAction
}

// New creates a new scheduler.
func New(persistFile string, action TaskAction) *Scheduler {
	var stateFile string
	if persistFile != "" {
		// Derive state file path from persist file: scheduler.json -> scheduler_state.json
		ext := filepath.Ext(persistFile)
		base := strings.TrimSuffix(persistFile, ext)
		stateFile = base + "_state" + ext
	}
	return &Scheduler{
		cron:        cron.New(),
		tasks:       make(map[string]*Task),
		entryIDs:    make(map[string]cron.EntryID),
		persistFile: persistFile,
		stateFile:   stateFile,
		action:      action,
	}
}

// Start begins the cron scheduler.
// After loading persisted tasks it checks for any missed executions that
// should have fired while the process was not running, and runs them once.
func (s *Scheduler) Start() error {
	// Load persisted user tasks
	if err := s.load(); err != nil {
		slog.Warn("failed to load scheduled tasks", "error", err)
	}

	// Restore LastRun times for all tasks (user + system).
	// System tasks must be registered via AddSystem before Start is called.
	if err := s.loadState(); err != nil {
		slog.Warn("failed to load scheduler state", "error", err)
	}

	s.cron.Start()
	slog.Debug("scheduler started", "tasks", len(s.tasks))

	// Catch up on tasks that were missed while we were offline.
	s.checkMissed()

	return nil
}

// Stop gracefully stops the scheduler.
func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
	slog.Debug("scheduler stopped")
}

// registerTask registers a task and its execution function in the cron scheduler.
// It is used by both Add and load to avoid duplicated registration logic.
func (s *Scheduler) registerTask(task *Task, exec func()) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entryID, err := s.cron.AddFunc(task.Schedule, exec)
	if err != nil {
		return err
	}

	s.tasks[task.ID] = task
	s.entryIDs[task.ID] = entryID
	return nil
}

// Add creates a new scheduled task.
func (s *Scheduler) Add(task *Task) error {
	if task.ID == "" {
		return fmt.Errorf("task ID cannot be empty")
	}
	if task.Schedule == "" {
		return fmt.Errorf("task schedule cannot be empty")
	}
	if task.Action == "" {
		return fmt.Errorf("task action cannot be empty")
	}

	if err := s.registerTask(task, func() {
		slog.Info("executing scheduled task", "id", task.ID, "action", task.Action)
		if err := s.action(context.Background(), task); err != nil {
			slog.Error("scheduled task failed", "id", task.ID, "error", err)
			return
		}
		s.markRun(task.ID)
	}); err != nil {
		return fmt.Errorf("add cron entry: %w", err)
	}

	slog.Info("scheduled task added",
		"id", task.ID,
		"schedule", task.Schedule,
		"action", task.Action,
		"total_tasks", len(s.tasks),
	)

	return s.persist()
}

// AddSystem registers a system task with a custom callback function.
// Unlike Add, the task uses the provided function instead of the default
// TaskAction, and it is NOT persisted to disk (it is re-registered on startup).
// System tasks are hidden from List and cannot be removed via Remove.
func (s *Scheduler) AddSystem(task *Task, fn func()) error {
	if task.ID == "" {
		return fmt.Errorf("system task ID cannot be empty")
	}
	if task.Schedule == "" {
		return fmt.Errorf("system task schedule cannot be empty")
	}

	task.system = true

	if err := s.registerTask(task, func() {
		slog.Info("executing system task", "id", task.ID)
		fn()
		s.markRun(task.ID)
	}); err != nil {
		return fmt.Errorf("add system cron entry: %w", err)
	}

	slog.Info("system task registered",
		"id", task.ID,
		"schedule", task.Schedule,
	)

	// System tasks are not persisted; they are re-registered on every startup.
	return nil
}

// Remove deletes a scheduled task. System tasks cannot be removed.
func (s *Scheduler) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("task not found: %s", id)
	}

	if task.system {
		return fmt.Errorf("cannot remove system task: %s", id)
	}

	entryID := s.entryIDs[id]
	s.cron.Remove(entryID)
	delete(s.tasks, id)
	delete(s.entryIDs, id)

	slog.Info("scheduled task removed", "id", id, "remaining_tasks", len(s.tasks))

	return s.persist()
}

// List returns all user-created scheduled tasks (system tasks are excluded).
func (s *Scheduler) List() []*Task {
	s.mu.Lock()
	defer s.mu.Unlock()

	tasks := make([]*Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		if !t.system {
			tasks = append(tasks, t)
		}
	}
	return tasks
}

// checkMissed inspects all registered tasks and fires any that should have
// executed while the scheduler was not running. For each task with a recorded
// LastRun, it computes the next scheduled time after LastRun. If that time is
// in the past (i.e. before now), the task is considered missed and is executed
// once immediately. Tasks that have never run (LastRun == nil) are skipped
// because there is no baseline to determine whether an execution was missed.
func (s *Scheduler) checkMissed() {
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	// Collect tasks that were missed so we can release the lock before executing.
	type missedTask struct {
		task *Task
		fn   func()
	}
	var missed []missedTask

	for _, task := range s.tasks {
		if task.LastRun == nil {
			continue
		}

		sched, err := cron.ParseStandard(task.Schedule)
		if err != nil {
			slog.Warn("checkMissed: invalid schedule, skipping", "id", task.ID, "error", err)
			continue
		}

		nextAfterLastRun := sched.Next(*task.LastRun)
		if nextAfterLastRun.Before(now) {
			if task.system {
				// System tasks use custom callbacks registered via AddSystem.
				// We retrieve the cron entry's wrapped job to invoke it.
				if entryID, ok := s.entryIDs[task.ID]; ok {
					entry := s.cron.Entry(entryID)
					if entry.WrappedJob != nil {
						missed = append(missed, missedTask{task: task, fn: func() { entry.WrappedJob.Run() }})
					}
				}
			} else {
				t := task // capture for closure
				missed = append(missed, missedTask{task: task, fn: func() {
					if err := s.action(context.Background(), t); err != nil {
						slog.Error("missed task execution failed", "id", t.ID, "error", err)
						return
					}
					s.markRun(t.ID)
				}})
			}
		}
	}

	// Execute missed tasks outside the lock.
	go func() {
		for _, m := range missed {
			slog.Info("executing missed scheduled task", "id", m.task.ID, "last_run", m.task.LastRun)
			m.fn()
		}
	}()
}

// markRun records the current time as the last run time for a task and persists.
func (s *Scheduler) markRun(taskID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if t, ok := s.tasks[taskID]; ok {
		now := time.Now()
		t.LastRun = &now
	}

	// Best-effort persist; errors are logged but not fatal.
	if err := s.persistState(); err != nil {
		slog.Warn("failed to persist state after marking task run", "id", taskID, "error", err)
	}
	if err := s.persist(); err != nil {
		slog.Warn("failed to persist tasks after marking task run", "id", taskID, "error", err)
	}
}

// persistState saves LastRun times for all tasks (including system tasks) to the state file.
func (s *Scheduler) persistState() error {
	if s.stateFile == "" {
		return nil
	}

	state := make(map[string]*time.Time, len(s.tasks))
	for id, t := range s.tasks {
		if t.LastRun != nil {
			state[id] = t.LastRun
		}
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	return os.WriteFile(s.stateFile, data, 0o600)
}

// loadState restores LastRun times from the state file into registered tasks.
func (s *Scheduler) loadState() error {
	if s.stateFile == "" {
		return nil
	}

	data, err := os.ReadFile(s.stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read state file: %w", err)
	}

	var state map[string]*time.Time
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("unmarshal state: %w", err)
	}

	for id, lastRun := range state {
		if t, ok := s.tasks[id]; ok {
			t.LastRun = lastRun
		}
	}

	return nil
}

// persist saves user-created tasks to disk (system tasks are excluded).
func (s *Scheduler) persist() error {
	if s.persistFile == "" {
		return nil
	}

	userTasks := make(map[string]*Task, len(s.tasks))
	for id, t := range s.tasks {
		if !t.system {
			userTasks[id] = t
		}
	}

	data, err := json.MarshalIndent(userTasks, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal tasks: %w", err)
	}

	return os.WriteFile(s.persistFile, data, 0o600)
}

// load reads tasks from disk and re-registers them.
func (s *Scheduler) load() error {
	if s.persistFile == "" {
		return nil
	}

	data, err := os.ReadFile(s.persistFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read persist file: %w", err)
	}

	var tasks map[string]*Task
	if err := json.Unmarshal(data, &tasks); err != nil {
		return fmt.Errorf("unmarshal tasks: %w", err)
	}

	for _, task := range tasks {
		t := task // capture for closure
		if err := s.registerTask(t, func() {
			slog.Info("executing scheduled task", "id", t.ID, "action", t.Action)
			if err := s.action(context.Background(), t); err != nil {
				slog.Error("scheduled task failed", "id", t.ID, "error", err)
				return
			}
			s.markRun(t.ID)
		}); err != nil {
			slog.Warn("failed to restore task", "id", t.ID, "error", err)
			continue
		}
	}

	return nil
}
