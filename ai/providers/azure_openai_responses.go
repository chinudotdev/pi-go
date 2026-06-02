package providers

// Azure OpenAI Responses API provider.
// Wraps the OpenAI Responses provider with Azure-specific authentication.
//
// Dependencies needed:
//   - github.com/openai/openai-go  (official OpenAI Go SDK)
//   - github.com/Azure/azure-sdk-for-go/sdk/azidentity  (for Entra ID auth)
//
// Supports:
//   - Azure API key and Entra ID (Azure AD) authentication
//   - Azure-specific URL format: {endpoint}/openai/deployments/{deployment}
//   - API version query parameter

import (
	"context"

	"github.com/chinudotdev/pi-go/ai"
)

// StreamAzureOpenAIResponses streams from Azure OpenAI.
func StreamAzureOpenAIResponses(ctx context.Context, model *ai.Model, convCtx *ai.Context, options *ai.StreamOptions) (*ai.EventStream, error) {
	stream := ai.NewEventStream(ctx)

	go func() {
		output := ai.NewAssistantOutput(string(ai.ApiAzureOpenAIResponses), model.Provider, model.ID)

		// TODO: Implement Azure OpenAI:
		// 1. Build Azure-specific URL with deployment name and api-version
		// 2. Support both API key and Azure AD token authentication
		// 3. Use OpenAI Responses API format with Azure endpoint
		// 4. Process response stream events

		errMsg := "Azure OpenAI Responses provider not yet implemented"
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

// StreamSimpleAzureOpenAIResponses streams with simplified options.
func StreamSimpleAzureOpenAIResponses(ctx context.Context, model *ai.Model, convCtx *ai.Context, options *ai.StreamOptions) (*ai.EventStream, error) {
	apiKey, err := ai.ResolveAPIKey(model, options)
	if err != nil {
		return nil, err
	}
	baseOpts := ai.BuildBaseOptions(model, nil, apiKey)
	return StreamAzureOpenAIResponses(ctx, model, convCtx, &baseOpts)
}

