package planner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func LoadPlan(path string) (Plan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Plan{}, fmt.Errorf("read plan: %w", err)
	}
	var plan Plan
	if err := json.Unmarshal(data, &plan); err != nil {
		return Plan{}, fmt.Errorf("parse plan json: %w", err)
	}
	if err := ValidatePlan(plan); err != nil {
		return Plan{}, err
	}
	return plan, nil
}

func ResolvePlanPath(inputPath string) (string, error) {
	if inputPath == "" {
		return "", fmt.Errorf("plan path is required")
	}
	info, err := os.Stat(inputPath)
	if err != nil {
		return "", fmt.Errorf("stat plan path: %w", err)
	}
	if info.IsDir() {
		return filepath.Join(inputPath, "plan.json"), nil
	}
	return inputPath, nil
}
