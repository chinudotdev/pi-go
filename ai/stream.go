package ai

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// GetEnvApiKey resolves an API key for a provider from environment variables.
// Returns empty string and false if no key is found.
func GetEnvApiKey(provider Provider) (string, bool) {
	envVars := getEnvVarsForProvider(provider)
	for _, envVar := range envVars {
		if val := os.Getenv(envVar); val != "" {
			return val, true
		}
	}

	// Special handling for providers with ambient credentials
	switch provider {
	case "google-vertex":
		return resolveVertexCredentials()
	case "amazon-bedrock":
		return resolveBedrockCredentials()
	}

	return "", false
}

// FindEnvKeys returns the environment variable names that are set for a provider.
func FindEnvKeys(provider string) []string {
	envVars := getEnvVarsForProvider(provider)
	var found []string
	for _, envVar := range envVars {
		if os.Getenv(envVar) != "" {
			found = append(found, envVar)
		}
	}
	return found
}

// getEnvVarsForProvider returns the env var names for a given provider.
func getEnvVarsForProvider(provider string) []string {
	switch provider {
	case "github-copilot":
		return []string{"COPILOT_GITHUB_TOKEN"}
	case "anthropic":
		return []string{"ANTHROPIC_OAUTH_TOKEN", "ANTHROPIC_API_KEY"}
	}

	envMap := map[string]string{
		"openai":                 "OPENAI_API_KEY",
		"azure-openai-responses": "AZURE_OPENAI_API_KEY",
		"deepseek":               "DEEPSEEK_API_KEY",
		"google":                 "GEMINI_API_KEY",
		"google-vertex":          "GOOGLE_CLOUD_API_KEY",
		"groq":                   "GROQ_API_KEY",
		"cerebras":               "CEREBRAS_API_KEY",
		"xai":                    "XAI_API_KEY",
		"openrouter":             "OPENROUTER_API_KEY",
		"vercel-ai-gateway":      "AI_GATEWAY_API_KEY",
		"zai":                    "ZAI_API_KEY",
		"mistral":                "MISTRAL_API_KEY",
		"minimax":                "MINIMAX_API_KEY",
		"minimax-cn":             "MINIMAX_CN_API_KEY",
		"moonshotai":             "MOONSHOT_API_KEY",
		"moonshotai-cn":          "MOONSHOT_API_KEY",
		"huggingface":            "HF_TOKEN",
		"fireworks":              "FIREWORKS_API_KEY",
		"together":               "TOGETHER_API_KEY",
		"opencode":               "OPENCODE_API_KEY",
		"opencode-go":            "OPENCODE_API_KEY",
		"kimi-coding":            "KIMI_API_KEY",
		"cloudflare-workers-ai":  "CLOUDFLARE_API_KEY",
		"cloudflare-ai-gateway":  "CLOUDFLARE_API_KEY",
		"xiaomi":                 "XIAOMI_API_KEY",
		"xiaomi-token-plan-cn":   "XIAOMI_TOKEN_PLAN_CN_API_KEY",
		"xiaomi-token-plan-ams":  "XIAOMI_TOKEN_PLAN_AMS_API_KEY",
		"xiaomi-token-plan-sgp":  "XIAOMI_TOKEN_PLAN_SGP_API_KEY",
	}

	if envVar, ok := envMap[provider]; ok {
		return []string{envVar}
	}
	return nil
}

func resolveVertexCredentials() (string, bool) {
	if key := os.Getenv("GOOGLE_CLOUD_API_KEY"); key != "" {
		return key, true
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	adcPath := homeDir + "/.config/gcloud/application_default_credentials.json"
	if _, err := os.Stat(adcPath); err == nil {
		project := os.Getenv("GOOGLE_CLOUD_PROJECT")
		if project == "" {
			project = os.Getenv("GCLOUD_PROJECT")
		}
		location := os.Getenv("GOOGLE_CLOUD_LOCATION")
		if project != "" && location != "" {
			return "<authenticated>", true
		}
	}
	return "", false
}

func resolveBedrockCredentials() (string, bool) {
	if os.Getenv("AWS_PROFILE") != "" {
		return "<authenticated>", true
	}
	if os.Getenv("AWS_ACCESS_KEY_ID") != "" && os.Getenv("AWS_SECRET_ACCESS_KEY") != "" {
		return "<authenticated>", true
	}
	if os.Getenv("AWS_BEARER_TOKEN_BEDROCK") != "" {
		return "<authenticated>", true
	}
	if os.Getenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI") != "" {
		return "<authenticated>", true
	}
	if os.Getenv("AWS_CONTAINER_CREDENTIALS_FULL_URI") != "" {
		return "<authenticated>", true
	}
	if os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE") != "" {
		return "<authenticated>", true
	}
	return "", false
}

// ResolveAPIKey attempts to find an API key for a model's provider.
// If one is provided in options, it takes precedence.
func ResolveAPIKey(model *Model, options *StreamOptions) (string, error) {
	if options != nil && options.APIKey != nil && strings.TrimSpace(*options.APIKey) != "" {
		return *options.APIKey, nil
	}
	if key, ok := GetEnvApiKey(model.Provider); ok {
		return key, nil
	}
	return "", fmt.Errorf("no API key configured for provider: %s", model.Provider)
}

// withEnvAPIKey ensures options has an API key from environment if not explicitly set.
func withEnvAPIKey(model *Model, options *StreamOptions) *StreamOptions {
	if options != nil && options.APIKey != nil && strings.TrimSpace(*options.APIKey) != "" {
		return options
	}
	if key, ok := GetEnvApiKey(model.Provider); ok {
		if options == nil {
			opts := defaultStreamOptions()
			opts.APIKey = &key
			return &opts
		}
		clone := *options
		clone.APIKey = &key
		return &clone
	}
	return options
}

func defaultStreamOptions() StreamOptions {
	retries := 0
	delay := 60000
	return StreamOptions{
		MaxRetries:      &retries,
		MaxRetryDelayMs: &delay,
	}
}

// Stream sends a streaming request using the provider-specific stream function.
func Stream(ctx context.Context, model *Model, convCtx *Context, options *StreamOptions) (*EventStream, error) {
	provider, err := ResolveApiProvider(model.API)
	if err != nil {
		return nil, err
	}
	opts := withEnvAPIKey(model, options)
	return provider.Stream(ctx, model, convCtx, opts)
}

// Complete sends a streaming request and returns the final assistant message.
func Complete(ctx context.Context, model *Model, convCtx *Context, options *StreamOptions) (*AssistantMessage, error) {
	stream, err := Stream(ctx, model, convCtx, options)
	if err != nil {
		return nil, err
	}
	select {
	case msg := <-stream.Result:
		return &msg, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// StreamSimple sends a streaming request using the simplified options interface.
// It maps the Reasoning level to a provider-appropriate form and delegates
// to the registered provider's StreamSimple function.
func StreamSimple(ctx context.Context, model *Model, convCtx *Context, options *SimpleStreamOptions) (*EventStream, error) {
	provider, err := ResolveApiProvider(model.API)
	if err != nil {
		return nil, err
	}

	// Resolve API key and build base options
	apiKey, err := ResolveAPIKey(model, &options.StreamOptions)
	if err != nil {
		return nil, err
	}
	baseOpts := BuildBaseOptions(model, options, apiKey)

	return provider.StreamSimple(ctx, model, convCtx, &baseOpts)
}

// CompleteSimple sends a streaming request with simple options and returns the final message.
func CompleteSimple(ctx context.Context, model *Model, convCtx *Context, options *SimpleStreamOptions) (*AssistantMessage, error) {
	stream, err := StreamSimple(ctx, model, convCtx, options)
	if err != nil {
		return nil, err
	}
	select {
	case msg := <-stream.Result:
		return &msg, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
