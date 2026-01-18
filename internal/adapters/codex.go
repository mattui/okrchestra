package adapters

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// CodexAdapter shells out to the codex CLI.
type CodexAdapter struct{}

func (a *CodexAdapter) Name() string {
	return "codex"
}

// findCodexBinary attempts to locate the codex executable.
// First tries the PATH, then checks common installation locations.
func findCodexBinary() (string, error) {
	// Try PATH first
	if path, err := exec.LookPath("codex"); err == nil {
		return path, nil
	}

	// Check common installation locations
	commonPaths := []string{
		"/opt/homebrew/bin/codex",
		"/usr/local/bin/codex",
		"/usr/bin/codex",
	}

	for _, path := range commonPaths {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", errors.New("codex executable not found in PATH or common locations")
}

func (a *CodexAdapter) Run(ctx context.Context, cfg RunConfig) (*RunResult, error) {
	if cfg.WorkDir == "" {
		return nil, errors.New("workdir is required")
	}
	if cfg.ArtifactsDir == "" {
		return nil, errors.New("artifacts dir is required")
	}
	if cfg.PromptPath == "" {
		return nil, errors.New("prompt path is required")
	}

	workDir, err := filepath.Abs(cfg.WorkDir)
	if err != nil {
		return nil, fmt.Errorf("resolve workdir: %w", err)
	}
	workDirInfo, err := os.Stat(workDir)
	if err != nil {
		return nil, fmt.Errorf("stat workdir: %w", err)
	}
	if !workDirInfo.IsDir() {
		return nil, fmt.Errorf("workdir is not a directory: %s", workDir)
	}

	artifactsDir, err := filepath.Abs(cfg.ArtifactsDir)
	if err != nil {
		return nil, fmt.Errorf("resolve artifacts dir: %w", err)
	}
	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		return nil, fmt.Errorf("create artifacts dir: %w", err)
	}

	transcriptPath := filepath.Join(artifactsDir, "transcript.log")

	resultPath := filepath.Join(artifactsDir, "result.json")
	if cfg.Env != nil {
		if override, ok := cfg.Env["OKRCHESTRA_AGENT_RESULT"]; ok && override != "" {
			resultPath = override
		}
	}
	schemaPath := filepath.Join(artifactsDir, "result.schema.json")
	if err := os.WriteFile(schemaPath, []byte(defaultResultSchema), 0o644); err != nil {
		return nil, fmt.Errorf("write result schema: %w", err)
	}

	runCtx := ctx
	var cancel context.CancelFunc
	if cfg.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
	}

	args := []string{
		"--full-auto",
		"exec",
		"-C", workDir,
		"--output-schema", schemaPath,
		"--output-last-message", resultPath,
		"-",
	}

	result := &RunResult{
		ExitCode:       0,
		TranscriptPath: transcriptPath,
		ArtifactsDir:   artifactsDir,
		SummaryPath:    resultPath,
	}

	runOnce := func(env map[string]string) error {
		transcriptFile, err := os.OpenFile(transcriptPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return fmt.Errorf("open transcript: %w", err)
		}
		defer func() {
			_ = transcriptFile.Close()
		}()

		promptFile, err := os.Open(cfg.PromptPath)
		if err != nil {
			return fmt.Errorf("open prompt: %w", err)
		}
		defer func() {
			_ = promptFile.Close()
		}()

		// Find codex binary (with fallback to common locations)
		codexBinary, err := findCodexBinary()
		if err != nil {
			return fmt.Errorf("find codex: %w", err)
		}

		cmd := exec.CommandContext(runCtx, codexBinary, args...)
		cmd.Dir = workDir
		cmd.Stdout = transcriptFile
		cmd.Stderr = io.MultiWriter(transcriptFile)
		cmd.Env = mergeEnv(os.Environ(), env)
		cmd.Stdin = promptFile
		return cmd.Run()
	}

	envAttempts := []map[string]string{cfg.Env}
	if cfg.Env == nil {
		envAttempts = []map[string]string{nil}
	}

	var lastErr error
	for idx, env := range envAttempts {
		tryEnv := env
		for attempt := 0; attempt < 2; attempt++ {
			if err := runOnce(tryEnv); err != nil {
				lastErr = err
				result.ExitCode = exitCodeFromError(err)

				// If Codex can't access its default session directory (common in sandboxed envs),
				// retry once with an isolated CODEX_HOME under the run artifacts.
				if attempt == 0 && shouldRetryWithIsolatedCodexHome(transcriptPath) && (tryEnv == nil || tryEnv["CODEX_HOME"] == "") {
					if tryEnv == nil {
						tryEnv = map[string]string{}
					}
					tryEnv["CODEX_HOME"] = filepath.Join(artifactsDir, "codex_home")
					if mkErr := os.MkdirAll(tryEnv["CODEX_HOME"], 0o755); mkErr != nil {
						return result, err
					}
					continue
				}

				// Best-effort retry for transient network failures after Codex's internal reconnects.
				if attempt == 0 && shouldRetryAfterNetworkError(transcriptPath) {
					select {
					case <-runCtx.Done():
						return result, err
					case <-time.After(2 * time.Second):
					}
					continue
				}

				if idx < len(envAttempts)-1 {
					break
				}
				return result, err
			}
			return result, nil
		}
	}

	if lastErr != nil {
		return result, lastErr
	}
	return result, fmt.Errorf("codex run failed with no error")
}

func shouldRetryWithIsolatedCodexHome(transcriptPath string) bool {
	data, err := os.ReadFile(transcriptPath)
	if err != nil {
		return false
	}
	text := string(data)
	if strings.Contains(text, "Codex cannot access session files") && strings.Contains(text, "permission denied") {
		return true
	}
	if strings.Contains(text, ".codex/sessions") && strings.Contains(text, "permission denied") {
		return true
	}
	return false
}

func shouldRetryAfterNetworkError(transcriptPath string) bool {
	data, err := os.ReadFile(transcriptPath)
	if err != nil {
		return false
	}
	text := string(data)
	if strings.Contains(text, "error=network error:") &&
		strings.Contains(text, "error sending request for url (https://api.openai.com/v1/responses)") {
		return true
	}
	return false
}

const defaultResultSchema = `{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "additionalProperties": false,
  "required": ["summary", "proposed_changes", "kr_impact_claim"],
  "properties": {
    "summary": { "type": "string" },
    "proposed_changes": { "type": "array", "items": { "type": "string" } },
    "kr_impact_claim": { "type": "string" }
  }
}
`

func mergeEnv(base []string, overrides map[string]string) []string {
	if len(overrides) == 0 {
		return base
	}
	merged := make([]string, 0, len(base)+len(overrides))
	seen := make(map[string]struct{}, len(overrides))
	for key := range overrides {
		seen[key] = struct{}{}
	}
	for _, entry := range base {
		key := entry
		if idx := indexEnvKey(entry); idx >= 0 {
			key = entry[:idx]
		}
		if _, ok := seen[key]; ok {
			continue
		}
		merged = append(merged, entry)
	}
	for key, value := range overrides {
		merged = append(merged, fmt.Sprintf("%s=%s", key, value))
	}
	return merged
}

func indexEnvKey(entry string) int {
	for i := 0; i < len(entry); i++ {
		if entry[i] == '=' {
			return i
		}
	}
	return -1
}

func exitCodeFromError(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return 124
	}
	return 1
}
