package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"okrchestra/internal/adapters"
	"okrchestra/internal/audit"
)

type RunOptions struct {
	PlanPath string
	WorkDir  string
	Adapter  adapters.AgentAdapter
	Timeout  time.Duration
}

type RunResult struct {
	RunID     string
	RunDir    string
	Plan      Plan
	ItemRuns  []ItemRunResult
	StartedAt time.Time
	EndedAt   time.Time
}

type ItemRunResult struct {
	ItemID     string
	ItemDir    string
	ResultPath string
}

func RunPlan(ctx context.Context, opts RunOptions) (*RunResult, error) {
	if opts.Adapter == nil {
		return nil, fmt.Errorf("adapter is required")
	}
	planPath, err := ResolvePlanPath(opts.PlanPath)
	if err != nil {
		return nil, err
	}
	plan, err := LoadPlan(planPath)
	if err != nil {
		return nil, err
	}
	planDir := filepath.Dir(planPath)

	runID := time.Now().UTC().Format("20060102T150405Z")
	runDir := filepath.Join(planDir, "runs", runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return nil, fmt.Errorf("ensure run dir: %w", err)
	}

	result := &RunResult{
		RunID:     runID,
		RunDir:    runDir,
		Plan:      plan,
		StartedAt: time.Now().UTC(),
	}

	for idx, item := range plan.Items {
		itemDir := filepath.Join(runDir, fmt.Sprintf("item-%04d", idx+1))
		if err := os.MkdirAll(itemDir, 0o755); err != nil {
			return result, fmt.Errorf("ensure item dir: %w", err)
		}

		startPayload := map[string]any{
			"run_id":       runID,
			"run_dir":      runDir,
			"plan_id":      plan.ID,
			"plan_as_of":   plan.AsOf,
			"plan_item_id": item.ID,
			"objective_id": item.ObjectiveID,
			"kr_id":        item.KRID,
			"metric_key":   item.ExpectedMetricChange.MetricKey,
			"adapter":      opts.Adapter.Name(),
			"workdir":      opts.WorkDir,
			"item_dir":     itemDir,
		}
		if err := audit.LogEvent("scheduler", "plan_item_started", startPayload); err != nil {
			// Best-effort logging; do not fail runs due to audit issues.
		}

		promptPath := filepath.Join(itemDir, "prompt.md")
		if err := os.WriteFile(promptPath, []byte(renderPrompt(item, itemDir)), 0o644); err != nil {
			return result, fmt.Errorf("write prompt: %w", err)
		}

		cfg := adapters.RunConfig{
			PromptPath:   promptPath,
			WorkDir:      opts.WorkDir,
			ArtifactsDir: itemDir,
			Env: map[string]string{
				"OKRCHESTRA_PLAN_ID":         plan.ID,
				"OKRCHESTRA_PLAN_ITEM_ID":    item.ID,
				"OKRCHESTRA_PLAN_ITEM_DIR":   itemDir,
				"OKRCHESTRA_AGENT_RESULT":    filepath.Join(itemDir, "result.json"),
				"OKRCHESTRA_OBJECTIVE_ID":    item.ObjectiveID,
				"OKRCHESTRA_KR_ID":           item.KRID,
				"OKRCHESTRA_METRIC_KEY":      item.ExpectedMetricChange.MetricKey,
				"OKRCHESTRA_METRIC_TARGET":   fmt.Sprintf("%g", item.ExpectedMetricChange.Target),
				"OKRCHESTRA_METRIC_BASELINE": fmt.Sprintf("%g", item.ExpectedMetricChange.Baseline),
			},
			Timeout: opts.Timeout,
		}

		adapterResult, runErr := opts.Adapter.Run(ctx, cfg)

		finishPayload := map[string]any{
			"run_id":       runID,
			"run_dir":      runDir,
			"plan_id":      plan.ID,
			"plan_item_id": item.ID,
			"objective_id": item.ObjectiveID,
			"kr_id":        item.KRID,
			"metric_key":   item.ExpectedMetricChange.MetricKey,
			"adapter":      opts.Adapter.Name(),
			"item_dir":     itemDir,
		}
		if adapterResult != nil {
			finishPayload["exit_code"] = adapterResult.ExitCode
			finishPayload["transcript"] = adapterResult.TranscriptPath
		}

		resultPath := filepath.Join(itemDir, "result.json")
		validateErr := validateAgentResult(resultPath)
		if runErr != nil {
			if validateErr == nil {
				finishPayload["adapter_error"] = runErr.Error()
			} else {
				finishPayload["error"] = runErr.Error()
				finishPayload["result_error"] = validateErr.Error()
				_ = audit.LogEvent("scheduler", "plan_item_finished", finishPayload)
				if adapterResult != nil && adapterResult.TranscriptPath != "" {
					return result, fmt.Errorf("agent run failed for item %s (see %s): %w", item.ID, adapterResult.TranscriptPath, runErr)
				}
				return result, fmt.Errorf("agent run failed for item %s: %w", item.ID, runErr)
			}
		}
		if validateErr != nil {
			finishPayload["error"] = validateErr.Error()
			_ = audit.LogEvent("scheduler", "plan_item_finished", finishPayload)
			return result, fmt.Errorf("agent result invalid for item %s: %w", item.ID, validateErr)
		}

		finishPayload["result_json"] = resultPath
		_ = audit.LogEvent("scheduler", "plan_item_finished", finishPayload)

		result.ItemRuns = append(result.ItemRuns, ItemRunResult{
			ItemID:     item.ID,
			ItemDir:    itemDir,
			ResultPath: resultPath,
		})
	}

	result.EndedAt = time.Now().UTC()
	return result, nil
}

func renderPrompt(item PlanItem, itemDir string) string {
	var b strings.Builder
	b.WriteString("# OKRchestra Plan Item\n\n")
	b.WriteString("You are executing a single plan item for OKR-driven work.\n\n")
	fmt.Fprintf(&b, "- objective_id: %s\n", item.ObjectiveID)
	fmt.Fprintf(&b, "- kr_id: %s\n", item.KRID)
	fmt.Fprintf(&b, "- agent_role: %s\n\n", item.AgentRole)
	fmt.Fprintf(&b, "## Task\n%s\n\n", item.Task)
	fmt.Fprintf(&b, "## Hypothesis\n%s\n\n", item.Hypothesis)
	fmt.Fprintf(&b, "## Expected Metric Change\n- metric_key: %s\n- direction: %s\n- baseline: %g\n- target: %g\n- delta: %g\n\n",
		item.ExpectedMetricChange.MetricKey,
		item.ExpectedMetricChange.Direction,
		item.ExpectedMetricChange.Baseline,
		item.ExpectedMetricChange.Target,
		item.ExpectedMetricChange.Delta,
	)
	if len(item.EvidencePlan) > 0 {
		b.WriteString("## Evidence Plan\n")
		for _, step := range item.EvidencePlan {
			fmt.Fprintf(&b, "- %s\n", step)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Required Output\n")
	b.WriteString("Write `result.json` to the artifacts directory for this item:\n\n")
	fmt.Fprintf(&b, "- %s\n\n", filepath.Join(itemDir, "result.json"))
	b.WriteString("The file must be valid JSON and include these fields:\n")
	b.WriteString("- `summary` (string)\n")
	b.WriteString("- `proposed_changes` (array of strings)\n")
	b.WriteString("- `kr_impact_claim` (string)\n\n")
	b.WriteString("If you made no code changes, keep `proposed_changes` empty but explain why in `summary`.\n")
	return b.String()
}

func validateAgentResult(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read result.json: %w", err)
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return fmt.Errorf("parse result.json: %w", err)
	}

	if _, ok := obj["summary"]; !ok {
		return fmt.Errorf("missing field: summary")
	}
	if _, ok := obj["proposed_changes"]; !ok {
		return fmt.Errorf("missing field: proposed_changes")
	}
	if _, ok := obj["kr_impact_claim"]; !ok {
		return fmt.Errorf("missing field: kr_impact_claim")
	}

	var summary string
	if err := json.Unmarshal(obj["summary"], &summary); err != nil || strings.TrimSpace(summary) == "" {
		return fmt.Errorf("summary must be a non-empty string")
	}
	var changes []string
	if err := json.Unmarshal(obj["proposed_changes"], &changes); err != nil {
		return fmt.Errorf("proposed_changes must be an array of strings")
	}
	var claim string
	if err := json.Unmarshal(obj["kr_impact_claim"], &claim); err != nil || strings.TrimSpace(claim) == "" {
		return fmt.Errorf("kr_impact_claim must be a non-empty string")
	}
	return nil
}
