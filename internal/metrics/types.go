package metrics

import (
	"context"
	"sort"
	"strings"
	"time"
)

// Provider collects metric points from a single source.
type Provider interface {
	Name() string
	Collect(ctx context.Context) ([]MetricPoint, error)
}

// Dimension is a single key/value attribute attached to a metric point.
// It is represented as a slice (not a map) to keep JSON output deterministic.
type Dimension struct {
	Key   string `json:"key" yaml:"key"`
	Value string `json:"value" yaml:"value"`
}

// MetricPoint is a single observed value.
type MetricPoint struct {
	Key        string      `json:"key" yaml:"key"`
	Value      float64     `json:"value" yaml:"value"`
	Unit       string      `json:"unit,omitempty" yaml:"unit,omitempty"`
	Timestamp  string      `json:"timestamp" yaml:"timestamp"`
	Source     string      `json:"source" yaml:"source"`
	Evidence   []string    `json:"evidence,omitempty" yaml:"evidence,omitempty"`
	Dimensions []Dimension `json:"dimensions,omitempty" yaml:"dimensions,omitempty"`
}

// CanonicalizePoints sorts and normalizes metric points for deterministic output.
func CanonicalizePoints(points []MetricPoint) []MetricPoint {
	normalized := make([]MetricPoint, 0, len(points))
	for _, point := range points {
		point.Evidence = canonicalizeStrings(point.Evidence)
		point.Dimensions = CanonicalizeDimensions(point.Dimensions)
		normalized = append(normalized, point)
	}

	sort.SliceStable(normalized, func(i, j int) bool {
		a := normalized[i]
		b := normalized[j]
		if a.Key != b.Key {
			return a.Key < b.Key
		}
		ad := dimensionsKey(a.Dimensions)
		bd := dimensionsKey(b.Dimensions)
		if ad != bd {
			return ad < bd
		}
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		if a.Unit != b.Unit {
			return a.Unit < b.Unit
		}
		if a.Timestamp != b.Timestamp {
			return a.Timestamp < b.Timestamp
		}
		return a.Value < b.Value
	})

	return normalized
}

func canonicalizeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

// CanonicalizeDimensions sorts dimensions and drops empty entries.
func CanonicalizeDimensions(dimensions []Dimension) []Dimension {
	if len(dimensions) == 0 {
		return nil
	}
	out := make([]Dimension, 0, len(dimensions))
	seen := make(map[string]struct{}, len(dimensions))
	for _, d := range dimensions {
		key := strings.TrimSpace(d.Key)
		value := strings.TrimSpace(d.Value)
		if key == "" || value == "" {
			continue
		}
		fingerprint := key + "\x00" + value
		if _, ok := seen[fingerprint]; ok {
			continue
		}
		seen[fingerprint] = struct{}{}
		out = append(out, Dimension{Key: key, Value: value})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Key != out[j].Key {
			return out[i].Key < out[j].Key
		}
		return out[i].Value < out[j].Value
	})
	if len(out) == 0 {
		return nil
	}
	return out
}

func dimensionsKey(dimensions []Dimension) string {
	if len(dimensions) == 0 {
		return ""
	}
	var b strings.Builder
	for i, d := range dimensions {
		if i > 0 {
			b.WriteByte(';')
		}
		b.WriteString(d.Key)
		b.WriteByte('=')
		b.WriteString(d.Value)
	}
	return b.String()
}

// AsOfTimestamp returns the RFC3339 timestamp used for snapshot points for the given date.
func AsOfTimestamp(asOf time.Time) string {
	return asOf.UTC().Truncate(24 * time.Hour).Format(time.RFC3339)
}
