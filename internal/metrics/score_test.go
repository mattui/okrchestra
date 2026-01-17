package metrics

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"okrchestra/internal/okrstore"
)

func TestCanonicalizePointsDeterministic(t *testing.T) {
	points := []MetricPoint{
		{
			Key:       "b",
			Value:     2,
			Timestamp: "2026-01-17T00:00:00Z",
			Source:    "x",
			Evidence:  []string{"  e2  ", "e1", "e1"},
		},
		{
			Key:       "a",
			Value:     1,
			Timestamp: "2026-01-17T00:00:00Z",
			Source:    "x",
			Dimensions: []Dimension{
				{Key: "z", Value: "9"},
				{Key: "a", Value: "1"},
			},
		},
	}

	c := CanonicalizePoints(points)
	if got, want := c[0].Key, "a"; got != want {
		t.Fatalf("first key = %q, want %q", got, want)
	}
	if len(c[1].Evidence) != 2 || c[1].Evidence[0] != "e1" || c[1].Evidence[1] != "e2" {
		t.Fatalf("evidence not canonicalized: %#v", c[1].Evidence)
	}
	if len(c[0].Dimensions) != 2 || c[0].Dimensions[0].Key != "a" || c[0].Dimensions[1].Key != "z" {
		t.Fatalf("dimensions not canonicalized: %#v", c[0].Dimensions)
	}
}

func TestScoreKRsDeterministic(t *testing.T) {
	tmp := t.TempDir()
	okrsDir := filepath.Join(tmp, "okrs")
	if err := os.MkdirAll(okrsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	okrsYAML := []byte(`scope: org
objectives:
  - objective_id: OBJ-1
    objective: Objective
    key_results:
      - kr_id: KR-1
        description: Improve
        owner_id: team
        metric_key: m.one
        baseline: 10
        target: 20
        confidence: 0.5
        status: in_progress
        evidence: []
      - kr_id: KR-2
        description: Reduce
        owner_id: team
        metric_key: m.two
        baseline: 100
        target: 80
        confidence: 0.5
        status: in_progress
        evidence: []
`)
	if err := os.WriteFile(filepath.Join(okrsDir, "org.yml"), okrsYAML, 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := okrstore.LoadFromDir(okrsDir)
	if err != nil {
		t.Fatal(err)
	}

	asOf := time.Date(2026, 1, 17, 0, 0, 0, 0, time.UTC)
	snap := &Snapshot{
		SchemaVersion: SnapshotSchemaVersion,
		AsOf:          "2026-01-17",
		Points: []MetricPoint{
			{Key: "m.two", Value: 90, Unit: "count", Timestamp: AsOfTimestamp(asOf), Source: "manual"},
			{Key: "m.one", Value: 15, Unit: "count", Timestamp: AsOfTimestamp(asOf), Source: "manual"},
			{Key: "m.ignored", Value: 1, Timestamp: AsOfTimestamp(asOf), Source: "manual", Dimensions: []Dimension{{Key: "env", Value: "prod"}}},
		},
	}

	report, err := ScoreKRs(store, snap, "snap.json")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(report.Results), 2; got != want {
		t.Fatalf("results len = %d, want %d", got, want)
	}
	if report.Results[0].KRID != "KR-1" || report.Results[1].KRID != "KR-2" {
		t.Fatalf("unexpected order: %#v", report.Results)
	}
	if got, want := report.Results[0].PercentToTarget, 50.0; got != want {
		t.Fatalf("KR-1 percent = %v, want %v", got, want)
	}
	if got, want := report.Results[1].PercentToTarget, 50.0; got != want {
		t.Fatalf("KR-2 percent = %v, want %v", got, want)
	}
}
