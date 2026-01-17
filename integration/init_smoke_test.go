package integration_test

import (
	"os"
	"path/filepath"
	"testing"

	"okrchestra/integration/harness"
)

func TestInitSmoke(t *testing.T) {
	binPath := harness.BuildBinary(t)
	runDir := t.TempDir()
	workspaceRoot := filepath.Join(t.TempDir(), "workspace-init")

	args := []string{
		"init",
		"--workspace", workspaceRoot,
	}
	stdout, stderr, code := harness.Run(t, binPath, runDir, args)
	if code != 0 {
		t.Fatalf("okrchestra init exit code %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}

	paths := []string{
		filepath.Join(workspaceRoot, "okrs"),
		filepath.Join(workspaceRoot, "culture"),
		filepath.Join(workspaceRoot, "metrics"),
		filepath.Join(workspaceRoot, "artifacts"),
		filepath.Join(workspaceRoot, "audit"),
		filepath.Join(workspaceRoot, "metrics", "snapshots"),
		filepath.Join(workspaceRoot, "artifacts", "plans"),
		filepath.Join(workspaceRoot, "artifacts", "runs"),
		filepath.Join(workspaceRoot, "artifacts", "proposals"),
		filepath.Join(workspaceRoot, "okrs", "org.yml"),
		filepath.Join(workspaceRoot, "okrs", "permissions.yml"),
		filepath.Join(workspaceRoot, "culture", "values.md"),
		filepath.Join(workspaceRoot, "culture", "standards.md"),
		filepath.Join(workspaceRoot, "metrics", "manual.yml"),
		filepath.Join(workspaceRoot, "metrics", "ci_report.json"),
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing init path %s: %v", path, err)
		}
	}

	auditPath := filepath.Join(workspaceRoot, "audit", "audit.sqlite")
	if _, err := os.Stat(auditPath); err != nil {
		t.Fatalf("audit db not written at %s: %v", auditPath, err)
	}
	requireAuditEvents(t, auditPath, []string{
		"workspace_init_started",
		"workspace_init_finished",
	})
}
