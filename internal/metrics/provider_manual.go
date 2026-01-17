package metrics

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"gopkg.in/yaml.v3"
)

type ManualProvider struct {
	Path string
	AsOf time.Time
}

func (p *ManualProvider) Name() string { return "manual" }

type manualFile struct {
	Metrics []manualMetric `yaml:"metrics"`
}

type manualMetric struct {
	Key        string            `yaml:"key"`
	Value      float64           `yaml:"value"`
	Unit       string            `yaml:"unit"`
	Evidence   []string          `yaml:"evidence"`
	Dimensions map[string]string `yaml:"dimensions"`
}

func (p *ManualProvider) Collect(ctx context.Context) ([]MetricPoint, error) {
	_ = ctx

	if p.Path == "" {
		p.Path = filepath.Join("metrics", "manual.yml")
	}

	data, err := os.ReadFile(p.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read manual metrics: %w", err)
	}

	var file manualFile
	if err := yaml.Unmarshal(data, &file); err == nil && file.Metrics != nil {
		return p.pointsFrom(file.Metrics)
	}

	var list []manualMetric
	if err := yaml.Unmarshal(data, &list); err == nil && list != nil {
		return p.pointsFrom(list)
	}

	return nil, fmt.Errorf("manual metrics file must contain `metrics:` list or a top-level list")
}

func (p *ManualProvider) pointsFrom(metrics []manualMetric) ([]MetricPoint, error) {
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

	return points, nil
}
