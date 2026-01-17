package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"okrchestra/internal/daemon"
	"okrchestra/internal/workspace"
)

// TestWatchTriggersEndToEnd verifies that watch_tick jobs correctly detect
// file changes and enqueue follow-up jobs.
func TestWatchTriggersEndToEnd(t *testing.T) {
	tmpDir := t.TempDir()
	ws := &workspace.Workspace{
		Root:         tmpDir,
		OKRsDir:      filepath.Join(tmpDir, "okrs"),
		CultureDir:   filepath.Join(tmpDir, "culture"),
		MetricsDir:   filepath.Join(tmpDir, "metrics"),
		ArtifactsDir: filepath.Join(tmpDir, "artifacts"),
		AuditDir:     filepath.Join(tmpDir, "audit"),
		AuditDBPath:  filepath.Join(tmpDir, "audit", "audit.sqlite"),
		StateDBPath:  filepath.Join(tmpDir, "audit", "daemon.sqlite"),
		LogDir:       filepath.Join(tmpDir, "audit", "logs"),
	}

	// Create directories
	if err := ws.EnsureDirs(); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}

	// Create daemon
	d, err := daemon.New(daemon.Config{
		Workspace:    ws,
		StorePath:    ws.StateDBPath,
		TimeZone:     "UTC",
		LeaseOwner:   "test-daemon",
		LeaseFor:     30 * time.Second,
		PollInterval: 100 * time.Millisecond, // Fast polling for test
	})
	if err != nil {
		t.Fatalf("create daemon: %v", err)
	}
	defer d.Close()

	// Enqueue an initial watch_tick job
	now := time.Now()
	_, _, err = d.Store.EnqueueUnique("watch_tick", now, map[string]any{
		"scheduled_time": now.Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("enqueue initial watch_tick: %v", err)
	}

	// Execute the initial watch_tick (establishes baseline)
	ctx := context.Background()
	if err := d.Store.SetKV("daemon_store", "test"); err != nil {
		t.Fatalf("set kv: %v", err)
	}

	// Claim and execute the watch_tick job
	job, err := d.Store.ClaimNext(now, "test-daemon", 30*time.Second)
	if err != nil {
		t.Fatalf("claim job: %v", err)
	}
	if job == nil {
		t.Fatal("expected job to be available")
	}

	// Execute with store in context
	ctxWithStore := context.WithValue(ctx, "daemon_store", d.Store)
	handler := d.Handlers["watch_tick"]
	result, err := handler(ctxWithStore, ws, job)
	if err != nil {
		t.Fatalf("execute watch_tick: %v", err)
	}
	t.Logf("Initial watch result: %v", result)

	// Mark job as succeeded
	if err := d.Store.Succeed(job.ID, result); err != nil {
		t.Fatalf("succeed job: %v", err)
	}

	// Now make changes to trigger follow-up jobs
	// Ensure directories exist
	if err := os.MkdirAll(ws.OKRsDir, 0o755); err != nil {
		t.Fatalf("create okrs dir: %v", err)
	}
	if err := os.MkdirAll(ws.MetricsDir, 0o755); err != nil {
		t.Fatalf("create metrics dir: %v", err)
	}

	// 1. Add a file to okrs directory
	okrFile := filepath.Join(ws.OKRsDir, "org.yml")
	okrContent := `objectives:
  - id: test-obj-1
    title: Test Objective
    status: active
    key_results:
      - id: kr-1
        title: Test KR
        metric: test_metric
        target: 100
`
	if err := os.WriteFile(okrFile, []byte(okrContent), 0o644); err != nil {
		t.Fatalf("write okr file: %v", err)
	}

	// 2. Add manual metrics file
	manualMetricsFile := filepath.Join(ws.MetricsDir, "manual.yml")
	manualContent := `metrics:
  - name: test_metric
    value: 50
    date: 2024-01-01
`
	if err := os.WriteFile(manualMetricsFile, []byte(manualContent), 0o644); err != nil {
		t.Fatalf("write manual metrics: %v", err)
	}

	// 3. Create a plan directory and file
	planDir := filepath.Join(ws.ArtifactsDir, "plans", "2024-01-01")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatalf("create plan dir: %v", err)
	}
	planFile := filepath.Join(planDir, "plan.json")
	planContent := `{"as_of": "2024-01-01", "items": []}`
	if err := os.WriteFile(planFile, []byte(planContent), 0o644); err != nil {
		t.Fatalf("write plan file: %v", err)
	}

	// Wait a bit to ensure time has progressed
	time.Sleep(10 * time.Millisecond)

	// Create a second job directly for testing
	job2 := &daemon.Job{
		ID:   "watch_tick_2",
		Type: "watch_tick",
	}

	// Execute the second watch_tick directly (simulates another poll)
	result2, err := handler(ctxWithStore, ws, job2)
	if err != nil {
		t.Fatalf("execute second watch_tick: %v", err)
	}
	t.Logf("Second watch result: %v", result2)

	// Verify that follow-up jobs were enqueued
	queuedJobs, err := d.Store.ListQueued(100)
	if err != nil {
		t.Fatalf("list queued jobs: %v", err)
	}

	t.Logf("Found %d queued jobs", len(queuedJobs))

	// Check for expected job types
	jobCounts := make(map[string]int)
	for _, j := range queuedJobs {
		jobCounts[j.Type]++
		t.Logf("  - Job: %s (type: %s)", j.ID, j.Type)
	}

	// Should have kr_measure jobs (triggered by okrs change and manual.yml change)
	if jobCounts["kr_measure"] < 1 {
		t.Errorf("expected at least 1 kr_measure job, got %d", jobCounts["kr_measure"])
	}

	// Should have plan_generate job (triggered by okrs change)
	if jobCounts["plan_generate"] < 1 {
		t.Errorf("expected at least 1 plan_generate job, got %d", jobCounts["plan_generate"])
	}

	// Should have plan_execute job (triggered by new plan)
	if jobCounts["plan_execute"] < 1 {
		t.Errorf("expected at least 1 plan_execute job, got %d", jobCounts["plan_execute"])
	}

	// Verify the result indicated changes were detected
	resultMap := result2.(map[string]any)
	if resultMap["status"] != "changes_detected" {
		t.Errorf("expected status 'changes_detected', got %v", resultMap["status"])
	}
}

// TestSchedulerWatchTickIntegration verifies that the scheduler correctly
// schedules watch_tick jobs at 30-second intervals.
func TestSchedulerWatchTickIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "daemon.sqlite")

	store, err := daemon.Open(storePath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	scheduler, err := daemon.NewScheduler(store, "UTC")
	if err != nil {
		t.Fatalf("create scheduler: %v", err)
	}

	// Set initial watermark
	lastWatermark := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	if err := store.SetKV("scheduler_watermark", lastWatermark.Format(time.RFC3339)); err != nil {
		t.Fatalf("set initial watermark: %v", err)
	}

	// Run scheduler tick simulating 2 minutes passing
	now := time.Date(2024, 1, 1, 10, 2, 0, 0, time.UTC)
	if err := scheduler.Tick(now); err != nil {
		t.Fatalf("scheduler tick: %v", err)
	}

	// Check scheduled jobs
	queuedJobs, err := store.ListQueued(100)
	if err != nil {
		t.Fatalf("list queued jobs: %v", err)
	}

	// Count watch_tick jobs
	watchTickCount := 0
	for _, j := range queuedJobs {
		if j.Type == "watch_tick" {
			watchTickCount++
			t.Logf("Scheduled watch_tick: %s at %s", j.ID, j.ScheduledAt)
		}
	}

	// Should have scheduled 4 watch_tick jobs:
	// 10:00:30, 10:01:00, 10:01:30, 10:02:00
	expectedWatchTicks := 4
	if watchTickCount != expectedWatchTicks {
		t.Errorf("expected %d watch_tick jobs, got %d", expectedWatchTicks, watchTickCount)
	}

	// Verify watch_ticks are scheduled at 30-second intervals
	watchTicks := []time.Time{}
	for _, j := range queuedJobs {
		if j.Type == "watch_tick" {
			watchTicks = append(watchTicks, j.ScheduledAt)
		}
	}

	for i := 1; i < len(watchTicks); i++ {
		diff := watchTicks[i].Sub(watchTicks[i-1])
		if diff != 30*time.Second {
			t.Errorf("watch_tick interval mismatch: expected 30s, got %v between tick %d and %d", diff, i-1, i)
		}
	}
}
