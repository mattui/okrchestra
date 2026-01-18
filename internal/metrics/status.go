package metrics

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"okrchestra/internal/okrstore"
)

// StatusChange represents a change in KR status.
type StatusChange struct {
	KRID       string
	OldStatus  string
	NewStatus  string
	Current    float64
	Target     float64
	Evidence   string
	KRDesc     string
	ObjectiveID string
}

// UpdateKRStatus updates KR status fields based on metric snapshots.
// It returns a list of status changes for notification purposes.
func UpdateKRStatus(okrsDir string, snapshot *Snapshot) ([]StatusChange, error) {
	if okrsDir == "" {
		okrsDir = "okrs"
	}

	// Load current OKR store
	store, err := okrstore.LoadFromDir(okrsDir)
	if err != nil {
		return nil, fmt.Errorf("load okrs: %w", err)
	}

	// Build map of metric_key -> current value
	metricValues := make(map[string]float64)
	for _, point := range snapshot.Points {
		metricValues[point.Key] = point.Value
	}

	// Track status changes
	var changes []StatusChange

	// Update status for each KR based on metrics
	for _, doc := range store.Org.Documents {
		updated := false
		for objIdx := range doc.Objectives {
			for krIdx := range doc.Objectives[objIdx].KeyResults {
				kr := &doc.Objectives[objIdx].KeyResults[krIdx]
				
				// Check if we have a metric value for this KR
				currentVal, hasMetric := metricValues[kr.MetricKey]
				if !hasMetric {
					continue
				}

				// Determine new status based on progress
				oldStatus := kr.Status
				newStatus := determineStatus(currentVal, kr.Baseline, kr.Target, oldStatus)

				// Update if status changed
				if newStatus != oldStatus {
					kr.Status = newStatus
					kr.Current = &currentVal
					kr.LastUpdated = time.Now().UTC().Format(time.RFC3339)
					
					// Add evidence reference to snapshot
					evidencePath := fmt.Sprintf("metrics/snapshots/%s", filepath.Base(snapshot.AsOf))
					if !contains(kr.Evidence, evidencePath) {
						kr.Evidence = append(kr.Evidence, evidencePath)
					}
					
					updated = true
					changes = append(changes, StatusChange{
						KRID:        kr.ID,
						OldStatus:   oldStatus,
						NewStatus:   newStatus,
						Current:     currentVal,
						Target:      kr.Target,
						Evidence:    evidencePath,
						KRDesc:      kr.Description,
						ObjectiveID: doc.Objectives[objIdx].ID,
					})
				}
			}
		}

		// Write back to file if any changes
		if updated {
			if err := writeDocumentToYAML(doc, doc.Source); err != nil {
				return changes, fmt.Errorf("write %s: %w", doc.Source, err)
			}
		}
	}

	return changes, nil
}

// determineStatus calculates the appropriate status based on progress.
func determineStatus(current, baseline, target float64, oldStatus string) string {
	// Never override manually-set blocked or at_risk status
	if oldStatus == "blocked" || oldStatus == "at_risk" {
		return oldStatus
	}

	// Check if achieved
	if current >= target {
		return "achieved"
	}

	// Check if in progress
	if current > baseline {
		return "in_progress"
	}

	// Still not started
	return "not_started"
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// writeDocumentToYAML writes a Document back to its source YAML file.
func writeDocumentToYAML(doc okrstore.Document, path string) error {
	// Convert to raw format for YAML marshaling
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
		Current     *float64 `yaml:"current,omitempty"`
		LastUpdated string   `yaml:"last_updated,omitempty"`
	}

	type rawObjective struct {
		ID         string         `yaml:"objective_id"`
		Title      string         `yaml:"objective"`
		OwnerID    string         `yaml:"owner_id,omitempty"`
		Notes      string         `yaml:"notes,omitempty"`
		KeyResults []rawKeyResult `yaml:"key_results"`
	}

	type rawDocument struct {
		Scope      string         `yaml:"scope"`
		Objectives []rawObjective `yaml:"objectives"`
	}

	raw := rawDocument{
		Scope:      string(doc.Scope),
		Objectives: make([]rawObjective, len(doc.Objectives)),
	}

	for i, obj := range doc.Objectives {
		rawObj := rawObjective{
			ID:         obj.ID,
			Title:      obj.Objective,
			OwnerID:    obj.OwnerID,
			Notes:      obj.Notes,
			KeyResults: make([]rawKeyResult, len(obj.KeyResults)),
		}

		for j, kr := range obj.KeyResults {
			rawKR := rawKeyResult{
				ID:          kr.ID,
				Description: kr.Description,
				OwnerID:     kr.OwnerID,
				MetricKey:   kr.MetricKey,
				Baseline:    &kr.Baseline,
				Target:      &kr.Target,
				Confidence:  &kr.Confidence,
				Status:      kr.Status,
				Evidence:    kr.Evidence,
				Current:     kr.Current,
				LastUpdated: kr.LastUpdated,
			}
			rawObj.KeyResults[j] = rawKR
		}

		raw.Objectives[i] = rawObj
	}

	// Marshal to YAML
	data, err := yaml.Marshal(&raw)
	if err != nil {
		return fmt.Errorf("marshal yaml: %w", err)
	}

	// Write atomically via temp file
	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}
