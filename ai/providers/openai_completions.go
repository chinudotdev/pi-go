package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"

	"github.com/chinudotdev/pi-go/ai"
)

// ============================================================================
// Compat detection
// ============================================================================

// resolvedCompat holds fully resolved compatibility flags for a model.
type resolvedCompat struct {
	SupportsStore                               bool
	SupportsDeveloperRole                       bool
	SupportsReasoningEffort                     bool
	SupportsUsageInStreaming                    bool
	MaxTokensField                              string // "max_completion_tokens" or "max_tokens"
	RequiresToolResultName                      bool
	RequiresAssistantAfterToolResult            bool
	RequiresThinkingAsText                      bool
	RequiresReasoningContentOnAssistantMessages bool
	ThinkingFormat                              string // "openai", "openrouter", "deepseek", "together", "zai", "qwen", "qwen-chat-template", "string-thinking"
	SupportsStrictMode                          bool
	CacheControlFormat                          string // "" or "anthropic"
	SendSessionAffinityHeaders                  bool
	SupportsLongCacheRetention                  bool
}

func detectCompat(model *ai.Model) resolvedCompat {
	provider := model.Provider
	baseURL := model.BaseURL

	isZAI := provider == "zai" || strings.Contains(baseURL, "api.z.ai")
	isTogether := provider == "together" || strings.Contains(baseURL, "api.together.ai") || strings.Contains(baseURL, "api.together.xyz")
	isMoonshot := provider == "moonshotai" || provider == "moonshotai-cn" || strings.Contains(baseURL, "api.moonshot.")
	isOpenRouter := provider == "openrouter" || strings.Contains(baseURL, "openrouter.ai")
	isCFWorkersAI := provider == "cloudflare-workers-ai" || strings.Contains(baseURL, "api.cloudflare.com")
	isCFGateway := provider == "cloudflare-ai-gateway" || strings.Contains(baseURL, "gateway.ai.cloudflare.com")

	isNonStandard := provider == "cerebras" || strings.Contains(baseURL, "cerebras.ai") ||
		provider == "xai" || strings.Contains(baseURL, "api.x.ai") ||
		isTogether || strings.Contains(baseURL, "chutes.ai") ||
		strings.Contains(baseURL, "deepseek.com") ||
		isZAI || isMoonshot ||
		provider == "opencode" || strings.Contains(baseURL, "opencode.ai") ||
		isCFWorkersAI || isCFGateway

	useMaxTokens := strings.Contains(baseURL, "chutes.ai") || isMoonshot || isCFGateway || isTogether

	isGrok := provider == "xai" || strings.Contains(baseURL, "api.x.ai")
	isDeepSeek := provider == "deepseek" || strings.Contains(baseURL, "deepseek.com")

	cacheControlFmt := ""
	if provider == "openrouter" && strings.HasPrefix(model.ID, "anthropic/") {
		cacheControlFmt = "anthropic"
	}

	thinkingFmt := "openai"
	if isDeepSeek {
		thinkingFmt = "deepseek"
	} else if isZAI {
		thinkingFmt = "zai"
	} else if isTogether {
		thinkingFmt = "together"
	} else if isOpenRouter {
		thinkingFmt = "openrouter"
	}

	maxTokensField := "max_completion_tokens"
	if useMaxTokens {
		maxTokensField = "max_tokens"
	}

	return resolvedCompat{
		SupportsStore:                               !isNonStandard,
		SupportsDeveloperRole:                       !isNonStandard && !isOpenRouter,
		SupportsReasoningEffort:                     !isGrok && !isZAI && !isMoonshot && !isTogether && !isCFGateway,
		SupportsUsageInStreaming:                    true,
		MaxTokensField:                              maxTokensField,
		RequiresToolResultName:                      false,
		RequiresAssistantAfterToolResult:            false,
		RequiresThinkingAsText:                      false,
		RequiresReasoningContentOnAssistantMessages: isDeepSeek,
		ThinkingFormat:                              thinkingFmt,
		SupportsStrictMode:                          !isMoonshot && !isTogether && !isCFGateway,
		CacheControlFormat:                          cacheControlFmt,
		SendSessionAffinityHeaders:                  false,
		SupportsLongCacheRetention:                  !(isTogether || isCFWorkersAI || isCFGateway),
	}
}

// getCompat returns resolved compat settings, merging model-level overrides.
func getCompat(model *ai.Model) resolvedCompat {
	detected := detectCompat(model)
	if model.Compat == nil {
		return detected
	}

	// Try to extract OpenAICompletionsCompat fields from model.Compat (any)
	data, _ := json.Marshal(model.Compat)
	var raw map[string]any
	if json.Unmarshal(data, &raw) != nil {
		return detected
	}

	// Helper: extract bool with default
	boolOr := func(key string, def bool) bool {
		if v, ok := raw[key]; ok && v != nil {
			if b, ok := v.(bool); ok {
				return b
			}
		}
		return def
	}
	strOr := func(key, def string) string {
		if v, ok := raw[key]; ok && v != nil {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
		return def
	}

	return resolvedCompat{
		SupportsStore:                               boolOr("supportsStore", detected.SupportsStore),
		SupportsDeveloperRole:                       boolOr("supportsDeveloperRole", detected.SupportsDeveloperRole),
		SupportsReasoningEffort:                     boolOr("supportsReasoningEffort", detected.SupportsReasoningEffort),
		SupportsUsageInStreaming:                    boolOr("supportsUsageInStreaming", detected.SupportsUsageInStreaming),
		MaxTokensField:                              strOr("maxTokensField", detected.MaxTokensField),
		RequiresToolResultName:                      boolOr("requiresToolResultName", detected.RequiresToolResultName),
		RequiresAssistantAfterToolResult:            boolOr("requiresAssistantAfterToolResult", detected.RequiresAssistantAfterToolResult),
		RequiresThinkingAsText:                      boolOr("requiresThinkingAsText", detected.RequiresThinkingAsText),
		RequiresReasoningContentOnAssistantMessages: boolOr("requiresReasoningContentOnAssistantMessages", detected.RequiresReasoningContentOnAssistantMessages),
		ThinkingFormat:                              strOr("thinkingFormat", detected.ThinkingFormat),
		SupportsStrictMode:                          boolOr("supportsStrictMode", detected.SupportsStrictMode),
		CacheControlFormat:                          strOr("cacheControlFormat", detected.CacheControlFormat),
		SendSessionAffinityHeaders:                  boolOr("sendSessionAffinityHeaders", detected.SendSessionAffinityHeaders),
		SupportsLongCacheRetention:                  boolOr("supportsLongCacheRetention", detected.SupportsLongCacheRetention),
	}
}

// ============================================================================
// Client creation
// ============================================================================

func createOpenAIClient(model *ai.Model, apiKey string, optionsHeaders map[string]string, sessionID string, compat resolvedCompat) openai.Client {
	var opts []option.RequestOption
	opts = append(opts, option.WithAPIKey(apiKey))

	if model.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(model.BaseURL))
	}

	// Merge model-level headers
	for k, v := range model.Headers {
		opts = append(opts, option.WithHeader(k, v))
	}

	// Session affinity headers
	if sessionID != "" && compat.SendSessionAffinityHeaders {
		opts = append(opts,
			option.WithHeader("session_id", sessionID),
			option.WithHeader("x-client-request-id", sessionID),
			option.WithHeader("x-session-affinity", sessionID),
		)
	}

	// Options headers (can override defaults)
	for k, v := range optionsHeaders {
		opts = append(opts, option.WithHeader(k, v))
	}

	return openai.NewClient(opts...)
}

// ============================================================================
// Params building
// ============================================================================

func buildOpenAIParams(model *ai.Model, convCtx *ai.Context, options *ai.StreamOptions, compat resolvedCompat, cacheRetention ai.CacheRetention) openai.ChatCompletionNewParams {
	messages := convertMessages(model, convCtx, compat)

	params := openai.ChatCompletionNewParams{
		Model:    model.ID,
		Messages: messages,
	}

	if compat.SupportsUsageInStreaming {
		params.StreamOptions = openai.ChatCompletionStreamOptionsParam{
			IncludeUsage: param.NewOpt(true),
		}
	}

	if compat.SupportsStore {
		params.Store = param.NewOpt(false)
	}

	if options != nil && options.MaxTokens != nil {
		if compat.MaxTokensField == "max_tokens" {
			params.MaxTokens = param.NewOpt(int64(*options.MaxTokens))
		} else {
			params.MaxCompletionTokens = param.NewOpt(int64(*options.MaxTokens))
		}
	}

	if options != nil && options.Temperature != nil {
		params.Temperature = param.NewOpt(*options.Temperature)
	}

	// Tools
	if convCtx.Tools != nil && len(convCtx.Tools) > 0 {
		params.Tools = convertTools(convCtx.Tools, compat)
	} else if hasToolHistory(convCtx.Messages) {
		params.Tools = []openai.ChatCompletionToolParam{}
	}

	// Apply thinking/reasoning config
	if options != nil {
		applyThinkingConfig(&params, model, options, compat)
	}

	// Max retries
	if options != nil && options.MaxRetries != nil {
		// Applied at request time via option
	}

	return params
}

func applyThinkingConfig(params *openai.ChatCompletionNewParams, model *ai.Model, options *ai.StreamOptions, compat resolvedCompat) {
	// Determine if reasoning effort is set via options metadata
	reasoningEffort := ""
	if options.Metadata != nil {
		if v, ok := options.Metadata["reasoningEffort"]; ok {
			if s, ok := v.(string); ok {
				reasoningEffort = s
			}
		}
	}

	if !model.Reasoning {
		return
	}

	// Map the reasoning effort to the provider-specific format
	if reasoningEffort != "" {
		mapped := mapThinkingEffort(model, reasoningEffort)
		if compat.SupportsReasoningEffort {
			params.ReasoningEffort = openai.ReasoningEffort(mapped)
		}
	}
}

func mapThinkingEffort(model *ai.Model, level string) string {
	// Try the model's ThinkingLevelMap first
	var mtl ai.ModelThinkingLevel
	switch level {
	case "minimal":
		mtl = ai.ThinkingMMin
	case "low":
		mtl = ai.ThinkingMLow
	case "medium":
		mtl = ai.ThinkingMMedium
	case "high":
		mtl = ai.ThinkingMHigh
	case "xhigh":
		mtl = ai.ThinkingMXHigh
	default:
		return level
	}

	if model.ThinkingLevelMap != nil {
		if mapped, ok := model.ThinkingLevelMap[mtl]; ok && mapped != nil && *mapped != "" {
			return *mapped
		}
	}
	return level
}

// ============================================================================
// Message conversion
// ============================================================================

func convertMessages(model *ai.Model, convCtx *ai.Context, compat resolvedCompat) []openai.ChatCompletionMessageParamUnion {
	var params []openai.ChatCompletionMessageParamUnion

	normalizeToolCallID := func(id string) string {
		if strings.Contains(id, "|") {
			parts := strings.SplitN(id, "|", 2)
			callID := parts[0]
			// Sanitize to allowed chars and truncate to 40
			callID = strings.Map(func(r rune) rune {
				if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
					return r
				}
				return '_'
			}, callID)
			if len(callID) > 40 {
				callID = callID[:40]
			}
			return callID
		}
		if model.Provider == "openai" && len(id) > 40 {
			return id[:40]
		}
		return id
	}

	transformed := ai.TransformMessages(convCtx.Messages, model, func(id string, m *ai.Model, _ *ai.AssistantMessage) string {
		return normalizeToolCallID(id)
	})

	// System prompt
	if convCtx.SystemPrompt != nil && *convCtx.SystemPrompt != "" {
		text := ai.SanitizeSurrogates(*convCtx.SystemPrompt)
		if model.Reasoning && compat.SupportsDeveloperRole {
			params = append(params, openai.ChatCompletionMessageParamUnion{
				OfDeveloper: &openai.ChatCompletionDeveloperMessageParam{
					Content: openai.ChatCompletionDeveloperMessageParamContentUnion{
						OfString: param.NewOpt(text),
					},
				},
			})
		} else {
			params = append(params, openai.ChatCompletionMessageParamUnion{
				OfSystem: &openai.ChatCompletionSystemMessageParam{
					Content: openai.ChatCompletionSystemMessageParamContentUnion{
						OfString: param.NewOpt(text),
					},
				},
			})
		}
	}

	lastRole := ""
	for i, msg := range transformed {
		// Insert synthetic assistant between tool result and user message for some providers
		if compat.RequiresAssistantAfterToolResult && lastRole == "toolResult" && msg.Role == "user" {
			params = append(params, openai.ChatCompletionMessageParamUnion{
				OfAssistant: &openai.ChatCompletionAssistantMessageParam{
					Content: openai.ChatCompletionAssistantMessageParamContentUnion{
						OfString: param.NewOpt("I have processed the tool results."),
					},
				},
			})
		}

		switch msg.Role {
		case "user":
			if str, ok := msg.Content.(string); ok {
				params = append(params, openai.ChatCompletionMessageParamUnion{
					OfUser: &openai.ChatCompletionUserMessageParam{
						Content: openai.ChatCompletionUserMessageParamContentUnion{
							OfString: param.NewOpt(ai.SanitizeSurrogates(str)),
						},
					},
				})
			} else if blocks, ok := msg.Content.([]ai.ContentBlock); ok && len(blocks) > 0 {
				var parts []openai.ChatCompletionContentPartUnionParam
				for _, block := range blocks {
					switch block.Type {
					case "text":
						parts = append(parts, openai.ChatCompletionContentPartUnionParam{
							OfText: &openai.ChatCompletionContentPartTextParam{
								Text: ai.SanitizeSurrogates(block.Text),
							},
						})
					case "image":
						// Use available fields for image data
						mimeType := block.ToolCallName // reused for mime type in image blocks
						data := block.Thinking         // reused for base64 data
						if mimeType != "" && data != "" {
							parts = append(parts, openai.ChatCompletionContentPartUnionParam{
								OfImageURL: &openai.ChatCompletionContentPartImageParam{
									ImageURL: openai.ChatCompletionContentPartImageImageURLParam{
										URL: fmt.Sprintf("data:%s;base64,%s", mimeType, data),
									},
								},
							})
						}
					}
				}
				if len(parts) > 0 {
					params = append(params, openai.ChatCompletionMessageParamUnion{
						OfUser: &openai.ChatCompletionUserMessageParam{
							Content: openai.ChatCompletionUserMessageParamContentUnion{
								OfArrayOfContentParts: parts,
							},
						},
					})
				}
			}
			lastRole = "user"

		case "assistant":
			var textParts []string
			var thinkingBlocks []ai.ContentBlock
			var toolCalls []ai.ContentBlock

			for _, block := range msg.AssistantContent {
				switch block.Type {
				case "text":
					if strings.TrimSpace(block.Text) != "" {
						textParts = append(textParts, block.Text)
					}
				case "thinking":
					if strings.TrimSpace(block.Thinking) != "" {
						thinkingBlocks = append(thinkingBlocks, block)
					}
				case "toolCall":
					toolCalls = append(toolCalls, block)
				}
			}

			assistantMsg := &openai.ChatCompletionAssistantMessageParam{}

			// Handle content
			assistantText := strings.Join(textParts, "")

			if len(thinkingBlocks) > 0 && compat.RequiresThinkingAsText {
				// Convert thinking to plain text
				var thinkingTexts []string
				for _, b := range thinkingBlocks {
					thinkingTexts = append(thinkingTexts, ai.SanitizeSurrogates(b.Thinking))
				}
				thinkingText := strings.Join(thinkingTexts, "\n\n")
				combined := thinkingText + "\n" + assistantText
				assistantMsg.Content = openai.ChatCompletionAssistantMessageParamContentUnion{
					OfString: param.NewOpt(ai.SanitizeSurrogates(combined)),
				}
			} else if assistantText != "" {
				assistantMsg.Content = openai.ChatCompletionAssistantMessageParamContentUnion{
					OfString: param.NewOpt(ai.SanitizeSurrogates(assistantText)),
				}
			}

			// Tool calls
			if len(toolCalls) > 0 {
				var tcParams []openai.ChatCompletionMessageToolCallParam
				for _, tc := range toolCalls {
					argsJSON, _ := json.Marshal(tc.ToolCallArguments)
					tcParams = append(tcParams, openai.ChatCompletionMessageToolCallParam{
						ID: tc.ToolCallID,
						Function: openai.ChatCompletionMessageToolCallFunctionParam{
							Arguments: string(argsJSON),
							Name:      tc.ToolCallName,
						},
					})
				}
				assistantMsg.ToolCalls = tcParams
			}

			// Skip empty assistant messages with no tool calls
			hasContent := assistantText != "" || len(thinkingBlocks) > 0
			if !hasContent && len(toolCalls) == 0 {
				continue
			}

			params = append(params, openai.ChatCompletionMessageParamUnion{
				OfAssistant: assistantMsg,
			})
			lastRole = "assistant"

		case "toolResult":
			textResult := ""
			for _, block := range msg.ToolResultContent {
				if block.Type == "text" {
					if textResult != "" {
						textResult += "\n"
					}
					textResult += block.Text
				}
			}

			if textResult == "" {
				textResult = "(see attached image)"
			}

			params = append(params, openai.ChatCompletionMessageParamUnion{
				OfTool: &openai.ChatCompletionToolMessageParam{
					Content: openai.ChatCompletionToolMessageParamContentUnion{
						OfString: param.NewOpt(ai.SanitizeSurrogates(textResult)),
					},
					ToolCallID: msg.ToolCallID,
				},
			})
			lastRole = "toolResult"
		}

		_ = i // suppress unused warning
	}

	return params
}

func convertTools(tools []ai.Tool, compat resolvedCompat) []openai.ChatCompletionToolParam {
	result := make([]openai.ChatCompletionToolParam, len(tools))
	for i, tool := range tools {
		t := openai.ChatCompletionToolParam{
			Function: openai.FunctionDefinitionParam{
				Name:        tool.Name,
				Description: param.NewOpt(tool.Description),
				Parameters:  tool.Parameters.(map[string]any),
			},
		}
		if compat.SupportsStrictMode {
			t.Function.Strict = param.NewOpt(false)
		}
		result[i] = t
	}
	return result
}

func hasToolHistory(messages []ai.Message) bool {
	for _, msg := range messages {
		if msg.Role == "toolResult" {
			return true
		}
		if msg.Role == "assistant" {
			for _, block := range msg.AssistantContent {
				if block.Type == "toolCall" {
					return true
				}
			}
		}
	}
	return false
}

// ============================================================================
// Usage parsing
// ============================================================================

func parseChunkUsage(raw openai.CompletionUsage, model *ai.Model) ai.Usage {
	promptTokens := raw.PromptTokens
	cacheReadTokens := raw.PromptTokensDetails.CachedTokens
	cacheWriteTokens := int64(0) // OpenAI doesn't expose cache_write_tokens in the standard struct

	inputTokens := promptTokens - cacheReadTokens - cacheWriteTokens
	if inputTokens < 0 {
		inputTokens = 0
	}
	outputTokens := raw.CompletionTokens

	usage := ai.Usage{
		Input:       int(inputTokens),
		Output:      int(outputTokens),
		CacheRead:   int(cacheReadTokens),
		CacheWrite:  int(cacheWriteTokens),
		TotalTokens: int(inputTokens + outputTokens + cacheReadTokens + cacheWriteTokens),
	}
	usage.Cost = ai.CalculateCost(model, usage)
	return usage
}

// ============================================================================
// Stop reason mapping
// ============================================================================

func mapStopReason(reason string) (ai.StopReason, *string) {
	switch reason {
	case "stop", "end", "":
		return ai.StopReasonStop, nil
	case "length":
		return ai.StopReasonLength, nil
	case "function_call", "tool_calls":
		return ai.StopReasonToolUse, nil
	case "content_filter":
		msg := "Provider finish_reason: content_filter"
		return ai.StopReasonError, &msg
	case "network_error":
		msg := "Provider finish_reason: network_error"
		return ai.StopReasonError, &msg
	default:
		msg := fmt.Sprintf("Provider finish_reason: %s", reason)
		return ai.StopReasonError, &msg
	}
}

// ============================================================================
// Streaming tool call state
// ============================================================================

type streamingToolCallBlock struct {
	ai.ContentBlock
	partialArgs string
	streamIndex int
}

// intPtr returns a pointer to an int.
func intPtr(v int) *int { return &v }

// ============================================================================
// StreamOpenAICompletions — main streaming implementation
// ============================================================================

// StreamOpenAICompletions streams responses from the OpenAI Chat Completions API
// and compatible endpoints.
func StreamOpenAICompletions(ctx context.Context, model *ai.Model, convCtx *ai.Context, options *ai.StreamOptions) (*ai.EventStream, error) {
	stream := ai.NewEventStream(ctx)

	go func() {
		output := ai.NewAssistantOutput(string(ai.ApiOpenAICompletions), model.Provider, model.ID)

		defer func() {
			stream.End(output)
		}()

		// Validate API key
		apiKey := ""
		if options != nil && options.APIKey != nil {
			apiKey = *options.APIKey
		}
		if apiKey == "" {
			errMsg := fmt.Sprintf("No API key for provider: %s", model.Provider)
			output.StopReason = ai.StopReasonError
			output.ErrorMessage = &errMsg
			stream.Push(ai.AssistantMessageEvent{
				Type:  "error",
				Error: &output,
			})
			return
		}

		compat := getCompat(model)
		cacheRetention := ai.CacheShort
		if options != nil && options.CacheRetention != "" {
			cacheRetention = options.CacheRetention
		}
		sessionID := ""
		if options != nil && options.SessionID != nil {
			sessionID = *options.SessionID
		}

		headers := map[string]string{}
		if options != nil && options.Headers != nil {
			headers = options.Headers
		}

		client := createOpenAIClient(model, apiKey, headers, sessionID, compat)
		params := buildOpenAIParams(model, convCtx, options, compat, cacheRetention)

		// Build request options
		var reqOpts []option.RequestOption
		if options != nil && options.TimeoutMs != nil {
			reqOpts = append(reqOpts, option.WithRequestTimeout(time.Duration(*options.TimeoutMs)*time.Millisecond))
		}
		if options != nil && options.MaxRetries != nil {
			reqOpts = append(reqOpts, option.WithMaxRetries(*options.MaxRetries))
		} else {
			reqOpts = append(reqOpts, option.WithMaxRetries(0))
		}

		// Start streaming
		sseStream := client.Chat.Completions.NewStreaming(ctx, params, reqOpts...)
		defer sseStream.Close()

		// Streaming state
		var textBlock *ai.ContentBlock
		var thinkingBlock *ai.ContentBlock
		toolCallBlocksByIndex := make(map[int]*streamingToolCallBlock)
		toolCallBlocksByID := make(map[string]*streamingToolCallBlock)
		hasFinishReason := false

		contentIndex := func(block *ai.ContentBlock) int {
			for i, b := range output.Content {
				if &b == block {
					return i
				}
			}
			return len(output.Content)
		}

		_ = contentIndex // used below

		// Push start event
		stream.Push(ai.AssistantMessageEvent{
			Type:    "start",
			Partial: &output,
		})

		for sseStream.Next() {
			chunk := sseStream.Current()

			// Capture response ID
			if output.ResponseID == nil && chunk.ID != "" {
				id := chunk.ID
				output.ResponseID = &id
			}

			// Capture response model
			if chunk.Model != "" && chunk.Model != model.ID && (output.ResponseModel == nil || *output.ResponseModel == "") {
				m := chunk.Model
				output.ResponseModel = &m
			}

			// Parse usage
			if chunk.Usage.CompletionTokens > 0 || chunk.Usage.PromptTokens > 0 {
				output.Usage = parseChunkUsage(chunk.Usage, model)
			}

			// Process choices
			if len(chunk.Choices) == 0 {
				continue
			}
			choice := chunk.Choices[0]

			// Finish reason
			if choice.FinishReason != "" {
				reason, errMsg := mapStopReason(choice.FinishReason)
				output.StopReason = reason
				if errMsg != nil {
					output.ErrorMessage = errMsg
				}
				hasFinishReason = true
			}

			delta := choice.Delta

			// Text content
			if delta.Content != "" {
				if textBlock == nil {
					block := ai.NewTextContent("")
					output.Content = append(output.Content, block)
					textBlock = &output.Content[len(output.Content)-1]
					stream.Push(ai.AssistantMessageEvent{
						Type:         "text_start",
						ContentIndex: intPtr(len(output.Content) - 1),
						Partial:      &output,
					})
				}
				textBlock.Text += delta.Content
				idx := len(output.Content) - 1
				stream.Push(ai.AssistantMessageEvent{
					Type:         "text_delta",
					ContentIndex: &idx,
					Delta:        &delta.Content,
					Partial:      &output,
				})
			}

			// Reasoning/thinking content (from provider-specific fields in extra JSON)
			// The SDK uses ExtraFields on the delta's JSON struct for unknown fields
			reasoningDelta := extractReasoningDelta(chunk)
			if reasoningDelta != "" {
				reasoningSig := extractReasoningField(chunk)
				if thinkingBlock == nil {
					block := ai.ContentBlock{Type: "thinking", Thinking: "", ThinkingSignature: &reasoningSig}
					output.Content = append(output.Content, block)
					thinkingBlock = &output.Content[len(output.Content)-1]
					stream.Push(ai.AssistantMessageEvent{
						Type:         "thinking_start",
						ContentIndex: intPtr(len(output.Content) - 1),
						Partial:      &output,
					})
				}
				thinkingBlock.Thinking += reasoningDelta
				idx := len(output.Content) - 1
				stream.Push(ai.AssistantMessageEvent{
					Type:         "thinking_delta",
					ContentIndex: &idx,
					Delta:        &reasoningDelta,
					Partial:      &output,
				})
			}

			// Tool calls
			for _, tc := range delta.ToolCalls {
				var block *streamingToolCallBlock
				streamIdx := int(tc.Index)

				// Find or create block
				if existing, ok := toolCallBlocksByIndex[streamIdx]; ok {
					block = existing
				} else if tc.ID != "" {
					if existing, ok := toolCallBlocksByID[tc.ID]; ok {
						block = existing
					}
				}

				if block == nil {
					cb := ai.NewToolCallContent(tc.ID, tc.Function.Name, map[string]any{})
					block = &streamingToolCallBlock{
						ContentBlock: cb,
						streamIndex:  streamIdx,
					}
					toolCallBlocksByIndex[streamIdx] = block
					if tc.ID != "" {
						toolCallBlocksByID[tc.ID] = block
					}
					output.Content = append(output.Content, block.ContentBlock)
					idx := len(output.Content) - 1
					stream.Push(ai.AssistantMessageEvent{
						Type:         "toolcall_start",
						ContentIndex: &idx,
						Partial:      &output,
					})
				}

				// Update ID and name
				if tc.ID != "" && block.ToolCallID == "" {
					block.ToolCallID = tc.ID
					toolCallBlocksByID[tc.ID] = block
				}
				if tc.Function.Name != "" && block.ToolCallName == "" {
					block.ToolCallName = tc.Function.Name
				}

				// Accumulate arguments
				if tc.Function.Arguments != "" {
					block.partialArgs += tc.Function.Arguments
					parsed, _ := ai.ParseStreamingJSON(block.partialArgs)
					if parsed != nil {
						block.ToolCallArguments = parsed
					}
				}

				idx := len(output.Content) - 1
				stream.Push(ai.AssistantMessageEvent{
					Type:         "toolcall_delta",
					ContentIndex: &idx,
					Partial:      &output,
				})
			}
		}

		// Check stream error
		if err := sseStream.Err(); err != nil {
			// Check if context was cancelled
			if ctx.Err() != nil {
				output.StopReason = ai.StopReasonAborted
				abortedMsg := "Request was aborted"
				output.ErrorMessage = &abortedMsg
			} else {
				errMsg := err.Error()
				output.StopReason = ai.StopReasonError
				output.ErrorMessage = &errMsg
			}
			stream.Push(ai.AssistantMessageEvent{
				Type:  "error",
				Error: &output,
			})
			return
		}

		if !hasFinishReason {
			errMsg := "Stream ended without finish_reason"
			output.StopReason = ai.StopReasonError
			output.ErrorMessage = &errMsg
			stream.Push(ai.AssistantMessageEvent{
				Type:  "error",
				Error: &output,
			})
			return
		}

		if output.StopReason == ai.StopReasonAborted || output.StopReason == ai.StopReasonError {
			stream.Push(ai.AssistantMessageEvent{
				Type:   "error",
				Reason: output.StopReason,
				Error:  &output,
			})
			return
		}

		// Push done
		stream.Push(ai.AssistantMessageEvent{
			Type:    "done",
			Reason:  output.StopReason,
			Message: &output,
		})
	}()

	return stream, nil
}

// extractReasoningDelta attempts to extract reasoning content from chunk extra fields.
// Different providers use different field names: reasoning_content, reasoning, reasoning_text.
func extractReasoningDelta(chunk openai.ChatCompletionChunk) string {
	// Check ExtraFields on the delta's JSON struct
	if chunk.JSON.ExtraFields != nil {
		return ""
	}

	// The SDK parses known fields; reasoning fields are extra.
	// We need to check the raw JSON for these fields.
	raw := chunk.RawJSON()
	if raw == "" {
		return ""
	}

	// Use gjson-like parsing on the choices[0].delta
	// Since the SDK doesn't expose extra delta fields easily, we parse raw JSON.
	// The chunk has Choices[0].Delta with known fields; reasoning fields are extras.
	// We look at the raw JSON directly.
	reasoningFields := []string{"reasoning_content", "reasoning", "reasoning_text"}
	for _, field := range reasoningFields {
		val := extractJSONFieldFromDelta(raw, field)
		if val != "" {
			return val
		}
	}
	return ""
}

// extractReasoningField returns which reasoning field name was found.
func extractReasoningField(chunk openai.ChatCompletionChunk) string {
	raw := chunk.RawJSON()
	if raw == "" {
		return "reasoning_content"
	}
	reasoningFields := []string{"reasoning_content", "reasoning", "reasoning_text"}
	for _, field := range reasoningFields {
		val := extractJSONFieldFromDelta(raw, field)
		if val != "" {
			return field
		}
	}
	return "reasoning_content"
}

// extractJSONFieldFromDelta extracts a string field from the delta in a raw chunk JSON.
func extractJSONFieldFromDelta(raw, field string) string {
	// Simple extraction: find "reasoning_content":"..." in the delta
	// We use a basic JSON approach since the SDK doesn't expose extra fields.
	decoder := json.NewDecoder(strings.NewReader(raw))
	// Use a generic map parse
	var obj map[string]any
	if err := decoder.Decode(&obj); err != nil {
		return ""
	}

	choices, ok := obj["choices"].([]any)
	if !ok || len(choices) == 0 {
		return ""
	}
	firstChoice, ok := choices[0].(map[string]any)
	if !ok {
		return ""
	}
	delta, ok := firstChoice["delta"].(map[string]any)
	if !ok {
		return ""
	}
	val, ok := delta[field].(string)
	if !ok || val == "" {
		return ""
	}
	return val
}

// StreamSimpleOpenAICompletions streams with simplified options, mapping reasoning levels.
func StreamSimpleOpenAICompletions(ctx context.Context, model *ai.Model, convCtx *ai.Context, options *ai.StreamOptions) (*ai.EventStream, error) {
	apiKey, err := ai.ResolveAPIKey(model, options)
	if err != nil {
		return nil, err
	}
	baseOpts := ai.BuildBaseOptions(model, nil, apiKey)
	return StreamOpenAICompletions(ctx, model, convCtx, &baseOpts)
}
