package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// MockAdapter is a deterministic, offline adapter used for end-to-end testing of the scheduler.
type MockAdapter struct{}

func (a *MockAdapter) Name() string {
	return "mock"
}

func (a *MockAdapter) Run(ctx context.Context, cfg RunConfig) (*RunResult, error) {
	if cfg.WorkDir == "" {
		return nil, errors.New("workdir is required")
	}
	if cfg.ArtifactsDir == "" {
		return nil, errors.New("artifacts dir is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	artifactsDir, err := filepath.Abs(cfg.ArtifactsDir)
	if err != nil {
		return nil, fmt.Errorf("resolve artifacts dir: %w", err)
	}
	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		return nil, fmt.Errorf("create artifacts dir: %w", err)
	}

	transcriptPath := filepath.Join(artifactsDir, "transcript.log")
	if err := os.WriteFile(transcriptPath, []byte("mock adapter: no agent executed\n"), 0o644); err != nil {
		return nil, fmt.Errorf("write transcript: %w", err)
	}

	resultPath := filepath.Join(artifactsDir, "result.json")
	if cfg.Env != nil {
		if override, ok := cfg.Env["OKRCHESTRA_AGENT_RESULT"]; ok && override != "" {
			resultPath = override
		}
	}

	metricKey := ""
	if cfg.Env != nil {
		metricKey = cfg.Env["OKRCHESTRA_METRIC_KEY"]
	}

	payload := map[string]any{
		"summary":          "mock run completed (no changes applied)",
		"proposed_changes": []string{},
		"kr_impact_claim":  fmt.Sprintf("No claim (mock adapter). Metric key: %s.", metricKey),
		"generated_at":     time.Now().UTC().Format(time.RFC3339),
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal result: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(resultPath, data, 0o644); err != nil {
		return nil, fmt.Errorf("write result.json: %w", err)
	}

	return &RunResult{
		ExitCode:       0,
		TranscriptPath: transcriptPath,
		ArtifactsDir:   artifactsDir,
		SummaryPath:    resultPath,
	}, nil
}
