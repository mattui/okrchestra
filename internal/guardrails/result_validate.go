package guardrails

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// ResultSchema defines the expected structure of result.json per AGENTS.md
type ResultSchema struct {
	SchemaVersion   string   `json:"schema_version"`
	Summary         string   `json:"summary"`
	ProposedChanges []string `json:"proposed_changes"`
	KRTargets       []string `json:"kr_targets"`
	KRImpactClaim   string   `json:"kr_impact_claim"`
}

// ValidateResultJSON performs comprehensive validation of result.json according to AGENTS.md requirements.
// - Requires schema_version == "1.0"
// - Requires all mandatory fields: schema_version, summary, proposed_changes, kr_targets, kr_impact_claim
// - Rejects any unknown/extra fields
// - Validates field types and non-empty constraints
func ValidateResultJSON(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read result.json: %w", err)
	}

	// First, unmarshal into generic map to check for extra fields
	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawMap); err != nil {
		return fmt.Errorf("parse result.json: %w", err)
	}

	// Define allowed fields
	allowedFields := map[string]bool{
		"schema_version":   true,
		"summary":          true,
		"proposed_changes": true,
		"kr_targets":       true,
		"kr_impact_claim":  true,
	}

	// Check for unknown fields
	var extraFields []string
	for field := range rawMap {
		if !allowedFields[field] {
			extraFields = append(extraFields, field)
		}
	}
	if len(extraFields) > 0 {
		return fmt.Errorf("result.json contains disallowed fields: %v (only schema_version, summary, proposed_changes, kr_targets, kr_impact_claim are allowed)", extraFields)
	}

	// Check for required fields
	requiredFields := []string{"schema_version", "summary", "proposed_changes", "kr_targets", "kr_impact_claim"}
	for _, field := range requiredFields {
		if _, ok := rawMap[field]; !ok {
			return fmt.Errorf("missing required field: %s", field)
		}
	}

	// Now unmarshal into typed struct for detailed validation
	var result ResultSchema
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("parse result.json structure: %w", err)
	}

	// Validate schema_version
	if result.SchemaVersion != "1.0" {
		return fmt.Errorf("schema_version must be \"1.0\", got: %q", result.SchemaVersion)
	}

	// Validate summary is non-empty
	if strings.TrimSpace(result.Summary) == "" {
		return fmt.Errorf("summary must be a non-empty string")
	}

	// Validate proposed_changes is an array (can be empty)
	if result.ProposedChanges == nil {
		return fmt.Errorf("proposed_changes must be an array of strings (can be empty)")
	}

	// Validate kr_targets is an array (can be empty)
	if result.KRTargets == nil {
		return fmt.Errorf("kr_targets must be an array of strings (can be empty)")
	}

	// Validate kr_impact_claim is non-empty
	if strings.TrimSpace(result.KRImpactClaim) == "" {
		return fmt.Errorf("kr_impact_claim must be a non-empty string")
	}

	return nil
}

// ValidateResultJSONWithDetails returns a detailed error report if validation fails.
func ValidateResultJSONWithDetails(path string) (bool, []string) {
	err := ValidateResultJSON(path)
	if err == nil {
		return true, nil
	}

	errors := []string{err.Error()}
	return false, errors
}
