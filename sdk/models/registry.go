// Package models provides model discovery, registration, and API key resolution.
// It loads built-in models from the ai package and custom models from models.json.
package models

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/chinudotdev/pi-go/ai"
	"github.com/chinudotdev/pi-go/sdk/auth"
	"github.com/chinudotdev/pi-go/sdk/internal/configvalue"
	"github.com/chinudotdev/pi-go/sdk/internal/jsonutil"
	"github.com/chinudotdev/pi-go/sdk/internal/paths"
)

// ============================================================================
// Types
// ============================================================================

// ResolvedAuth holds the result of API key + headers resolution.
type ResolvedAuth struct {
	OK      bool
	APIKey  string
	Headers map[string]string
	Error   string
}

// AuthOK returns a successful auth result.
func AuthOK(apiKey string, headers map[string]string) ResolvedAuth {
	return ResolvedAuth{OK: true, APIKey: apiKey, Headers: headers}
}

// AuthError returns a failed auth result.
func AuthError(err string) ResolvedAuth {
	return ResolvedAuth{OK: false, Error: err}
}

// ProviderOverride holds provider-level baseUrl/compat overrides from models.json.
type providerOverride struct {
	BaseURL string
	Compat  any
}

// ProviderRequestConfig holds auth config from models.json.
type providerRequestConfig struct {
	APIKey     string
	Headers    map[string]string
	AuthHeader bool
}

// modelsJSONConfig is the top-level models.json schema.
type modelsJSONConfig struct {
	Providers map[string]providerConfig `json:"providers"`
}

type providerConfig struct {
	Name           string                   `json:"name,omitempty"`
	BaseURL        string                   `json:"baseUrl,omitempty"`
	APIKey         string                   `json:"apiKey,omitempty"`
	API            string                   `json:"api,omitempty"`
	Headers        map[string]string        `json:"headers,omitempty"`
	Compat         any                      `json:"compat,omitempty"`
	AuthHeader     bool                     `json:"authHeader,omitempty"`
	Models         []modelDefinition        `json:"models,omitempty"`
	ModelOverrides map[string]modelOverride `json:"modelOverrides,omitempty"`
}

type modelDefinition struct {
	ID               string            `json:"id"`
	Name             string            `json:"name,omitempty"`
	API              string            `json:"api,omitempty"`
	BaseURL          string            `json:"baseUrl,omitempty"`
	Reasoning        bool              `json:"reasoning,omitempty"`
	ThinkingLevelMap map[string]any    `json:"thinkingLevelMap,omitempty"`
	Input            []string          `json:"input,omitempty"`
	Cost             *modelCost        `json:"cost,omitempty"`
	ContextWindow    int               `json:"contextWindow,omitempty"`
	MaxTokens        int               `json:"maxTokens,omitempty"`
	Headers          map[string]string `json:"headers,omitempty"`
	Compat           any               `json:"compat,omitempty"`
}

type modelOverride struct {
	Name             string            `json:"name,omitempty"`
	Reasoning        *bool             `json:"reasoning,omitempty"`
	ThinkingLevelMap map[string]any    `json:"thinkingLevelMap,omitempty"`
	Input            []string          `json:"input,omitempty"`
	Cost             *modelCost        `json:"cost,omitempty"`
	ContextWindow    *int              `json:"contextWindow,omitempty"`
	MaxTokens        *int              `json:"maxTokens,omitempty"`
	Headers          map[string]string `json:"headers,omitempty"`
	Compat           any               `json:"compat,omitempty"`
}

type modelCost struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite"`
}

// ============================================================================
// Display names
// ============================================================================

// BuiltinProviderDisplayNames maps provider IDs to human-readable names.
var BuiltinProviderDisplayNames = map[string]string{
	"anthropic":              "Anthropic",
	"amazon-bedrock":         "Amazon Bedrock",
	"azure-openai-responses": "Azure OpenAI Responses",
	"cerebras":               "Cerebras",
	"deepseek":               "DeepSeek",
	"fireworks":              "Fireworks",
	"google":                 "Google Gemini",
	"google-vertex":          "Google Vertex AI",
	"groq":                   "Groq",
	"huggingface":            "Hugging Face",
	"mistral":                "Mistral",
	"minimax":                "MiniMax",
	"moonshotai":             "Moonshot AI",
	"openai":                 "OpenAI",
	"openrouter":             "OpenRouter",
	"together":               "Together AI",
	"xai":                    "xAI",
	"zai":                    "ZAI",
}

// ============================================================================
// Registry
// ============================================================================

// Registry manages model discovery and API key resolution.
type Registry struct {
	mu                  sync.RWMutex
	models              []*ai.Model
	authStorage         *auth.Storage
	providerConfigs     map[string]*providerRequestConfig
	modelHeaders        map[string]map[string]string // "provider:modelId" -> headers
	registeredProviders map[string]*providerConfig
	loadError           string
	modelsJSONPath      string
}

// NewRegistry creates a new model registry.
func NewRegistry(authStorage *auth.Storage, modelsJSONPath ...string) *Registry {
	path := ""
	if len(modelsJSONPath) > 0 && modelsJSONPath[0] != "" {
		path = paths.NormalizePath(modelsJSONPath[0])
	}

	r := &Registry{
		authStorage:         authStorage,
		providerConfigs:     make(map[string]*providerRequestConfig),
		modelHeaders:        make(map[string]map[string]string),
		registeredProviders: make(map[string]*providerConfig),
		modelsJSONPath:      path,
	}
	r.loadModels()
	return r
}

// InMemory creates a registry with no models.json (built-in only).
func InMemory(authStorage *auth.Storage) *Registry {
	return NewRegistry(authStorage, "")
}

// Refresh reloads models from disk and re-applies dynamic provider configs.
func (r *Registry) Refresh() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.providerConfigs = make(map[string]*providerRequestConfig)
	r.modelHeaders = make(map[string]map[string]string)
	r.loadError = ""

	r.loadModels()

	// Re-apply dynamic providers
	for name, config := range r.registeredProviders {
		r.applyProviderConfig(name, config)
	}
}

// GetError returns any error from loading models.json.
func (r *Registry) GetError() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.loadError
}

// loadModels loads built-in + custom models.
func (r *Registry) loadModels() {
	customModels, overrides, modelOverrides, loadErr := r.loadCustomModels()
	if loadErr != "" {
		r.loadError = loadErr
	}

	builtIn := r.loadBuiltInModels(overrides, modelOverrides)
	combined := r.mergeModels(builtIn, customModels)
	r.models = combined
}

func (r *Registry) loadBuiltInModels(
	overrides map[string]*providerOverride,
	modelOverrides map[string]map[string]*modelOverride,
) []*ai.Model {
	var models []*ai.Model

	for _, provider := range ai.GetProviders() {
		for _, m := range ai.GetModels(provider) {
			model := *m // copy

			// Apply provider-level overrides
			if ov, ok := overrides[provider]; ok {
				if ov.BaseURL != "" {
					model.BaseURL = ov.BaseURL
				}
			}

			// Apply per-model overrides
			if perProvider, ok := modelOverrides[provider]; ok {
				if ov, ok := perProvider[model.ID]; ok {
					model = applyOverride(model, ov)
				}
			}

			models = append(models, &model)
		}
	}

	return models
}

func applyOverride(m ai.Model, ov *modelOverride) ai.Model {
	result := m
	if ov.Name != "" {
		result.Name = ov.Name
	}
	if ov.Reasoning != nil {
		result.Reasoning = *ov.Reasoning
	}
	if ov.ContextWindow != nil {
		result.ContextWindow = *ov.ContextWindow
	}
	if ov.MaxTokens != nil {
		result.MaxTokens = *ov.MaxTokens
	}
	return result
}

func (r *Registry) mergeModels(builtIn, custom []*ai.Model) []*ai.Model {
	merged := make([]*ai.Model, len(builtIn))
	copy(merged, builtIn)

	for _, customModel := range custom {
		found := false
		for i, existing := range merged {
			if existing.Provider == customModel.Provider && existing.ID == customModel.ID {
				merged[i] = customModel
				found = true
				break
			}
		}
		if !found {
			merged = append(merged, customModel)
		}
	}
	return merged
}

func (r *Registry) loadCustomModels() (
	models []*ai.Model,
	overrides map[string]*providerOverride,
	modelOverrides map[string]map[string]*modelOverride,
	errMsg string,
) {
	overrides = make(map[string]*providerOverride)
	modelOverrides = make(map[string]map[string]*modelOverride)

	if r.modelsJSONPath == "" {
		return nil, overrides, modelOverrides, ""
	}

	data, err := os.ReadFile(r.modelsJSONPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, overrides, modelOverrides, ""
		}
		return nil, overrides, modelOverrides, fmt.Sprintf("Failed to read models.json: %v", err)
	}

	var config modelsJSONConfig
	if err := json.Unmarshal([]byte(jsonutil.StripComments(string(data))), &config); err != nil {
		return nil, overrides, modelOverrides, fmt.Sprintf("Failed to parse models.json: %v", err)
	}

	builtInProviders := make(map[string]bool)
	for _, p := range ai.GetProviders() {
		builtInProviders[p] = true
	}

	for providerName, providerCfg := range config.Providers {
		// Store provider-level override
		if providerCfg.BaseURL != "" || providerCfg.Compat != nil {
			overrides[providerName] = &providerOverride{
				BaseURL: providerCfg.BaseURL,
				Compat:  providerCfg.Compat,
			}
		}

		// Store provider request config (apiKey, headers, etc.)
		if providerCfg.APIKey != "" || len(providerCfg.Headers) > 0 || providerCfg.AuthHeader {
			r.providerConfigs[providerName] = &providerRequestConfig{
				APIKey:     providerCfg.APIKey,
				Headers:    providerCfg.Headers,
				AuthHeader: providerCfg.AuthHeader,
			}
		}

		// Store model overrides
		if len(providerCfg.ModelOverrides) > 0 {
			perProvider := make(map[string]*modelOverride)
			for modelID, ov := range providerCfg.ModelOverrides {
				ovCopy := ov
				perProvider[modelID] = &ovCopy
			}
			modelOverrides[providerName] = perProvider
		}

		// Parse custom models
		for _, modelDef := range providerCfg.Models {
			api := modelDef.API
			if api == "" {
				api = providerCfg.API
			}

			// Try to inherit from built-in
			if api == "" && builtInProviders[providerName] {
				if builtInModels := ai.GetModels(providerName); len(builtInModels) > 0 {
					api = builtInModels[0].API
				}
			}
			if api == "" {
				continue
			}

			baseURL := modelDef.BaseURL
			if baseURL == "" {
				baseURL = providerCfg.BaseURL
			}
			if baseURL == "" && builtInProviders[providerName] {
				if builtInModels := ai.GetModels(providerName); len(builtInModels) > 0 {
					baseURL = builtInModels[0].BaseURL
				}
			}
			if baseURL == "" {
				continue
			}

			input := modelDef.Input
			if len(input) == 0 {
				input = []string{"text"}
			}

			cost := ai.ModelCost{}
			if modelDef.Cost != nil {
				cost = ai.ModelCost{
					Input:      modelDef.Cost.Input,
					Output:     modelDef.Cost.Output,
					CacheRead:  modelDef.Cost.CacheRead,
					CacheWrite: modelDef.Cost.CacheWrite,
				}
			}

			contextWindow := modelDef.ContextWindow
			if contextWindow == 0 {
				contextWindow = 128000
			}
			maxTokens := modelDef.MaxTokens
			if maxTokens == 0 {
				maxTokens = 16384
			}

			// Store model-level headers
			if len(modelDef.Headers) > 0 {
				key := providerName + ":" + modelDef.ID
				r.modelHeaders[key] = modelDef.Headers
			}

			models = append(models, &ai.Model{
				ID:            modelDef.ID,
				Name:          modelDef.Name,
				API:           ai.Api(api),
				Provider:      providerName,
				BaseURL:       baseURL,
				Reasoning:     modelDef.Reasoning,
				Input:         input,
				Cost:          cost,
				ContextWindow: contextWindow,
				MaxTokens:     maxTokens,
				Compat:        modelDef.Compat,
			})
		}
	}

	return models, overrides, modelOverrides, ""
}

// applyProviderConfig applies a dynamically registered provider config.
func (r *Registry) applyProviderConfig(providerName string, config *providerConfig) {
	// Store request config
	if config.APIKey != "" || len(config.Headers) > 0 || config.AuthHeader {
		r.providerConfigs[providerName] = &providerRequestConfig{
			APIKey:     config.APIKey,
			Headers:    config.Headers,
			AuthHeader: config.AuthHeader,
		}
	}

	if len(config.Models) > 0 {
		// Remove existing models for this provider
		var filtered []*ai.Model
		for _, m := range r.models {
			if m.Provider != providerName {
				filtered = append(filtered, m)
			}
		}

		for _, modelDef := range config.Models {
			api := modelDef.API
			if api == "" {
				api = config.API
			}

			input := modelDef.Input
			if len(input) == 0 {
				input = []string{"text"}
			}

			contextWindow := modelDef.ContextWindow
			if contextWindow == 0 {
				contextWindow = 128000
			}
			maxTokens := modelDef.MaxTokens
			if maxTokens == 0 {
				maxTokens = 16384
			}

			filtered = append(filtered, &ai.Model{
				ID:            modelDef.ID,
				Name:          modelDef.Name,
				API:           ai.Api(api),
				Provider:      providerName,
				BaseURL:       modelDef.BaseURL,
				Reasoning:     modelDef.Reasoning,
				Input:         input,
				ContextWindow: contextWindow,
				MaxTokens:     maxTokens,
				Compat:        modelDef.Compat,
			})
			// Inherit provider baseUrl if model doesn't have one
			last := filtered[len(filtered)-1]
			if last.BaseURL == "" {
				last.BaseURL = config.BaseURL
			}
		}
		r.models = filtered
	} else if config.BaseURL != "" {
		// Override-only: update baseUrl for existing models
		for _, m := range r.models {
			if m.Provider == providerName {
				m.BaseURL = config.BaseURL
			}
		}
	}
}

// RegisterProvider dynamically registers a provider.
func (r *Registry) RegisterProvider(providerName string, config providerConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.applyProviderConfig(providerName, &config)
	r.registeredProviders[providerName] = &config
}

// UnregisterProvider removes a dynamically registered provider and reloads.
func (r *Registry) UnregisterProvider(providerName string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.registeredProviders[providerName]; !ok {
		return
	}
	delete(r.registeredProviders, providerName)
	r.providerConfigs = make(map[string]*providerRequestConfig)
	r.modelHeaders = make(map[string]map[string]string)
	r.loadError = ""
	r.loadModels()

	for name, config := range r.registeredProviders {
		r.applyProviderConfig(name, config)
	}
}

// ============================================================================
// Query methods
// ============================================================================

// GetAll returns all models (built-in + custom).
func (r *Registry) GetAll() []*ai.Model {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.models
}

// GetAvailable returns models that have auth configured.
func (r *Registry) GetAvailable() []*ai.Model {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var available []*ai.Model
	for _, m := range r.models {
		if r.hasConfiguredAuth(m) {
			available = append(available, m)
		}
	}
	return available
}

// Find finds a model by provider and ID.
func (r *Registry) Find(provider string, modelID string) *ai.Model {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, m := range r.models {
		if m.Provider == provider && m.ID == modelID {
			return m
		}
	}
	return nil
}

// HasConfiguredAuth checks if auth is configured for a model's provider.
func (r *Registry) HasConfiguredAuth(model *ai.Model) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.hasConfiguredAuth(model)
}

func (r *Registry) hasConfiguredAuth(model *ai.Model) bool {
	if r.authStorage.HasAuth(model.Provider) {
		return true
	}
	pc, ok := r.providerConfigs[model.Provider]
	if ok && pc.APIKey != "" {
		return configvalue.IsConfigValueConfigured(pc.APIKey)
	}
	return false
}

// GetAPIKeyAndHeaders resolves API key and headers for a model.
func (r *Registry) GetAPIKeyAndHeaders(model *ai.Model) ResolvedAuth {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var apiKey string
	var headers map[string]string

	// 1. Try auth storage first
	if key := r.authStorage.GetAPIKey(model.Provider, false); key != nil {
		apiKey = *key
	}

	// 2. Try provider config from models.json
	if apiKey == "" {
		if pc, ok := r.providerConfigs[model.Provider]; ok && pc.APIKey != "" {
			val := configvalue.ResolveConfigValueOrThrow(pc.APIKey, fmt.Sprintf("API key for provider %q", model.Provider))
			apiKey = val
		}
	}

	// 3. Merge headers: model < provider config < model config
	pc, hasPC := r.providerConfigs[model.Provider]
	if hasPC && len(pc.Headers) > 0 {
		headers = configvalue.ResolveHeadersOrThrow(pc.Headers, fmt.Sprintf("provider %q", model.Provider))
	}

	modelKey := model.Provider + ":" + model.ID
	if modelHeaders, ok := r.modelHeaders[modelKey]; ok {
		resolved := configvalue.ResolveHeadersOrThrow(modelHeaders, fmt.Sprintf("model %q/%q", model.Provider, model.ID))
		if headers == nil {
			headers = resolved
		} else {
			for k, v := range resolved {
				headers[k] = v
			}
		}
	}

	// 4. AuthHeader mode: put API key in Authorization header
	if hasPC && pc.AuthHeader {
		if apiKey == "" {
			return AuthError(fmt.Sprintf("No API key found for %q", model.Provider))
		}
		if headers == nil {
			headers = make(map[string]string)
		}
		headers["Authorization"] = "Bearer " + apiKey
	}

	return AuthOK(apiKey, headers)
}

// GetProviderAuthStatus returns auth status for a provider.
func (r *Registry) GetProviderAuthStatus(provider string) auth.Status {
	status := r.authStorage.GetStatus(provider)
	if status.Source != "" {
		return status
	}

	r.mu.RLock()
	pc, ok := r.providerConfigs[provider]
	r.mu.RUnlock()

	if !ok || pc.APIKey == "" {
		return status
	}

	if configvalue.IsCommandConfigValue(pc.APIKey) {
		return auth.Status{Configured: true, Source: auth.SourceModelsCmd}
	}

	envVars := configvalue.GetConfigValueEnvVarNames(pc.APIKey)
	if len(envVars) > 0 {
		if configvalue.IsConfigValueConfigured(pc.APIKey) {
			return auth.Status{Configured: true, Source: auth.SourceEnv, Label: envVars[0]}
		}
		return auth.Status{Configured: false}
	}

	return auth.Status{Configured: true, Source: auth.SourceModelsKey}
}

// GetProviderDisplayName returns a human-readable provider name.
func (r *Registry) GetProviderDisplayName(provider string) string {
	if name, ok := BuiltinProviderDisplayNames[provider]; ok {
		return name
	}
	if pc, ok := r.registeredProviders[provider]; ok && pc.Name != "" {
		return pc.Name
	}
	return provider
}

// GetAPIKeyForProvider resolves the API key for a provider.
func (r *Registry) GetAPIKeyForProvider(provider string) *string {
	if key := r.authStorage.GetAPIKey(provider, false); key != nil {
		return key
	}

	r.mu.RLock()
	pc, ok := r.providerConfigs[provider]
	r.mu.RUnlock()

	if ok && pc.APIKey != "" {
		return configvalue.ResolveConfigValueUncached(pc.APIKey)
	}
	return nil
}
