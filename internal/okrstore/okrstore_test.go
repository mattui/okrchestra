package okrstore

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestParseAndValidateDocumentValid(t *testing.T) {
	yml := `
scope: org
objectives:
  - objective_id: OBJ-1
    objective: Test objective
    owner_id: team-alpha
    key_results:
      - kr_id: KR-1
        description: desc
        owner_id: team-alpha
        metric_key: m1
        baseline: 0
        target: 1
        confidence: 0.5
        status: not_started
        evidence: ["init"]
`
	doc, err := ParseAndValidateDocument([]byte(yml), "test.yml")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if doc.Scope != ScopeOrg {
		t.Fatalf("expected scope org, got %s", doc.Scope)
	}
	if len(doc.Objectives) != 1 || len(doc.Objectives[0].KeyResults) != 1 {
		t.Fatalf("unexpected objectives/key_results count %+v", doc.Objectives)
	}
}

func TestParseAndValidateDocumentMissingFields(t *testing.T) {
	yml := `
scope: team
objectives:
  - objective_id: ""
    objective: ""
    key_results:
      - kr_id: ""
        description: ""
        owner_id: ""
        metric_key: ""
        baseline:
        target:
        confidence:
        status: ""
        evidence:
`
	_, err := ParseAndValidateDocument([]byte(yml), "bad.yml")
	if err == nil {
		t.Fatalf("expected validation error")
	}
	ves, ok := err.(ValidationErrors)
	if !ok {
		t.Fatalf("expected ValidationErrors, got %T", err)
	}
	if len(ves) == 0 {
		t.Fatalf("expected at least one validation error")
	}
}

func TestLoadFromDirAndLookup(t *testing.T) {
	dir := t.TempDir()

	org := `
scope: org
objectives:
  - objective_id: OBJ-A
    objective: Org objective
    owner_id: team-alpha
    key_results:
      - kr_id: KR-A1
        description: desc
        owner_id: team-alpha
        metric_key: m1
        baseline: 1
        target: 2
        confidence: 0.4
        status: in_progress
        evidence: ["seed"]
`
	team := `
scope: team
objectives:
  - objective_id: OBJ-B
    objective: Team objective
    owner_id: team-beta
    key_results:
      - kr_id: KR-B1
        description: desc
        owner_id: team-beta
        metric_key: m2
        baseline: 10
        target: 20
        confidence: 0.6
        status: not_started
        evidence: ["seed"]
`
	writeFile(t, filepath.Join(dir, "org.yml"), org)
	writeFile(t, filepath.Join(dir, "team.yml"), team)

	store, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if _, ok := store.ObjectiveLookup("OBJ-A"); !ok {
		t.Fatalf("expected objective OBJ-A in lookup")
	}
	if kr, ok := store.KeyResultLookup("KR-B1"); !ok || kr.Objective.ID != "OBJ-B" {
		t.Fatalf("expected KR-B1 mapped to OBJ-B, got %#v", kr)
	}
}

func TestLoadFromDirDuplicateObjective(t *testing.T) {
	dir := t.TempDir()

	docA := `
scope: org
objectives:
  - objective_id: OBJ-DUP
    objective: First
    key_results:
      - kr_id: KR-1
        description: desc
        owner_id: a
        metric_key: m
        baseline: 1
        target: 2
        confidence: 0.2
        status: at_risk
        evidence: ["seed"]
`
	docB := `
scope: org
objectives:
  - objective_id: OBJ-DUP
    objective: Second
    key_results:
      - kr_id: KR-2
        description: desc
        owner_id: b
        metric_key: m
        baseline: 3
        target: 4
        confidence: 0.8
        status: achieved
        evidence: ["seed"]
`
	writeFile(t, filepath.Join(dir, "one.yml"), docA)
	writeFile(t, filepath.Join(dir, "two.yml"), docB)

	_, err := LoadFromDir(dir)
	if err == nil {
		t.Fatalf("expected duplicate objective error")
	}
}

func TestCanPropose(t *testing.T) {
	dir := t.TempDir()
	perm := `
permissions:
  read: ["all"]
  write: ["owner_id_match", "delegated_explicitly"]
delegations:
  owner-a:
    - agent-1
`
	permPath := filepath.Join(dir, "permissions.yml")
	writeFile(t, permPath, perm)

	oldPath := defaultPermissionsPath
	oldOnce := permOnce
	oldCache := permCache
	oldErr := permErr
	defaultPermissionsPath = permPath
	permOnce = sync.Once{}
	permCache = nil
	permErr = nil
	t.Cleanup(func() {
		defaultPermissionsPath = oldPath
		permOnce = oldOnce
		permCache = oldCache
		permErr = oldErr
	})

	if !CanPropose("owner-a", "owner-a") {
		t.Fatalf("owner match should be allowed")
	}
	if !CanPropose("agent-1", "owner-a") {
		t.Fatalf("delegated agent should be allowed")
	}
	if CanPropose("agent-x", "owner-a") {
		t.Fatalf("undelegated agent should not be allowed")
	}
}

func TestCreateAndApplyProposal(t *testing.T) {
	root := t.TempDir()
	okrsDir := filepath.Join(root, "okrs")
	updatesDir := filepath.Join(root, "updates")
	proposalsDir := filepath.Join(root, "artifacts", "proposals")

	if err := os.MkdirAll(okrsDir, 0o755); err != nil {
		t.Fatalf("mkdir okrs: %v", err)
	}
	if err := os.MkdirAll(updatesDir, 0o755); err != nil {
		t.Fatalf("mkdir updates: %v", err)
	}

	perm := `
permissions:
  read: ["all"]
  write: ["owner_id_match", "delegated_explicitly"]
`
	writeFile(t, filepath.Join(okrsDir, "permissions.yml"), perm)
	writeFile(t, filepath.Join(updatesDir, "permissions.yml"), perm)

	baseOrg := `
scope: org
objectives:
  - objective_id: OBJ-1
    objective: Baseline
    owner_id: team-alpha
    key_results:
      - kr_id: KR-1
        description: desc
        owner_id: team-alpha
        metric_key: m
        baseline: 1
        target: 2
        confidence: 0.5
        status: in_progress
        evidence: ["seed"]
`
	updatedOrg := `
scope: org
objectives:
  - objective_id: OBJ-1
    objective: Baseline
    owner_id: team-alpha
    key_results:
      - kr_id: KR-1
        description: desc
        owner_id: team-alpha
        metric_key: m
        baseline: 1
        target: 5
        confidence: 0.6
        status: in_progress
        evidence: ["seed"]
`
	writeFile(t, filepath.Join(okrsDir, "org.yml"), baseOrg)
	writeFile(t, filepath.Join(updatesDir, "org.yml"), updatedOrg)

	meta, err := CreateProposal("team-alpha", updatesDir, okrsDir, proposalsDir, "test note")
	if err != nil {
		t.Fatalf("create proposal: %v", err)
	}
	if _, err := os.Stat(filepath.Join(meta.ProposalDir, "proposal.json")); err != nil {
		t.Fatalf("missing proposal.json: %v", err)
	}
	if len(meta.Files) == 0 {
		t.Fatalf("expected files listed in metadata")
	}

	if _, err := ApplyProposal(meta.ProposalDir, true); err != nil {
		t.Fatalf("apply proposal: %v", err)
	}

	applied, err := os.ReadFile(filepath.Join(okrsDir, "org.yml"))
	if err != nil {
		t.Fatalf("read applied okrs: %v", err)
	}
	if !strings.Contains(string(applied), "target: 5") {
		t.Fatalf("proposal changes not applied: %s", string(applied))
	}
}

func TestApplyProposalRequiresConfirmation(t *testing.T) {
	if _, err := ApplyProposal("some/path", false); err == nil {
		t.Fatalf("expected error for missing confirmation")
	}
}

func writeFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}
