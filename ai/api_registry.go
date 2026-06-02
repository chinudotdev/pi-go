package ai

import (
	"context"
	"fmt"
	"sync"
)

// StreamFunction is the interface that all provider implementations must satisfy.
// It takes a model, context, and options, and returns an EventStream.
type StreamFunction func(ctx context.Context, model *Model, convCtx *Context, options *StreamOptions) (*EventStream, error)

// ApiProvider wraps a pair of stream functions (raw + simple) for a given API.
type ApiProvider struct {
	API          Api
	Stream       StreamFunction
	StreamSimple StreamFunction
}

// apiProviderEntry tracks a registered API provider with optional source ID.
type apiProviderEntry struct {
	Provider ApiProvider
	SourceID string
}

// apiRegistry manages registered API providers.
type apiRegistry struct {
	mu       sync.RWMutex
	providers map[Api]apiProviderEntry
}

var globalRegistry = &apiRegistry{
	providers: make(map[Api]apiProviderEntry),
}

// RegisterApiProvider registers a new API provider.
func RegisterApiProvider(provider ApiProvider, sourceID ...string) {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()

	sid := ""
	if len(sourceID) > 0 {
		sid = sourceID[0]
	}
	globalRegistry.providers[provider.API] = apiProviderEntry{
		Provider: provider,
		SourceID: sid,
	}
}

// GetApiProvider retrieves a registered API provider by API name.
func GetApiProvider(api Api) (*ApiProvider, bool) {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()

	entry, ok := globalRegistry.providers[api]
	if !ok {
		return nil, false
	}
	return &entry.Provider, true
}

// GetApiProviders returns all registered API providers.
func GetApiProviders() []ApiProvider {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()

	result := make([]ApiProvider, 0, len(globalRegistry.providers))
	for _, entry := range globalRegistry.providers {
		result = append(result, entry.Provider)
	}
	return result
}

// UnregisterApiProviders removes all providers registered with the given sourceID.
func UnregisterApiProviders(sourceID string) {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()

	for api, entry := range globalRegistry.providers {
		if entry.SourceID == sourceID {
			delete(globalRegistry.providers, api)
		}
	}
}

// ClearApiProviders removes all registered providers.
func ClearApiProviders() {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()
	globalRegistry.providers = make(map[Api]apiProviderEntry)
}

// ResolveApiProvider finds the provider for the given API or returns an error.
func ResolveApiProvider(api Api) (*ApiProvider, error) {
	provider, ok := GetApiProvider(api)
	if !ok {
		return nil, fmt.Errorf("no API provider registered for api: %s", api)
	}
	return provider, nil
}
