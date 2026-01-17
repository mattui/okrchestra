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

type CIProvider struct {
	ReportPath string
	AsOf       time.Time
}

func (p *CIProvider) Name() string { return "ci" }

func (p *CIProvider) Collect(ctx context.Context) ([]MetricPoint, error) {
	_ = ctx

	if p.ReportPath == "" {
		p.ReportPath = filepath.Join("metrics", "ci_report.json")
	}

	data, err := os.ReadFile(p.ReportPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read ci report: %w", err)
	}

	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse ci report: %w", err)
	}

	var metrics map[string]any
	switch v := raw.(type) {
	case map[string]any:
		if inner, ok := v["metrics"].(map[string]any); ok {
			metrics = inner
		} else {
			metrics = v
		}
	default:
		return nil, fmt.Errorf("ci report must be a JSON object")
	}

	asOf := p.AsOf.UTC().Truncate(24 * time.Hour)
	ts := AsOfTimestamp(asOf)

	var keys []string
	for k, val := range metrics {
		switch val.(type) {
		case float64, int, int64, json.Number:
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	points := make([]MetricPoint, 0, len(keys))
	for _, k := range keys {
		value, ok := toFloat64(metrics[k])
		if !ok {
			continue
		}
		points = append(points, MetricPoint{
			Key:       "ci." + k,
			Value:     value,
			Unit:      inferCIUnit(k),
			Timestamp: ts,
			Source:    p.Name(),
		})
	}
	return points, nil
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}

func inferCIUnit(key string) string {
	if key == "pass_rate_30d" || key == "success_rate_30d" {
		return "ratio"
	}
	return ""
}
