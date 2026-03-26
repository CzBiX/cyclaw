package scheduler

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func noopAction(_ context.Context, _ *Task) error {
	return nil
}

func TestScheduler_AddAndList(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "scheduler.json")
	s := New(tmpFile, noopAction)

	task := &Task{
		ID:       "test-1",
		Schedule: "0 * * * *", // every hour
		Action:   "say hello",
		AgentID:  "main",
	}

	if err := s.Add(task); err != nil {
		t.Fatalf("Add() error: %v", err)
	}

	tasks := s.List()
	if len(tasks) != 1 {
		t.Fatalf("List() returned %d tasks, want 1", len(tasks))
	}
	if tasks[0].ID != "test-1" {
		t.Errorf("task ID = %q, want %q", tasks[0].ID, "test-1")
	}
}

func TestScheduler_Remove(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "scheduler.json")
	s := New(tmpFile, noopAction)

	task := &Task{
		ID:       "test-1",
		Schedule: "0 * * * *", // every hour
		Action:   "say hello",
		AgentID:  "main",
	}

	if err := s.Add(task); err != nil {
		t.Fatal(err)
	}

	if err := s.Remove("test-1"); err != nil {
		t.Fatalf("Remove() error: %v", err)
	}

	tasks := s.List()
	if len(tasks) != 0 {
		t.Errorf("List() returned %d tasks after removal, want 0", len(tasks))
	}
}

func TestScheduler_RemoveNotFound(t *testing.T) {
	s := New("", noopAction)
	if err := s.Remove("nonexistent"); err == nil {
		t.Fatal("Remove() expected error for nonexistent task")
	}
}

func TestScheduler_PersistAndLoad(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "scheduler.json")

	// Create scheduler, add tasks, and let it persist
	s1 := New(tmpFile, noopAction)

	if err := s1.Add(&Task{
		ID:       "persist-1",
		Schedule: "0 * * * *", // every hour
		Action:   "test persist",
		AgentID:  "main",
	}); err != nil {
		t.Fatal(err)
	}

	// Create a new scheduler and load from the same file
	s2 := New(tmpFile, noopAction)
	if err := s2.Start(); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer s2.Stop()

	tasks := s2.List()
	if len(tasks) != 1 {
		t.Fatalf("List() after reload returned %d tasks, want 1", len(tasks))
	}
	if tasks[0].ID != "persist-1" {
		t.Errorf("task ID = %q, want %q", tasks[0].ID, "persist-1")
	}
}

func TestScheduler_AddInvalidSchedule(t *testing.T) {
	s := New("", noopAction)

	task := &Task{
		ID:       "bad",
		Schedule: "invalid cron",
		Action:   "fail",
	}

	if err := s.Add(task); err == nil {
		t.Fatal("Add() expected error for invalid cron expression")
	}
}

func TestScheduler_SystemHiddenFromList(t *testing.T) {
	s := New("", noopAction)

	// Add a regular task
	if err := s.Add(&Task{
		ID:       "user-task",
		Schedule: "0 * * * *", // every hour
		Action:   "user action",
	}); err != nil {
		t.Fatal(err)
	}

	// Add a system task
	called := false
	if err := s.AddSystem(&Task{
		ID:       "system-task",
		Schedule: "0 0 * * *", // every day at midnight
		Action:   "__system__",
	}, func() { called = true }); err != nil {
		t.Fatal(err)
	}
	_ = called

	// List should only return the user task
	tasks := s.List()
	if len(tasks) != 1 {
		t.Fatalf("List() returned %d tasks, want 1 (system should be hidden)", len(tasks))
	}
	if tasks[0].ID != "user-task" {
		t.Errorf("task ID = %q, want %q", tasks[0].ID, "user-task")
	}
}

func TestScheduler_SystemCannotBeRemoved(t *testing.T) {
	s := New("", noopAction)

	if err := s.AddSystem(&Task{
		ID:       "system-task",
		Schedule: "0 0 * * *", // every day at midnight
		Action:   "__system__",
	}, func() {}); err != nil {
		t.Fatal(err)
	}

	err := s.Remove("system-task")
	if err == nil {
		t.Fatal("Remove() expected error for system task")
	}
}

func TestScheduler_SystemNotPersisted(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "scheduler.json")
	s1 := New(tmpFile, noopAction)

	// Add a regular task and a system task
	if err := s1.Add(&Task{
		ID:       "user-task",
		Schedule: "0 * * * *", // every hour
		Action:   "user action",
	}); err != nil {
		t.Fatal(err)
	}

	if err := s1.AddSystem(&Task{
		ID:       "system-task",
		Schedule: "0 0 * * *", // every day at midnight
		Action:   "__system__",
	}, func() {}); err != nil {
		t.Fatal(err)
	}

	// Load from the same file in a new scheduler — only user task should appear
	s2 := New(tmpFile, noopAction)
	if err := s2.Start(); err != nil {
		t.Fatal(err)
	}
	defer s2.Stop()

	tasks := s2.List()
	if len(tasks) != 1 {
		t.Fatalf("List() after reload returned %d tasks, want 1", len(tasks))
	}
	if tasks[0].ID != "user-task" {
		t.Errorf("task ID = %q, want %q", tasks[0].ID, "user-task")
	}
}

func TestScheduler_NoPersistFile(t *testing.T) {
	s := New("", noopAction)

	task := &Task{
		ID:       "test",
		Schedule: "0 * * * *", // every hour
		Action:   "test",
	}

	// Should work without a persist file
	if err := s.Add(task); err != nil {
		t.Fatalf("Add() error: %v", err)
	}

	tasks := s.List()
	if len(tasks) != 1 {
		t.Fatalf("List() returned %d tasks, want 1", len(tasks))
	}
}

func TestScheduler_MarkRunPersistsLastRun(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "scheduler.json")
	s := New(tmpFile, noopAction)

	if err := s.Add(&Task{
		ID:       "task-1",
		Schedule: "0 * * * *", // every hour
		Action:   "test",
	}); err != nil {
		t.Fatal(err)
	}

	// Simulate a successful run
	s.markRun("task-1")

	tasks := s.List()
	if len(tasks) != 1 {
		t.Fatal("expected 1 task")
	}
	if tasks[0].LastRun == nil {
		t.Fatal("LastRun should be set after markRun")
	}
	if time.Since(*tasks[0].LastRun) > 5*time.Second {
		t.Error("LastRun should be approximately now")
	}
}

func TestScheduler_CheckMissedFiresTask(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "scheduler.json")

	var executed atomic.Int32
	action := func(_ context.Context, _ *Task) error {
		executed.Add(1)
		return nil
	}

	// Create scheduler with a task that "last ran" 2 hours ago on an hourly schedule.
	s1 := New(tmpFile, action)
	twoHoursAgo := time.Now().Add(-2 * time.Hour)

	if err := s1.Add(&Task{
		ID:       "missed-task",
		Schedule: "0 * * * *", // every hour
		Action:   "should be caught up",
	}); err != nil {
		t.Fatal(err)
	}
	// Persist the state so it can be reloaded.
	s1.markRun("missed-task")
	// Overwrite LastRun back to 2 hours ago (markRun set it to now).
	s1.mu.Lock()
	s1.tasks["missed-task"].LastRun = &twoHoursAgo
	_ = s1.persistState()
	_ = s1.persist()
	s1.mu.Unlock()

	// Create a new scheduler that loads from persisted files.
	s2 := New(tmpFile, action)
	if err := s2.Start(); err != nil {
		t.Fatal(err)
	}
	defer s2.Stop()

	// Give the goroutine a moment to execute.
	time.Sleep(200 * time.Millisecond)

	if executed.Load() != 1 {
		t.Errorf("expected missed task to be executed once, got %d", executed.Load())
	}
}

func TestScheduler_CheckMissedSkipsRecentTask(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "scheduler.json")

	var executed atomic.Int32
	action := func(_ context.Context, _ *Task) error {
		executed.Add(1)
		return nil
	}

	// Create scheduler with a task that "last ran" just now — it should NOT be
	// treated as missed since the next scheduled time is in the future.
	s1 := New(tmpFile, action)
	now := time.Now()

	if err := s1.Add(&Task{
		ID:       "recent-task",
		Schedule: "0 * * * *", // every hour
		Action:   "recently ran",
	}); err != nil {
		t.Fatal(err)
	}
	s1.mu.Lock()
	s1.tasks["recent-task"].LastRun = &now
	_ = s1.persistState()
	s1.mu.Unlock()

	// Reload in a new scheduler
	s2 := New(tmpFile, action)
	if err := s2.Start(); err != nil {
		t.Fatal(err)
	}
	defer s2.Stop()

	time.Sleep(200 * time.Millisecond)

	if executed.Load() != 0 {
		t.Errorf("expected no missed execution for recent task, got %d", executed.Load())
	}
}

func TestScheduler_CheckMissedSystemTask(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "scheduler.json")

	var executed atomic.Int32

	// Phase 1: register system task and simulate a last run 2 hours ago.
	s1 := New(tmpFile, noopAction)
	if err := s1.AddSystem(&Task{
		ID:       "sys-task",
		Schedule: "0 * * * *", // every hour
	}, func() { executed.Add(1) }); err != nil {
		t.Fatal(err)
	}
	// Set LastRun to 2 hours ago and persist state.
	twoHoursAgo := time.Now().Add(-2 * time.Hour)
	s1.mu.Lock()
	s1.tasks["sys-task"].LastRun = &twoHoursAgo
	_ = s1.persistState()
	s1.mu.Unlock()

	// Phase 2: simulate a restart — register the same system task, then Start.
	s2 := New(tmpFile, noopAction)
	if err := s2.AddSystem(&Task{
		ID:       "sys-task",
		Schedule: "0 * * * *", // every hour
	}, func() { executed.Add(1) }); err != nil {
		t.Fatal(err)
	}
	if err := s2.Start(); err != nil {
		t.Fatal(err)
	}
	defer s2.Stop()

	time.Sleep(200 * time.Millisecond)

	if executed.Load() != 1 {
		t.Errorf("expected missed system task to be executed once, got %d", executed.Load())
	}
}

func TestScheduler_StatePersistAndLoad(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "scheduler.json")

	s1 := New(tmpFile, noopAction)
	if err := s1.Add(&Task{
		ID:       "state-test",
		Schedule: "0 * * * *", // every hour
		Action:   "test state",
	}); err != nil {
		t.Fatal(err)
	}
	s1.markRun("state-test")
	lastRun := s1.tasks["state-test"].LastRun

	// Reload and verify LastRun is restored
	s2 := New(tmpFile, noopAction)
	if err := s2.Start(); err != nil {
		t.Fatal(err)
	}
	defer s2.Stop()

	tasks := s2.List()
	if len(tasks) != 1 {
		t.Fatal("expected 1 task")
	}
	if tasks[0].LastRun == nil {
		t.Fatal("LastRun should be restored from state file")
	}
	if !tasks[0].LastRun.Equal(*lastRun) {
		t.Errorf("LastRun = %v, want %v", tasks[0].LastRun, lastRun)
	}
}
