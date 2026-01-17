package adapters

import (
	"context"
	"time"
)

// AgentAdapter defines the interface for running an agent via an adapter.
type AgentAdapter interface {
	Name() string
	Run(ctx context.Context, cfg RunConfig) (*RunResult, error)
}

// RunConfig configures an agent execution.
type RunConfig struct {
	PromptPath   string
	WorkDir      string
	ArtifactsDir string
	Env          map[string]string
	Timeout      time.Duration
}

// RunResult captures the result of a run.
type RunResult struct {
	ExitCode       int
	TranscriptPath string
	ArtifactsDir   string
	SummaryPath    string
}
