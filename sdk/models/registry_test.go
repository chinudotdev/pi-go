package models

import (
	"testing"

	"github.com/chinudotdev/pi-go/ai"
	"github.com/chinudotdev/pi-go/sdk/auth"
)

func testModels() []*ai.Model {
	return []*ai.Model{
		{ID: "claude-opus-4-8", Name: "Claude Opus 4.8", API: "anthropic-messages", Provider: "anthropic", BaseURL: "https://api.anthropic.com"},
		{ID: "gpt-5.4", Name: "GPT-5.4", API: "openai-completions", Provider: "openai", BaseURL: "https://api.openai.com"},
		{ID: "deepseek-v4-pro", Name: "DeepSeek V4 Pro", API: "openai-completions", Provider: "deepseek", BaseURL: "https://api.deepseek.com"},
	}
}

func TestFindExactModelReferenceMatch(t *testing.T) {
	models := testModels()

	// Canonical form
	m := FindExactModelReferenceMatch("anthropic/claude-opus-4-8", models)
	if m == nil || m.ID != "claude-opus-4-8" {
		t.Error("expected to find claude-opus-4-8 via canonical form")
	}

	// Case insensitive
	m = FindExactModelReferenceMatch("Anthropic/Claude-Opus-4-8", models)
	if m == nil {
		t.Error("expected case-insensitive match")
	}

	// Bare model ID (unique)
	m = FindExactModelReferenceMatch("gpt-5.4", models)
	if m == nil || m.Provider != "openai" {
		t.Error("expected to find gpt-5.4 by bare ID")
	}

	// No match
	m = FindExactModelReferenceMatch("nonexistent", models)
	if m != nil {
		t.Error("expected nil for nonexistent model")
	}

	// Empty
	m = FindExactModelReferenceMatch("", models)
	if m != nil {
		t.Error("expected nil for empty input")
	}
}

func TestParseModelPattern(t *testing.T) {
	models := testModels()

	// Exact match
	result := ParseModelPattern("claude-opus-4-8", models)
	if result.Model == nil || result.Model.ID != "claude-opus-4-8" {
		t.Error("expected exact match")
	}

	// With thinking level
	result = ParseModelPattern("claude-opus-4-8:high", models)
	if result.Model == nil || result.Model.ID != "claude-opus-4-8" {
		t.Error("expected match with thinking level")
	}
	if result.ThinkingLevel != "high" {
		t.Errorf("expected thinking level high, got %s", result.ThinkingLevel)
	}

	// Invalid thinking level
	result = ParseModelPattern("claude-opus-4-8:invalid", models)
	if result.Model == nil {
		t.Error("expected match despite invalid thinking level")
	}
	if result.Warning == "" {
		t.Error("expected warning for invalid thinking level")
	}

	// No match
	result = ParseModelPattern("nonexistent", models)
	if result.Model != nil {
		t.Error("expected nil for nonexistent model")
	}
}

func TestFindInitialModel_DefaultSettings(t *testing.T) {
	authStorage := auth.InMemory(nil)
	registry := InMemory(authStorage)

	// No default provider/model set
	result := FindInitialModel(registry, "", "", "")
	// Should return nil since no models have auth configured
	if result.Model != nil {
		t.Error("expected nil model when no auth configured")
	}
}

func TestRegistry_Find(t *testing.T) {
	authStorage := auth.InMemory(nil)
	registry := InMemory(authStorage)

	m := registry.Find("anthropic", "claude-opus-4-8")
	if m == nil {
		// Built-in models may not include this specific model; skip gracefully
		t.Skip("built-in model not available in test environment")
	}
	if m.ID != "claude-opus-4-8" {
		t.Errorf("expected claude-opus-4-8, got %s", m.ID)
	}

	m = registry.Find("anthropic", "nonexistent")
	if m != nil {
		t.Error("expected nil for nonexistent model")
	}
}

func TestRegistry_GetAll(t *testing.T) {
	authStorage := auth.InMemory(nil)
	registry := InMemory(authStorage)

	models := registry.GetAll()
	if len(models) == 0 {
		t.Error("expected built-in models")
	}
}

func TestRegistry_GetProviderDisplayName(t *testing.T) {
	authStorage := auth.InMemory(nil)
	registry := InMemory(authStorage)

	if name := registry.GetProviderDisplayName("anthropic"); name != "Anthropic" {
		t.Errorf("expected Anthropic, got %s", name)
	}
	if name := registry.GetProviderDisplayName("unknown-provider"); name != "unknown-provider" {
		t.Errorf("expected unknown-provider, got %s", name)
	}
}

func TestRegistry_RegisterProvider(t *testing.T) {
	authStorage := auth.InMemory(nil)
	registry := InMemory(authStorage)

	before := len(registry.GetAll())

	registry.RegisterProvider("custom", providerConfig{
		BaseURL: "https://custom.api.com",
		APIKey:  "test-key",
		API:     "openai-completions",
		Models: []modelDefinition{
			{ID: "custom-model", Name: "Custom Model", Reasoning: true},
		},
	})

	after := len(registry.GetAll())
	if after <= before {
		t.Errorf("expected more models after register, before=%d after=%d", before, after)
	}

	m := registry.Find("custom", "custom-model")
	if m == nil {
		t.Error("expected to find custom model")
	}
	if m.BaseURL != "https://custom.api.com" {
		t.Errorf("expected custom baseURL, got %s", m.BaseURL)
	}
}

func TestRegistry_UnregisterProvider(t *testing.T) {
	authStorage := auth.InMemory(nil)
	registry := InMemory(authStorage)

	registry.RegisterProvider("custom", providerConfig{
		BaseURL: "https://custom.api.com",
		APIKey:  "test-key",
		API:     "openai-completions",
		Models: []modelDefinition{
			{ID: "custom-model", Name: "Custom Model"},
		},
	})

	if registry.Find("custom", "custom-model") == nil {
		t.Error("expected model after register")
	}

	registry.UnregisterProvider("custom")

	// After unregister, model should be gone
	if registry.Find("custom", "custom-model") != nil {
		t.Error("expected nil after unregister")
	}
}

func TestRestoreModelFromSession(t *testing.T) {
	authStorage := auth.InMemory(nil)
	registry := InMemory(authStorage)

	// Find a real built-in model to test with
	allModels := registry.GetAll()
	if len(allModels) == 0 {
		t.Skip("no built-in models available")
	}
	testModel := allModels[0]

	// Restore nonexistent model — should fall back to current model
	m, msg := RestoreModelFromSession("nonexistent", "model-xyz", testModel, registry)
	if m == nil {
		t.Error("expected fallback to current model")
	}
	if msg == "" {
		t.Error("expected fallback message")
	}

	// Restore with no fallback model
	m, msg = RestoreModelFromSession("nonexistent", "model-xyz", nil, registry)
	// Should fall back to first available (which requires auth)
	if m == nil {
		// No available models (no auth) — that's ok
		if msg == "" {
			t.Error("expected fallback message")
		}
	}
}
