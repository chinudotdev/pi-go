package providers

// Mistral Conversations API provider.
// Implements streaming via the Mistral chat.stream endpoint.
//
// Dependencies needed:
//   - github.com/gmistralai/mistralai-go  (Mistral Go SDK, if available)
//   - Or use raw HTTP/SSE: github.com/volatiletech/ssse  (SSE client)
//
// Supports:
//   - Reasoning mode with "high" effort
//   - Tool use (function calling)
//   - Tool call ID normalization (9-char IDs)

import (
	"context"

	"github.com/chinudotdev/pi-go/ai"
)

// MistralOptions extends StreamOptions with Mistral-specific parameters.
type MistralOptions struct {
	ai.StreamOptions
	ToolChoice      any    `json:"toolChoice,omitempty"`
	PromptMode      string `json:"promptMode,omitempty"` // "reasoning"
	ReasoningEffort string `json:"reasoningEffort,omitempty"` // "none" | "high"
}

// StreamMistral streams from Mistral's chat API.
func StreamMistral(ctx context.Context, model *ai.Model, convCtx *ai.Context, options *ai.StreamOptions) (*ai.EventStream, error) {
	stream := ai.NewEventStream(ctx)

	go func() {
		output := ai.NewAssistantOutput(string(ai.ApiMistralConversations), model.Provider, model.ID)

		// TODO: Implement Mistral streaming:
		// 1. Create HTTP client with API key
		// 2. Transform messages for Mistral format
		// 3. Build chat stream request with tools, thinking config
		// 4. POST to /chat/completions with stream=true
		// 5. Parse SSE events
		// 6. Process content chunks, tool calls, thinking content
		// 7. Calculate cost

		errMsg := "Mistral provider not yet implemented"
		output.StopReason = ai.StopReasonError
		output.ErrorMessage = &errMsg
		stream.Push(ai.AssistantMessageEvent{
			Type:   "error",
			Reason: ai.StopReasonError,
			Error:  &output,
		})
		stream.End(output)
	}()

	return stream, nil
}

// StreamSimpleMistral streams with simplified options.
func StreamSimpleMistral(ctx context.Context, model *ai.Model, convCtx *ai.Context, options *ai.StreamOptions) (*ai.EventStream, error) {
	apiKey, err := ai.ResolveAPIKey(model, options)
	if err != nil {
		return nil, err
	}
	baseOpts := ai.BuildBaseOptions(model, nil, apiKey)
	return StreamMistral(ctx, model, convCtx, &baseOpts)
}

