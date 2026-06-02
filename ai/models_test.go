package ai

import (
	"math"
	"testing"
)

func TestCalculateCost_NoMutation(t *testing.T) {
	model := &Model{
		Cost: ModelCost{
			Input:      3.0,
			Output:     15.0,
			CacheRead:  0.3,
			CacheWrite: 3.75,
		},
	}
	usage := Usage{
		Input:      1000,
		Output:     500,
		CacheRead:  200,
		CacheWrite: 100,
	}

	// CalculateCost must not mutate usage
	originalInput := usage.Input
	originalCost := usage.Cost

	cost := CalculateCost(model, usage)

	if usage.Input != originalInput {
		t.Errorf("CalculateCost mutated usage.Input: got %d, want %d", usage.Input, originalInput)
	}
	if usage.Cost != originalCost {
		t.Errorf("CalculateCost mutated usage.Cost: got %v, want %v", usage.Cost, originalCost)
	}

	// Verify cost values
	expectedInput := 3.0 / 1_000_000 * 1000
	if math.Abs(cost.Input-expectedInput) > 1e-9 {
		t.Errorf("cost.Input = %f, want %f", cost.Input, expectedInput)
	}
	expectedOutput := 15.0 / 1_000_000 * 500
	if math.Abs(cost.Output-expectedOutput) > 1e-9 {
		t.Errorf("cost.Output = %f, want %f", cost.Output, expectedOutput)
	}
	expectedTotal := cost.Input + cost.Output + cost.CacheRead + cost.CacheWrite
	if math.Abs(cost.Total-expectedTotal) > 1e-9 {
		t.Errorf("cost.Total = %f, want %f", cost.Total, expectedTotal)
	}
}

func TestApplyCost_MutatesUsage(t *testing.T) {
	model := &Model{
		Cost: ModelCost{Input: 3.0, Output: 15.0, CacheRead: 0.3, CacheWrite: 3.75},
	}
	usage := Usage{Input: 1000, Output: 500}

	cost := ApplyCost(model, &usage)

	// ApplyCost should mutate usage
	if usage.Cost.Input != cost.Input {
		t.Errorf("ApplyCost did not set usage.Cost.Input: got %f, want %f", usage.Cost.Input, cost.Input)
	}
	if usage.Cost.Total != cost.Total {
		t.Errorf("ApplyCost did not set usage.Cost.Total: got %f, want %f", usage.Cost.Total, cost.Total)
	}
}

func TestCalculateCost_ZeroUsage(t *testing.T) {
	model := &Model{
		Cost: ModelCost{Input: 3.0, Output: 15.0, CacheRead: 0.3, CacheWrite: 3.75},
	}
	usage := Usage{}

	cost := CalculateCost(model, usage)
	if cost.Total != 0 {
		t.Errorf("cost.Total = %f, want 0", cost.Total)
	}
}
