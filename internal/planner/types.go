package planner

type Plan struct {
	ID          string     `json:"id"`
	AsOf        string     `json:"as_of"`
	GeneratedAt string     `json:"generated_at"`
	OKRsDir     string     `json:"okrs_dir"`
	Items       []PlanItem `json:"items"`
}

type PlanItem struct {
	ID                   string               `json:"id"`
	ObjectiveID          string               `json:"objective_id"`
	KRID                 string               `json:"kr_id"`
	Hypothesis           string               `json:"hypothesis"`
	Task                 string               `json:"task"`
	AgentRole            string               `json:"agent_role"`
	ExpectedMetricChange ExpectedMetricChange `json:"expected_metric_change"`
	EvidencePlan         []string             `json:"evidence_plan"`
}

type ExpectedMetricChange struct {
	MetricKey  string  `json:"metric_key"`
	Direction  string  `json:"direction"`
	Baseline   float64 `json:"baseline"`
	Target     float64 `json:"target"`
	Delta      float64 `json:"delta"`
	Rationale  string  `json:"rationale,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
}
