package integration_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"okrchestra/integration/harness"
)

func TestPlanSmoke(t *testing.T) {
	binPath := harness.BuildBinary(t)
	workspace := t.TempDir()
	runDir := t.TempDir()

	fixture := filepath.Join(harness.RepoRoot(t), "integration", "fixtures", "workspace-min")
	harness.CopyDir(t, fixture, workspace)

	plansDir := filepath.Join(workspace, "artifacts", "plans")

	genArgs := []string{
		"plan", "generate",
		"--workspace", workspace,
		"--as-of", testAsOf,
	}
	stdout, stderr, code := harness.Run(t, binPath, runDir, genArgs)
	if code != 0 {
		t.Fatalf("okrchestra plan generate exit code %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}

	planPath := filepath.Join(plansDir, testAsOf, "plan.json")
	if _, err := os.Stat(planPath); err != nil {
		t.Fatalf("plan not written at %s: %v", planPath, err)
	}

	runArgs := []string{
		"plan", "run",
		"--adapter", "mock",
		"--workspace", workspace,
		filepath.Join("artifacts", "plans", testAsOf, "plan.json"),
	}
	stdout, stderr, code = harness.Run(t, binPath, runDir, runArgs)
	if code != 0 {
		t.Fatalf("okrchestra plan run exit code %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}

	runsDir := filepath.Join(workspace, "artifacts", "runs")
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		t.Fatalf("read runs dir: %v", err)
	}

	planRunDir := ""
	for _, entry := range entries {
		if entry.IsDir() {
			planRunDir = filepath.Join(runsDir, entry.Name())
			break
		}
	}
	if planRunDir == "" {
		t.Fatalf("no run directory found in %s", runsDir)
	}

	itemEntries, err := os.ReadDir(planRunDir)
	if err != nil {
		t.Fatalf("read run dir: %v", err)
	}

	itemCount := 0
	for _, entry := range itemEntries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "item-") {
			continue
		}
		itemCount++
		resultPath := filepath.Join(planRunDir, entry.Name(), "result.json")
		if _, err := os.Stat(resultPath); err != nil {
			t.Fatalf("missing result.json at %s: %v", resultPath, err)
		}
	}
	if itemCount == 0 {
		t.Fatalf("no item results found in %s", planRunDir)
	}

	auditPath := filepath.Join(workspace, "audit", "audit.sqlite")
	if _, err := os.Stat(auditPath); err != nil {
		t.Fatalf("audit db not written at %s: %v", auditPath, err)
	}
	requireAuditEvents(t, auditPath, []string{
		"plan_generate_started",
		"plan_generate_finished",
		"plan_run_started",
		"plan_run_finished",
		"plan_item_started",
		"plan_item_finished",
	})

	enginePlan := filepath.Join(harness.RepoRoot(t), "artifacts", "plans", testAsOf, "plan.json")
	if _, err := os.Stat(enginePlan); err == nil {
		t.Fatalf("engine repo plan should not exist at %s", enginePlan)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat engine plan: %v", err)
	}
}
