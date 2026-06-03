package auth

import (
	"testing"
)

func TestInMemoryStorage_Lifecycle(t *testing.T) {
	s := InMemory(nil)

	// No credentials initially
	if s.Has("test") {
		t.Error("expected no credentials initially")
	}
	if s.HasAuth("test") {
		t.Error("expected HasAuth=false initially")
	}

	// Set API key credential
	err := s.Set("test", &ApiKeyCredential{
		Type: CredentialTypeAPIKey,
		Key:  "secret-key",
	})
	if err != nil {
		t.Fatal(err)
	}

	if !s.Has("test") {
		t.Error("expected Has=true after Set")
	}
	if !s.HasAuth("test") {
		t.Error("expected HasAuth=true after Set")
	}

	// Get credential
	cred := s.Get("test")
	if cred == nil {
		t.Fatal("expected credential, got nil")
	}
	apiKey, ok := cred.(*ApiKeyCredential)
	if !ok {
		t.Fatal("expected ApiKeyCredential")
	}
	if apiKey.Key != "secret-key" {
		t.Errorf("expected secret-key, got %s", apiKey.Key)
	}

	// List
	list := s.List()
	if len(list) != 1 || list[0] != "test" {
		t.Errorf("expected [test], got %v", list)
	}

	// Remove
	err = s.Remove("test")
	if err != nil {
		t.Fatal(err)
	}
	if s.Has("test") {
		t.Error("expected Has=false after Remove")
	}
}

func TestInMemoryStorage_RuntimeOverride(t *testing.T) {
	s := InMemory(nil)

	s.SetRuntimeOverride("test", "runtime-key")
	defer s.RemoveRuntimeOverride("test")

	// GetAPIKey returns runtime override
	key := s.GetAPIKey("test")
	if key == nil || *key != "runtime-key" {
		t.Errorf("expected runtime-key, got %v", key)
	}

	// HasAuth returns true
	if !s.HasAuth("test") {
		t.Error("expected HasAuth=true for runtime override")
	}

	// Status shows runtime source
	status := s.GetStatus("test")
	if status.Source != SourceRuntime {
		t.Errorf("expected runtime source, got %s", status.Source)
	}
}

func TestInMemoryStorage_APIKeyResolution(t *testing.T) {
	s := InMemory(nil)

	err := s.Set("test", &ApiKeyCredential{
		Type: CredentialTypeAPIKey,
		Key:  "literal-key",
	})
	if err != nil {
		t.Fatal(err)
	}

	key := s.GetAPIKey("test")
	if key == nil || *key != "literal-key" {
		t.Errorf("expected literal-key, got %v", key)
	}
}

func TestInMemoryStorage_EnvFallback(t *testing.T) {
	s := InMemory(nil)

	// No stored credentials — should fall back to env
	// (We can't easily test env vars in unit tests without modifying process env)
	// Just verify that GetAPIKey returns nil for unknown providers
	key := s.GetAPIKey("nonexistent-provider-xyz")
	if key != nil {
		t.Errorf("expected nil for nonexistent provider, got %v", key)
	}
}

func TestInMemoryStorage_StatusStored(t *testing.T) {
	s := InMemory(nil)

	s.Set("test", &ApiKeyCredential{
		Type: CredentialTypeAPIKey,
		Key:  "key",
	})

	status := s.GetStatus("test")
	if !status.Configured {
		t.Error("expected Configured=true for stored credential")
	}
	if status.Source != SourceStored {
		t.Errorf("expected stored source, got %s", status.Source)
	}
}

func TestInMemoryStorage_DrainErrors(t *testing.T) {
	s := InMemory(nil)
	errs := s.DrainErrors()
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestFileBackend_EnsureFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/auth.json"
	backend := NewFileBackend(path)

	// Should create file
	result, err := backend.WithLock(func(current []byte) LockResult {
		return LockResult{Result: current}
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestFileBackend_ReadWrite(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/auth.json"
	backend := NewFileBackend(path)

	// Write
	content := `{"test":{"type":"api_key","key":"secret"}}`
	_, err := backend.WithLock(func(current []byte) LockResult {
		return LockResult{Next: &content}
	})
	if err != nil {
		t.Fatal(err)
	}

	// Read
	result, err := backend.WithLock(func(current []byte) LockResult {
		return LockResult{Result: string(current)}
	})
	if err != nil {
		t.Fatal(err)
	}

	got, ok := result.(string)
	if !ok {
		t.Fatal("expected string result")
	}
	if got != content {
		t.Errorf("expected %s, got %s", content, got)
	}
}
