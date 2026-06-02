// Package ai provides a unified LLM API with automatic model discovery
// and provider configuration.
//
// It supports streaming chat completions across 9 API backends from 30+
// providers through a single, provider-agnostic interface.
//
// # Quick Start
//
//	model, ok := ai.GetModel("anthropic", "claude-sonnet-4-20250514")
//	stream, err := ai.Stream(ctx, model, &ai.Context{
//	    SystemPrompt: ptr("You are helpful"),
//	    Messages: []ai.Message{
//	        ai.NewUserMessage("Hello!"),
//	    },
//	}, nil)
//
//	for event := range stream.Iterate() {
//	    if event.Type == "text_delta" && event.Delta != nil {
//	        fmt.Print(*event.Delta)
//	    }
//	}
//
// # Supported Providers
//
// Built-in providers include: OpenAI, Anthropic, Google (Gemini + Vertex AI),
// Amazon Bedrock, Mistral, Azure OpenAI, DeepSeek, Groq, Cerebras, xAI,
// OpenRouter, Together, Fireworks, HuggingFace, GitHub Copilot, and more.
//
// # Architecture
//
// The package uses a registry pattern where API providers register themselves.
// Each provider implements a StreamFunction that takes a Model, Context, and
// StreamOptions, returning an EventStream of AssistantMessageEvents.
//
// Models are loaded from an embedded models.generated.json file that contains
// metadata for all known models including costs, context windows, and capabilities.
// # Message Types
//
// The Message struct is a discriminated union using the Role field. Each role
// ("user", "assistant", "toolResult") uses a different subset of fields.
// The AsUserMessage(), AsAssistantMessage(), and AsToolResultMessage() methods
// provide typed access. For tool result messages, the content blocks are stored
// in the ToolResultContent field (not AssistantContent).
package ai
