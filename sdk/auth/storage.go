// Package auth provides credential storage for API keys.
// Handles loading, saving, and resolving credentials from auth.json,
// runtime overrides, environment variables, and fallback resolvers.
package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/chinudotdev/pi-go/ai"
	"github.com/chinudotdev/pi-go/sdk/internal/configvalue"
	"github.com/chinudotdev/pi-go/sdk/internal/paths"
)

// ============================================================================
// Types
// ============================================================================

// CredentialType indicates how the credential is stored.
type CredentialType string

const (
	CredentialTypeAPIKey CredentialType = "api_key"
	CredentialTypeOAuth  CredentialType = "oauth"
)

// ApiKeyCredential holds an API key (plain or config-value reference).
type ApiKeyCredential struct {
	Type CredentialType `json:"type"`
	Key  string         `json:"key"`
}

// OAuthCredential holds an OAuth token set.
type OAuthCredential struct {
	Type     CredentialType `json:"type"`
	Token    string         `json:"accessToken"`
	Refresh  string         `json:"refreshToken,omitempty"`
	Expires  int64          `json:"expires"`
	Scopes   []string       `json:"scopes,omitempty"`
	ClientID string         `json:"clientId,omitempty"`
}

// Credential is a union of credential types.
type Credential interface {
	CredentialType() CredentialType
}

func (c *ApiKeyCredential) CredentialType() CredentialType { return CredentialTypeAPIKey }
func (c *OAuthCredential) CredentialType() CredentialType  { return CredentialTypeOAuth }

// StorageData is the JSON structure of auth.json.
type StorageData map[string]json.RawMessage

// AuthSource indicates where a credential was found.
type AuthSource string

const (
	SourceStored    AuthSource = "stored"
	SourceRuntime   AuthSource = "runtime"
	SourceEnv       AuthSource = "environment"
	SourceFallback  AuthSource = "fallback"
	SourceModelsKey AuthSource = "models_json_key"
	SourceModelsCmd AuthSource = "models_json_command"
)

// Status describes auth state for a provider.
type Status struct {
	Configured bool
	Source     AuthSource
	Label      string
}

// ============================================================================
// Backend interface
// ============================================================================

// LockResult is returned from the locked operation.
type LockResult struct {
	Result any
	Next   *string // if non-nil, write this to storage
}

// Backend provides locked read/write access to credential storage.
type Backend interface {
	// WithLock executes fn while holding an exclusive lock on storage.
	// fn receives the current raw content (or nil) and may return updated content.
	WithLock(fn func(current []byte) LockResult) (any, error)
}

// ============================================================================
// File backend
// ============================================================================

// FileBackend stores credentials in a JSON file with file locking.
type FileBackend struct {
	path   string
	mu     sync.Mutex // process-level lock (file-level lock via flock for multi-process)
	fileMu sync.Mutex // serialize file I/O
}

// NewFileBackend creates a file-backed auth storage at the given path.
func NewFileBackend(authPath string) *FileBackend {
	return &FileBackend{path: paths.NormalizePath(authPath)}
}

func (fb *FileBackend) ensureDir() error {
	dir := filepath.Dir(fb.path)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return os.MkdirAll(dir, 0o700)
	}
	return nil
}

func (fb *FileBackend) ensureFile() error {
	if _, err := os.Stat(fb.path); os.IsNotExist(err) {
		if err := fb.ensureDir(); err != nil {
			return err
		}
		return os.WriteFile(fb.path, []byte("{}"), 0o600)
	}
	return nil
}

func (fb *FileBackend) WithLock(fn func(current []byte) LockResult) (any, error) {
	fb.mu.Lock()
	defer fb.mu.Unlock()

	if err := fb.ensureFile(); err != nil {
		return nil, fmt.Errorf("auth storage init: %w", err)
	}

	content, err := os.ReadFile(fb.path)
	if err != nil {
		content = nil
	}

	result := fn(content)
	if result.Next != nil {
		if err := os.WriteFile(fb.path, []byte(*result.Next), 0o600); err != nil {
			return nil, fmt.Errorf("auth storage write: %w", err)
		}
	}

	return result.Result, nil
}

// ============================================================================
// In-memory backend
// ============================================================================

// MemoryBackend stores credentials in memory only (no file I/O).
type MemoryBackend struct {
	mu      sync.Mutex
	content []byte
}

// NewMemoryBackend creates an in-memory auth storage backend.
func NewMemoryBackend() *MemoryBackend {
	return &MemoryBackend{content: []byte("{}")}
}

func (mb *MemoryBackend) WithLock(fn func(current []byte) LockResult) (any, error) {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	result := fn(mb.content)
	if result.Next != nil {
		mb.content = []byte(*result.Next)
	}
	return result.Result, nil
}

// ============================================================================
// Storage (main API)
// ============================================================================

// Storage manages credentials with support for multiple resolution sources.
type Storage struct {
	backend          Backend
	data             StorageData
	runtimeOverrides map[string]string
	fallbackResolver func(provider string) *string
	loadError        error
	errors           []error
	mu               sync.RWMutex
}

// NewStorage creates a Storage using the given backend.
func NewStorage(backend Backend) *Storage {
	s := &Storage{
		backend:          backend,
		data:             make(StorageData),
		runtimeOverrides: make(map[string]string),
	}
	s.reload()
	return s
}

// InMemory creates an in-memory Storage with optional initial data.
func InMemory(data map[string]Credential) *Storage {
	raw := make(map[string]json.RawMessage)
	for k, v := range data {
		b, _ := json.Marshal(v)
		raw[k] = b
	}
	content, _ := json.MarshalIndent(raw, "", "  ")
	contentStr := string(content)
	backend := NewMemoryBackend()
	backend.WithLock(func(current []byte) LockResult {
		return LockResult{Next: &contentStr}
	})
	return NewStorage(backend)
}

// reload reads credentials from storage.
func (s *Storage) reload() {
	result, err := s.backend.WithLock(func(current []byte) LockResult {
		return LockResult{Result: current}
	})
	if err != nil {
		s.loadError = err
		s.errors = append(s.errors, err)
		return
	}
	s.loadError = nil

	if raw, ok := result.([]byte); ok && raw != nil {
		s.data = make(StorageData)
		if err := json.Unmarshal(raw, &s.data); err != nil {
			s.data = make(StorageData)
			s.errors = append(s.errors, err)
		}
	} else {
		s.data = make(StorageData)
	}
}

// SetRuntimeOverride sets a runtime API key override (not persisted to disk).
func (s *Storage) SetRuntimeOverride(provider string, apiKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runtimeOverrides[provider] = apiKey
}

// RemoveRuntimeOverride removes a runtime API key override.
func (s *Storage) RemoveRuntimeOverride(provider string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.runtimeOverrides, provider)
}

// SetFallbackResolver sets a resolver for providers not found in auth.json or env vars.
func (s *Storage) SetFallbackResolver(resolver func(provider string) *string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fallbackResolver = resolver
}

// Get returns the raw credential for a provider, or nil.
func (s *Storage) Get(provider string) Credential {
	s.mu.RLock()
	defer s.mu.RUnlock()

	raw, ok := s.data[provider]
	if !ok {
		return nil
	}

	// Peek at the type field
	var peek struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &peek); err != nil {
		return nil
	}

	switch CredentialType(peek.Type) {
	case CredentialTypeAPIKey:
		var cred ApiKeyCredential
		if err := json.Unmarshal(raw, &cred); err != nil {
			return nil
		}
		return &cred
	case CredentialTypeOAuth:
		var cred OAuthCredential
		if err := json.Unmarshal(raw, &cred); err != nil {
			return nil
		}
		return &cred
	}
	return nil
}

// Set stores a credential for a provider.
func (s *Storage) Set(provider string, cred Credential) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := json.Marshal(cred)
	if err != nil {
		return err
	}
	s.data[provider] = raw

	if s.loadError != nil {
		return nil
	}

	_, err = s.backend.WithLock(func(current []byte) LockResult {
		currentData := make(StorageData)
		if current != nil {
			json.Unmarshal(current, &currentData)
		}
		currentData[provider] = raw

		next, _ := json.MarshalIndent(currentData, "", "  ")
		nextStr := string(next)
		return LockResult{Next: &nextStr}
	})
	if err != nil {
		s.errors = append(s.errors, err)
	}
	return err
}

// Remove deletes a credential for a provider.
func (s *Storage) Remove(provider string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.data, provider)

	if s.loadError != nil {
		return nil
	}

	_, err := s.backend.WithLock(func(current []byte) LockResult {
		currentData := make(StorageData)
		if current != nil {
			json.Unmarshal(current, &currentData)
		}
		delete(currentData, provider)

		next, _ := json.MarshalIndent(currentData, "", "  ")
		nextStr := string(next)
		return LockResult{Next: &nextStr}
	})
	if err != nil {
		s.errors = append(s.errors, err)
	}
	return err
}

// List returns all provider names with credentials.
func (s *Storage) List() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	names := make([]string, 0, len(s.data))
	for k := range s.data {
		names = append(names, k)
	}
	return names
}

// Has returns true if credentials exist for a provider in auth.json.
func (s *Storage) Has(provider string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.data[provider]
	return ok
}

// HasAuth returns true if any form of auth is configured for a provider.
func (s *Storage) HasAuth(provider string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, ok := s.runtimeOverrides[provider]; ok {
		return true
	}
	if _, ok := s.data[provider]; ok {
		return true
	}
	if _, ok := ai.GetEnvApiKey(provider); ok {
		return true
	}
	if s.fallbackResolver != nil {
		if val := s.fallbackResolver(provider); val != nil {
			return true
		}
	}
	return false
}

// GetStatus returns the auth status for a provider without exposing credentials.
func (s *Storage) GetStatus(provider string) Status {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, ok := s.data[provider]; ok {
		return Status{Configured: true, Source: SourceStored}
	}
	if _, ok := s.runtimeOverrides[provider]; ok {
		return Status{Configured: false, Source: SourceRuntime, Label: "--api-key"}
	}
	if envKeys := ai.FindEnvKeys(provider); len(envKeys) > 0 {
		return Status{Configured: false, Source: SourceEnv, Label: envKeys[0]}
	}
	if s.fallbackResolver != nil {
		if val := s.fallbackResolver(provider); val != nil {
			return Status{Configured: false, Source: SourceFallback, Label: "custom provider config"}
		}
	}
	return Status{Configured: false}
}

// GetAll returns a copy of all credential data.
func (s *Storage) GetAll() StorageData {
	s.mu.RLock()
	defer s.mu.RUnlock()

	clone := make(StorageData, len(s.data))
	for k, v := range s.data {
		clone[k] = v
	}
	return clone
}

// DrainErrors returns and clears accumulated errors.
func (s *Storage) DrainErrors() []error {
	s.mu.Lock()
	defer s.mu.Unlock()
	errs := s.errors
	s.errors = nil
	return errs
}

// GetAPIKey resolves the API key for a provider.
//
// Priority:
//  1. Runtime override
//  2. API key from auth.json (resolved via config value)
//  3. OAuth token from auth.json (TODO: OAuth refresh)
//  4. Environment variable
//  5. Fallback resolver
func (s *Storage) GetAPIKey(provider string, includeFallback ...bool) *string {
	useFallback := true
	if len(includeFallback) > 0 {
		useFallback = includeFallback[0]
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// 1. Runtime override
	if key, ok := s.runtimeOverrides[provider]; ok {
		return &key
	}

	// 2. Stored credential
	raw, ok := s.data[provider]
	if ok {
		var peek struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(raw, &peek) == nil {
			switch CredentialType(peek.Type) {
			case CredentialTypeAPIKey:
				var cred ApiKeyCredential
				if json.Unmarshal(raw, &cred) == nil {
					return configvalue.ResolveConfigValue(cred.Key)
				}
			case CredentialTypeOAuth:
				// TODO: OAuth token refresh
				var cred OAuthCredential
				if json.Unmarshal(raw, &cred) == nil {
					if cred.Token != "" {
						return &cred.Token
					}
				}
			}
		}
	}

	// 3. Environment variable
	if key, ok := ai.GetEnvApiKey(provider); ok {
		return &key
	}

	// 4. Fallback resolver
	if useFallback && s.fallbackResolver != nil {
		return s.fallbackResolver(provider)
	}

	return nil
}
