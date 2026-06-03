package settings

import (
	"encoding/json"
	"testing"
)

func TestInMemoryDefaults(t *testing.T) {
	m := InMemory()

	if m.GetDefaultProvider() != "" {
		t.Errorf("expected empty default provider, got %s", m.GetDefaultProvider())
	}
	if m.GetDefaultModel() != "" {
		t.Errorf("expected empty default model, got %s", m.GetDefaultModel())
	}
	if m.GetTransport() != "auto" {
		t.Errorf("expected auto transport, got %s", m.GetTransport())
	}
	if !m.GetCompactionEnabled() {
		t.Error("expected compaction enabled by default")
	}
	if m.GetCompactionReserveTokens() != 16384 {
		t.Errorf("expected 16384 reserve tokens, got %d", m.GetCompactionReserveTokens())
	}
	if m.GetCompactionKeepRecentTokens() != 20000 {
		t.Errorf("expected 20000 keep recent tokens, got %d", m.GetCompactionKeepRecentTokens())
	}
	if !m.GetRetryEnabled() {
		t.Error("expected retry enabled by default")
	}
	if m.GetRetryMaxRetries() != 3 {
		t.Errorf("expected 3 retries, got %d", m.GetRetryMaxRetries())
	}
	if m.GetRetryBaseDelayMs() != 2000 {
		t.Errorf("expected 2000ms base delay, got %d", m.GetRetryBaseDelayMs())
	}
	if m.GetSteeringMode() != "one-at-a-time" {
		t.Errorf("expected one-at-a-time steering mode, got %s", m.GetSteeringMode())
	}
	if m.GetFollowUpMode() != "one-at-a-time" {
		t.Errorf("expected one-at-a-time follow-up mode, got %s", m.GetFollowUpMode())
	}
	if !m.GetImageAutoResize() {
		t.Error("expected image auto-resize enabled by default")
	}
	if m.GetBlockImages() {
		t.Error("expected block images disabled by default")
	}
	if m.GetHTTPIdleTimeoutMs() != 300_000 {
		t.Errorf("expected 300000ms HTTP idle timeout, got %d", m.GetHTTPIdleTimeoutMs())
	}
	if !m.GetEnableSkillCommands() {
		t.Error("expected skill commands enabled by default")
	}
}

func TestInMemoryWithSettings(t *testing.T) {
	provider := "anthropic"
	model := "claude-opus-4-8"
	level := "high"

	m := InMemory(Settings{
		DefaultProvider:      &provider,
		DefaultModel:         &model,
		DefaultThinkingLevel: &level,
	})

	if m.GetDefaultProvider() != "anthropic" {
		t.Errorf("expected anthropic, got %s", m.GetDefaultProvider())
	}
	if m.GetDefaultModel() != "claude-opus-4-8" {
		t.Errorf("expected claude-opus-4-8, got %s", m.GetDefaultModel())
	}
	if m.GetDefaultThinkingLevel() != "high" {
		t.Errorf("expected high, got %s", m.GetDefaultThinkingLevel())
	}
}

func TestSetDefaultModelAndProvider(t *testing.T) {
	m := InMemory()
	m.SetDefaultModelAndProvider("openai", "gpt-4")

	if m.GetDefaultProvider() != "openai" {
		t.Errorf("expected openai, got %s", m.GetDefaultProvider())
	}
	if m.GetDefaultModel() != "gpt-4" {
		t.Errorf("expected gpt-4, got %s", m.GetDefaultModel())
	}
}

func TestDeepMerge(t *testing.T) {
	base := Settings{
		DefaultProvider: ptrString("anthropic"),
		Compaction: &CompactionSettings{
			Enabled:       ptrBool(true),
			ReserveTokens: ptrInt(10000),
		},
	}

	override := Settings{
		DefaultModel: ptrString("claude-4"),
		Compaction: &CompactionSettings{
			ReserveTokens: ptrInt(20000),
		},
	}

	result := deepMerge(base, override)

	// Base field preserved
	if *result.DefaultProvider != "anthropic" {
		t.Errorf("expected anthropic, got %v", result.DefaultProvider)
	}
	// Override field added
	if *result.DefaultModel != "claude-4" {
		t.Errorf("expected claude-4, got %v", result.DefaultModel)
	}
	// Nested merge: base.Enabled + override.ReserveTokens
	if !*result.Compaction.Enabled {
		t.Error("expected compaction enabled to be preserved")
	}
	if *result.Compaction.ReserveTokens != 20000 {
		t.Errorf("expected 20000 reserve tokens, got %d", *result.Compaction.ReserveTokens)
	}
}

func TestSettingsJSON(t *testing.T) {
	// Verify Settings round-trips through JSON
	s := Settings{
		DefaultProvider: ptrString("anthropic"),
		DefaultModel:    ptrString("claude-4"),
		Compaction: &CompactionSettings{
			Enabled:       ptrBool(true),
			ReserveTokens: ptrInt(16384),
		},
		Skills: []string{"/path/to/skills"},
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	var parsed Settings
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}

	if *parsed.DefaultProvider != "anthropic" {
		t.Errorf("expected anthropic, got %v", parsed.DefaultProvider)
	}
	if len(parsed.Skills) != 1 || parsed.Skills[0] != "/path/to/skills" {
		t.Errorf("expected skills, got %v", parsed.Skills)
	}
}

func TestMemoryStorageBackend(t *testing.T) {
	backend := NewMemoryStorageBackend()

	// Write
	content := `{"defaultProvider":"openai"}`
	backend.WithLock(ScopeGlobal, func(current []byte) *[]byte {
		result := []byte(content)
		return &result
	})

	// Read
	var read []byte
	backend.WithLock(ScopeGlobal, func(current []byte) *[]byte {
		read = current
		return nil
	})

	if string(read) != content {
		t.Errorf("expected %s, got %s", content, string(read))
	}
}

func TestProviderRetrySettings(t *testing.T) {
	m := InMemory()

	timeout, retries, delay := m.GetProviderRetrySettings()
	if timeout != 0 || retries != 0 || delay != 60000 {
		t.Errorf("expected defaults (0, 0, 60000), got (%d, %d, %d)", timeout, retries, delay)
	}

	maxDelay := 30000
	m = InMemory(Settings{
		Retry: &RetrySettings{
			Provider: &ProviderRetrySettings{
				MaxRetryDelayMs: &maxDelay,
			},
		},
	})

	_, _, delay = m.GetProviderRetrySettings()
	if delay != 30000 {
		t.Errorf("expected 30000, got %d", delay)
	}
}
