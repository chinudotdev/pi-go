package agent

import (
	"context"
	"fmt"
	"sync"

	"github.com/chinudotdev/pi-go/ai"
)

// ============================================================================
// Core types
// ============================================================================

// ThinkingLevel controls reasoning effort for models that support it.
type ThinkingLevel string

const (
	ThinkingOff     ThinkingLevel = "off"
	ThinkingMinimal ThinkingLevel = "minimal"
	ThinkingLow     ThinkingLevel = "low"
	ThinkingMedium  ThinkingLevel = "medium"
	ThinkingHigh    ThinkingLevel = "high"
	ThinkingXHigh   ThinkingLevel = "xhigh"
)

// ToolExecutionMode controls how tool calls from a single assistant message are executed.
type ToolExecutionMode string

const (
	ToolExecutionSequential ToolExecutionMode = "sequential"
	ToolExecutionParallel   ToolExecutionMode = "parallel"
)

// QueueMode controls how queued messages are drained.
type QueueMode string

const (
	QueueAll        QueueMode = "all"
	QueueOneAtATime QueueMode = "one-at-a-time"
)

// StreamFn is the function signature for streaming LLM responses.
// It must not panic for request/model/runtime failures — errors must be
// encoded in the returned stream via protocol events.
type StreamFn func(ctx context.Context, model *ai.Model, convCtx *ai.Context, options *ai.SimpleStreamOptions) (*ai.EventStream, error)

// AgentMessage is the universal message type for the agent layer.
// It can be an ai.Message or any custom type added by the application.
type AgentMessage = ai.Message

// AgentToolCall is a tool call content block from an assistant message.
type AgentToolCall = ai.ContentBlock

// ============================================================================
// Tool definition
// ============================================================================

// ToolResult is the result of executing a tool call.
type ToolResult struct {
	// Content is the text/image content returned to the model.
	Content []ai.ContentBlock
	// Details is arbitrary structured data for logs or UI rendering.
	Details any
	// Terminate hints that the agent should stop after the current tool batch.
	Terminate bool
}

// ToolUpdateCallback is used by tools to stream partial execution updates.
type ToolUpdateCallback func(partialResult ToolResult)

// Tool defines a tool that the agent can execute.
type Tool struct {
	// Name is the tool identifier used by the model.
	Name string
	// Description is the human-readable description for the model.
	Description string
	// Parameters is the JSON schema for tool arguments (map[string]any representation).
	Parameters map[string]any
	// Label is a human-readable label for UI display.
	Label string
	// PrepareArguments is an optional shim for raw tool-call arguments before validation.
	PrepareArguments func(args map[string]any) map[string]any
	// Execute runs the tool. Throw/panic on failure.
	Execute func(ctx context.Context, toolCallID string, params map[string]any, onUpdate ToolUpdateCallback) (*ToolResult, error)
	// ExecutionMode overrides the default tool execution mode for this tool.
	ExecutionMode ToolExecutionMode
}

// ToAITool converts an agent Tool to an ai.Tool for provider calls.
func (t Tool) ToAITool() ai.Tool {
	return ai.Tool{
		Name:        t.Name,
		Description: t.Description,
		Parameters:  t.Parameters,
	}
}

// ============================================================================
// Hook types
// ============================================================================

// BeforeToolCallResult is returned from BeforeToolCall to block execution.
type BeforeToolCallResult struct {
	Block  bool
	Reason string
}

// AfterToolCallResult allows partial override of a tool result.
type AfterToolCallResult struct {
	Content   []ai.ContentBlock
	Details   any
	IsError   *bool
	Terminate *bool
}

// BeforeToolCallContext is passed to BeforeToolCall hooks.
type BeforeToolCallContext struct {
	AssistantMessage *ai.AssistantMessage
	ToolCall         AgentToolCall
	Args             map[string]any
	Context          *AgentContext
}

// AfterToolCallContext is passed to AfterToolCall hooks.
type AfterToolCallContext struct {
	AssistantMessage *ai.AssistantMessage
	ToolCall         AgentToolCall
	Args             map[string]any
	Result           *ToolResult
	IsError          bool
	Context          *AgentContext
}

// ShouldStopAfterTurnContext is passed to ShouldStopAfterTurn.
type ShouldStopAfterTurnContext struct {
	Message     *ai.AssistantMessage
	ToolResults []ai.Message
	Context     *AgentContext
	NewMessages []AgentMessage
}

// PrepareNextTurnContext is passed to PrepareNextTurn.
type PrepareNextTurnContext = ShouldStopAfterTurnContext

// AgentLoopTurnUpdate contains replacement state for the next turn.
type AgentLoopTurnUpdate struct {
	Context       *AgentContext
	Model         *ai.Model
	ThinkingLevel ThinkingLevel
}

// ============================================================================
// Agent context
// ============================================================================

// AgentContext is a snapshot of the agent's state passed into the loop.
type AgentContext struct {
	SystemPrompt string
	Messages     []AgentMessage
	Tools        []*Tool
}

// ============================================================================
// Agent events
// ============================================================================

// Event represents a single agent lifecycle event.
// The Type field discriminates between event kinds.
type Event struct {
	Type string `json:"type"`

	// agent_start, agent_end — no extra fields

	// agent_end
	Messages []AgentMessage `json:"messages,omitempty"`

	// turn_start, turn_end
	Message *ai.AssistantMessage `json:"message,omitempty"`

	// turn_end
	ToolResults []ai.Message `json:"toolResults,omitempty"`

	// message_start, message_update, message_end
	Msg AgentMessage `json:"msg,omitempty"`

	// message_update
	StreamEvent *ai.AssistantMessageEvent `json:"streamEvent,omitempty"`

	// tool_execution_start, tool_execution_update, tool_execution_end
	ToolCallID string `json:"toolCallId,omitempty"`
	ToolName   string `json:"toolName,omitempty"`
	Args       any    `json:"args,omitempty"`

	// tool_execution_update
	PartialResult any `json:"partialResult,omitempty"`

	// tool_execution_end
	Result  any  `json:"result,omitempty"`
	IsError bool `json:"isError,omitempty"`
}

// Event type constants.
const (
	EventAgentStart          = "agent_start"
	EventAgentEnd            = "agent_end"
	EventTurnStart           = "turn_start"
	EventTurnEnd             = "turn_end"
	EventMessageStart        = "message_start"
	EventMessageUpdate       = "message_update"
	EventMessageEnd          = "message_end"
	EventToolExecutionStart  = "tool_execution_start"
	EventToolExecutionUpdate = "tool_execution_update"
	EventToolExecutionEnd    = "tool_execution_end"
)

// ============================================================================
// Agent loop config
// ============================================================================

// LoopConfig is the configuration for a single agent loop run.
type LoopConfig struct {
	Model           *ai.Model
	Reasoning       ThinkingLevel
	SessionID       *string
	Transport       ai.Transport
	ThinkingBudgets *ai.ThinkingBudgets
	MaxRetryDelayMs *int
	ToolExecution   ToolExecutionMode

	// ConvertToLlm converts AgentMessage[] to ai.Message[] before each LLM call.
	ConvertToLlm func(messages []AgentMessage) ([]ai.Message, error)

	// TransformContext is an optional transform applied before ConvertToLlm.
	TransformContext func(ctx context.Context, messages []AgentMessage) ([]AgentMessage, error)

	// GetApiKey resolves an API key dynamically for each LLM call.
	GetApiKey func(provider string) (string, error)

	// ShouldStopAfterTurn is called after each turn completes. Return true to stop.
	ShouldStopAfterTurn func(ctx ShouldStopAfterTurnContext) (bool, error)

	// PrepareNextTurn is called before the next turn starts.
	PrepareNextTurn func(ctx PrepareNextTurnContext) (*AgentLoopTurnUpdate, error)

	// GetSteeringMessages returns messages to inject mid-run.
	GetSteeringMessages func() ([]AgentMessage, error)

	// GetFollowUpMessages returns messages to process after the agent would stop.
	GetFollowUpMessages func() ([]AgentMessage, error)

	// BeforeToolCall is called before a tool is executed.
	BeforeToolCall func(ctx BeforeToolCallContext) (*BeforeToolCallResult, error)

	// AfterToolCall is called after a tool finishes executing.
	AfterToolCall func(ctx AfterToolCallContext) (*AfterToolCallResult, error)

	// OnPayload is forwarded to the stream options.
	OnPayload func(payload any, model *ai.Model) any

	// OnResponse is forwarded to the stream options.
	OnResponse func(status int, headers map[string]string, model *ai.Model)
}

// ============================================================================
// Event sink
// ============================================================================

// EventSink receives agent events. Must not panic.
type EventSink func(event Event) error

// ============================================================================
// Pending message queue
// ============================================================================

type pendingMessageQueue struct {
	messages []AgentMessage
	mode     QueueMode
	mu       sync.Mutex
}

func newPendingMessageQueue(mode QueueMode) *pendingMessageQueue {
	return &pendingMessageQueue{mode: mode}
}

func (q *pendingMessageQueue) Enqueue(msg AgentMessage) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.messages = append(q.messages, msg)
}

func (q *pendingMessageQueue) HasItems() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.messages) > 0
}

func (q *pendingMessageQueue) Drain() []AgentMessage {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.mode == QueueAll {
		drained := q.messages
		q.messages = nil
		return drained
	}

	if len(q.messages) == 0 {
		return nil
	}
	first := q.messages[0]
	q.messages = q.messages[1:]
	return []AgentMessage{first}
}

func (q *pendingMessageQueue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.messages = nil
}

// ============================================================================
// Helpers
// ============================================================================

func newErrorToolResult(message string) *ToolResult {
	return &ToolResult{
		Content: []ai.ContentBlock{ai.NewTextContent(message)},
		Details: map[string]any{},
	}
}

func toolCallsFromMessage(msg *ai.AssistantMessage) []ai.ContentBlock {
	var calls []ai.ContentBlock
	for _, block := range msg.Content {
		if block.Type == "toolCall" {
			calls = append(calls, block)
		}
	}
	return calls
}

func agentToolsToAITools(tools []*Tool) []ai.Tool {
	if len(tools) == 0 {
		return nil
	}
	result := make([]ai.Tool, len(tools))
	for i, t := range tools {
		result[i] = t.ToAITool()
	}
	return result
}

func stringPtr(s string) *string    { return &s }
func intPtr(i int) *int             { return &i }
func float64Ptr(f float64) *float64 { return &f }

func errorf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}
