package adapters

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// CodexAdapter shells out to the codex CLI.
type CodexAdapter struct{}

func (a *CodexAdapter) Name() string {
	return "codex"
}

func (a *CodexAdapter) Run(ctx context.Context, cfg RunConfig) (*RunResult, error) {
	if cfg.WorkDir == "" {
		return nil, errors.New("workdir is required")
	}
	if cfg.ArtifactsDir == "" {
		return nil, errors.New("artifacts dir is required")
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
	transcriptFile, err := os.OpenFile(transcriptPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open transcript: %w", err)
	}
	defer func() {
		_ = transcriptFile.Close()
	}()

	runCtx := ctx
	var cancel context.CancelFunc
	if cfg.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
	}

	args := []string{}
	if cfg.PromptPath != "" {
		args = append(args, "--prompt-file", cfg.PromptPath)
	}

	cmd := exec.CommandContext(runCtx, "codex", args...)
	cmd.Dir = workDir
	cmd.Stdout = transcriptFile
	cmd.Stderr = io.MultiWriter(transcriptFile)
	cmd.Env = mergeEnv(os.Environ(), cfg.Env)

	result := &RunResult{
		ExitCode:       0,
		TranscriptPath: transcriptPath,
		ArtifactsDir:   artifactsDir,
		SummaryPath:    "",
	}

	if err := cmd.Run(); err != nil {
		result.ExitCode = exitCodeFromError(err)
		return result, err
	}

	return result, nil
}

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
