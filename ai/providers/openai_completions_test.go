package providers

import (
	"encoding/json"
	"testing"

	"github.com/openai/openai-go/packages/param"

	"github.com/chinudotdev/pi-go/ai"
)

// ============================================================================
// Compat detection tests
// ============================================================================

func TestDetectCompat_OpenAI(t *testing.T) {
	model := &ai.Model{
		ID:       "gpt-4o",
		Provider: "openai",
		BaseURL:  "https://api.openai.com/v1",
	}
	compat := detectCompat(model)
	if !compat.SupportsStore {
		t.Error("OpenAI should support store")
	}
	if !compat.SupportsDeveloperRole {
		t.Error("OpenAI should support developer role")
	}
	if !compat.SupportsReasoningEffort {
		t.Error("OpenAI should support reasoning effort")
	}
	if compat.ThinkingFormat != "openai" {
		t.Errorf("ThinkingFormat = %q, want %q", compat.ThinkingFormat, "openai")
	}
	if compat.MaxTokensField != "max_completion_tokens" {
		t.Errorf("MaxTokensField = %q, want %q", compat.MaxTokensField, "max_completion_tokens")
	}
}

func TestDetectCompat_DeepSeek(t *testing.T) {
	model := &ai.Model{
		ID:       "deepseek-chat",
		Provider: "deepseek",
		BaseURL:  "https://api.deepseek.com/v1",
	}
	compat := detectCompat(model)
	if compat.ThinkingFormat != "deepseek" {
		t.Errorf("ThinkingFormat = %q, want %q", compat.ThinkingFormat, "deepseek")
	}
	if !compat.RequiresReasoningContentOnAssistantMessages {
		t.Error("DeepSeek should require reasoning_content on assistant messages")
	}
}

func TestDetectCompat_OpenRouter(t *testing.T) {
	model := &ai.Model{
		ID:       "anthropic/claude-sonnet-4",
		Provider: "openrouter",
		BaseURL:  "https://openrouter.ai/api/v1",
	}
	compat := detectCompat(model)
	if compat.ThinkingFormat != "openrouter" {
		t.Errorf("ThinkingFormat = %q, want %q", compat.ThinkingFormat, "openrouter")
	}
	if compat.CacheControlFormat != "anthropic" {
		t.Errorf("CacheControlFormat = %q, want %q", compat.CacheControlFormat, "anthropic")
	}
	if compat.SupportsDeveloperRole {
		t.Error("OpenRouter should not support developer role")
	}
}

func TestDetectCompat_Grok(t *testing.T) {
	model := &ai.Model{
		ID:       "grok-3",
		Provider: "xai",
		BaseURL:  "https://api.x.ai/v1",
	}
	compat := detectCompat(model)
	if compat.SupportsReasoningEffort {
		t.Error("xAI/Grok should not support reasoning_effort")
	}
}

func TestDetectCompat_Together(t *testing.T) {
	model := &ai.Model{
		ID:       "meta-llama/Llama-3",
		Provider: "together",
		BaseURL:  "https://api.together.ai/v1",
	}
	compat := detectCompat(model)
	if compat.ThinkingFormat != "together" {
		t.Errorf("ThinkingFormat = %q, want %q", compat.ThinkingFormat, "together")
	}
	if compat.MaxTokensField != "max_tokens" {
		t.Errorf("Together should use max_tokens, got %q", compat.MaxTokensField)
	}
}

func TestDetectCompat_Moonshot(t *testing.T) {
	model := &ai.Model{
		ID:       "moonshot-v1",
		Provider: "moonshotai",
		BaseURL:  "https://api.moonshot.cn/v1",
	}
	compat := detectCompat(model)
	if compat.SupportsStrictMode {
		t.Error("Moonshot should not support strict mode")
	}
	if compat.MaxTokensField != "max_tokens" {
		t.Errorf("Moonshot should use max_tokens, got %q", compat.MaxTokensField)
	}
}

// ============================================================================
// GetCompat with model overrides
// ============================================================================

func TestGetCompat_NoOverrides(t *testing.T) {
	model := &ai.Model{
		ID:       "gpt-4o",
		Provider: "openai",
		BaseURL:  "https://api.openai.com/v1",
	}
	compat := getCompat(model)
	if !compat.SupportsStore {
		t.Error("should support store")
	}
}

func TestGetCompat_WithOverrides(t *testing.T) {
	rawCompat := map[string]any{
		"supportsStore":         false,
		"supportsDeveloperRole": false,
	}
	data, _ := json.Marshal(rawCompat)

	model := &ai.Model{
		ID:       "gpt-4o",
		Provider: "openai",
		BaseURL:  "https://api.openai.com/v1",
		Compat:   json.RawMessage(data),
	}
	compat := getCompat(model)
	if compat.SupportsStore {
		t.Error("model override should disable store")
	}
	if compat.SupportsDeveloperRole {
		t.Error("model override should disable developer role")
	}
}

// ============================================================================
// Message conversion tests
// ============================================================================

func TestConvertMessages_SystemPrompt(t *testing.T) {
	model := &ai.Model{
		ID:       "gpt-4o",
		Provider: "openai",
		BaseURL:  "https://api.openai.com/v1",
		Input:    []string{"text"},
	}
	sysPrompt := "You are helpful."
	convCtx := &ai.Context{
		SystemPrompt: &sysPrompt,
		Messages: []ai.Message{
			ai.NewUserMessage("hello"),
		},
	}
	compat := getCompat(model)
	params := convertMessages(model, convCtx, compat)

	if len(params) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(params))
	}
	// First should be system message
	if params[0].OfSystem == nil {
		t.Error("expected system message, got nil")
	}
}

func TestConvertMessages_DeveloperPromptForReasoning(t *testing.T) {
	model := &ai.Model{
		ID:        "o3",
		Provider:  "openai",
		BaseURL:   "https://api.openai.com/v1",
		Input:     []string{"text"},
		Reasoning: true,
	}
	sysPrompt := "Think carefully."
	convCtx := &ai.Context{
		SystemPrompt: &sysPrompt,
		Messages: []ai.Message{
			ai.NewUserMessage("hello"),
		},
	}
	compat := getCompat(model)
	params := convertMessages(model, convCtx, compat)

	if len(params) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(params))
	}
	if params[0].OfDeveloper == nil {
		t.Error("expected developer message for reasoning model, got nil")
	}
}

func TestConvertMessages_ToolCalls(t *testing.T) {
	model := &ai.Model{
		ID:       "gpt-4o",
		Provider: "openai",
		BaseURL:  "https://api.openai.com/v1",
		Input:    []string{"text"},
	}
	convCtx := &ai.Context{
		Messages: []ai.Message{
			ai.NewUserMessage("read the file"),
			ai.NewAssistantMessage(
				"openai-completions", "openai", "gpt-4o",
				[]ai.ContentBlock{
					ai.NewToolCallContent("call_123", "read_file", map[string]any{"path": "/tmp/test.txt"}),
				},
				ai.Usage{},
				ai.StopReasonToolUse,
			),
			ai.NewToolResultMessage("call_123", "read_file", []ai.ContentBlock{ai.NewTextContent("file contents")}, false),
		},
	}
	compat := getCompat(model)
	params := convertMessages(model, convCtx, compat)

	if len(params) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(params))
	}

	// User message
	if params[0].OfUser == nil {
		t.Error("first message should be user")
	}

	// Assistant with tool calls
	if params[1].OfAssistant == nil {
		t.Fatal("second message should be assistant")
	}
	if len(params[1].OfAssistant.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(params[1].OfAssistant.ToolCalls))
	}
	if params[1].OfAssistant.ToolCalls[0].ID != "call_123" {
		t.Errorf("tool call ID = %q, want %q", params[1].OfAssistant.ToolCalls[0].ID, "call_123")
	}
	if params[1].OfAssistant.ToolCalls[0].Function.Name != "read_file" {
		t.Errorf("tool call name = %q, want %q", params[1].OfAssistant.ToolCalls[0].Function.Name, "read_file")
	}

	// Tool result
	if params[2].OfTool == nil {
		t.Fatal("third message should be tool")
	}
	if params[2].OfTool.ToolCallID != "call_123" {
		t.Errorf("tool message ToolCallID = %q, want %q", params[2].OfTool.ToolCallID, "call_123")
	}
}

func TestConvertMessages_ToolCallIDNormalization(t *testing.T) {
	model := &ai.Model{
		ID:       "claude-3",
		Provider: "anthropic", // different provider → triggers normalization
		API:      "anthropic-messages",
		BaseURL:  "https://api.anthropic.com",
		Input:    []string{"text"},
	}
	longID := "very_long_tool_call_id_with_special+chars/that=needs|truncation"
	convCtx := &ai.Context{
		Messages: []ai.Message{
			ai.NewUserMessage("do it"),
			ai.NewAssistantMessage(
				"openai-responses", "openai", "gpt-4o",
				[]ai.ContentBlock{
					ai.NewToolCallContent(longID, "tool", map[string]any{}),
				},
				ai.Usage{},
				ai.StopReasonToolUse,
			),
			ai.NewToolResultMessage(longID, "tool", []ai.ContentBlock{ai.NewTextContent("ok")}, false),
		},
	}
	compat := getCompat(model)
	params := convertMessages(model, convCtx, compat)

	// The pipe-separated part should be stripped, special chars replaced, truncated to 40
	asm := params[1].OfAssistant
	if asm == nil {
		t.Fatal("expected assistant message")
	}
	tcID := asm.ToolCalls[0].ID
	if len(tcID) > 40 {
		t.Errorf("normalized tool call ID too long: %q (%d chars)", tcID, len(tcID))
	}
}

func TestConvertMessages_SkipsEmptyAssistant(t *testing.T) {
	model := &ai.Model{
		ID:       "gpt-4o",
		Provider: "openai",
		BaseURL:  "https://api.openai.com/v1",
		Input:    []string{"text"},
	}
	convCtx := &ai.Context{
		Messages: []ai.Message{
			ai.NewUserMessage("hello"),
			{
				Role:             "assistant",
				AssistantContent: []ai.ContentBlock{}, // empty
				StopReason:       ai.StopReasonStop,
			},
			ai.NewUserMessage("world"),
		},
	}
	compat := getCompat(model)
	params := convertMessages(model, convCtx, compat)

	// Empty assistant should be skipped
	if len(params) != 2 {
		t.Fatalf("expected 2 messages (empty assistant skipped), got %d", len(params))
	}
}

func TestConvertMessages_UserStringContent(t *testing.T) {
	model := &ai.Model{
		ID:       "gpt-4o",
		Provider: "openai",
		BaseURL:  "https://api.openai.com/v1",
		Input:    []string{"text"},
	}
	convCtx := &ai.Context{
		Messages: []ai.Message{
			ai.NewUserMessage("hello world"),
		},
	}
	compat := getCompat(model)
	params := convertMessages(model, convCtx, compat)

	if len(params) != 1 {
		t.Fatalf("expected 1 message, got %d", len(params))
	}
	if params[0].OfUser == nil {
		t.Fatal("expected user message")
	}
}

// ============================================================================
// Tool conversion tests
// ============================================================================

func TestConvertTools(t *testing.T) {
	tools := []ai.Tool{
		{
			Name:        "read_file",
			Description: "Read a file",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string"},
				},
			},
		},
	}
	compat := resolvedCompat{SupportsStrictMode: true}
	result := convertTools(tools, compat)

	if len(result) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result))
	}
	if result[0].Function.Name != "read_file" {
		t.Errorf("tool name = %q, want %q", result[0].Function.Name, "read_file")
	}
}

// ============================================================================
// Stop reason mapping
// ============================================================================

func TestMapStopReason(t *testing.T) {
	tests := []struct {
		input    string
		expected ai.StopReason
		hasError bool
	}{
		{"stop", ai.StopReasonStop, false},
		{"end", ai.StopReasonStop, false},
		{"", ai.StopReasonStop, false},
		{"length", ai.StopReasonLength, false},
		{"tool_calls", ai.StopReasonToolUse, false},
		{"function_call", ai.StopReasonToolUse, false},
		{"content_filter", ai.StopReasonError, true},
		{"network_error", ai.StopReasonError, true},
		{"unknown_reason", ai.StopReasonError, true},
	}

	for _, tc := range tests {
		reason, errMsg := mapStopReason(tc.input)
		if reason != tc.expected {
			t.Errorf("mapStopReason(%q) = %q, want %q", tc.input, reason, tc.expected)
		}
		if tc.hasError && errMsg == nil {
			t.Errorf("mapStopReason(%q) should have error message", tc.input)
		}
		if !tc.hasError && errMsg != nil {
			t.Errorf("mapStopReason(%q) should not have error message, got %q", tc.input, *errMsg)
		}
	}
}

// ============================================================================
// Params building
// ============================================================================

func TestBuildOpenAIParams_Basic(t *testing.T) {
	model := &ai.Model{
		ID:       "gpt-4o",
		Provider: "openai",
		BaseURL:  "https://api.openai.com/v1",
		Input:    []string{"text"},
	}
	temp := 0.7
	maxTokens := 1024
	options := &ai.StreamOptions{
		Temperature: &temp,
		MaxTokens:   &maxTokens,
	}
	convCtx := &ai.Context{
		Messages: []ai.Message{
			ai.NewUserMessage("hello"),
		},
	}
	compat := getCompat(model)
	params := buildOpenAIParams(model, convCtx, options, compat, ai.CacheShort)

	if params.Model != "gpt-4o" {
		t.Errorf("Model = %q, want %q", params.Model, "gpt-4o")
	}
	if !param.IsOmitted(params.Temperature) && params.Temperature.Or(0) != 0.7 {
		t.Errorf("Temperature = %v, want 0.7", params.Temperature)
	}
}

func TestBuildOpenAIParams_WithTools(t *testing.T) {
	model := &ai.Model{
		ID:       "gpt-4o",
		Provider: "openai",
		BaseURL:  "https://api.openai.com/v1",
		Input:    []string{"text"},
	}
	convCtx := &ai.Context{
		Messages: []ai.Message{
			ai.NewUserMessage("hello"),
		},
		Tools: []ai.Tool{
			{Name: "read_file", Description: "Read a file", Parameters: map[string]any{"type": "object"}},
		},
	}
	compat := getCompat(model)
	params := buildOpenAIParams(model, convCtx, nil, compat, ai.CacheShort)

	if len(params.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(params.Tools))
	}
}

func TestBuildOpenAIParams_DeepSeekMaxTokens(t *testing.T) {
	model := &ai.Model{
		ID:       "deepseek-chat",
		Provider: "deepseek",
		BaseURL:  "https://api.deepseek.com",
		Input:    []string{"text"},
	}
	maxTokens := 2048
	options := &ai.StreamOptions{
		MaxTokens: &maxTokens,
	}
	convCtx := &ai.Context{
		Messages: []ai.Message{ai.NewUserMessage("hello")},
	}
	compat := getCompat(model)

	// DeepSeek uses max_completion_tokens by default
	_ = buildOpenAIParams(model, convCtx, options, compat, ai.CacheShort)
}

// ============================================================================
// Has tool history
// ============================================================================

func TestHasToolHistory(t *testing.T) {
	tests := []struct {
		name     string
		messages []ai.Message
		expected bool
	}{
		{
			"no tools",
			[]ai.Message{ai.NewUserMessage("hello")},
			false,
		},
		{
			"has tool result",
			[]ai.Message{
				ai.NewToolResultMessage("tc1", "tool", []ai.ContentBlock{ai.NewTextContent("ok")}, false),
			},
			true,
		},
		{
			"has tool call in assistant",
			[]ai.Message{
				ai.NewAssistantMessage(
					"openai-completions", "openai", "gpt-4o",
					[]ai.ContentBlock{ai.NewToolCallContent("tc1", "tool", map[string]any{})},
					ai.Usage{},
					ai.StopReasonToolUse,
				),
			},
			true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := hasToolHistory(tc.messages)
			if result != tc.expected {
				t.Errorf("hasToolHistory = %v, want %v", result, tc.expected)
			}
		})
	}
}
