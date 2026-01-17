package metrics

import (
	"context"
	"fmt"
)

type ProviderResult struct {
	Provider string
	Points   []MetricPoint
}

// CollectAll runs providers and merges their points.
func CollectAll(ctx context.Context, providers []Provider) ([]MetricPoint, error) {
	var all []MetricPoint
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		points, err := provider.Collect(ctx)
		if err != nil {
			return nil, fmt.Errorf("%s provider: %w", provider.Name(), err)
		}
		all = append(all, points...)
	}
	return CanonicalizePoints(all), nil
}
