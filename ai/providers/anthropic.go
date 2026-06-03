package providers

// Anthropic Messages API provider.
// Implements streaming via the Anthropic Messages API (SSE).
//
// Dependencies needed:
//   - github.com/anthropics/anthropic-sdk-go  (official Anthropic Go SDK)
//
// The Anthropic provider supports:
//   - Extended thinking / reasoning with configurable budgets
//   - Tool use (function calling)
//   - Prompt caching (ephemeral cache_control with optional TTL)
//   - Multi-turn conversation with thinking signature replay
//   - Image inputs (vision)
//   - GitHub Copilot dynamic headers
//   - Cloudflare AI Gateway proxy support
//   - Stealth mode (Claude Code tool naming compatibility)

import (
	"context"

	"github.com/chinudotdev/pi-go/ai"
)

// AnthropicOptions extends StreamOptions with Anthropic-specific parameters.
type AnthropicOptions struct {
	ai.StreamOptions
	ToolChoice               any    `json:"toolChoice,omitempty"`      // "auto" | "none" | "required" | {type:"tool",name:"..."}
	AnthropicThinkingDisplay string `json:"thinkingDisplay,omitempty"` // "summarized" | "omitted"
	InterleavedThinking      bool   `json:"interleavedThinking,omitempty"`
}

// StreamAnthropic streams responses from the Anthropic Messages API.
func StreamAnthropic(ctx context.Context, model *ai.Model, convCtx *ai.Context, options *ai.StreamOptions) (*ai.EventStream, error) {
	stream := ai.NewEventStream(ctx)

	go func() {
		output := ai.NewAssistantOutput(string(ai.ApiAnthropicMessages), model.Provider, model.ID)

		// TODO: Implement Anthropic streaming:
		// 1. Create Anthropic SDK client with API key + base URL
		//    client := anthropic.NewClient(
		//      option.WithAPIKey(*options.APIKey),
		//      option.WithBaseURL(model.BaseURL),
		//    )
		// 2. Transform messages using TransformMessages()
		// 3. Build MessageCreateParams with:
		//    - System prompt with cache_control
		//    - Messages (user/assistant/toolResult)
		//    - Tools with cache_control
		//    - Thinking config (budget_tokens, type: "enabled"/"adaptive")
		//    - Temperature, max_tokens
		// 4. Call client.Messages.NewStreaming() and iterate events
		// 5. Push events to stream: text_start/delta/end, thinking_start/delta/end, toolcall_start/delta/end
		// 6. Calculate cost using CalculateCost()
		// 7. Push done/error event

		errMsg := "Anthropic provider not yet implemented"
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

// StreamSimpleAnthropic streams with simplified options, mapping reasoning levels to Anthropic thinking config.
func StreamSimpleAnthropic(ctx context.Context, model *ai.Model, convCtx *ai.Context, options *ai.StreamOptions) (*ai.EventStream, error) {
	apiKey, err := ai.ResolveAPIKey(model, options)
	if err != nil {
		return nil, err
	}
	baseOpts := ai.BuildBaseOptions(model, nil, apiKey)
	return StreamAnthropic(ctx, model, convCtx, &baseOpts)
}
