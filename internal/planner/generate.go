package planner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"okrchestra/internal/okrstore"
)

type GenerateOptions struct {
	OKRsDir       string
	OutputBaseDir string
	AsOf          time.Time
	ObjectiveID   string
	KRID          string
	AgentRole     string
}

type GenerateResult struct {
	Plan     Plan
	PlanPath string
}

func GeneratePlan(opts GenerateOptions) (GenerateResult, error) {
	if opts.OKRsDir == "" {
		opts.OKRsDir = "okrs"
	}
	if opts.OutputBaseDir == "" {
		opts.OutputBaseDir = filepath.Join("artifacts", "plans")
	}
	if opts.AsOf.IsZero() {
		opts.AsOf = time.Now().UTC().Truncate(24 * time.Hour)
	}
	if opts.AgentRole == "" {
		opts.AgentRole = "software_engineer"
	}

	store, err := okrstore.LoadFromDir(opts.OKRsDir)
	if err != nil {
		return GenerateResult{}, err
	}

	obj, kr, err := selectOrgKR(store, opts.ObjectiveID, opts.KRID)
	if err != nil {
		return GenerateResult{}, err
	}
	if kr.MetricKey == "" {
		return GenerateResult{}, fmt.Errorf("selected KR %s has no metric_key", kr.ID)
	}

	direction := "increase"
	if kr.Target < kr.Baseline {
		direction = "decrease"
	}
	delta := kr.Target - kr.Baseline

	asOfStr := opts.AsOf.UTC().Format("2006-01-02")
	plan := Plan{
		ID:          fmt.Sprintf("PLAN-%s", asOfStr),
		AsOf:        asOfStr,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		OKRsDir:     opts.OKRsDir,
		Items: []PlanItem{
			{
				ID:          "ITEM-1",
				ObjectiveID: obj.ID,
				KRID:        kr.ID,
				Hypothesis: fmt.Sprintf(
					"If we execute this task, %s will %s from %g toward %g (Δ %g).",
					kr.MetricKey, direction, kr.Baseline, kr.Target, delta,
				),
				Task:      fmt.Sprintf("Deliver work that advances KR %s: %s", kr.ID, kr.Description),
				AgentRole: opts.AgentRole,
				ExpectedMetricChange: ExpectedMetricChange{
					MetricKey:  kr.MetricKey,
					Direction:  direction,
					Baseline:   kr.Baseline,
					Target:     kr.Target,
					Delta:      delta,
					Rationale:  kr.Description,
					Confidence: kr.Confidence,
				},
				EvidencePlan: []string{
					fmt.Sprintf("Capture evidence for %s and attach references in result.json.", kr.MetricKey),
					"Run `okrchestra kr measure` to record a fresh metric snapshot.",
					"Run `okrchestra kr score` to verify progress against baseline/target.",
				},
			},
		},
	}

	if err := ValidatePlan(plan); err != nil {
		return GenerateResult{}, err
	}

	planPath := filepath.Join(opts.OutputBaseDir, asOfStr, "plan.json")
	if err := os.MkdirAll(filepath.Dir(planPath), 0o755); err != nil {
		return GenerateResult{}, fmt.Errorf("ensure plan dir: %w", err)
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return GenerateResult{}, fmt.Errorf("marshal plan: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(planPath, data, 0o644); err != nil {
		return GenerateResult{}, fmt.Errorf("write plan: %w", err)
	}

	return GenerateResult{Plan: plan, PlanPath: planPath}, nil
}

func selectOrgKR(store *okrstore.Store, objectiveID string, krID string) (okrstore.Objective, okrstore.KeyResult, error) {
	if store == nil {
		return okrstore.Objective{}, okrstore.KeyResult{}, fmt.Errorf("okr store is required")
	}

	if krID != "" {
		rec, ok := store.KeyResultLookup(krID)
		if !ok {
			return okrstore.Objective{}, okrstore.KeyResult{}, fmt.Errorf("unknown kr_id: %s", krID)
		}
		if rec.Scope != okrstore.ScopeOrg {
			return okrstore.Objective{}, okrstore.KeyResult{}, fmt.Errorf("kr_id %s is not in org scope", krID)
		}
		return rec.Objective, rec.KeyResult, nil
	}

	if objectiveID != "" {
		rec, ok := store.ObjectiveLookup(objectiveID)
		if !ok {
			return okrstore.Objective{}, okrstore.KeyResult{}, fmt.Errorf("unknown objective_id: %s", objectiveID)
		}
		if rec.Scope != okrstore.ScopeOrg {
			return okrstore.Objective{}, okrstore.KeyResult{}, fmt.Errorf("objective_id %s is not in org scope", objectiveID)
		}
		for _, kr := range rec.Objective.KeyResults {
			if kr.MetricKey == "" {
				continue
			}
			if kr.Status == "achieved" {
				continue
			}
			return rec.Objective, kr, nil
		}
		return okrstore.Objective{}, okrstore.KeyResult{}, fmt.Errorf("objective_id %s has no runnable org key results", objectiveID)
	}

	for _, doc := range store.Org.Documents {
		for _, obj := range doc.Objectives {
			for _, kr := range obj.KeyResults {
				if kr.MetricKey == "" {
					continue
				}
				if kr.Status == "achieved" {
					continue
				}
				return obj, kr, nil
			}
		}
	}

	return okrstore.Objective{}, okrstore.KeyResult{}, fmt.Errorf("no runnable org key results found")
}
