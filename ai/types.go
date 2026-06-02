package ai

import (
	"context"
	"time"
)

// Known API identifiers for supported LLM backends.
type KnownApi string

const (
	ApiOpenAICompletions     KnownApi = "openai-completions"
	ApiMistralConversations  KnownApi = "mistral-conversations"
	ApiOpenAIResponses       KnownApi = "openai-responses"
	ApiAzureOpenAIResponses  KnownApi = "azure-openai-responses"
	ApiOpenAICodexResponses  KnownApi = "openai-codex-responses"
	ApiAnthropicMessages     KnownApi = "anthropic-messages"
	ApiBedrockConverseStream KnownApi = "bedrock-converse-stream"
	ApiGoogleGenerativeAI    KnownApi = "google-generative-ai"
	ApiGoogleVertex          KnownApi = "google-vertex"
)

// Api is a string that can be a known API or a custom one.
type Api = string

// KnownProvider identifies a known LLM provider.
type KnownProvider string

const (
	ProviderAmazonBedrock       KnownProvider = "amazon-bedrock"
	ProviderAnthropic            KnownProvider = "anthropic"
	ProviderGoogle               KnownProvider = "google"
	ProviderGoogleVertex         KnownProvider = "google-vertex"
	ProviderOpenAI               KnownProvider = "openai"
	ProviderAzureOpenAIResponses KnownProvider = "azure-openai-responses"
	ProviderOpenAICodex          KnownProvider = "openai-codex"
	ProviderDeepSeek             KnownProvider = "deepseek"
	ProviderGitHubCopilot        KnownProvider = "github-copilot"
	ProviderXAI                  KnownProvider = "xai"
	ProviderGroq                 KnownProvider = "groq"
	ProviderCerebras             KnownProvider = "cerebras"
	ProviderOpenRouter           KnownProvider = "openrouter"
	ProviderVercelAIGateway      KnownProvider = "vercel-ai-gateway"
	ProviderZAI                  KnownProvider = "zai"
	ProviderMistral              KnownProvider = "mistral"
	ProviderMinimax              KnownProvider = "minimax"
	ProviderMinimaxCN            KnownProvider = "minimax-cn"
	ProviderMoonshotAI           KnownProvider = "moonshotai"
	ProviderMoonshotAICN         KnownProvider = "moonshotai-cn"
	ProviderHuggingFace          KnownProvider = "huggingface"
	ProviderFireworks            KnownProvider = "fireworks"
	ProviderTogether             KnownProvider = "together"
	ProviderOpenCode             KnownProvider = "opencode"
	ProviderOpenCodeGo           KnownProvider = "opencode-go"
	ProviderKimiCoding           KnownProvider = "kimi-coding"
	ProviderCloudflareWorkersAI  KnownProvider = "cloudflare-workers-ai"
	ProviderCloudflareAIGateway  KnownProvider = "cloudflare-ai-gateway"
	ProviderXiaomi               KnownProvider = "xiaomi"
	ProviderXiaomiTokenPlanCN    KnownProvider = "xiaomi-token-plan-cn"
	ProviderXiaomiTokenPlanAMS   KnownProvider = "xiaomi-token-plan-ams"
	ProviderXiaomiTokenPlanSGP   KnownProvider = "xiaomi-token-plan-sgp"
)

// Provider is a string that can be a known provider or a custom one.
type Provider = string

// ThinkingLevel represents the reasoning effort level.
type ThinkingLevel string

const (
	ThinkingMinimal ThinkingLevel = "minimal"
	ThinkingLow     ThinkingLevel = "low"
	ThinkingMedium  ThinkingLevel = "medium"
	ThinkingHigh    ThinkingLevel = "high"
	ThinkingXHigh   ThinkingLevel = "xhigh"
)

// ModelThinkingLevel extends ThinkingLevel with "off".
type ModelThinkingLevel string

const (
	ThinkingOff     ModelThinkingLevel = "off"
	ThinkingMMin    ModelThinkingLevel = "minimal"
	ThinkingMLow    ModelThinkingLevel = "low"
	ThinkingMMedium ModelThinkingLevel = "medium"
	ThinkingMHigh   ModelThinkingLevel = "high"
	ThinkingMXHigh  ModelThinkingLevel = "xhigh"
)

// ThinkingLevelMap maps pi thinking levels to provider-specific values.
// A nil value means the level is unsupported.
type ThinkingLevelMap map[ModelThinkingLevel]*string

// ThinkingBudgets defines token budgets for each thinking level.
type ThinkingBudgets struct {
	Minimal *int `json:"minimal,omitempty"`
	Low     *int `json:"low,omitempty"`
	Medium  *int `json:"medium,omitempty"`
	High    *int `json:"high,omitempty"`
}

// CacheRetention controls prompt cache duration.
type CacheRetention string

const (
	CacheNone  CacheRetention = "none"
	CacheShort CacheRetention = "short"
	CacheLong  CacheRetention = "long"
)

// Transport indicates the preferred streaming transport.
type Transport string

const (
	TransportSSE             Transport = "sse"
	TransportWebSocket       Transport = "websocket"
	TransportWebSocketCached Transport = "websocket-cached"
	TransportAuto            Transport = "auto"
)

// Model represents a specific LLM model configuration.
type Model struct {
	ID               string           `json:"id"`
	Name             string           `json:"name"`
	API              Api              `json:"api"`
	Provider         Provider         `json:"provider"`
	BaseURL          string           `json:"baseUrl"`
	Reasoning        bool             `json:"reasoning"`
	ThinkingLevelMap ThinkingLevelMap `json:"thinkingLevelMap,omitempty"`
	Input            []string         `json:"input"` // "text", "image"
	Cost             ModelCost        `json:"cost"`
	ContextWindow    int              `json:"contextWindow"`
	MaxTokens        int              `json:"maxTokens"`
	Headers          map[string]string `json:"headers,omitempty"`
	Compat           any              `json:"compat,omitempty"` // Provider-specific compat struct
}

// ModelCost represents the per-token cost for a model.
type ModelCost struct {
	Input      float64 `json:"input"`      // $/million tokens
	Output     float64 `json:"output"`     // $/million tokens
	CacheRead  float64 `json:"cacheRead"`  // $/million tokens
	CacheWrite float64 `json:"cacheWrite"` // $/million tokens
}

// Usage tracks token usage for a request.
type Usage struct {
	Input       int  `json:"input"`
	Output      int  `json:"output"`
	CacheRead   int  `json:"cacheRead"`
	CacheWrite  int  `json:"cacheWrite"`
	TotalTokens int  `json:"totalTokens"`
	Cost        Cost `json:"cost"`
}

// Cost tracks monetary cost breakdown.
type Cost struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite"`
	Total      float64 `json:"total"`
}

// StopReason indicates why the model stopped generating.
type StopReason string

const (
	StopReasonStop    StopReason = "stop"
	StopReasonLength  StopReason = "length"
	StopReasonToolUse StopReason = "toolUse"
	StopReasonError   StopReason = "error"
	StopReasonAborted StopReason = "aborted"
)

// ============================================================================
// Content blocks
// ============================================================================

// ContentBlock is a discriminated union for message content.
// Use the Type field ("text", "thinking", "toolCall") to determine which
// fields are populated. For type-safe access, use the AsText/AsThinking/AsToolCall methods.
type ContentBlock struct {
	Type      string `json:"type"` // "text", "thinking", "toolCall"

	// Populated when Type == "text"
	Text         string  `json:"text,omitempty"`
	TextSignature *string `json:"textSignature,omitempty"`

	// Populated when Type == "thinking"
	Thinking          string  `json:"thinking,omitempty"`
	ThinkingSignature *string `json:"thinkingSignature,omitempty"`
	Redacted          bool    `json:"redacted,omitempty"`

	// Populated when Type == "toolCall"
	ToolCallID        string         `json:"id,omitempty"`
	ToolCallName      string         `json:"name,omitempty"`
	ToolCallArguments map[string]any `json:"arguments,omitempty"`
	ThoughtSignature  *string        `json:"thoughtSignature,omitempty"`
}

// AsText returns the text content if this is a text block.
func (c ContentBlock) AsText() (text string, ok bool) {
	if c.Type != "text" {
		return "", false
	}
	return c.Text, true
}

// AsThinking returns the thinking content if this is a thinking block.
func (c ContentBlock) AsThinking() (thinking string, signature *string, redacted bool, ok bool) {
	if c.Type != "thinking" {
		return "", nil, false, false
	}
	return c.Thinking, c.ThinkingSignature, c.Redacted, true
}

// AsToolCall returns the tool call fields if this is a toolCall block.
func (c ContentBlock) AsToolCall() (id, name string, args map[string]any, ok bool) {
	if c.Type != "toolCall" {
		return "", "", nil, false
	}
	return c.ToolCallID, c.ToolCallName, c.ToolCallArguments, true
}

// NewTextContent creates a text content block.
func NewTextContent(text string) ContentBlock {
	return ContentBlock{Type: "text", Text: text}
}

// NewThinkingContent creates a thinking content block.
func NewThinkingContent(thinking string) ContentBlock {
	return ContentBlock{Type: "thinking", Thinking: thinking}
}

// NewToolCallContent creates a tool call content block.
func NewToolCallContent(id, name string, args map[string]any) ContentBlock {
	return ContentBlock{Type: "toolCall", ToolCallID: id, ToolCallName: name, ToolCallArguments: args}
}

// ============================================================================
// Messages
// ============================================================================

// UserMessage represents a message from the user.
type UserMessage struct {
	Role      string `json:"role"` // always "user"
	Content   any    `json:"content"` // string or []ContentBlock
	Timestamp int64  `json:"timestamp"`
}

// AssistantMessage represents a message from the assistant.
type AssistantMessage struct {
	Role          string         `json:"role"` // always "assistant"
	Content       []ContentBlock `json:"content"`
	API           Api            `json:"api"`
	Provider      Provider       `json:"provider"`
	Model         string         `json:"model"`
	ResponseModel *string        `json:"responseModel,omitempty"`
	ResponseID    *string        `json:"responseId,omitempty"`
	Diagnostics   []Diagnostic   `json:"diagnostics,omitempty"`
	Usage         Usage          `json:"usage"`
	StopReason    StopReason     `json:"stopReason"`
	ErrorMessage  *string        `json:"errorMessage,omitempty"`
	Timestamp     int64          `json:"timestamp"`
}

// ToolResultMessage represents a tool execution result.
type ToolResultMessage struct {
	Role       string        `json:"role"` // always "toolResult"
	ToolCallID string        `json:"toolCallId"`
	ToolName   string        `json:"toolName"`
	Content    []ContentBlock `json:"content"`
	Details    any           `json:"details,omitempty"`
	IsError    bool          `json:"isError"`
	Timestamp  int64         `json:"timestamp"`
}

// Diagnostic represents a redacted provider/runtime diagnostic.
type Diagnostic struct {
	Message string `json:"message"`
}

// ToolCall represents a tool invocation in the streaming protocol.
// For content blocks within messages, use ContentBlock with Type="toolCall" instead.
type ToolCall struct {
	Type           string         `json:"type"` // always "toolCall"
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Arguments      map[string]any `json:"arguments"`
	ThoughtSignature *string      `json:"thoughtSignature,omitempty"`
}

// Message is a discriminated union of all message types. Use the Role field
// to determine which fields are valid:
//   - "user":       Content, Timestamp
//   - "assistant":  AssistantContent, API, Provider, Model, ResponseModel, ResponseID,
//                   Diagnostics, Usage, StopReason, ErrorMessage, Timestamp
//   - "toolResult": ToolCallID, ToolName, ToolResultContent, Details, IsError, Timestamp
//
// Use AsUserMessage(), AsAssistantMessage(), AsToolResultMessage() for typed access.
type Message struct {
	Role string `json:"role"` // "user", "assistant", "toolResult"

	// User fields (Role == "user")
	Content any `json:"content,omitempty"` // string or []ContentBlock

	// Assistant fields (Role == "assistant")
	AssistantContent []ContentBlock `json:"assistantContent,omitempty"`
	API              Api            `json:"api,omitempty"`
	Provider         Provider       `json:"provider,omitempty"`
	Model            string         `json:"model,omitempty"`
	ResponseModel    *string        `json:"responseModel,omitempty"`
	ResponseID       *string        `json:"responseId,omitempty"`
	Diagnostics      []Diagnostic   `json:"diagnostics,omitempty"`
	Usage            Usage          `json:"usage,omitempty"`
	StopReason       StopReason     `json:"stopReason,omitempty"`
	ErrorMessage     *string        `json:"errorMessage,omitempty"`

	// ToolResult fields (Role == "toolResult")
	ToolCallID        string `json:"toolCallId,omitempty"`
	ToolName          string `json:"toolName,omitempty"`
	ToolResultContent []ContentBlock `json:"toolResultContent,omitempty"`
	Details           any    `json:"details,omitempty"`
	IsError           bool   `json:"isError,omitempty"`

	Timestamp int64 `json:"timestamp"`
}

// AsUserMessage converts a Message to a UserMessage (if role is "user").
func (m *Message) AsUserMessage() (*UserMessage, bool) {
	if m.Role != "user" {
		return nil, false
	}
	return &UserMessage{
		Role:      m.Role,
		Content:   m.Content,
		Timestamp: m.Timestamp,
	}, true
}

// AsAssistantMessage converts a Message to an AssistantMessage.
func (m *Message) AsAssistantMessage() (*AssistantMessage, bool) {
	if m.Role != "assistant" {
		return nil, false
	}
	return &AssistantMessage{
		Role:          m.Role,
		Content:       m.AssistantContent,
		API:           m.API,
		Provider:      m.Provider,
		Model:         m.Model,
		ResponseModel: m.ResponseModel,
		ResponseID:    m.ResponseID,
		Diagnostics:   m.Diagnostics,
		Usage:         m.Usage,
		StopReason:    m.StopReason,
		ErrorMessage:  m.ErrorMessage,
		Timestamp:     m.Timestamp,
	}, true
}

// AsToolResultMessage converts a Message to a ToolResultMessage.
func (m *Message) AsToolResultMessage() (*ToolResultMessage, bool) {
	if m.Role != "toolResult" {
		return nil, false
	}
	return &ToolResultMessage{
		Role:       m.Role,
		ToolCallID: m.ToolCallID,
		ToolName:   m.ToolName,
		Content:    m.ToolResultContent,
		Details:    m.Details,
		IsError:    m.IsError,
		Timestamp:  m.Timestamp,
	}, true
}

// NewUserMessage creates a new user message with a string content.
func NewUserMessage(text string) Message {
	return Message{
		Role:      "user",
		Content:   text,
		Timestamp: time.Now().UnixMilli(),
	}
}

// NewUserMessageWithContent creates a new user message with structured content.
func NewUserMessageWithContent(blocks []ContentBlock) Message {
	return Message{
		Role:      "user",
		Content:   blocks,
		Timestamp: time.Now().UnixMilli(),
	}
}

// NewAssistantMessage creates a new assistant message.
func NewAssistantMessage(api Api, provider Provider, modelID string, content []ContentBlock, usage Usage, stopReason StopReason) Message {
	return Message{
		Role:             "assistant",
		AssistantContent: content,
		API:              api,
		Provider:         provider,
		Model:            modelID,
		Usage:            usage,
		StopReason:       stopReason,
		Timestamp:        time.Now().UnixMilli(),
	}
}

// NewToolResultMessage creates a new tool result message.
func NewToolResultMessage(toolCallID, toolName string, content []ContentBlock, isError bool) Message {
	return Message{
		Role:              "toolResult",
		ToolCallID:        toolCallID,
		ToolName:          toolName,
		ToolResultContent: content,
		IsError:           isError,
		Timestamp:         time.Now().UnixMilli(),
	}
}

// Tool describes a function the model can call.
type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"` // JSON Schema object
}

// Context holds the full conversation context for a request.
type Context struct {
	SystemPrompt *string  `json:"systemPrompt,omitempty"`
	Messages     []Message `json:"messages"`
	Tools        []Tool    `json:"tools,omitempty"`
}

// StreamOptions contains options for streaming requests.
type StreamOptions struct {
	Temperature               *float64          `json:"temperature,omitempty"`
	MaxTokens                 *int              `json:"maxTokens,omitempty"`
	APIKey                    *string           `json:"apiKey,omitempty"`
	Transport                 Transport         `json:"transport,omitempty"`
	CacheRetention            CacheRetention    `json:"cacheRetention,omitempty"`
	SessionID                 *string           `json:"sessionId,omitempty"`
	Headers                   map[string]string `json:"headers,omitempty"`
	TimeoutMs                 *int              `json:"timeoutMs,omitempty"`
	WebSocketConnectTimeoutMs *int              `json:"websocketConnectTimeoutMs,omitempty"`
	MaxRetries                *int              `json:"maxRetries,omitempty"`
	MaxRetryDelayMs           *int              `json:"maxRetryDelayMs,omitempty"`
	Metadata                  map[string]any    `json:"metadata,omitempty"`
	OnPayload                 func(payload any, model *Model) any
	OnResponse                func(status int, headers map[string]string, model *Model)
}

// SimpleStreamOptions extends StreamOptions with reasoning support.
type SimpleStreamOptions struct {
	StreamOptions
	Reasoning       ThinkingLevel    `json:"reasoning,omitempty"`
	ThinkingBudgets *ThinkingBudgets `json:"thinkingBudgets,omitempty"`
}

// ProviderResponse wraps HTTP response metadata.
type ProviderResponse struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
}

// ============================================================================
// Event Stream Protocol
// ============================================================================

// AssistantMessageEvent represents a single event in the streaming protocol.
type AssistantMessageEvent struct {
	Type         string            `json:"type"`
	ContentIndex *int              `json:"contentIndex,omitempty"`
	Delta        *string           `json:"delta,omitempty"`
	Content      *string           `json:"content,omitempty"`
	ToolCall     *ToolCall         `json:"toolCall,omitempty"`
	Reason       StopReason        `json:"reason,omitempty"`
	Message      *AssistantMessage `json:"message,omitempty"` // for "done" events
	Error        *AssistantMessage `json:"error,omitempty"`    // for "error" events
	Partial      *AssistantMessage `json:"partial,omitempty"`  // for streaming partial updates
}

// EventStream is a Go channel-based event stream for assistant messages.
type EventStream struct {
	Events chan AssistantMessageEvent `json:"-"`
	Result chan AssistantMessage      `json:"-"`
	ctx    context.Context
	cancel context.CancelFunc
}

// NewEventStream creates a new event stream.
func NewEventStream(parentCtx context.Context) *EventStream {
	ctx, cancel := context.WithCancel(parentCtx)
	return &EventStream{
		Events: make(chan AssistantMessageEvent, 256),
		Result: make(chan AssistantMessage, 1),
		ctx:    ctx,
		cancel: cancel,
	}
}

// Push sends an event to the stream.
func (s *EventStream) Push(event AssistantMessageEvent) {
	select {
	case s.Events <- event:
	case <-s.ctx.Done():
	}
}

// End signals the stream is complete with the final message.
func (s *EventStream) End(msg AssistantMessage) {
	select {
	case s.Result <- msg:
	case <-s.ctx.Done():
	}
	s.cancel()
}

// Cancel aborts the stream.
func (s *EventStream) Cancel() {
	s.cancel()
}

// Context returns the stream's context.
func (s *EventStream) Context() context.Context {
	return s.ctx
}

// Iterate returns a channel to range over events. Closes when done.
// It drains all buffered events even after the stream is cancelled.
func (s *EventStream) Iterate() <-chan AssistantMessageEvent {
	out := make(chan AssistantMessageEvent, 256)
	go func() {
		defer close(out)
		for {
			// Drain all buffered events first
			select {
			case ev, ok := <-s.Events:
				if !ok {
					return
				}
				out <- ev
				continue
			default:
			}
			// No buffered events — wait for new event or cancellation
			select {
			case ev, ok := <-s.Events:
				if !ok {
					return
				}
				out <- ev
			case <-s.ctx.Done():
				// Context cancelled — drain any remaining buffered events
				for {
					select {
					case ev, ok := <-s.Events:
						if !ok {
							return
						}
						out <- ev
					default:
						return
					}
				}
			}
		}
	}()
	return out
}

// ============================================================================
// OpenAI Compatibility Types
// ============================================================================

// OpenAICompletionsCompat configures behavior for OpenAI-compatible completions APIs.
type OpenAICompletionsCompat struct {
	SupportsStore                               *bool                `json:"supportsStore,omitempty"`
	SupportsDeveloperRole                       *bool                `json:"supportsDeveloperRole,omitempty"`
	SupportsReasoningEffort                     *bool                `json:"supportsReasoningEffort,omitempty"`
	SupportsUsageInStreaming                    *bool                `json:"supportsUsageInStreaming,omitempty"`
	MaxTokensField                              *string              `json:"maxTokensField,omitempty"`
	RequiresToolResultName                      *bool                `json:"requiresToolResultName,omitempty"`
	RequiresAssistantAfterToolResult            *bool                `json:"requiresAssistantAfterToolResult,omitempty"`
	RequiresThinkingAsText                      *bool                `json:"requiresThinkingAsText,omitempty"`
	RequiresReasoningContentOnAssistantMessages *bool                `json:"requiresReasoningContentOnAssistantMessages,omitempty"`
	ThinkingFormat                              *string              `json:"thinkingFormat,omitempty"`
	OpenRouterRouting                           *OpenRouterRouting   `json:"openRouterRouting,omitempty"`
	VercelGatewayRouting                        *VercelGatewayRouting `json:"vercelGatewayRouting,omitempty"`
	ZAIToolStream                               *bool                `json:"zaiToolStream,omitempty"`
	SupportsStrictMode                          *bool                `json:"supportsStrictMode,omitempty"`
	CacheControlFormat                          *string              `json:"cacheControlFormat,omitempty"`
	SendSessionAffinityHeaders                  *bool                `json:"sendSessionAffinityHeaders,omitempty"`
	SupportsLongCacheRetention                  *bool                `json:"supportsLongCacheRetention,omitempty"`
}

// OpenAIResponsesCompat configures behavior for OpenAI Responses-compatible APIs.
type OpenAIResponsesCompat struct {
	SendSessionIDHeader        *bool `json:"sendSessionIdHeader,omitempty"`
	SupportsLongCacheRetention *bool `json:"supportsLongCacheRetention,omitempty"`
}

// AnthropicMessagesCompat configures behavior for Anthropic Messages-compatible APIs.
type AnthropicMessagesCompat struct {
	SupportsEagerToolInputStreaming *bool  `json:"supportsEagerToolInputStreaming,omitempty"`
	SupportsLongCacheRetention      *bool  `json:"supportsLongCacheRetention,omitempty"`
	SendSessionAffinityHeaders      *bool  `json:"sendSessionAffinityHeaders,omitempty"`
	SupportsCacheControlOnTools     *bool  `json:"supportsCacheControlOnTools,omitempty"`
	SupportsTemperature             *bool  `json:"supportsTemperature,omitempty"`
	ForceAdaptiveThinking           *bool  `json:"forceAdaptiveThinking,omitempty"`
	AllowEmptySignature             *bool  `json:"allowEmptySignature,omitempty"`
}

// OpenRouterRouting controls OpenRouter upstream provider selection.
type OpenRouterRouting struct {
	AllowFallbacks         *bool    `json:"allow_fallbacks,omitempty"`
	RequireParameters      *bool    `json:"require_parameters,omitempty"`
	DataCollection         *string  `json:"data_collection,omitempty"`
	ZDR                    *bool    `json:"zdr,omitempty"`
	EnforceDistillableText *bool    `json:"enforce_distillable_text,omitempty"`
	Order                  []string `json:"order,omitempty"`
	Only                   []string `json:"only,omitempty"`
	Ignore                 []string `json:"ignore,omitempty"`
	Quantizations          []string `json:"quantizations,omitempty"`
	Sort                   any      `json:"sort,omitempty"`
	MaxPrice               any      `json:"max_price,omitempty"`
	PreferredMinThroughput any      `json:"preferred_min_throughput,omitempty"`
	PreferredMaxLatency    any      `json:"preferred_max_latency,omitempty"`
}

// VercelGatewayRouting controls Vercel AI Gateway upstream provider selection.
type VercelGatewayRouting struct {
	Only  []string `json:"only,omitempty"`
	Order []string `json:"order,omitempty"`
}
