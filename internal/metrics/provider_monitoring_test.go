package metrics

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMonitoringProviderCollect_MissingFile(t *testing.T) {
	t.Parallel()

	p := &MonitoringProvider{
		ReportPath: filepath.Join(t.TempDir(), "missing.json"),
		AsOf:       time.Date(2026, 1, 17, 12, 0, 0, 0, time.UTC),
	}

	points, err := p.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}
	if points != nil {
		t.Fatalf("expected nil points, got %#v", points)
	}
}

func TestMonitoringProviderCollect_ParsesPoints(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "monitoring_report.json")
	if err := os.WriteFile(path, []byte(`{
  "metrics": [
    {
      "key": "api.uptime.pct",
      "value": 99.3,
      "unit": "pct",
      "evidence": [" monitoring:uptime-report:2026-01 ", "monitoring:uptime-report:2026-01"],
      "dimensions": {"region": "us-east-1", "service": "api"}
    }
  ]
}`), 0o644); err != nil {
		t.Fatalf("write report: %v", err)
	}

	p := &MonitoringProvider{
		ReportPath: path,
		AsOf:       time.Date(2026, 1, 17, 12, 0, 0, 0, time.UTC),
	}

	points, err := p.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}
	points = CanonicalizePoints(points)

	if len(points) != 1 {
		t.Fatalf("expected 1 point, got %d: %#v", len(points), points)
	}

	point := points[0]
	if point.Key != "api.uptime.pct" {
		t.Fatalf("unexpected key: %q", point.Key)
	}
	if point.Value != 99.3 {
		t.Fatalf("unexpected value: %v", point.Value)
	}
	if point.Unit != "pct" {
		t.Fatalf("unexpected unit: %q", point.Unit)
	}
	if point.Source != "monitoring" {
		t.Fatalf("unexpected source: %q", point.Source)
	}
	if point.Timestamp != "2026-01-17T00:00:00Z" {
		t.Fatalf("unexpected timestamp: %q", point.Timestamp)
	}
	if len(point.Evidence) != 1 || point.Evidence[0] != "monitoring:uptime-report:2026-01" {
		t.Fatalf("unexpected evidence: %#v", point.Evidence)
	}
	if len(point.Dimensions) != 2 ||
		point.Dimensions[0] != (Dimension{Key: "region", Value: "us-east-1"}) ||
		point.Dimensions[1] != (Dimension{Key: "service", Value: "api"}) {
		t.Fatalf("unexpected dimensions: %#v", point.Dimensions)
	}
}

func TestMonitoringProviderCollect_InvalidJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "monitoring_report.json")
	if err := os.WriteFile(path, []byte(`{ "metrics": [`), 0o644); err != nil {
		t.Fatalf("write report: %v", err)
	}

	p := &MonitoringProvider{
		ReportPath: path,
		AsOf:       time.Date(2026, 1, 17, 12, 0, 0, 0, time.UTC),
	}
	_, err := p.Collect(context.Background())
	if err == nil {
		t.Fatalf("expected error")
	}
}
