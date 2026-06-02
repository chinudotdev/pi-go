package ai_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/chinudotdev/pi-go/ai"
	"github.com/chinudotdev/pi-go/ai/providers"
)

func init() {
	providers.RegisterBuiltInApiProviders()
}

// ============================================================================
// Faux provider E2E tests
// ============================================================================

func TestE2E_Faux_Complete(t *testing.T) {
	fauxModel := &ai.Model{
		ID:       "faux-1",
		Provider: "faux",
		API:      "faux",
		Input:    []string{"text"},
	}

	apiKey := "test-key"
	convCtx := &ai.Context{
		Messages: []ai.Message{
			ai.NewUserMessage("hello"),
		},
	}

	msg, err := ai.Complete(context.Background(), fauxModel, convCtx, &ai.StreamOptions{APIKey: &apiKey})
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}

	if msg.Role != "assistant" {
		t.Errorf("msg.Role = %q, want %q", msg.Role, "assistant")
	}
	if msg.StopReason != ai.StopReasonStop {
		t.Errorf("msg.StopReason = %q, want %q", msg.StopReason, ai.StopReasonStop)
	}
	if len(msg.Content) == 0 {
		t.Fatal("expected content in response")
	}
	if msg.Content[0].Type != "text" {
		t.Errorf("content type = %q, want %q", msg.Content[0].Type, "text")
	}
	if !strings.Contains(msg.Content[0].Text, "faux") {
		t.Errorf("content = %q, should mention faux", msg.Content[0].Text)
	}
	t.Logf("Faux response: %s", msg.Content[0].Text)
}

func TestE2E_Faux_Stream(t *testing.T) {
	fauxModel := &ai.Model{
		ID:       "faux-1",
		Provider: "faux",
		API:      "faux",
		Input:    []string{"text"},
	}
	apiKey := "test-key"
	convCtx := &ai.Context{
		Messages: []ai.Message{
			ai.NewUserMessage("hello"),
		},
	}

	stream, err := ai.Stream(context.Background(), fauxModel, convCtx, &ai.StreamOptions{APIKey: &apiKey})
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}

	var events []string
	for ev := range stream.Iterate() {
		events = append(events, ev.Type)
	}

	msg, ok := <-stream.Result
	if !ok {
		t.Fatal("stream.Result channel closed without message")
	}

	expectedEvents := []string{"start", "text_delta", "done"}
	for i, expected := range expectedEvents {
		if i >= len(events) {
			t.Fatalf("missing event %q (got %d events, want at least %d)", expected, len(events), i+1)
		}
		if events[i] != expected {
			t.Errorf("event[%d] = %q, want %q", i, events[i], expected)
		}
	}

	if msg.StopReason != ai.StopReasonStop {
		t.Errorf("msg.StopReason = %q, want %q", msg.StopReason, ai.StopReasonStop)
	}
	t.Logf("Received %d events, final message has %d content blocks", len(events), len(msg.Content))
}

func TestE2E_Faux_ContextCancellation(t *testing.T) {
	fauxModel := &ai.Model{
		ID:       "faux-1",
		Provider: "faux",
		API:      "faux",
		Input:    []string{"text"},
	}
	apiKey := "test-key"
	convCtx := &ai.Context{
		Messages: []ai.Message{ai.NewUserMessage("hello")},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := ai.Stream(ctx, fauxModel, convCtx, &ai.StreamOptions{APIKey: &apiKey})
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}

	// Read from result channel
	select {
	case msg, ok := <-stream.Result:
		if !ok {
			t.Fatal("stream.Result closed")
		}
		if msg.StopReason != ai.StopReasonStop {
			t.Errorf("StopReason = %q, want %q", msg.StopReason, ai.StopReasonStop)
		}
	case <-ctx.Done():
		t.Fatalf("context cancelled: %v", ctx.Err())
	}
}

func TestE2E_Faux_MultiTurn(t *testing.T) {
	fauxModel := &ai.Model{
		ID:       "faux-1",
		Provider: "faux",
		API:      "faux",
		Input:    []string{"text"},
	}
	apiKey := "test-key"
	convCtx := &ai.Context{
		Messages: []ai.Message{
			ai.NewUserMessage("hello"),
			ai.NewAssistantMessage("faux", "faux", "faux-1",
				[]ai.ContentBlock{ai.NewTextContent("Hi there!")},
				ai.Usage{}, ai.StopReasonStop,
			),
			ai.NewUserMessage("how are you?"),
		},
	}

	msg, err := ai.Complete(context.Background(), fauxModel, convCtx, &ai.StreamOptions{APIKey: &apiKey})
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	if len(msg.Content) == 0 {
		t.Fatal("expected content in multi-turn response")
	}
	t.Logf("Multi-turn response: %s", msg.Content[0].Text)
}

func TestE2E_UnknownProvider(t *testing.T) {
	model := &ai.Model{
		ID:       "unknown-model",
		Provider: "nonexistent",
		API:      "nonexistent-api",
	}
	apiKey := "test-key"
	convCtx := &ai.Context{
		Messages: []ai.Message{ai.NewUserMessage("hello")},
	}

	_, err := ai.Stream(context.Background(), model, convCtx, &ai.StreamOptions{APIKey: &apiKey})
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
	if !strings.Contains(err.Error(), "no API provider registered") {
		t.Errorf("error = %q, should mention 'no API provider registered'", err.Error())
	}
}

// ============================================================================
// OpenAI E2E test (only runs if OPENAI_API_KEY is set)
// ============================================================================

func TestE2E_OpenAI_Complete(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping real OpenAI E2E test")
	}

	model, ok := ai.GetModel("openai", "gpt-4o")
	if !ok {
		t.Fatal("expected to find gpt-4o model")
	}

	convCtx := &ai.Context{
		SystemPrompt: func() *string { s := "Respond in one short sentence."; return &s }(),
		Messages: []ai.Message{
			ai.NewUserMessage("What is 2+2?"),
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	msg, err := ai.Complete(ctx, model, convCtx, &ai.StreamOptions{
		MaxTokens: func() *int { v := 100; return &v }(),
	})
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}

	if msg.Role != "assistant" {
		t.Errorf("msg.Role = %q, want %q", msg.Role, "assistant")
	}
	if msg.StopReason != ai.StopReasonStop {
		t.Errorf("msg.StopReason = %q, want %q", msg.StopReason, ai.StopReasonStop)
	}
	if len(msg.Content) == 0 {
		t.Fatal("expected content in response")
	}

	text := msg.Content[0].Text
	if len(text) == 0 {
		t.Error("expected non-empty text response")
	}

	t.Logf("OpenAI response: %s", text)
	t.Logf("Usage: input=%d output=%d total=%d cost=$%.6f",
		msg.Usage.Input, msg.Usage.Output, msg.Usage.TotalTokens, msg.Usage.Cost.Total)
}

func TestE2E_OpenAI_Stream(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping real OpenAI E2E test")
	}

	model, ok := ai.GetModel("openai", "gpt-4o")
	if !ok {
		t.Fatal("expected to find gpt-4o model")
	}

	convCtx := &ai.Context{
		Messages: []ai.Message{
			ai.NewUserMessage("Count from 1 to 5, one number per line."),
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	stream, err := ai.Stream(ctx, model, convCtx, &ai.StreamOptions{
		MaxTokens: func() *int { v := 200; return &v }(),
	})
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}

	var deltas []string
	for ev := range stream.Iterate() {
		if ev.Type == "text_delta" && ev.Delta != nil {
			deltas = append(deltas, *ev.Delta)
		}
	}

	msg := <-stream.Result
	fullText := strings.Join(deltas, "")

	t.Logf("Streamed %d deltas", len(deltas))
	t.Logf("Full text: %s", fullText)
	t.Logf("Response model: %v", msg.ResponseModel)
	t.Logf("Response ID: %v", msg.ResponseID)

	if len(deltas) == 0 {
		t.Error("expected at least one text delta")
	}
}

func TestE2E_OpenAI_ToolCalling(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping real OpenAI E2E test")
	}

	model, ok := ai.GetModel("openai", "gpt-4o")
	if !ok {
		t.Fatal("expected to find gpt-4o model")
	}

	convCtx := &ai.Context{
		Messages: []ai.Message{
			ai.NewUserMessage("What's the weather in San Francisco?"),
		},
		Tools: []ai.Tool{
			{
				Name:        "get_weather",
				Description: "Get the current weather for a location",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"location": map[string]any{
							"type":        "string",
							"description": "City name",
						},
					},
					"required": []any{"location"},
				},
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	msg, err := ai.Complete(ctx, model, convCtx, &ai.StreamOptions{
		MaxTokens: func() *int { v := 200; return &v }(),
	})
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}

	if msg.StopReason != ai.StopReasonToolUse {
		t.Fatalf("expected toolUse stop reason, got %q", msg.StopReason)
	}

	var toolCalls []ai.ContentBlock
	for _, block := range msg.Content {
		if block.Type == "toolCall" {
			toolCalls = append(toolCalls, block)
		}
	}

	if len(toolCalls) == 0 {
		t.Fatal("expected at least one tool call")
	}

	tc := toolCalls[0]
	if tc.ToolCallName != "get_weather" {
		t.Errorf("tool name = %q, want %q", tc.ToolCallName, "get_weather")
	}
	if tc.ToolCallID == "" {
		t.Error("tool call ID should not be empty")
	}
	if tc.ToolCallArguments == nil {
		t.Error("tool call arguments should not be nil")
	}

	argsJSON, _ := fmt.Printf("%v", tc.ToolCallArguments)
	_ = argsJSON

	t.Logf("Tool call: %s(%v) [id=%s]", tc.ToolCallName, tc.ToolCallArguments, tc.ToolCallID)
	t.Logf("Usage: input=%d output=%d cost=$%.6f", msg.Usage.Input, msg.Usage.Output, msg.Usage.Cost.Total)
}
