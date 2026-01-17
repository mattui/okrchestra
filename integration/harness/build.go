package harness

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

var buildOnce sync.Once
var buildPath string
var buildErr error

var repoRootOnce sync.Once
var repoRoot string
var repoRootErr error

// RepoRoot returns the repository root for the current module.
func RepoRoot(t *testing.T) string {
	t.Helper()
	root, err := repoRootPath()
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return root
}

func repoRootPath() (string, error) {
	repoRootOnce.Do(func() {
		_, file, _, ok := runtime.Caller(0)
		if !ok {
			repoRootErr = fmt.Errorf("runtime.Caller failed")
			return
		}

		root := filepath.Dir(filepath.Dir(filepath.Dir(file)))
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
			repoRootErr = fmt.Errorf("verify repo root: %w", err)
			return
		}
		repoRoot = root
	})
	return repoRoot, repoRootErr
}

// BuildBinary compiles the okrchestra CLI once per test run and returns the path.
func BuildBinary(t *testing.T) string {
	t.Helper()
	root := RepoRoot(t)

	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "okrchestra-bin-")
		if err != nil {
			buildErr = fmt.Errorf("create temp dir: %w", err)
			return
		}
		outPath := filepath.Join(dir, "okrchestra")

		cmd := exec.Command("go", "build", "-o", outPath, "./cmd/okrchestra")
		cmd.Dir = root
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			buildErr = fmt.Errorf("go build failed: %w\nstderr:\n%s", err, stderr.String())
			return
		}
		buildPath = outPath
	})

	if buildErr != nil {
		t.Fatalf("build okrchestra binary: %v", buildErr)
	}
	return buildPath
}
