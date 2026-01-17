package okrstore

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pmezard/go-difflib/difflib"
)

// ProposalMetadata describes a stored OKR proposal.
type ProposalMetadata struct {
	ID          string    `json:"id"`
	AgentID     string    `json:"agent_id"`
	CreatedAt   time.Time `json:"created_at"`
	OKRsDir     string    `json:"okrs_dir"`
	ProposalDir string    `json:"proposal_dir"`
	UpdatesDir  string    `json:"updates_dir"`
	Files       []string  `json:"files"`
	DiffFile    string    `json:"diff_file,omitempty"`
	Note        string    `json:"note,omitempty"`
}

// CreateProposal validates updated OKRs, enforces permissions, and writes a proposal package.
func CreateProposal(agentID, updatesDir, okrsDir, proposalsRoot, note string) (*ProposalMetadata, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, fmt.Errorf("agent id is required")
	}
	if updatesDir == "" {
		return nil, fmt.Errorf("updates directory is required")
	}
	if okrsDir == "" {
		okrsDir = "okrs"
	}
	if proposalsRoot == "" {
		proposalsRoot = filepath.Join("artifacts", "proposals")
	}

	if _, err := os.Stat(updatesDir); err != nil {
		return nil, fmt.Errorf("updates directory: %w", err)
	}
	if _, err := os.Stat(okrsDir); err != nil {
		return nil, fmt.Errorf("okrs directory: %w", err)
	}
	if filepath.Clean(updatesDir) == filepath.Clean(okrsDir) {
		return nil, fmt.Errorf("updates directory must differ from okrs directory; direct edits to okrs/ are not allowed")
	}

	if err := enforcePermissions(agentID, updatesDir); err != nil {
		return nil, err
	}

	if err := os.MkdirAll(proposalsRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create proposals root: %w", err)
	}

	timestamp := time.Now().UTC().Format("20060102-150405")
	proposalID := fmt.Sprintf("%s-%s", timestamp, sanitize(agentID))
	proposalDir := filepath.Join(proposalsRoot, proposalID)
	if err := os.MkdirAll(proposalDir, 0o755); err != nil {
		return nil, fmt.Errorf("create proposal dir: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(proposalDir)
		}
	}()

	updateFiles, err := collectYAMLFiles(updatesDir)
	if err != nil {
		return nil, err
	}
	if len(updateFiles) == 0 {
		return nil, fmt.Errorf("no YAML files found in %s", updatesDir)
	}

	var copied []string
	for _, src := range updateFiles {
		dst := filepath.Join(proposalDir, filepath.Base(src))
		if copyErr := copyFile(src, dst); copyErr != nil {
			return nil, fmt.Errorf("copy %s: %w", src, copyErr)
		}
		copied = append(copied, filepath.Base(src))
	}

	diffPath, err := renderDiff(updateFiles, okrsDir, proposalDir)
	if err != nil {
		return nil, err
	}

	meta := &ProposalMetadata{
		ID:          proposalID,
		AgentID:     agentID,
		CreatedAt:   time.Now().UTC(),
		OKRsDir:     okrsDir,
		ProposalDir: proposalDir,
		UpdatesDir:  updatesDir,
		Files:       copied,
		DiffFile:    diffPath,
		Note:        strings.TrimSpace(note),
	}

	if err := writeProposalMetadata(meta); err != nil {
		return nil, err
	}

	cleanup = false
	return meta, nil
}

// ApplyProposal applies a validated proposal to the target okrs directory.
func ApplyProposal(proposalDir string, confirm bool) (*ProposalMetadata, error) {
	if !confirm {
		return nil, fmt.Errorf("apply requires --i-understand confirmation")
	}
	if proposalDir == "" {
		return nil, fmt.Errorf("proposal path is required")
	}

	meta, err := readProposalMetadata(proposalDir)
	if err != nil {
		return nil, err
	}

	if err := enforcePermissions(meta.AgentID, proposalDir); err != nil {
		return nil, err
	}

	store, err := LoadFromDir(proposalDir)
	if err != nil {
		return nil, fmt.Errorf("proposal validation failed: %w", err)
	}
	if len(store.objectives) == 0 {
		return nil, fmt.Errorf("proposal contains no objectives")
	}
	if len(meta.Files) == 0 {
		return nil, fmt.Errorf("proposal metadata lists no files to apply")
	}

	if err := os.MkdirAll(meta.OKRsDir, 0o755); err != nil {
		return nil, fmt.Errorf("ensure okrs dir: %w", err)
	}

	for _, file := range meta.Files {
		src := filepath.Join(proposalDir, file)
		dst := filepath.Join(meta.OKRsDir, file)
		if copyErr := copyFile(src, dst); copyErr != nil {
			return nil, fmt.Errorf("apply %s: %w", file, copyErr)
		}
	}

	return meta, nil
}

func enforcePermissions(agentID, okrDir string) error {
	store, err := LoadFromDir(okrDir)
	if err != nil {
		return fmt.Errorf("validate okrs: %w", err)
	}

	permCfg, err := loadPermissionsForDir(okrDir)
	if err != nil {
		return fmt.Errorf("load permissions: %w", err)
	}

	for _, obj := range store.objectives {
		if obj.Objective.OwnerID != "" && !canProposeWithConfig(permCfg, agentID, obj.Objective.OwnerID) {
			return fmt.Errorf("agent %s is not permitted to modify owner %s", agentID, obj.Objective.OwnerID)
		}
		for _, kr := range obj.Objective.KeyResults {
			if !canProposeWithConfig(permCfg, agentID, kr.OwnerID) {
				return fmt.Errorf("agent %s is not permitted to modify owner %s", agentID, kr.OwnerID)
			}
		}
	}
	return nil
}

func collectYAMLFiles(dir string) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.yml"))
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", dir, err)
	}
	sort.Strings(files)
	return files, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		_ = in.Close()
	}()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		_ = out.Close()
	}()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

func renderDiff(updateFiles []string, okrsDir, proposalDir string) (string, error) {
	var diffStrings []string

	for _, src := range updateFiles {
		baseName := filepath.Base(src)
		newBytes, err := os.ReadFile(src)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", src, err)
		}
		oldPath := filepath.Join(okrsDir, baseName)
		oldBytes, _ := os.ReadFile(oldPath)

		diff := difflib.UnifiedDiff{
			A:        strings.Split(string(oldBytes), "\n"),
			B:        strings.Split(string(newBytes), "\n"),
			FromFile: filepath.Join("okrs", baseName),
			ToFile:   filepath.Join("proposal", baseName),
			Context:  3,
		}
		diffText, err := difflib.GetUnifiedDiffString(diff)
		if err != nil {
			return "", fmt.Errorf("diff %s: %w", baseName, err)
		}
		if strings.TrimSpace(diffText) != "" {
			diffStrings = append(diffStrings, diffText)
		}
	}

	if len(diffStrings) == 0 {
		return "", nil
	}

	diffPath := filepath.Join(proposalDir, "changes.diff")
	if err := os.WriteFile(diffPath, []byte(strings.Join(diffStrings, "\n")), 0o644); err != nil {
		return "", fmt.Errorf("write diff: %w", err)
	}
	return filepath.Base(diffPath), nil
}

func writeProposalMetadata(meta *ProposalMetadata) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("encode proposal.json: %w", err)
	}
	path := filepath.Join(meta.ProposalDir, "proposal.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write proposal.json: %w", err)
	}
	return nil
}

func readProposalMetadata(proposalDir string) (*ProposalMetadata, error) {
	path := filepath.Join(proposalDir, "proposal.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read proposal metadata: %w", err)
	}
	var meta ProposalMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("parse proposal metadata: %w", err)
	}
	if meta.ProposalDir == "" {
		meta.ProposalDir = proposalDir
	}
	if meta.OKRsDir == "" {
		meta.OKRsDir = "okrs"
	}
	if meta.AgentID == "" || meta.ID == "" {
		return nil, fmt.Errorf("proposal metadata is missing required fields")
	}
	return &meta, nil
}

func sanitize(value string) string {
	safe := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, value)
	if safe == "" {
		return "agent"
	}
	return safe
}
