package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// MonitoringProvider loads metric points from a JSON file exported by monitoring systems.
// It is intended for metrics like api.uptime.pct that are sourced outside the repo.
type MonitoringProvider struct {
	ReportPath string
	AsOf       time.Time
}

func (p *MonitoringProvider) Name() string { return "monitoring" }

type monitoringReport struct {
	Metrics []monitoringMetric `json:"metrics"`
}

type monitoringMetric struct {
	Key        string            `json:"key"`
	Value      float64           `json:"value"`
	Unit       string            `json:"unit,omitempty"`
	Evidence   []string          `json:"evidence,omitempty"`
	Dimensions map[string]string `json:"dimensions,omitempty"`
}

func (p *MonitoringProvider) Collect(ctx context.Context) ([]MetricPoint, error) {
	_ = ctx

	if p.ReportPath == "" {
		p.ReportPath = filepath.Join("metrics", "monitoring_report.json")
	}

	data, err := os.ReadFile(p.ReportPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read monitoring report: %w", err)
	}

	var report monitoringReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("parse monitoring report: %w", err)
	}

	return p.pointsFrom(report.Metrics), nil
}

func (p *MonitoringProvider) pointsFrom(metrics []monitoringMetric) []MetricPoint {
	asOf := p.AsOf.UTC().Truncate(24 * time.Hour)
	ts := AsOfTimestamp(asOf)

	points := make([]MetricPoint, 0, len(metrics))
	for _, metric := range metrics {
		if metric.Key == "" {
			continue
		}

		var dims []Dimension
		if len(metric.Dimensions) > 0 {
			keys := make([]string, 0, len(metric.Dimensions))
			for k := range metric.Dimensions {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				dims = append(dims, Dimension{Key: k, Value: metric.Dimensions[k]})
			}
		}
		dims = CanonicalizeDimensions(dims)

		points = append(points, MetricPoint{
			Key:        metric.Key,
			Value:      metric.Value,
			Unit:       metric.Unit,
			Timestamp:  ts,
			Source:     p.Name(),
			Evidence:   metric.Evidence,
			Dimensions: dims,
		})
	}
	return points
}
