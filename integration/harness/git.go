package harness

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// InitGitRepo creates a minimal git repository in dir if one does not exist.
func InitGitRepo(t *testing.T, dir string) {
	t.Helper()

	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return
	}

	runGit(t, dir, "init")
	readme := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readme, []byte("workspace\n"), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "-c", "user.name=okrchestra-test", "-c", "user.email=okrchestra-test@example.com", "commit", "-m", "init")
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %v failed: %v\nstdout:\n%s\nstderr:\n%s", args, err, stdout.String(), stderr.String())
	}
}
