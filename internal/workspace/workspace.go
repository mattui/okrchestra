package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Workspace defines workspace-relative paths for OKRchestra operations.
type Workspace struct {
	Root         string
	OKRsDir      string
	CultureDir   string
	MetricsDir   string
	ArtifactsDir string
	AuditDir     string
	AuditDBPath  string
	StateDBPath  string
}

// Resolve expands and validates the workspace root, ensuring it exists.
func Resolve(root string) (*Workspace, error) {
	abs, err := resolveRoot(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("workspace root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workspace root is not a directory: %s", abs)
	}
	return newWorkspace(abs), nil
}

// ResolveRoot resolves the workspace root without requiring it to exist.
func ResolveRoot(root string) (string, error) {
	return resolveRoot(root)
}

// EnsureDirs creates standard workspace directories for artifacts and audit data.
func (w *Workspace) EnsureDirs() error {
	if w == nil {
		return fmt.Errorf("workspace is nil")
	}
	dirs := []string{
		w.ArtifactsDir,
		w.AuditDir,
		filepath.Join(w.MetricsDir, "snapshots"),
		filepath.Join(w.ArtifactsDir, "plans"),
		filepath.Join(w.ArtifactsDir, "runs"),
		filepath.Join(w.ArtifactsDir, "proposals"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("ensure %s: %w", dir, err)
		}
	}
	return nil
}

// ResolvePath returns an absolute path, resolving relative paths from the workspace root.
func (w *Workspace) ResolvePath(path string) (string, error) {
	if w == nil {
		return "", fmt.Errorf("workspace is nil")
	}
	if strings.TrimSpace(path) == "" {
		return "", nil
	}
	expanded, err := expandHome(path)
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(expanded) {
		return filepath.Clean(expanded), nil
	}
	return filepath.Abs(filepath.Join(w.Root, expanded))
}

func newWorkspace(root string) *Workspace {
	return &Workspace{
		Root:         root,
		OKRsDir:      filepath.Join(root, "okrs"),
		CultureDir:   filepath.Join(root, "culture"),
		MetricsDir:   filepath.Join(root, "metrics"),
		ArtifactsDir: filepath.Join(root, "artifacts"),
		AuditDir:     filepath.Join(root, "audit"),
		AuditDBPath:  filepath.Join(root, "audit", "audit.sqlite"),
		StateDBPath:  filepath.Join(root, "audit", "daemon.sqlite"),
	}
}

func resolveRoot(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", fmt.Errorf("workspace root is required")
	}
	expanded, err := expandHome(root)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(expanded)
	if err != nil {
		return "", fmt.Errorf("resolve workspace: %w", err)
	}
	return abs, nil
}

func expandHome(path string) (string, error) {
	if path == "" || path[0] != '~' {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	if path == "~" {
		return home, nil
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:]), nil
	}
	return "", fmt.Errorf("unsupported home expansion: %s", path)
}
