package okrstore

import (
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type rawDocument struct {
	Scope      string         `yaml:"scope"`
	Objectives []rawObjective `yaml:"objectives"`
}

type rawObjective struct {
	ID         string         `yaml:"objective_id"`
	Title      string         `yaml:"objective"`
	OwnerID    string         `yaml:"owner_id"`
	Notes      string         `yaml:"notes"`
	KeyResults []rawKeyResult `yaml:"key_results"`
}

type rawKeyResult struct {
	ID          string   `yaml:"kr_id"`
	Description string   `yaml:"description"`
	OwnerID     string   `yaml:"owner_id"`
	MetricKey   string   `yaml:"metric_key"`
	Baseline    *float64 `yaml:"baseline"`
	Target      *float64 `yaml:"target"`
	Confidence  *float64 `yaml:"confidence"`
	Status      string   `yaml:"status"`
	Evidence    []string `yaml:"evidence"`
	Current     *float64 `yaml:"current"`
	LastUpdated string   `yaml:"last_updated"`
}

// ValidationError captures a single field-specific validation issue.
type ValidationError struct {
	File    string
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	if e.Field == "" {
		return fmt.Sprintf("%s: %s", e.File, e.Message)
	}
	return fmt.Sprintf("%s: %s: %s", e.File, e.Field, e.Message)
}

// ValidationErrors aggregates multiple validation problems.
type ValidationErrors []ValidationError

func (errs ValidationErrors) Error() string {
	parts := make([]string, 0, len(errs))
	for _, e := range errs {
		parts = append(parts, e.Error())
	}
	return strings.Join(parts, "\n")
}

// ParseAndValidateDocument unmarshals and validates a YAML OKR document.
func ParseAndValidateDocument(data []byte, source string) (Document, error) {
	var raw rawDocument
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return Document{}, ValidationErrors{{
			File:    source,
			Field:   "yaml",
			Message: err.Error(),
		}}
	}
	return validateRawDocument(raw, source)
}

func validateRawDocument(raw rawDocument, source string) (Document, error) {
	var errs ValidationErrors

	scope, scopeErr := parseScope(raw.Scope)
	if scopeErr != nil {
		errs = append(errs, ValidationError{
			File:    source,
			Field:   "scope",
			Message: scopeErr.Error(),
		})
	}

	if len(raw.Objectives) == 0 {
		errs = append(errs, ValidationError{
			File:    source,
			Field:   "objectives",
			Message: "must contain at least one objective",
		})
	}

	objIDs := make(map[string]struct{})
	var normalizedObjectives []Objective

	for idx, rawObj := range raw.Objectives {
		objPath := fmt.Sprintf("objectives[%d]", idx)
		obj, objErrs := validateObjective(rawObj, objPath, scope, source)
		errs = append(errs, objErrs...)

		if obj.ID != "" {
			if _, exists := objIDs[obj.ID]; exists {
				errs = append(errs, ValidationError{
					File:    source,
					Field:   objPath + ".objective_id",
					Message: fmt.Sprintf("duplicate objective_id %q within scope", obj.ID),
				})
			} else {
				objIDs[obj.ID] = struct{}{}
			}
		}
		normalizedObjectives = append(normalizedObjectives, obj)
	}

	if len(errs) > 0 {
		return Document{}, errs
	}

	return Document{
		Scope:      scope,
		Objectives: normalizedObjectives,
		Source:     source,
	}, nil
}

func validateObjective(raw rawObjective, fieldPath string, scope Scope, source string) (Objective, ValidationErrors) {
	var errs ValidationErrors

	if strings.TrimSpace(raw.ID) == "" {
		errs = append(errs, ValidationError{
			File:    source,
			Field:   fieldPath + ".objective_id",
			Message: "objective_id is required",
		})
	}
	if strings.TrimSpace(raw.Title) == "" {
		errs = append(errs, ValidationError{
			File:    source,
			Field:   fieldPath + ".objective",
			Message: "objective text is required",
		})
	}
	if len(raw.KeyResults) == 0 {
		errs = append(errs, ValidationError{
			File:    source,
			Field:   fieldPath + ".key_results",
			Message: "must contain at least one key result",
		})
	}

	krIDs := make(map[string]struct{})
	var normalizedKRs []KeyResult

	for krIdx, rawKR := range raw.KeyResults {
		krPath := fmt.Sprintf("%s.key_results[%d]", fieldPath, krIdx)
		kr, krErrs := validateKeyResult(rawKR, krPath, source)
		errs = append(errs, krErrs...)

		if kr.ID != "" {
			if _, exists := krIDs[kr.ID]; exists {
				errs = append(errs, ValidationError{
					File:    source,
					Field:   krPath + ".kr_id",
					Message: fmt.Sprintf("duplicate kr_id %q within objective", kr.ID),
				})
			} else {
				krIDs[kr.ID] = struct{}{}
			}
		}
		normalizedKRs = append(normalizedKRs, kr)
	}

	obj := Objective{
		ID:            strings.TrimSpace(raw.ID),
		Objective:     strings.TrimSpace(raw.Title),
		OwnerID:       strings.TrimSpace(raw.OwnerID),
		Notes:         strings.TrimSpace(raw.Notes),
		KeyResults:    normalizedKRs,
		SourceFile:    source,
		DocumentScope: scope,
	}

	return obj, errs
}

func validateKeyResult(raw rawKeyResult, fieldPath string, source string) (KeyResult, ValidationErrors) {
	var errs ValidationErrors

	if strings.TrimSpace(raw.ID) == "" {
		errs = append(errs, ValidationError{
			File:    source,
			Field:   fieldPath + ".kr_id",
			Message: "kr_id is required",
		})
	}
	if strings.TrimSpace(raw.Description) == "" {
		errs = append(errs, ValidationError{
			File:    source,
			Field:   fieldPath + ".description",
			Message: "description is required",
		})
	}
	if strings.TrimSpace(raw.OwnerID) == "" {
		errs = append(errs, ValidationError{
			File:    source,
			Field:   fieldPath + ".owner_id",
			Message: "owner_id is required",
		})
	}
	if strings.TrimSpace(raw.MetricKey) == "" {
		errs = append(errs, ValidationError{
			File:    source,
			Field:   fieldPath + ".metric_key",
			Message: "metric_key is required",
		})
	}
	if raw.Baseline == nil {
		errs = append(errs, ValidationError{
			File:    source,
			Field:   fieldPath + ".baseline",
			Message: "baseline is required",
		})
	}
	if raw.Target == nil {
		errs = append(errs, ValidationError{
			File:    source,
			Field:   fieldPath + ".target",
			Message: "target is required",
		})
	}
	if raw.Confidence == nil {
		errs = append(errs, ValidationError{
			File:    source,
			Field:   fieldPath + ".confidence",
			Message: "confidence is required",
		})
	} else if *raw.Confidence < 0.0 || *raw.Confidence > 1.0 {
		errs = append(errs, ValidationError{
			File:    source,
			Field:   fieldPath + ".confidence",
			Message: "must be between 0.0 and 1.0",
		})
	}
	if strings.TrimSpace(raw.Status) == "" {
		errs = append(errs, ValidationError{
			File:    source,
			Field:   fieldPath + ".status",
			Message: "status is required",
		})
	}
	if raw.Evidence == nil {
		errs = append(errs, ValidationError{
			File:    source,
			Field:   fieldPath + ".evidence",
			Message: "evidence list is required",
		})
	} else {
		for i, ev := range raw.Evidence {
			if strings.TrimSpace(ev) == "" {
				errs = append(errs, ValidationError{
					File:    source,
					Field:   fmt.Sprintf("%s.evidence[%d]", fieldPath, i),
					Message: "evidence entries cannot be empty",
				})
			}
		}
	}

	if raw.LastUpdated != "" {
		if _, parseErr := parseISO8601(raw.LastUpdated); parseErr != nil {
			errs = append(errs, ValidationError{
				File:    source,
				Field:   fieldPath + ".last_updated",
				Message: "must be ISO-8601 date or datetime",
			})
		}
	}

	kr := KeyResult{
		ID:          strings.TrimSpace(raw.ID),
		Description: strings.TrimSpace(raw.Description),
		OwnerID:     strings.TrimSpace(raw.OwnerID),
		MetricKey:   strings.TrimSpace(raw.MetricKey),
		Status:      strings.TrimSpace(raw.Status),
		Evidence:    append([]string{}, raw.Evidence...),
		Current:     raw.Current,
		LastUpdated: strings.TrimSpace(raw.LastUpdated),
	}

	if raw.Baseline != nil {
		kr.Baseline = *raw.Baseline
	}
	if raw.Target != nil {
		kr.Target = *raw.Target
	}
	if raw.Confidence != nil {
		kr.Confidence = *raw.Confidence
	}
	if raw.Current != nil {
		v := *raw.Current
		kr.Current = &v
	}

	return kr, errs
}

func parseScope(value string) (Scope, error) {
	switch Scope(strings.TrimSpace(value)) {
	case ScopeOrg:
		return ScopeOrg, nil
	case ScopeTeam:
		return ScopeTeam, nil
	case ScopePerson:
		return ScopePerson, nil
	default:
		return Scope(value), fmt.Errorf("invalid scope %q (expected org, team, or person)", value)
	}
}

func parseISO8601(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}

	if ts, err := time.Parse(time.RFC3339, value); err == nil {
		return ts, nil
	}
	return time.Parse("2006-01-02", value)
}
