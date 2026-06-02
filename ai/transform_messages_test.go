package ai

import (
	"testing"
)

// ---- TransformMessages tests ----

func TestTransformMessages_SkipsErroredAssistant(t *testing.T) {
	model := &Model{
		Input:    []string{"text"},
		Provider: "anthropic",
		API:      "anthropic-messages",
	}
	errMsg := "something went wrong"
	messages := []Message{
		NewUserMessage("hello"),
		{
			Role: "assistant", Provider: "anthropic", API: "anthropic-messages",
			StopReason: StopReasonError, ErrorMessage: &errMsg,
			AssistantContent: []ContentBlock{NewTextContent("partial")},
		},
		NewUserMessage("try again"),
	}

	result := TransformMessages(messages, model, nil)

	if len(result) != 2 {
		t.Fatalf("expected 2 messages (errored assistant filtered), got %d", len(result))
	}
	if result[0].Role != "user" || result[1].Role != "user" {
		t.Errorf("expected two user messages, got %s and %s", result[0].Role, result[1].Role)
	}
}

func TestTransformMessages_SkipsAbortedAssistant(t *testing.T) {
	model := &Model{
		Input:    []string{"text"},
		Provider: "openai",
		API:      "openai-completions",
	}
	messages := []Message{
		NewUserMessage("hello"),
		{
			Role: "assistant", Provider: "openai", API: "openai-completions",
			StopReason: StopReasonAborted,
		},
		NewUserMessage("retry"),
	}

	result := TransformMessages(messages, model, nil)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
}

func TestTransformMessages_InsertsSyntheticToolResults(t *testing.T) {
	model := &Model{
		Input:    []string{"text"},
		Provider: "anthropic", API: "anthropic-messages",
	}
	messages := []Message{
		NewUserMessage("hello"),
		{
			Role:     "assistant",
			Provider: "anthropic", API: "anthropic-messages",
			AssistantContent: []ContentBlock{
				NewToolCallContent("tc1", "read_file", map[string]any{"path": "/foo"}),
			},
			StopReason: StopReasonToolUse,
		},
		NewUserMessage("next message"), // no tool result before this user message
	}

	result := TransformMessages(messages, model, nil)

	// Should be: user, assistant, synthetic tool result, user
	if len(result) != 4 {
		t.Fatalf("expected 4 messages (with synthetic tool result), got %d: %+v", len(result), result)
	}
	if result[2].Role != "toolResult" {
		t.Errorf("expected synthetic tool result at index 2, got %s", result[2].Role)
	}
	if result[2].ToolCallID != "tc1" {
		t.Errorf("synthetic tool result ToolCallID = %q, want %q", result[2].ToolCallID, "tc1")
	}
	if !result[2].IsError {
		t.Error("synthetic tool result should have IsError=true")
	}
}

func TestTransformMessages_DowngradesImagesForNonVisionModel(t *testing.T) {
	model := &Model{
		Input:    []string{"text"}, // no "image"
		Provider: "deepseek", API:  "openai-completions",
	}
	messages := []Message{
		NewUserMessageWithContent([]ContentBlock{
			NewTextContent("describe this"),
			{Type: "image", ToolCallID: "img1", ToolCallName: "image/png"}, // using available fields
		}),
	}

	result := TransformMessages(messages, model, nil)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}

	blocks, ok := result[0].Content.([]ContentBlock)
	if !ok {
		t.Fatal("expected []ContentBlock content")
	}
	// Image should be replaced with placeholder
	for _, b := range blocks {
		if b.Type == "image" {
			t.Error("image block should have been replaced with placeholder")
		}
	}
}

func TestTransformMessages_ConvertsThinkingToText_CrossModel(t *testing.T) {
	model := &Model{
		Input:    []string{"text"},
		Provider: "openai", API: "openai-completions",
	}
	messages := []Message{
		NewUserMessage("hello"),
		{
			Role:     "assistant",
			Provider: "anthropic", API: "anthropic-messages", // different model
			AssistantContent: []ContentBlock{
				NewThinkingContent("let me think about this"),
			},
			StopReason: StopReasonStop,
		},
	}

	result := TransformMessages(messages, model, nil)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	asm := result[1]
	if len(asm.AssistantContent) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(asm.AssistantContent))
	}
	if asm.AssistantContent[0].Type != "text" {
		t.Errorf("thinking should be converted to text for cross-model, got %q", asm.AssistantContent[0].Type)
	}
	if asm.AssistantContent[0].Text != "let me think about this" {
		t.Errorf("text = %q, want %q", asm.AssistantContent[0].Text, "let me think about this")
	}
}

func TestTransformMessages_KeepsThinking_SameModel(t *testing.T) {
	model := &Model{
		ID:       "claude-sonnet-4-20250514",
		Input:    []string{"text"},
		Provider: "anthropic", API: "anthropic-messages",
	}
	sig := "sig123"
	messages := []Message{
		NewUserMessage("hello"),
		{
			Role:     "assistant",
			Provider: "anthropic", API: "anthropic-messages",
			Model:    "claude-sonnet-4-20250514",
			AssistantContent: []ContentBlock{
				{Type: "thinking", Thinking: "reasoning", ThinkingSignature: &sig},
			},
			StopReason: StopReasonStop,
		},
	}

	result := TransformMessages(messages, model, nil)
	asm := result[1]
	if len(asm.AssistantContent) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(asm.AssistantContent))
	}
	if asm.AssistantContent[0].Type != "thinking" {
		t.Errorf("thinking should be kept for same model, got %q", asm.AssistantContent[0].Type)
	}
}

func TestTransformMessages_DropsRedactedThinking_CrossModel(t *testing.T) {
	model := &Model{
		Input:    []string{"text"},
		Provider: "openai", API: "openai-completions",
	}
	messages := []Message{
		NewUserMessage("hello"),
		{
			Role:     "assistant",
			Provider: "anthropic", API: "anthropic-messages",
			AssistantContent: []ContentBlock{
				{Type: "thinking", Thinking: "redacted", Redacted: true},
			},
			StopReason: StopReasonStop,
		},
	}

	result := TransformMessages(messages, model, nil)
	asm := result[1]
	if len(asm.AssistantContent) != 0 {
		t.Errorf("redacted thinking should be dropped for cross-model, got %d blocks", len(asm.AssistantContent))
	}
}

func TestTransformMessages_NormalizesToolCallID(t *testing.T) {
	model := &Model{
		Input:    []string{"text"},
		Provider: "anthropic", API: "anthropic-messages",
	}
	normalize := func(id string, m *Model, a *AssistantMessage) string {
		if len(id) > 10 {
			return id[:10]
		}
		return id
	}
	longID := "call_very_long_tool_call_id_that_is_over_10_chars"
	messages := []Message{
		NewUserMessage("hello"),
		{
			Role:     "assistant",
			Provider: "openai", API: "openai-responses", // different provider
			AssistantContent: []ContentBlock{
				NewToolCallContent(longID, "read", map[string]any{}),
			},
			StopReason: StopReasonToolUse,
		},
		NewToolResultMessage(longID, "read", []ContentBlock{NewTextContent("result")}, false),
	}

	result := TransformMessages(messages, model, normalize)

	// Assistant's tool call should have normalized ID
	asm := result[1]
	if asm.AssistantContent[0].ToolCallID != longID[:10] {
		t.Errorf("tool call ID = %q, want %q", asm.AssistantContent[0].ToolCallID, longID[:10])
	}

	// Tool result should have matching normalized ID
	tr := result[2]
	if tr.ToolCallID != longID[:10] {
		t.Errorf("tool result ID = %q, want %q", tr.ToolCallID, longID[:10])
	}
}

func TestTransformMessages_EndsWithOrphanedToolCalls(t *testing.T) {
	model := &Model{
		Input:    []string{"text"},
		Provider: "anthropic", API: "anthropic-messages",
	}
	messages := []Message{
		NewUserMessage("hello"),
		{
			Role:     "assistant",
			Provider: "anthropic", API: "anthropic-messages",
			AssistantContent: []ContentBlock{
				NewToolCallContent("tc1", "read", map[string]any{}),
			},
			StopReason: StopReasonToolUse,
		},
		// No tool result, no user message — conversation ends here
	}

	result := TransformMessages(messages, model, nil)

	if len(result) != 3 {
		t.Fatalf("expected 3 messages (user, assistant, synthetic), got %d", len(result))
	}
	if result[2].Role != "toolResult" {
		t.Errorf("last message should be synthetic tool result, got %s", result[2].Role)
	}
}
