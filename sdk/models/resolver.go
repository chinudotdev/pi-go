// Package models provides model discovery and resolution.
// This file implements initial model selection and CLI model resolution.
package models

import (
	"fmt"
	"strings"

	"github.com/chinudotdev/pi-go/ai"
	"github.com/chinudotdev/pi-go/sdk/config"
)

// DefaultModelPerProvider maps known providers to their default model IDs.
var DefaultModelPerProvider = map[string]string{
	"anthropic":              "claude-opus-4-8",
	"openai":                 "gpt-5.4",
	"deepseek":               "deepseek-v4-pro",
	"google":                 "gemini-3.1-pro-preview",
	"google-vertex":          "gemini-3.1-pro-preview",
	"openrouter":             "moonshotai/kimi-k2.6",
	"xai":                    "grok-4.20-0309-reasoning",
	"groq":                   "openai/gpt-oss-120b",
	"mistral":                "devstral-medium-latest",
	"minimax":                "MiniMax-M2.7",
	"moonshotai":             "kimi-k2.6",
	"fireworks":              "accounts/fireworks/models/kimi-k2p6",
	"together":               "moonshotai/Kimi-K2.6",
	"zai":                    "glm-5.1",
	"openai-codex":           "gpt-5.5",
	"amazon-bedrock":         "us.anthropic.claude-opus-4-6-v1",
	"azure-openai-responses": "gpt-5.4",
	"github-copilot":         "gpt-5.4",
	"vercel-ai-gateway":      "zai/glm-5.1",
	"cerebras":               "zai-glm-4.7",
	"huggingface":            "moonshotai/Kimi-K2.6",
	"opencode":               "kimi-k2.6",
	"kimi-coding":            "kimi-for-coding",
}

// ScopedModel pairs a model with an optional thinking level.
type ScopedModel struct {
	Model         *ai.Model
	ThinkingLevel string // empty means use default
}

// ParsedModelResult holds the outcome of parsing a model pattern.
type ParsedModelResult struct {
	Model         *ai.Model
	ThinkingLevel string
	Warning       string
}

// InitialModelResult holds the outcome of initial model selection.
type InitialModelResult struct {
	Model           *ai.Model
	ThinkingLevel   string
	FallbackMessage string
}

// FindExactModelReferenceMatch finds an exact match for a model reference string.
// Supports "provider/modelId" or bare "modelId" format.
func FindExactModelReferenceMatch(ref string, models []*ai.Model) *ai.Model {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		return nil
	}
	lower := strings.ToLower(trimmed)

	// Try canonical "provider/modelId" match
	for _, m := range models {
		if strings.ToLower(m.Provider+"/"+m.ID) == lower {
			return m
		}
	}

	// Try "provider/modelId" with separate fields
	if slashIdx := strings.Index(trimmed, "/"); slashIdx != -1 {
		provider := strings.TrimSpace(trimmed[:slashIdx])
		modelID := strings.TrimSpace(trimmed[slashIdx+1:])
		if provider != "" && modelID != "" {
			var matches []*ai.Model
			for _, m := range models {
				if strings.EqualFold(m.Provider, provider) && strings.EqualFold(m.ID, modelID) {
					matches = append(matches, m)
				}
			}
			if len(matches) == 1 {
				return matches[0]
			}
		}
	}

	// Try bare model ID match (must be unique)
	var matches []*ai.Model
	for _, m := range models {
		if strings.EqualFold(m.ID, lower) {
			matches = append(matches, m)
		}
	}
	if len(matches) == 1 {
		return matches[0]
	}
	return nil
}

// ParseModelPattern parses a pattern like "claude-4:high" into model + thinking level.
func ParseModelPattern(pattern string, models []*ai.Model) ParsedModelResult {
	// Try exact match first
	if exact := FindExactModelReferenceMatch(pattern, models); exact != nil {
		return ParsedModelResult{Model: exact}
	}

	// Split on last colon
	lastColon := strings.LastIndex(pattern, ":")
	if lastColon == -1 {
		return ParsedModelResult{}
	}

	prefix := pattern[:lastColon]
	suffix := pattern[lastColon+1:]

	if isValidThinkingLevel(suffix) {
		result := ParseModelPattern(prefix, models)
		if result.Model != nil && result.Warning == "" {
			result.ThinkingLevel = suffix
		}
		return result
	}

	// Invalid suffix — recurse with warning
	result := ParseModelPattern(prefix, models)
	if result.Model != nil {
		result.Warning = fmt.Sprintf("Invalid thinking level %q in pattern %q", suffix, pattern)
	}
	return result
}

// FindInitialModel selects the initial model based on priority:
//  1. Saved default from settings
//  2. Default model per provider
//  3. First available model
func FindInitialModel(registry *Registry, defaultProvider string, defaultModelID string, defaultThinkingLevel string) InitialModelResult {
	thinkingLevel := defaultThinkingLevel
	if thinkingLevel == "" {
		thinkingLevel = string(config.DefaultThinkingLevel)
	}

	// 1. Try saved default from settings
	if defaultProvider != "" && defaultModelID != "" {
		if found := registry.Find(defaultProvider, defaultModelID); found != nil {
			return InitialModelResult{Model: found, ThinkingLevel: thinkingLevel}
		}
	}

	// 2. Try default model per provider
	available := registry.GetAvailable()
	if len(available) > 0 {
		for provider, defaultID := range DefaultModelPerProvider {
			for _, m := range available {
				if m.Provider == provider && m.ID == defaultID {
					return InitialModelResult{Model: m, ThinkingLevel: string(config.DefaultThinkingLevel)}
				}
			}
		}
		// 3. First available
		return InitialModelResult{Model: available[0], ThinkingLevel: string(config.DefaultThinkingLevel)}
	}

	return InitialModelResult{ThinkingLevel: thinkingLevel}
}

// RestoreModelFromSession restores a model from session data with fallback.
func RestoreModelFromSession(
	savedProvider string,
	savedModelID string,
	currentModel *ai.Model,
	registry *Registry,
) (*ai.Model, string) {
	restored := registry.Find(savedProvider, savedModelID)
	if restored != nil && registry.HasConfiguredAuth(restored) {
		return restored, ""
	}

	reason := "model no longer exists"
	if restored != nil {
		reason = "no auth configured"
	}

	fallbackMsg := fmt.Sprintf("Could not restore model %s/%s (%s)", savedProvider, savedModelID, reason)

	if currentModel != nil {
		fallbackMsg += fmt.Sprintf(". Using %s/%s", currentModel.Provider, currentModel.ID)
		return currentModel, fallbackMsg
	}

	// Try any available model
	available := registry.GetAvailable()
	if len(available) > 0 {
		fallback := available[0]
		// Prefer known default models
		for provider, defaultID := range DefaultModelPerProvider {
			for _, m := range available {
				if m.Provider == provider && m.ID == defaultID {
					fallback = m
					break
				}
			}
		}
		fallbackMsg += fmt.Sprintf(". Using %s/%s", fallback.Provider, fallback.ID)
		return fallback, fallbackMsg
	}

	return nil, fallbackMsg
}

func isValidThinkingLevel(s string) bool {
	switch s {
	case "off", "minimal", "low", "medium", "high", "xhigh":
		return true
	}
	return false
}
