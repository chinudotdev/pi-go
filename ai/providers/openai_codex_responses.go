package providers

// OpenAI Codex Responses API provider.
// Implements streaming via the OpenAI Codex (chatgpt.com) backend.
// Supports both SSE and WebSocket transports with automatic JWT authentication.
//
// Dependencies needed:
//   - github.com/openai/openai-go  (official OpenAI Go SDK)
//   - github.com/gorilla/websocket  (for WebSocket transport)
//   - nhooyr.io/websocket           (alternative WebSocket library)
//
// Supports:
//   - SSE and WebSocket transports with automatic fallback
//   - JWT token refresh and session management
//   - Reasoning effort with encrypted content
//   - Session resource cleanup
//   - Retry with exponential backoff

import (
	"context"

	"github.com/chinudotdev/pi-go/ai"
)

// StreamOpenAICodexResponses streams from the OpenAI Codex backend.
func StreamOpenAICodexResponses(ctx context.Context, model *ai.Model, convCtx *ai.Context, options *ai.StreamOptions) (*ai.EventStream, error) {
	stream := ai.NewEventStream(ctx)

	go func() {
		output := ai.NewAssistantOutput(string(ai.ApiOpenAICodexResponses), model.Provider, model.ID)

		// TODO: Implement Codex streaming:
		// 1. Acquire/refresh JWT token from chatgpt.com/backend-api
		// 2. Determine transport (SSE vs WebSocket)
		// 3. For SSE: use Responses API streaming
		// 4. For WebSocket: connect to wss:// and handle binary frames
		// 5. Process response events
		// 6. Handle session cleanup

		errMsg := "OpenAI Codex Responses provider not yet implemented"
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

// StreamSimpleOpenAICodexResponses streams with simplified options.
func StreamSimpleOpenAICodexResponses(ctx context.Context, model *ai.Model, convCtx *ai.Context, options *ai.StreamOptions) (*ai.EventStream, error) {
	apiKey, err := ai.ResolveAPIKey(model, options)
	if err != nil {
		return nil, err
	}
	baseOpts := ai.BuildBaseOptions(model, nil, apiKey)
	return StreamOpenAICodexResponses(ctx, model, convCtx, &baseOpts)
}
