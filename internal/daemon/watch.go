package daemon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"okrchestra/internal/workspace"
)

// WatchState tracks file modification times and hashes.
type WatchState struct {
	Path     string `json:"path"`
	ModTime  string `json:"mod_time"`
	Hash     string `json:"hash"`
	LastSeen string `json:"last_seen"`
}

// handleWatchTick implements the watch_tick job handler.
// It polls watched files and directories for changes and enqueues follow-up jobs.
// The store must be passed via the daemon's store field.
func handleWatchTick(ctx context.Context, ws *workspace.Workspace, job *Job) (any, error) {
	// Get store from context (passed by daemon)
	store, ok := ctx.Value("daemon_store").(*Store)
	if !ok || store == nil {
		return nil, fmt.Errorf("daemon store not available in context")
	}

	changes := []string{}
	now := time.Now()

	// Watch 1: okrs directory (human applied proposals)
	okrsChanges, err := watchDirectory(store, ws.OKRsDir, "watch_okrs_dir")
	if err != nil {
		return nil, fmt.Errorf("watch okrs dir: %w", err)
	}
	if len(okrsChanges) > 0 {
		changes = append(changes, fmt.Sprintf("okrs: %d files changed", len(okrsChanges)))
		// Enqueue kr_measure and plan_generate
		if _, _, err := store.EnqueueUnique("kr_measure", now, map[string]any{
			"trigger": "okrs_changed",
			"files":   okrsChanges,
		}); err != nil {
			return nil, fmt.Errorf("enqueue kr_measure: %w", err)
		}
		if _, _, err := store.EnqueueUnique("plan_generate", now, map[string]any{
			"trigger": "okrs_changed",
			"files":   okrsChanges,
		}); err != nil {
			return nil, fmt.Errorf("enqueue plan_generate: %w", err)
		}
	}

	// Watch 2: metrics/manual.yml
	manualMetricsPath := filepath.Join(ws.MetricsDir, "manual.yml")
	manualChanged, err := watchFile(store, manualMetricsPath, "watch_manual_yml")
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("watch manual.yml: %w", err)
	}
	if manualChanged {
		changes = append(changes, "manual.yml changed")
		// Enqueue kr_measure
		if _, _, err := store.EnqueueUnique("kr_measure", now, map[string]any{
			"trigger": "manual_yml_changed",
		}); err != nil {
			return nil, fmt.Errorf("enqueue kr_measure: %w", err)
		}
	}

	// Watch 3: new plans generated
	plansDir := filepath.Join(ws.ArtifactsDir, "plans")
	plansChanges, err := watchDirectory(store, plansDir, "watch_plans_dir")
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("watch plans dir: %w", err)
	}
	if len(plansChanges) > 0 {
		changes = append(changes, fmt.Sprintf("plans: %d files changed", len(plansChanges)))
		// Enqueue plan_execute for newly generated plans
		for _, planFile := range plansChanges {
			if filepath.Base(planFile) == "plan.json" {
				if _, _, err := store.EnqueueUnique("plan_execute", now, map[string]any{
					"trigger":   "new_plan_generated",
					"plan_path": planFile,
				}); err != nil {
					return nil, fmt.Errorf("enqueue plan_execute: %w", err)
				}
			}
		}
	}

	result := map[string]any{
		"checked_at":     now.Format(time.RFC3339),
		"changes_count":  len(changes),
		"changes_detail": changes,
	}

	if len(changes) > 0 {
		result["status"] = "changes_detected"
	} else {
		result["status"] = "no_changes"
	}

	return result, nil
}

// watchFile checks if a single file has changed since last check.
func watchFile(store *Store, filePath, kvKey string) (bool, error) {
	// Get file info
	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist yet, check if it existed before
			stateJSON, err := store.GetKV(kvKey)
			if err != nil {
				return false, fmt.Errorf("get watch state: %w", err)
			}
			if stateJSON == "" {
				// Never existed, no change
				return false, nil
			}
			// File was deleted, consider this a change
			return true, nil
		}
		return false, err
	}

	// Calculate file hash
	hash, err := hashFile(filePath)
	if err != nil {
		return false, fmt.Errorf("hash file: %w", err)
	}

	// Get previous state
	stateJSON, err := store.GetKV(kvKey)
	if err != nil {
		return false, fmt.Errorf("get watch state: %w", err)
	}

	var prevState WatchState
	if stateJSON != "" {
		if err := json.Unmarshal([]byte(stateJSON), &prevState); err != nil {
			return false, fmt.Errorf("parse watch state: %w", err)
		}
	}

	// Check if changed
	modTime := info.ModTime().UTC().Format(time.RFC3339)
	changed := prevState.Hash != hash

	// Update state
	newState := WatchState{
		Path:     filePath,
		ModTime:  modTime,
		Hash:     hash,
		LastSeen: time.Now().UTC().Format(time.RFC3339),
	}
	newStateJSON, err := json.Marshal(newState)
	if err != nil {
		return false, fmt.Errorf("marshal watch state: %w", err)
	}
	if err := store.SetKV(kvKey, string(newStateJSON)); err != nil {
		return false, fmt.Errorf("save watch state: %w", err)
	}

	return changed, nil
}

// watchDirectory checks if any files in a directory have changed since last check.
// Returns a list of file paths that have changed.
func watchDirectory(store *Store, dirPath, kvKeyPrefix string) ([]string, error) {
	// Get current files
	currentFiles := make(map[string]WatchState)
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.IsDir() {
			return nil
		}

		// Only watch certain file types
		ext := filepath.Ext(path)
		if ext != ".yml" && ext != ".yaml" && ext != ".json" {
			return nil
		}

		hash, err := hashFile(path)
		if err != nil {
			return fmt.Errorf("hash file %s: %w", path, err)
		}

		currentFiles[path] = WatchState{
			Path:     path,
			ModTime:  info.ModTime().UTC().Format(time.RFC3339),
			Hash:     hash,
			LastSeen: time.Now().UTC().Format(time.RFC3339),
		}

		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("walk directory: %w", err)
	}

	// Get previous state
	stateKey := kvKeyPrefix + "_state"
	stateJSON, err := store.GetKV(stateKey)
	if err != nil {
		return nil, fmt.Errorf("get watch state: %w", err)
	}

	var prevFiles map[string]WatchState
	if stateJSON != "" {
		if err := json.Unmarshal([]byte(stateJSON), &prevFiles); err != nil {
			return nil, fmt.Errorf("parse watch state: %w", err)
		}
	} else {
		prevFiles = make(map[string]WatchState)
	}

	// Detect changes
	changedFiles := []string{}

	// Check for new or modified files
	for path, currentState := range currentFiles {
		prevState, existed := prevFiles[path]
		if !existed || prevState.Hash != currentState.Hash {
			changedFiles = append(changedFiles, path)
		}
	}

	// Check for deleted files
	for path := range prevFiles {
		if _, exists := currentFiles[path]; !exists {
			changedFiles = append(changedFiles, path + " (deleted)")
		}
	}

	// Save new state
	newStateJSON, err := json.Marshal(currentFiles)
	if err != nil {
		return nil, fmt.Errorf("marshal watch state: %w", err)
	}
	if err := store.SetKV(stateKey, string(newStateJSON)); err != nil {
		return nil, fmt.Errorf("save watch state: %w", err)
	}

	return changedFiles, nil
}

// hashFile computes SHA256 hash of a file's contents.
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// scheduleWatchTicks schedules watch_tick jobs every 30 seconds.
// This should be called during scheduler Tick() to maintain the watch polling.
func (s *Scheduler) scheduleWatchTicks(lastWatermark, now time.Time) error {
	// Schedule a watch_tick for every 30-second interval between lastWatermark and now
	interval := 30 * time.Second
	
	// Start from the next 30-second boundary after lastWatermark
	start := lastWatermark.Truncate(interval).Add(interval)
	
	for current := start; !current.After(now); current = current.Add(interval) {
		payload := map[string]any{
			"scheduled_time": current.Format(time.RFC3339),
		}
		// Use EnqueueUnique to avoid duplicates
		if _, _, err := s.store.EnqueueUnique("watch_tick", current, payload); err != nil {
			return fmt.Errorf("enqueue watch_tick at %s: %w", current, err)
		}
	}
	
	return nil
}
