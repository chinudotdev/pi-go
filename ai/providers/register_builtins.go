package providers

import (
	"github.com/chinudotdev/pi-go/ai"
)

// RegisterBuiltInApiProviders registers all built-in API providers.
// This must be called before using Stream/Complete/StreamSimple/CompleteSimple.
// Typically called from an init() function in the consumer package.
func RegisterBuiltInApiProviders() {
	// Faux provider (for testing)
	ai.RegisterApiProvider(ai.ApiProvider{
		API:          "faux",
		Stream:       StreamFaux,
		StreamSimple: StreamSimpleFaux,
	})

	ai.RegisterApiProvider(ai.ApiProvider{
		API:          string(ai.ApiAnthropicMessages),
		Stream:       StreamAnthropic,
		StreamSimple: StreamSimpleAnthropic,
	})

	ai.RegisterApiProvider(ai.ApiProvider{
		API:          string(ai.ApiOpenAICompletions),
		Stream:       StreamOpenAICompletions,
		StreamSimple: StreamSimpleOpenAICompletions,
	})

	ai.RegisterApiProvider(ai.ApiProvider{
		API:          string(ai.ApiOpenAIResponses),
		Stream:       StreamOpenAIResponses,
		StreamSimple: StreamSimpleOpenAIResponses,
	})

	ai.RegisterApiProvider(ai.ApiProvider{
		API:          string(ai.ApiOpenAICodexResponses),
		Stream:       StreamOpenAICodexResponses,
		StreamSimple: StreamSimpleOpenAICodexResponses,
	})

	ai.RegisterApiProvider(ai.ApiProvider{
		API:          string(ai.ApiAzureOpenAIResponses),
		Stream:       StreamAzureOpenAIResponses,
		StreamSimple: StreamSimpleAzureOpenAIResponses,
	})

	ai.RegisterApiProvider(ai.ApiProvider{
		API:          string(ai.ApiGoogleGenerativeAI),
		Stream:       StreamGoogle,
		StreamSimple: StreamSimpleGoogle,
	})

	ai.RegisterApiProvider(ai.ApiProvider{
		API:          string(ai.ApiGoogleVertex),
		Stream:       StreamGoogleVertex,
		StreamSimple: StreamSimpleGoogleVertex,
	})

	ai.RegisterApiProvider(ai.ApiProvider{
		API:          string(ai.ApiBedrockConverseStream),
		Stream:       StreamBedrock,
		StreamSimple: StreamSimpleBedrock,
	})

	ai.RegisterApiProvider(ai.ApiProvider{
		API:          string(ai.ApiMistralConversations),
		Stream:       StreamMistral,
		StreamSimple: StreamSimpleMistral,
	})
}

// ResetApiProviders clears and re-registers all built-in providers.
func ResetApiProviders() {
	ai.ClearApiProviders()
	RegisterBuiltInApiProviders()
}
