package providers

// OpenAI Responses API provider.
// Implements streaming via the OpenAI /responses endpoint (SSE).
//
// Dependencies needed:
//   - github.com/openai/openai-go  (official OpenAI Go SDK)
//
// Supports:
//   - Reasoning effort with encrypted content
//   - Service tier pricing (flex, priority, default)
//   - Prompt caching (prompt_cache_key, prompt_cache_retention)
//   - Tool use via the Responses API format
//   - Session affinity headers

import (
	"context"

	"github.com/chinudotdev/pi-go/ai"
)

// OpenAIResponsesOptions extends StreamOptions with Responses-specific parameters.
type OpenAIResponsesOptions struct {
	ai.StreamOptions
	ReasoningEffort  string `json:"reasoningEffort,omitempty"`
	ReasoningSummary string `json:"reasoningSummary,omitempty"` // "auto" | "detailed" | "concise" | null
	ServiceTier      string `json:"serviceTier,omitempty"`
}

// StreamOpenAIResponses streams responses from the OpenAI Responses API.
func StreamOpenAIResponses(ctx context.Context, model *ai.Model, convCtx *ai.Context, options *ai.StreamOptions) (*ai.EventStream, error) {
	stream := ai.NewEventStream(ctx)

	go func() {
		output := ai.NewAssistantOutput(string(ai.ApiOpenAIResponses), model.Provider, model.ID)

		// TODO: Implement OpenAI Responses streaming:
		// 1. Create OpenAI SDK client
		// 2. Convert messages to Responses API input format
		// 3. Convert tools to Responses API tool format
		// 4. Apply reasoning config (effort, summary, encrypted_content)
		// 5. Call client.Responses.NewStreaming()
		// 6. Process response stream events (response.output_item.done, etc.)
		// 7. Push events to event stream
		// 8. Apply service tier pricing
		// 9. Calculate cost
		// 10. Push done/error

		errMsg := "OpenAI Responses provider not yet implemented"
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

// StreamSimpleOpenAIResponses streams with simplified options.
func StreamSimpleOpenAIResponses(ctx context.Context, model *ai.Model, convCtx *ai.Context, options *ai.StreamOptions) (*ai.EventStream, error) {
	apiKey, err := ai.ResolveAPIKey(model, options)
	if err != nil {
		return nil, err
	}
	baseOpts := ai.BuildBaseOptions(model, nil, apiKey)
	return StreamOpenAIResponses(ctx, model, convCtx, &baseOpts)
}

