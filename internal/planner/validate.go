package planner

import (
	"fmt"
	"strings"
)

func ValidatePlan(plan Plan) error {
	if strings.TrimSpace(plan.ID) == "" {
		return fmt.Errorf("plan id is required")
	}
	if strings.TrimSpace(plan.AsOf) == "" {
		return fmt.Errorf("plan as_of is required")
	}
	if len(plan.Items) == 0 {
		return fmt.Errorf("plan must include at least one item")
	}
	for idx, item := range plan.Items {
		if err := ValidatePlanItem(item); err != nil {
			return fmt.Errorf("plan item %d: %w", idx, err)
		}
	}
	return nil
}

func ValidatePlanItem(item PlanItem) error {
	if strings.TrimSpace(item.ObjectiveID) == "" {
		return fmt.Errorf("objective_id is required")
	}
	if strings.TrimSpace(item.KRID) == "" {
		return fmt.Errorf("kr_id is required")
	}
	if strings.TrimSpace(item.Task) == "" {
		return fmt.Errorf("task is required")
	}
	if strings.TrimSpace(item.AgentRole) == "" {
		return fmt.Errorf("agent_role is required")
	}
	metricKey := strings.TrimSpace(item.ExpectedMetricChange.MetricKey)
	if metricKey == "" {
		return fmt.Errorf("expected_metric_change.metric_key is required")
	}
	direction := strings.TrimSpace(item.ExpectedMetricChange.Direction)
	if direction == "" {
		return fmt.Errorf("expected_metric_change.direction is required")
	}
	if direction != "increase" && direction != "decrease" {
		return fmt.Errorf("expected_metric_change.direction must be \"increase\" or \"decrease\"")
	}
	return nil
}
