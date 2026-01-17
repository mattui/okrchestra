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

	if cfg.Env == nil {
		cfg.Env = map[string]string{}
	}
	if cfg.Env["CODEX_HOME"] == "" {
		codexHome := filepath.Join(artifactsDir, "codex_home")
		if err := os.MkdirAll(codexHome, 0o755); err != nil {
			return nil, fmt.Errorf("create CODEX_HOME: %w", err)
		}
		cfg.Env["CODEX_HOME"] = codexHome
	}

	transcriptPath := filepath.Join(artifactsDir, "transcript.log")
	transcriptFile, err := os.OpenFile(transcriptPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open transcript: %w", err)
	}
	defer func() {
		_ = transcriptFile.Close()
	}()

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
		"-a", "never",
		"-s", "workspace-write",
		"exec",
		"-C", workDir,
		"--output-schema", schemaPath,
		"--output-last-message", resultPath,
		"-",
	}

	cmd := exec.CommandContext(runCtx, "codex", args...)
	cmd.Dir = workDir
	cmd.Stdout = transcriptFile
	cmd.Stderr = io.MultiWriter(transcriptFile)
	cmd.Env = mergeEnv(os.Environ(), cfg.Env)

	promptFile, err := os.Open(cfg.PromptPath)
	if err != nil {
		return nil, fmt.Errorf("open prompt: %w", err)
	}
	defer func() {
		_ = promptFile.Close()
	}()
	cmd.Stdin = promptFile

	result := &RunResult{
		ExitCode:       0,
		TranscriptPath: transcriptPath,
		ArtifactsDir:   artifactsDir,
		SummaryPath:    resultPath,
	}

	if err := cmd.Run(); err != nil {
		result.ExitCode = exitCodeFromError(err)
		return result, err
	}

	return result, nil
}

const defaultResultSchema = `{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "additionalProperties": true,
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
