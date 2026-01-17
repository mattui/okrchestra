package integration_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"okrchestra/integration/harness"
)

const testAsOf = "2025-01-15"

func TestCLISmoke(t *testing.T) {
	binPath := harness.BuildBinary(t)
	workspace := t.TempDir()
	runDir := t.TempDir()

	fixture := filepath.Join(harness.RepoRoot(t), "integration", "fixtures", "workspace-min")
	harness.CopyDir(t, fixture, workspace)
	harness.InitGitRepo(t, workspace)

	stdout, stderr, code := harness.Run(t, binPath, runDir, []string{"--help"})
	if code != 0 {
		t.Fatalf("okrchestra --help exit code %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout+stderr, "OKR-driven agent orchestration") {
		t.Fatalf("expected help output to include header\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	}

	args := []string{
		"kr", "measure",
		"--workspace", workspace,
		"--as-of", testAsOf,
	}
	stdout, stderr, code = harness.Run(t, binPath, runDir, args)
	if code != 0 {
		t.Fatalf("okrchestra kr measure exit code %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}

	snapshotsDir := filepath.Join(workspace, "metrics", "snapshots")
	snapshotPath := filepath.Join(snapshotsDir, testAsOf+".json")
	if _, err := os.Stat(snapshotPath); err != nil {
		t.Fatalf("snapshot not written at %s: %v", snapshotPath, err)
	}

	auditPath := filepath.Join(workspace, "audit", "audit.sqlite")
	if _, err := os.Stat(auditPath); err != nil {
		t.Fatalf("audit db not written at %s: %v", auditPath, err)
	}
	requireAuditEvents(t, auditPath, []string{
		"kr_measure_started",
		"kr_measure_finished",
	})

	engineSnapshot := filepath.Join(harness.RepoRoot(t), "metrics", "snapshots", testAsOf+".json")
	if _, err := os.Stat(engineSnapshot); err == nil {
		t.Fatalf("engine repo snapshot should not exist at %s", engineSnapshot)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat engine snapshot: %v", err)
	}

	engineAudit := filepath.Join(harness.RepoRoot(t), "audit", "audit.sqlite")
	if _, err := os.Stat(engineAudit); err == nil {
		t.Fatalf("engine repo audit db should not exist at %s", engineAudit)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat engine audit db: %v", err)
	}
}
