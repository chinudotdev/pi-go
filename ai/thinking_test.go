package ai

import (
	"testing"
)

func TestClampThinkingLevel_NonReasoningModel(t *testing.T) {
	model := &Model{Reasoning: false}
	result := ClampThinkingLevel(model, ThinkingMHigh)
	if result != ThinkingOff {
		t.Errorf("ClampThinkingLevel(non-reasoning, high) = %v, want off", result)
	}
}

func TestClampThinkingLevel_ExactMatch(t *testing.T) {
	model := &Model{
		Reasoning: true,
		ThinkingLevelMap: ThinkingLevelMap{
			ThinkingOff:   strPtr("off"),
			ThinkingMLow:  strPtr("low"),
			ThinkingMHigh: strPtr("high"),
		},
	}
	result := ClampThinkingLevel(model, ThinkingMLow)
	if result != ThinkingMLow {
		t.Errorf("ClampThinkingLevel(low) = %v, want low", result)
	}
}

func TestClampThinkingLevel_MissingLevelStillAvailable(t *testing.T) {
	// Per TS semantics: levels not in the map are still available (default behavior).
	// Only explicit nil/null entries are excluded.
	model := &Model{
		Reasoning: true,
		ThinkingLevelMap: ThinkingLevelMap{
			ThinkingOff:   strPtr("off"),
			ThinkingMHigh: strPtr("high"),
		},
	}
	// "minimal" is not in the map, so it's still available with default
	result := ClampThinkingLevel(model, ThinkingMMin)
	if result != ThinkingMMin {
		t.Errorf("ClampThinkingLevel(minimal) = %v, want minimal", result)
	}
}

func TestClampThinkingLevel_ClampsDown(t *testing.T) {
	model := &Model{
		Reasoning: true,
		ThinkingLevelMap: ThinkingLevelMap{
			ThinkingOff:     strPtr("off"),
			ThinkingMLow:    strPtr("low"),
			ThinkingMMedium: nil, // explicitly unsupported
			ThinkingMHigh:   nil, // explicitly unsupported
			ThinkingMXHigh:  nil, // explicitly unsupported
		},
	}
	// "high" is explicitly nil; should clamp down to "low"
	result := ClampThinkingLevel(model, ThinkingMHigh)
	if result != ThinkingMLow {
		t.Errorf("ClampThinkingLevel(high) = %v, want low", result)
	}
}

func TestClampThinkingLevel_XHighRequiresExplicitMapping(t *testing.T) {
	model := &Model{
		Reasoning: true,
		ThinkingLevelMap: ThinkingLevelMap{
			ThinkingOff:    strPtr("off"),
			ThinkingMHigh:  strPtr("high"),
			ThinkingMXHigh: strPtr("xhigh"),
		},
	}
	result := ClampThinkingLevel(model, ThinkingMXHigh)
	if result != ThinkingMXHigh {
		t.Errorf("ClampThinkingLevel(xhigh) = %v, want xhigh", result)
	}
}

func TestClampThinkingLevel_XHighNotAvailable(t *testing.T) {
	model := &Model{
		Reasoning: true,
		ThinkingLevelMap: ThinkingLevelMap{
			ThinkingOff:   strPtr("off"),
			ThinkingMHigh: strPtr("high"),
		},
	}
	result := ClampThinkingLevel(model, ThinkingMXHigh)
	if result != ThinkingMHigh {
		t.Errorf("ClampThinkingLevel(xhigh without mapping) = %v, want high", result)
	}
}

func TestGetSupportedThinkingLevels_NullExcludesLevel(t *testing.T) {
	model := &Model{
		Reasoning: true,
		ThinkingLevelMap: ThinkingLevelMap{
			ThinkingOff:     strPtr("off"),
			ThinkingMMin:    nil, // unsupported
			ThinkingMLow:    strPtr("low"),
			ThinkingMMedium: strPtr("medium"),
			ThinkingMHigh:   strPtr("high"),
		},
	}
	levels := GetSupportedThinkingLevels(model)
	for _, l := range levels {
		if l == ThinkingMMin {
			t.Error("minimal should be excluded when mapped to nil")
		}
	}
}

func strPtr(s string) *string { return &s }
