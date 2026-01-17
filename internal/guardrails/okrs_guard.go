package guardrails

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// SnapshotDirHash computes a hash representing the state of all files in a directory.
// Returns empty string if directory doesn't exist.
func SnapshotDirHash(dir string) (string, error) {
	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("stat dir: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a directory: %s", dir)
	}

	var files []string
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files = append(files, relPath)
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("walk dir: %w", err)
	}

	sort.Strings(files)

	h := sha256.New()
	for _, relPath := range files {
		fullPath := filepath.Join(dir, relPath)
		f, err := os.Open(fullPath)
		if err != nil {
			return "", fmt.Errorf("open %s: %w", relPath, err)
		}

		fh := sha256.New()
		if _, err := io.Copy(fh, f); err != nil {
			_ = f.Close()
			return "", fmt.Errorf("hash %s: %w", relPath, err)
		}
		_ = f.Close()

		// Write relative path and file hash to main hash
		_, _ = h.Write([]byte(relPath))
		_, _ = h.Write(fh.Sum(nil))
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// DiffDir compares two directory hashes and returns a list of changed files.
// This is a simplified implementation that just indicates a change occurred.
func DiffDir(beforeHash, afterHash string) ([]string, error) {
	if beforeHash == afterHash {
		return nil, nil
	}
	return []string{"okrs/ directory modified (hash mismatch)"}, nil
}

// RevertOKRs attempts to revert changes to the okrs/ directory using git.
func RevertOKRs(wsRoot string) error {
	okrsDir := filepath.Join(wsRoot, "okrs")

	// Check if we're in a git repository
	gitCheck := exec.Command("git", "-C", wsRoot, "rev-parse", "--git-dir")
	if err := gitCheck.Run(); err != nil {
		return fmt.Errorf("workspace is not a git repository, cannot revert okrs/ changes")
	}

	// Revert changes to okrs/ directory
	revertCmd := exec.Command("git", "-C", wsRoot, "checkout", "--", "okrs")
	output, err := revertCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git checkout failed: %w (output: %s)", err, string(output))
	}

	// Verify okrs directory still exists
	if _, err := os.Stat(okrsDir); err != nil {
		return fmt.Errorf("okrs/ directory missing after revert: %w", err)
	}

	return nil
}

// WriteViolation writes a guardrail violation record to the artifacts directory.
func WriteViolation(artifactsDir string, violation map[string]any) error {
	violationPath := filepath.Join(artifactsDir, "violation.json")
	
	data, err := json.MarshalIndent(violation, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal violation: %w", err)
	}

	if err := os.WriteFile(violationPath, data, 0o644); err != nil {
		return fmt.Errorf("write violation.json: %w", err)
	}

	return nil
}

// CheckOKRsIntegrity captures before/after hashes and detects changes.
type OKRsIntegrityCheck struct {
	BeforeHash string
	AfterHash  string
	OKRsDir    string
}

// NewIntegrityCheck creates a new integrity check for the given workspace root.
func NewIntegrityCheck(wsRoot string) (*OKRsIntegrityCheck, error) {
	okrsDir := filepath.Join(wsRoot, "okrs")
	beforeHash, err := SnapshotDirHash(okrsDir)
	if err != nil {
		return nil, fmt.Errorf("capture before snapshot: %w", err)
	}

	return &OKRsIntegrityCheck{
		BeforeHash: beforeHash,
		OKRsDir:    okrsDir,
	}, nil
}

// CaptureAfter captures the post-execution state.
func (c *OKRsIntegrityCheck) CaptureAfter() error {
	afterHash, err := SnapshotDirHash(c.OKRsDir)
	if err != nil {
		return fmt.Errorf("capture after snapshot: %w", err)
	}
	c.AfterHash = afterHash
	return nil
}

// HasChanges returns true if the okrs/ directory was modified.
func (c *OKRsIntegrityCheck) HasChanges() bool {
	return c.BeforeHash != c.AfterHash
}

// GetChangedFiles returns a list of changed files (simplified).
func (c *OKRsIntegrityCheck) GetChangedFiles() ([]string, error) {
	return DiffDir(c.BeforeHash, c.AfterHash)
}

// BuildViolation creates a violation record map.
func BuildViolation(violationType string, details map[string]any) map[string]any {
	violation := map[string]any{
		"violation_type": violationType,
		"details":        details,
	}
	return violation
}

// GetWorkspaceRoot attempts to find the workspace root from a work directory.
// This walks up the directory tree looking for an okrs/ directory.
func GetWorkspaceRoot(workDir string) (string, error) {
	current := workDir
	for {
		okrsPath := filepath.Join(current, "okrs")
		if info, err := os.Stat(okrsPath); err == nil && info.IsDir() {
			return current, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("workspace root not found (no okrs/ directory in tree)")
		}
		current = parent
	}
}

// NormalizeWorkDir ensures workDir is resolved to workspace root if it's a subdir.
func NormalizeWorkDir(workDir string) (string, error) {
	absWork, err := filepath.Abs(workDir)
	if err != nil {
		return "", fmt.Errorf("resolve work dir: %w", err)
	}

	// Check if workDir itself contains okrs/
	okrsPath := filepath.Join(absWork, "okrs")
	if info, err := os.Stat(okrsPath); err == nil && info.IsDir() {
		return absWork, nil
	}

	// Try to find workspace root by walking up
	wsRoot, err := GetWorkspaceRoot(absWork)
	if err != nil {
		// If we can't find workspace root, use workDir as-is
		// (may not have okrs/ yet, which is valid for some operations)
		return absWork, nil
	}

	return wsRoot, nil
}

// IsGitRepo checks if a directory is part of a git repository.
func IsGitRepo(dir string) bool {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--git-dir")
	return cmd.Run() == nil
}

// RevertPath builds the okrs/ path from workspace root.
func RevertPath(wsRoot string) string {
	return filepath.Join(wsRoot, "okrs")
}

// SanitizeErrorForJSON strips newlines and truncates error messages for JSON safety.
func SanitizeErrorForJSON(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	msg = strings.ReplaceAll(msg, "\n", " ")
	msg = strings.ReplaceAll(msg, "\r", " ")
	if len(msg) > 500 {
		msg = msg[:497] + "..."
	}
	return msg
}
