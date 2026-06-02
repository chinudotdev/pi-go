package ai

import (
	"testing"
)

func TestGetModel_Found(t *testing.T) {
	model, ok := GetModel("anthropic", "claude-sonnet-4-20250514")
	if !ok {
		t.Fatal("expected to find claude-sonnet-4")
	}
	if model.Provider != "anthropic" {
		t.Errorf("model.Provider = %q, want %q", model.Provider, "anthropic")
	}
	if model.API != "anthropic-messages" {
		t.Errorf("model.API = %q, want %q", model.API, "anthropic-messages")
	}
}

func TestGetModel_NotFound(t *testing.T) {
	_, ok := GetModel("nonexistent", "no-model")
	if ok {
		t.Error("expected model not to be found")
	}
}

func TestGetProviders_Sorted(t *testing.T) {
	providers := GetProviders()
	if len(providers) == 0 {
		t.Fatal("expected at least one provider")
	}
	// Verify sorted order
	for i := 1; i < len(providers); i++ {
		if providers[i] < providers[i-1] {
			t.Errorf("providers not sorted: %q > %q at index %d", providers[i-1], providers[i], i)
		}
	}
}

func TestGetModels(t *testing.T) {
	models := GetModels("anthropic")
	if len(models) == 0 {
		t.Fatal("expected at least one Anthropic model")
	}
}

func TestRegisterModel(t *testing.T) {
	customModel := &Model{
		ID:       "custom-1",
		Name:     "Custom Model",
		API:      "openai-completions",
		Provider: "custom",
		Input:    []string{"text"},
		Cost:     ModelCost{Input: 1.0, Output: 2.0},
	}
	RegisterModel(customModel)

	model, ok := GetModel("custom", "custom-1")
	if !ok {
		t.Fatal("expected to find registered custom model")
	}
	if model.Name != "Custom Model" {
		t.Errorf("model.Name = %q, want %q", model.Name, "Custom Model")
	}
}

func TestModelsAreEqual(t *testing.T) {
	a := &Model{ID: "gpt-4", Provider: "openai"}
	b := &Model{ID: "gpt-4", Provider: "openai"}
	c := &Model{ID: "gpt-4", Provider: "anthropic"}

	if !ModelsAreEqual(a, b) {
		t.Error("identical models should be equal")
	}
	if ModelsAreEqual(a, c) {
		t.Error("different provider models should not be equal")
	}
	if ModelsAreEqual(a, nil) {
		t.Error("nil model should not be equal")
	}
}
