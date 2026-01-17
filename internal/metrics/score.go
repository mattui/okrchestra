package metrics

import (
	"fmt"
	"math"
	"sort"

	"okrchestra/internal/okrstore"
)

type KRScore struct {
	Scope           string   `json:"scope"`
	ObjectiveID     string   `json:"objective_id"`
	Objective       string   `json:"objective"`
	KRID            string   `json:"kr_id"`
	Description     string   `json:"description"`
	MetricKey       string   `json:"metric_key"`
	Baseline        float64  `json:"baseline"`
	Target          float64  `json:"target"`
	Current         *float64 `json:"current,omitempty"`
	Unit            string   `json:"unit,omitempty"`
	PercentToTarget float64  `json:"percent_to_target"`
}

type KRScoreReport struct {
	SchemaVersion     int       `json:"schema_version"`
	AsOf              string    `json:"as_of"`
	SnapshotPath      string    `json:"snapshot_path"`
	Results           []KRScore `json:"results"`
	MissingMetricKeys []string  `json:"missing_metric_keys,omitempty"`
}

const KRScoreSchemaVersion = 1

// ScoreKRs computes a deterministic percent-to-target for each KR based on snapshot metrics.
func ScoreKRs(store *okrstore.Store, snapshot *Snapshot, snapshotPath string) (*KRScoreReport, error) {
	if store == nil {
		return nil, fmt.Errorf("okr store is required")
	}
	if snapshot == nil {
		return nil, fmt.Errorf("snapshot is required")
	}

	metricValues := make(map[string]MetricPoint)
	for _, point := range snapshot.Points {
		if point.Key == "" {
			continue
		}
		if len(point.Dimensions) > 0 {
			// Current KR schema maps to a single metric_key; dimensioned points are ignored.
			continue
		}
		if existing, ok := metricValues[point.Key]; ok {
			return nil, fmt.Errorf("duplicate metric key %q from sources %q and %q", point.Key, existing.Source, point.Source)
		}
		metricValues[point.Key] = point
	}

	var results []KRScore
	missing := make(map[string]struct{})

	collect := func(scope okrstore.Scope, docs []okrstore.Document) {
		for _, doc := range docs {
			for _, obj := range doc.Objectives {
				for _, kr := range obj.KeyResults {
					score := KRScore{
						Scope:       string(scope),
						ObjectiveID: obj.ID,
						Objective:   obj.Objective,
						KRID:        kr.ID,
						Description: kr.Description,
						MetricKey:   kr.MetricKey,
						Baseline:    kr.Baseline,
						Target:      kr.Target,
					}
					if point, ok := metricValues[kr.MetricKey]; ok {
						score.Current = ptr(point.Value)
						score.Unit = point.Unit
						score.PercentToTarget = percentToTarget(kr.Baseline, kr.Target, point.Value)
					} else {
						score.Current = nil
						score.PercentToTarget = 0
						if kr.MetricKey != "" {
							missing[kr.MetricKey] = struct{}{}
						}
					}
					results = append(results, score)
				}
			}
		}
	}

	collect(okrstore.ScopeOrg, store.Org.Documents)
	collect(okrstore.ScopeTeam, store.Team.Documents)
	collect(okrstore.ScopePerson, store.Person.Documents)

	sort.SliceStable(results, func(i, j int) bool {
		a := results[i]
		b := results[j]
		if a.Scope != b.Scope {
			return a.Scope < b.Scope
		}
		if a.ObjectiveID != b.ObjectiveID {
			return a.ObjectiveID < b.ObjectiveID
		}
		return a.KRID < b.KRID
	})

	var missingKeys []string
	for k := range missing {
		missingKeys = append(missingKeys, k)
	}
	sort.Strings(missingKeys)

	return &KRScoreReport{
		SchemaVersion:     KRScoreSchemaVersion,
		AsOf:              snapshot.AsOf,
		SnapshotPath:      snapshotPath,
		Results:           results,
		MissingMetricKeys: missingKeys,
	}, nil
}

func percentToTarget(baseline, target, current float64) float64 {
	if baseline == target {
		if current >= target {
			return 100
		}
		return 0
	}

	var progress float64
	if target > baseline {
		progress = (current - baseline) / (target - baseline)
	} else {
		progress = (baseline - current) / (baseline - target)
	}

	if math.IsNaN(progress) || math.IsInf(progress, 0) {
		return 0
	}
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	return progress * 100
}

func ptr(v float64) *float64 {
	return &v
}
