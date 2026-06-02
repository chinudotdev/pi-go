package ai

import (
	"embed"
	"encoding/json"
	"sort"
	"sync"
)

//go:embed models.generated.json
var modelsFS embed.FS

// modelRegistry holds all known models organized by provider.
var modelRegistry struct {
	mu         sync.RWMutex
	byProvider map[string]map[string]*Model // provider -> modelID -> Model
}

func init() {
	modelRegistry.byProvider = make(map[string]map[string]*Model)
	loadModels()
}

// ModelsJSON represents the top-level structure of the generated models file.
type ModelsJSON map[string]map[string]Model // provider -> modelID -> Model

func loadModels() {
	data, err := modelsFS.ReadFile("models.generated.json")
	if err != nil {
		return
	}

	var models ModelsJSON
	if err := json.Unmarshal(data, &models); err != nil {
		return
	}

	modelRegistry.mu.Lock()
	defer modelRegistry.mu.Unlock()

	for provider, providerModels := range models {
		modelRegistry.byProvider[provider] = make(map[string]*Model)
		for id, m := range providerModels {
			model := m
			model.ID = id
			modelRegistry.byProvider[provider][id] = &model
		}
	}
}

// GetModel retrieves a model by provider and model ID.
func GetModel(provider Provider, modelID string) (*Model, bool) {
	modelRegistry.mu.RLock()
	defer modelRegistry.mu.RUnlock()

	providerModels, ok := modelRegistry.byProvider[provider]
	if !ok {
		return nil, false
	}
	m, ok := providerModels[modelID]
	return m, ok
}

// GetProviders returns all providers with registered models, sorted alphabetically.
func GetProviders() []string {
	modelRegistry.mu.RLock()
	defer modelRegistry.mu.RUnlock()

	providers := make([]string, 0, len(modelRegistry.byProvider))
	for p := range modelRegistry.byProvider {
		providers = append(providers, p)
	}
	sort.Strings(providers)
	return providers
}

// GetModels returns all models for a given provider.
func GetModels(provider Provider) []*Model {
	modelRegistry.mu.RLock()
	defer modelRegistry.mu.RUnlock()

	providerModels, ok := modelRegistry.byProvider[provider]
	if !ok {
		return nil
	}
	models := make([]*Model, 0, len(providerModels))
	for _, m := range providerModels {
		models = append(models, m)
	}
	return models
}

// CalculateCost computes and returns the cost for a given model and usage.
// The returned Cost is a new value — the input Usage is not mutated.
func CalculateCost(model *Model, usage Usage) Cost {
	cost := Cost{
		Input:      (model.Cost.Input / 1_000_000) * float64(usage.Input),
		Output:     (model.Cost.Output / 1_000_000) * float64(usage.Output),
		CacheRead:  (model.Cost.CacheRead / 1_000_000) * float64(usage.CacheRead),
		CacheWrite: (model.Cost.CacheWrite / 1_000_000) * float64(usage.CacheWrite),
	}
	cost.Total = cost.Input + cost.Output + cost.CacheRead + cost.CacheWrite
	return cost
}

// ApplyCost mutates the given Usage by computing and setting its Cost field.
// Returns the computed cost for convenience.
func ApplyCost(model *Model, usage *Usage) Cost {
	usage.Cost = CalculateCost(model, *usage)
	return usage.Cost
}

// Thinking level support

var extendedThinkingLevels = []ModelThinkingLevel{
	ThinkingOff, ThinkingMMin, ThinkingMLow, ThinkingMMedium, ThinkingMHigh, ThinkingMXHigh,
}

// GetSupportedThinkingLevels returns the thinking levels supported by a model.
func GetSupportedThinkingLevels(model *Model) []ModelThinkingLevel {
	if !model.Reasoning {
		return []ModelThinkingLevel{ThinkingOff}
	}

	var levels []ModelThinkingLevel
	for _, level := range extendedThinkingLevels {
		mapped, exists := model.ThinkingLevelMap[level]
		if exists && mapped == nil {
			// nil = null = unsupported
			continue
		}
		if exists && mapped != nil && *mapped == "" {
			// empty string = explicitly unsupported
			continue
		}
		if level == ThinkingMXHigh {
			// xhigh requires explicit mapping
			if !exists || mapped == nil {
				continue
			}
		}
		levels = append(levels, level)
	}

	if len(levels) == 0 {
		return []ModelThinkingLevel{ThinkingOff}
	}
	return levels
}

// ClampThinkingLevel finds the nearest supported thinking level.
func ClampThinkingLevel(model *Model, level ModelThinkingLevel) ModelThinkingLevel {
	available := GetSupportedThinkingLevels(model)
	for _, a := range available {
		if a == level {
			return level
		}
	}

	requestedIdx := -1
	for i, l := range extendedThinkingLevels {
		if l == level {
			requestedIdx = i
			break
		}
	}
	if requestedIdx == -1 {
		if len(available) > 0 {
			return available[0]
		}
		return ThinkingOff
	}

	// Try from requested level upward
	for i := requestedIdx; i < len(extendedThinkingLevels); i++ {
		for _, a := range available {
			if a == extendedThinkingLevels[i] {
				return a
			}
		}
	}
	// Try downward
	for i := requestedIdx - 1; i >= 0; i-- {
		for _, a := range available {
			if a == extendedThinkingLevels[i] {
				return a
			}
		}
	}

	if len(available) > 0 {
		return available[0]
	}
	return ThinkingOff
}

// ModelsAreEqual checks if two models are the same by id and provider.
func ModelsAreEqual(a, b *Model) bool {
	if a == nil || b == nil {
		return false
	}
	return a.ID == b.ID && a.Provider == b.Provider
}

// RegisterModel adds a custom model to the registry.
func RegisterModel(model *Model) {
	modelRegistry.mu.Lock()
	defer modelRegistry.mu.Unlock()

	if modelRegistry.byProvider[model.Provider] == nil {
		modelRegistry.byProvider[model.Provider] = make(map[string]*Model)
	}
	modelRegistry.byProvider[model.Provider][model.ID] = model
}
