package daemon

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"okrchestra/internal/workspace"
)

func TestWatchFile(t *testing.T) {
	// Create temporary directory and store
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "test.db")
	store, err := Open(storePath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.yml")
	if err := os.WriteFile(testFile, []byte("initial content"), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	// First watch should detect file (no previous state)
	changed, err := watchFile(store, testFile, "test_watch_key")
	if err != nil {
		t.Fatalf("first watch failed: %v", err)
	}
	if !changed {
		t.Error("expected change on first watch")
	}

	// Second watch with no changes should not detect change
	changed, err = watchFile(store, testFile, "test_watch_key")
	if err != nil {
		t.Fatalf("second watch failed: %v", err)
	}
	if changed {
		t.Error("expected no change on second watch")
	}

	// Modify file
	time.Sleep(10 * time.Millisecond) // Ensure different mtime
	if err := os.WriteFile(testFile, []byte("modified content"), 0o644); err != nil {
		t.Fatalf("modify test file: %v", err)
	}

	// Third watch should detect change
	changed, err = watchFile(store, testFile, "test_watch_key")
	if err != nil {
		t.Fatalf("third watch failed: %v", err)
	}
	if !changed {
		t.Error("expected change after modification")
	}

	// Fourth watch should not detect change
	changed, err = watchFile(store, testFile, "test_watch_key")
	if err != nil {
		t.Fatalf("fourth watch failed: %v", err)
	}
	if changed {
		t.Error("expected no change on fourth watch")
	}
}

func TestWatchDirectory(t *testing.T) {
	// Create temporary directory and store
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "test.db")
	store, err := Open(storePath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	watchDir := filepath.Join(tmpDir, "watched")
	if err := os.MkdirAll(watchDir, 0o755); err != nil {
		t.Fatalf("create watch dir: %v", err)
	}

	// Create initial files
	file1 := filepath.Join(watchDir, "file1.yml")
	if err := os.WriteFile(file1, []byte("content1"), 0o644); err != nil {
		t.Fatalf("write file1: %v", err)
	}

	// First watch should detect files
	changes, err := watchDirectory(store, watchDir, "test_watch_dir")
	if err != nil {
		t.Fatalf("first watch failed: %v", err)
	}
	if len(changes) != 1 {
		t.Errorf("expected 1 change, got %d", len(changes))
	}

	// Second watch with no changes
	changes, err = watchDirectory(store, watchDir, "test_watch_dir")
	if err != nil {
		t.Fatalf("second watch failed: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("expected 0 changes, got %d", len(changes))
	}

	// Add a new file
	file2 := filepath.Join(watchDir, "file2.json")
	if err := os.WriteFile(file2, []byte("content2"), 0o644); err != nil {
		t.Fatalf("write file2: %v", err)
	}

	// Third watch should detect new file
	changes, err = watchDirectory(store, watchDir, "test_watch_dir")
	if err != nil {
		t.Fatalf("third watch failed: %v", err)
	}
	if len(changes) != 1 {
		t.Errorf("expected 1 change (new file), got %d", len(changes))
	}

	// Modify existing file
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(file1, []byte("modified content1"), 0o644); err != nil {
		t.Fatalf("modify file1: %v", err)
	}

	// Fourth watch should detect modification
	changes, err = watchDirectory(store, watchDir, "test_watch_dir")
	if err != nil {
		t.Fatalf("fourth watch failed: %v", err)
	}
	if len(changes) != 1 {
		t.Errorf("expected 1 change (modified file), got %d", len(changes))
	}

	// Delete a file
	if err := os.Remove(file2); err != nil {
		t.Fatalf("remove file2: %v", err)
	}

	// Fifth watch should detect deletion
	changes, err = watchDirectory(store, watchDir, "test_watch_dir")
	if err != nil {
		t.Fatalf("fifth watch failed: %v", err)
	}
	if len(changes) != 1 {
		t.Errorf("expected 1 change (deleted file), got %d", len(changes))
	}
}

func TestHandleWatchTick(t *testing.T) {
	// Create temporary workspace
	tmpDir := t.TempDir()
	ws := &workspace.Workspace{
		Root:         tmpDir,
		OKRsDir:      filepath.Join(tmpDir, "okrs"),
		MetricsDir:   filepath.Join(tmpDir, "metrics"),
		ArtifactsDir: filepath.Join(tmpDir, "artifacts"),
	}

	// Create directories
	for _, dir := range []string{ws.OKRsDir, ws.MetricsDir, ws.ArtifactsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("create dir %s: %v", dir, err)
		}
	}

	// Create store
	storePath := filepath.Join(tmpDir, "test.db")
	store, err := Open(storePath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	// Create job
	job := &Job{
		ID:   "test_watch_tick",
		Type: "watch_tick",
	}

	// Add store to context
	ctx := context.WithValue(context.Background(), "daemon_store", store)

	// First run - should not detect changes (baseline)
	result, err := handleWatchTick(ctx, ws, job)
	if err != nil {
		t.Fatalf("first watch tick failed: %v", err)
	}
	if result.(map[string]any)["status"] != "no_changes" {
		t.Errorf("expected no_changes, got %v", result.(map[string]any)["status"])
	}

	// Add a file to okrs
	okrFile := filepath.Join(ws.OKRsDir, "org.yml")
	if err := os.WriteFile(okrFile, []byte("objectives: []"), 0o644); err != nil {
		t.Fatalf("write okr file: %v", err)
	}

	// Second run - should detect changes in okrs
	result, err = handleWatchTick(ctx, ws, job)
	if err != nil {
		t.Fatalf("second watch tick failed: %v", err)
	}
	if result.(map[string]any)["status"] != "changes_detected" {
		t.Errorf("expected changes_detected, got %v", result.(map[string]any)["status"])
	}
	if result.(map[string]any)["changes_count"].(int) < 1 {
		t.Errorf("expected at least 1 change, got %d", result.(map[string]any)["changes_count"])
	}

	// Check that jobs were enqueued
	jobs, err := store.ListQueued(10)
	if err != nil {
		t.Fatalf("list queued jobs: %v", err)
	}
	if len(jobs) < 2 {
		t.Errorf("expected at least 2 queued jobs (kr_measure and plan_generate), got %d", len(jobs))
	}

	// Verify job types
	jobTypes := make(map[string]bool)
	for _, j := range jobs {
		jobTypes[j.Type] = true
	}
	if !jobTypes["kr_measure"] {
		t.Error("expected kr_measure job to be enqueued")
	}
	if !jobTypes["plan_generate"] {
		t.Error("expected plan_generate job to be enqueued")
	}
}

func TestScheduleWatchTicks(t *testing.T) {
	// Create temporary store
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "test.db")
	store, err := Open(storePath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	// Create scheduler
	scheduler, err := NewScheduler(store, "UTC")
	if err != nil {
		t.Fatalf("create scheduler: %v", err)
	}

	// Schedule watch ticks over a 2-minute window
	lastWatermark := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	now := time.Date(2024, 1, 1, 10, 2, 0, 0, time.UTC)

	err = scheduler.scheduleWatchTicks(lastWatermark, now)
	if err != nil {
		t.Fatalf("schedule watch ticks: %v", err)
	}

	// Check that jobs were scheduled
	jobs, err := store.ListQueued(100)
	if err != nil {
		t.Fatalf("list queued jobs: %v", err)
	}

	// Should have scheduled 4 watch_tick jobs:
	// 10:00:30, 10:01:00, 10:01:30, 10:02:00
	expectedCount := 4
	actualCount := 0
	for _, job := range jobs {
		if job.Type == "watch_tick" {
			actualCount++
		}
	}

	if actualCount != expectedCount {
		t.Errorf("expected %d watch_tick jobs, got %d", expectedCount, actualCount)
	}
}
