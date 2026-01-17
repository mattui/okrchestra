package harness

import (
	"bytes"
	"os"
	"os/exec"
	"sort"
	"strings"
	"testing"
)

// Run executes the CLI in the provided working directory.
func Run(t *testing.T, binPath, workDir string, args []string) (string, string, int) {
	t.Helper()
	return run(t, binPath, workDir, args, nil)
}

// RunWithEnv executes the CLI with environment overrides.
func RunWithEnv(t *testing.T, binPath, workDir string, args []string, env map[string]string) (string, string, int) {
	t.Helper()
	return run(t, binPath, workDir, args, env)
}

func run(t *testing.T, binPath, workDir string, args []string, env map[string]string) (string, string, int) {
	t.Helper()

	cmd := exec.Command(binPath, args...)
	cmd.Dir = workDir
	if len(env) > 0 {
		cmd.Env = mergeEnv(env)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			t.Fatalf("run %s: %v", binPath, err)
		}
	}

	return stdout.String(), stderr.String(), exitCode
}

func mergeEnv(overrides map[string]string) []string {
	env := make(map[string]string, len(overrides))
	for _, entry := range os.Environ() {
		parts := strings.SplitN(entry, "=", 2)
		key := parts[0]
		val := ""
		if len(parts) > 1 {
			val = parts[1]
		}
		env[key] = val
	}

	for k, v := range overrides {
		env[k] = v
	}

	merged := make([]string, 0, len(env))
	for k, v := range env {
		merged = append(merged, k+"="+v)
	}
	sort.Strings(merged)
	return merged
}
