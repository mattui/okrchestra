package guardrails

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateResultJSON_Valid(t *testing.T) {
	tmpDir := t.TempDir()
	resultPath := filepath.Join(tmpDir, "result.json")

	validResult := ResultSchema{
		SchemaVersion:   "1.0",
		Summary:         "Completed the task successfully",
		ProposedChanges: []string{"Updated config.yml", "Added new feature"},
		KRTargets:       []string{"kr-123"},
		KRImpactClaim:   "Expected 10% improvement based on benchmark tests",
	}

	data, err := json.MarshalIndent(validResult, "", "  ")
	if err != nil {
		t.Fatalf("marshal valid result: %v", err)
	}

	if err := os.WriteFile(resultPath, data, 0644); err != nil {
		t.Fatalf("write result file: %v", err)
	}

	err = ValidateResultJSON(resultPath)
	if err != nil {
		t.Errorf("ValidateResultJSON() failed for valid result: %v", err)
	}
}

func TestValidateResultJSON_MissingSchemaVersion(t *testing.T) {
	tmpDir := t.TempDir()
	resultPath := filepath.Join(tmpDir, "result.json")

	invalidResult := map[string]any{
		"summary":          "Test",
		"proposed_changes": []string{},
		"kr_targets":       []string{},
		"kr_impact_claim":  "None",
	}

	data, _ := json.MarshalIndent(invalidResult, "", "  ")
	_ = os.WriteFile(resultPath, data, 0644)

	err := ValidateResultJSON(resultPath)
	if err == nil {
		t.Error("ValidateResultJSON() should fail for missing schema_version")
	}
}

func TestValidateResultJSON_WrongSchemaVersion(t *testing.T) {
	tmpDir := t.TempDir()
	resultPath := filepath.Join(tmpDir, "result.json")

	invalidResult := ResultSchema{
		SchemaVersion:   "2.0",
		Summary:         "Test",
		ProposedChanges: []string{},
		KRTargets:       []string{},
		KRImpactClaim:   "None",
	}

	data, _ := json.MarshalIndent(invalidResult, "", "  ")
	_ = os.WriteFile(resultPath, data, 0644)

	err := ValidateResultJSON(resultPath)
	if err == nil {
		t.Error("ValidateResultJSON() should fail for wrong schema_version")
	}
}

func TestValidateResultJSON_ExtraFields(t *testing.T) {
	tmpDir := t.TempDir()
	resultPath := filepath.Join(tmpDir, "result.json")

	invalidResult := map[string]any{
		"schema_version":   "1.0",
		"summary":          "Test",
		"proposed_changes": []string{},
		"kr_targets":       []string{},
		"kr_impact_claim":  "None",
		"extra_field":      "This should not be here",
	}

	data, _ := json.MarshalIndent(invalidResult, "", "  ")
	_ = os.WriteFile(resultPath, data, 0644)

	err := ValidateResultJSON(resultPath)
	if err == nil {
		t.Error("ValidateResultJSON() should fail for extra fields")
	}
}

func TestValidateResultJSON_EmptySummary(t *testing.T) {
	tmpDir := t.TempDir()
	resultPath := filepath.Join(tmpDir, "result.json")

	invalidResult := ResultSchema{
		SchemaVersion:   "1.0",
		Summary:         "",
		ProposedChanges: []string{},
		KRTargets:       []string{},
		KRImpactClaim:   "None",
	}

	data, _ := json.MarshalIndent(invalidResult, "", "  ")
	_ = os.WriteFile(resultPath, data, 0644)

	err := ValidateResultJSON(resultPath)
	if err == nil {
		t.Error("ValidateResultJSON() should fail for empty summary")
	}
}

func TestSnapshotDirHash(t *testing.T) {
	tmpDir := t.TempDir()

	// Create some test files
	_ = os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("content1"), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "file2.txt"), []byte("content2"), 0644)

	hash1, err := SnapshotDirHash(tmpDir)
	if err != nil {
		t.Fatalf("SnapshotDirHash() error: %v", err)
	}

	if hash1 == "" {
		t.Error("SnapshotDirHash() returned empty hash")
	}

	// Hash should be same for same content
	hash2, err := SnapshotDirHash(tmpDir)
	if err != nil {
		t.Fatalf("SnapshotDirHash() second call error: %v", err)
	}

	if hash1 != hash2 {
		t.Error("SnapshotDirHash() should return same hash for unchanged directory")
	}

	// Modify a file
	_ = os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("modified"), 0644)

	hash3, err := SnapshotDirHash(tmpDir)
	if err != nil {
		t.Fatalf("SnapshotDirHash() after modification error: %v", err)
	}

	if hash1 == hash3 {
		t.Error("SnapshotDirHash() should return different hash after modification")
	}
}

func TestDiffDir(t *testing.T) {
	hash1 := "abc123"
	hash2 := "abc123"
	hash3 := "def456"

	// Same hashes should return no changes
	changes, err := DiffDir(hash1, hash2)
	if err != nil {
		t.Errorf("DiffDir() error: %v", err)
	}
	if len(changes) != 0 {
		t.Error("DiffDir() should return no changes for same hashes")
	}

	// Different hashes should return changes
	changes, err = DiffDir(hash1, hash3)
	if err != nil {
		t.Errorf("DiffDir() error: %v", err)
	}
	if len(changes) == 0 {
		t.Error("DiffDir() should return changes for different hashes")
	}
}

func TestWriteViolation(t *testing.T) {
	tmpDir := t.TempDir()

	violation := BuildViolation("test_violation", map[string]any{
		"message": "Test violation message",
		"details": "Some details",
	})

	err := WriteViolation(tmpDir, violation)
	if err != nil {
		t.Fatalf("WriteViolation() error: %v", err)
	}

	// Verify file was written
	violationPath := filepath.Join(tmpDir, "violation.json")
	if _, err := os.Stat(violationPath); os.IsNotExist(err) {
		t.Error("WriteViolation() did not create violation.json")
	}

	// Verify content is valid JSON
	data, err := os.ReadFile(violationPath)
	if err != nil {
		t.Fatalf("read violation.json: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Errorf("violation.json is not valid JSON: %v", err)
	}

	if parsed["violation_type"] != "test_violation" {
		t.Errorf("violation_type mismatch: got %v", parsed["violation_type"])
	}
}

func TestGetWorkspaceRoot(t *testing.T) {
	tmpDir := t.TempDir()

	// Create okrs directory at root
	okrsDir := filepath.Join(tmpDir, "okrs")
	if err := os.MkdirAll(okrsDir, 0755); err != nil {
		t.Fatalf("create okrs dir: %v", err)
	}

	// Test from root
	root, err := GetWorkspaceRoot(tmpDir)
	if err != nil {
		t.Errorf("GetWorkspaceRoot() from root error: %v", err)
	}
	if root != tmpDir {
		t.Errorf("GetWorkspaceRoot() = %s, want %s", root, tmpDir)
	}

	// Create subdirectory
	subDir := filepath.Join(tmpDir, "sub", "nested")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("create nested dir: %v", err)
	}

	// Test from subdirectory
	root, err = GetWorkspaceRoot(subDir)
	if err != nil {
		t.Errorf("GetWorkspaceRoot() from subdir error: %v", err)
	}
	if root != tmpDir {
		t.Errorf("GetWorkspaceRoot() from subdir = %s, want %s", root, tmpDir)
	}

	// Test from directory without okrs
	noOkrsDir := t.TempDir()
	_, err = GetWorkspaceRoot(noOkrsDir)
	if err == nil {
		t.Error("GetWorkspaceRoot() should error for directory without okrs/")
	}
}
