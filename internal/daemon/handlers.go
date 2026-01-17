package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"okrchestra/internal/adapters"
	"okrchestra/internal/metrics"
	"okrchestra/internal/planner"
	"okrchestra/internal/workspace"
)

// DefaultHandlers returns the map of built-in daemon handlers.
func DefaultHandlers() map[string]HandlerFunc {
	return map[string]HandlerFunc{
		"kr_measure":    handleKRMeasure,
		"plan_generate": handlePlanGenerate,
		"plan_execute":  handlePlanExecute,
		"watch_tick":    handleWatchTick,
	}
}

// handleKRMeasure implements the kr_measure job handler.
// It invokes the metric collection logic and writes a snapshot to <workspace>/metrics/snapshots/
func handleKRMeasure(ctx context.Context, ws *workspace.Workspace, job *Job) (any, error) {
	// Parse payload
	var payload struct {
		AsOf       string `json:"as_of"`
		RepoDir    string `json:"repo_dir"`
		MetricsDir string `json:"metrics_dir"`
	}
	if job.PayloadJSON != "" && job.PayloadJSON != "{}" {
		if err := json.Unmarshal([]byte(job.PayloadJSON), &payload); err != nil {
			return nil, fmt.Errorf("parse payload: %w", err)
		}
	}

	// Defaults
	asOf := time.Now().UTC().Truncate(24 * time.Hour)
	if payload.AsOf != "" {
		parsed, err := time.ParseInLocation("2006-01-02", payload.AsOf, time.UTC)
		if err != nil {
			return nil, fmt.Errorf("parse as_of: %w", err)
		}
		asOf = parsed.UTC().Truncate(24 * time.Hour)
	}

	repoDir := ws.Root
	if payload.RepoDir != "" {
		repoDir = payload.RepoDir
	}

	metricsDir := ws.MetricsDir
	if payload.MetricsDir != "" {
		metricsDir = payload.MetricsDir
	}

	snapshotsDir := filepath.Join(metricsDir, "snapshots")
	ciReportPath := filepath.Join(metricsDir, "ci_report.json")
	manualPath := filepath.Join(metricsDir, "manual.yml")

	// Collect metrics using same logic as CLI
	providers := []metrics.Provider{
		&metrics.GitProvider{RepoDir: repoDir, AsOf: asOf},
		&metrics.CIProvider{ReportPath: ciReportPath, AsOf: asOf},
		&metrics.ManualProvider{Path: manualPath, AsOf: asOf},
	}

	points, err := metrics.CollectAll(ctx, providers)
	if err != nil {
		return nil, fmt.Errorf("collect metrics: %w", err)
	}

	snapshotPath := metrics.SnapshotPathForDate(snapshotsDir, asOf)
	snapshot := metrics.Snapshot{
		AsOf:   asOf.Format("2006-01-02"),
		Points: points,
	}

	if err := metrics.WriteSnapshot(snapshotPath, snapshot); err != nil {
		return nil, fmt.Errorf("write snapshot: %w", err)
	}

	return map[string]any{
		"snapshot_path": snapshotPath,
		"metric_count":  len(points),
	}, nil
}

// handlePlanGenerate implements the plan_generate job handler.
// It invokes planner.Generate using <workspace>/okrs and writes to <workspace>/artifacts/plans/<date>/plan.json
func handlePlanGenerate(ctx context.Context, ws *workspace.Workspace, job *Job) (any, error) {
	// Parse payload
	var payload struct {
		AsOf        string `json:"as_of"`
		ObjectiveID string `json:"objective_id"`
		KRID        string `json:"kr_id"`
		AgentRole   string `json:"agent_role"`
	}
	if job.PayloadJSON != "" && job.PayloadJSON != "{}" {
		if err := json.Unmarshal([]byte(job.PayloadJSON), &payload); err != nil {
			return nil, fmt.Errorf("parse payload: %w", err)
		}
	}

	// Defaults
	asOf := time.Now().UTC().Truncate(24 * time.Hour)
	if payload.AsOf != "" {
		parsed, err := time.ParseInLocation("2006-01-02", payload.AsOf, time.UTC)
		if err != nil {
			return nil, fmt.Errorf("parse as_of: %w", err)
		}
		asOf = parsed.UTC().Truncate(24 * time.Hour)
	}

	agentRole := "software_engineer"
	if payload.AgentRole != "" {
		agentRole = payload.AgentRole
	}

	outDir := filepath.Join(ws.ArtifactsDir, "plans")

	// Generate plan using same logic as CLI
	result, err := planner.GeneratePlan(planner.GenerateOptions{
		OKRsDir:       ws.OKRsDir,
		OutputBaseDir: outDir,
		AsOf:          asOf,
		ObjectiveID:   payload.ObjectiveID,
		KRID:          payload.KRID,
		AgentRole:     agentRole,
	})
	if err != nil {
		return nil, fmt.Errorf("generate plan: %w", err)
	}

	return map[string]any{
		"plan_path": result.PlanPath,
		"plan_date": result.Plan.AsOf,
	}, nil
}

// handlePlanExecute implements the plan_execute job handler.
// It finds the most recent plan (or uses plan_path from payload), runs it with the specified adapter,
// and writes run artifacts to <workspace>/artifacts/runs/<run-id>/
func handlePlanExecute(ctx context.Context, ws *workspace.Workspace, job *Job) (any, error) {
	// Parse payload
	var payload struct {
		Adapter  string `json:"adapter"`
		Timeout  string `json:"timeout"`
		Follow   bool   `json:"follow"`
		PlanPath string `json:"plan_path"`
	}
	if job.PayloadJSON != "" && job.PayloadJSON != "{}" {
		if err := json.Unmarshal([]byte(job.PayloadJSON), &payload); err != nil {
			return nil, fmt.Errorf("parse payload: %w", err)
		}
	}

	// Defaults
	adapterName := "mock"
	if payload.Adapter != "" {
		adapterName = payload.Adapter
	}

	var timeout time.Duration
	if payload.Timeout != "" {
		parsed, err := time.ParseDuration(payload.Timeout)
		if err != nil {
			return nil, fmt.Errorf("parse timeout: %w", err)
		}
		timeout = parsed
	}

	// Resolve adapter
	var adapter adapters.AgentAdapter
	switch adapterName {
	case "codex":
		adapter = &adapters.CodexAdapter{}
	case "mock":
		adapter = &adapters.MockAdapter{}
	default:
		return nil, fmt.Errorf("unknown adapter: %s", adapterName)
	}

	// Resolve plan path
	planPath := payload.PlanPath
	if planPath == "" {
		// Find most recent plan
		plansDir := filepath.Join(ws.ArtifactsDir, "plans")
		recent, err := findMostRecentPlan(plansDir)
		if err != nil {
			return nil, fmt.Errorf("find recent plan: %w", err)
		}
		planPath = recent
	}

	// Ensure plan path is absolute or resolved relative to workspace
	if !filepath.IsAbs(planPath) {
		planPath = filepath.Join(ws.Root, planPath)
	}

	// Set run base dir to workspace artifacts/runs
	runBaseDir := filepath.Join(ws.ArtifactsDir, "runs")

	// Run plan
	runResult, err := planner.RunPlan(ctx, planner.RunOptions{
		PlanPath:          planPath,
		WorkDir:           ws.Root,
		Adapter:           adapter,
		Timeout:           timeout,
		AuditLogger:       nil, // daemon has its own audit logger
		RunBaseDir:        runBaseDir,
		FollowTranscripts: false, // daemon doesn't follow output
	})

	if err != nil {
		return nil, fmt.Errorf("run plan: %w", err)
	}

	itemsSucceeded := len(runResult.ItemRuns)
	itemsFailed := len(runResult.Plan.Items) - itemsSucceeded

	return map[string]any{
		"run_id":          runResult.RunID,
		"run_dir":         runResult.RunDir,
		"items_total":     len(runResult.Plan.Items),
		"items_succeeded": itemsSucceeded,
		"items_failed":    itemsFailed,
	}, nil
}

// findMostRecentPlan searches for the most recent plan.json in the plans directory structure.
// It expects plans to be in subdirectories named by date (YYYY-MM-DD).
func findMostRecentPlan(plansDir string) (string, error) {
	entries, err := os.ReadDir(plansDir)
	if err != nil {
		return "", fmt.Errorf("read plans dir: %w", err)
	}

	var dateDirs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Check if it looks like a date directory (YYYY-MM-DD)
		if len(name) == 10 && name[4] == '-' && name[7] == '-' {
			dateDirs = append(dateDirs, name)
		}
	}

	if len(dateDirs) == 0 {
		return "", fmt.Errorf("no plan directories found in %s", plansDir)
	}

	// Sort date directories in reverse order (most recent first)
	sort.Slice(dateDirs, func(i, j int) bool {
		return strings.Compare(dateDirs[i], dateDirs[j]) > 0
	})

	// Return the most recent plan.json
	mostRecentDir := dateDirs[0]
	planPath := filepath.Join(plansDir, mostRecentDir, "plan.json")

	// Verify it exists
	if _, err := os.Stat(planPath); err != nil {
		return "", fmt.Errorf("plan.json not found in most recent dir %s: %w", mostRecentDir, err)
	}

	return planPath, nil
}
