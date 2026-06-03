package agent

import (
	"context"
	"sync"

	"github.com/chinudotdev/pi-go/ai"
)

// DefaultConvertToLlm filters messages to only LLM-compatible roles.
func DefaultConvertToLlm(messages []AgentMessage) ([]ai.Message, error) {
	var result []ai.Message
	for _, m := range messages {
		if m.Role == "user" || m.Role == "assistant" || m.Role == "toolResult" {
			result = append(result, m)
		}
	}
	return result, nil
}

// Agent is a stateful wrapper around the agent loop.
// It owns the conversation transcript, emits lifecycle events,
// executes tools, and exposes queueing APIs for steering/follow-up.
type Agent struct {
	mu sync.RWMutex

	// State
	state agentState

	// Configuration
	convertToLlm     func(messages []AgentMessage) ([]ai.Message, error)
	transformContext func(ctx context.Context, messages []AgentMessage) ([]AgentMessage, error)
	streamFn         StreamFn
	getApiKey        func(provider string) (string, error)
	beforeToolCall   func(ctx BeforeToolCallContext) (*BeforeToolCallResult, error)
	afterToolCall    func(ctx AfterToolCallContext) (*AfterToolCallResult, error)
	prepareNextTurn  func(ctx context.Context) (*AgentLoopTurnUpdate, error)

	// Queues
	steeringQueue *pendingMessageQueue
	followUpQueue *pendingMessageQueue

	// Listeners
	listeners []func(event Event) error
	listenMu  sync.RWMutex

	// Session
	sessionID       *string
	thinkingBudgets *ai.ThinkingBudgets
	transport       ai.Transport
	maxRetryDelayMs *int
	toolExecution   ToolExecutionMode

	// Active run
	activeRun *activeRun
}

// Options for constructing an Agent.
type Options struct {
	InitialState     *InitialState
	ConvertToLlm     func(messages []AgentMessage) ([]ai.Message, error)
	TransformContext func(ctx context.Context, messages []AgentMessage) ([]AgentMessage, error)
	StreamFn         StreamFn
	GetApiKey        func(provider string) (string, error)
	BeforeToolCall   func(ctx BeforeToolCallContext) (*BeforeToolCallResult, error)
	AfterToolCall    func(ctx AfterToolCallContext) (*AfterToolCallResult, error)
	PrepareNextTurn  func(ctx context.Context) (*AgentLoopTurnUpdate, error)
	SteeringMode     QueueMode
	FollowUpMode     QueueMode
	SessionID        string
	ThinkingBudgets  *ai.ThinkingBudgets
	Transport        ai.Transport
	MaxRetryDelayMs  int
	ToolExecution    ToolExecutionMode
}

// InitialState configures the agent's starting state.
type InitialState struct {
	SystemPrompt  string
	Model         *ai.Model
	ThinkingLevel ThinkingLevel
	Tools         []*Tool
	Messages      []AgentMessage
}

// New creates a new Agent with the given options.
func New(opts Options) *Agent {
	if opts.SteeringMode == "" {
		opts.SteeringMode = QueueOneAtATime
	}
	if opts.FollowUpMode == "" {
		opts.FollowUpMode = QueueOneAtATime
	}
	if opts.ToolExecution == "" {
		opts.ToolExecution = ToolExecutionParallel
	}
	if opts.Transport == "" {
		opts.Transport = ai.TransportAuto
	}

	convertToLlm := opts.ConvertToLlm
	if convertToLlm == nil {
		convertToLlm = DefaultConvertToLlm
	}

	state := agentState{
		pendingToolCalls: make(map[string]bool),
		thinkingLevel:    ThinkingOff,
	}

	if opts.InitialState != nil {
		is := opts.InitialState
		state.systemPrompt = is.SystemPrompt
		if is.Model != nil {
			state.model = is.Model
		}
		state.thinkingLevel = is.ThinkingLevel
		if is.Tools != nil {
			state.tools = append([]*Tool{}, is.Tools...)
		}
		if is.Messages != nil {
			state.messages = append([]AgentMessage{}, is.Messages...)
		}
	}

	var sessionID *string
	if opts.SessionID != "" {
		sessionID = &opts.SessionID
	}

	var maxRetryDelayMs *int
	if opts.MaxRetryDelayMs > 0 {
		maxRetryDelayMs = &opts.MaxRetryDelayMs
	}

	return &Agent{
		state:            state,
		convertToLlm:     convertToLlm,
		transformContext: opts.TransformContext,
		streamFn:         opts.StreamFn,
		getApiKey:        opts.GetApiKey,
		beforeToolCall:   opts.BeforeToolCall,
		afterToolCall:    opts.AfterToolCall,
		prepareNextTurn:  opts.PrepareNextTurn,
		steeringQueue:    newPendingMessageQueue(opts.SteeringMode),
		followUpQueue:    newPendingMessageQueue(opts.FollowUpMode),
		sessionID:        sessionID,
		thinkingBudgets:  opts.ThinkingBudgets,
		transport:        opts.Transport,
		maxRetryDelayMs:  maxRetryDelayMs,
		toolExecution:    opts.ToolExecution,
	}
}

// ============================================================================
// Public API
// ============================================================================

// Subscribe registers a listener for agent lifecycle events.
// Returns an unsubscribe function.
func (a *Agent) Subscribe(listener func(event Event) error) func() {
	a.listenMu.Lock()
	defer a.listenMu.Unlock()
	a.listeners = append(a.listeners, listener)
	i := len(a.listeners) - 1
	return func() {
		a.listenMu.Lock()
		defer a.listenMu.Unlock()
		a.listeners[i] = nil
	}
}

// State returns the current agent state (read-only snapshot).
func (a *Agent) State() StateSnapshot {
	a.mu.RLock()
	defer a.mu.RUnlock()

	pending := make(map[string]bool)
	for k, v := range a.state.pendingToolCalls {
		pending[k] = v
	}

	return StateSnapshot{
		SystemPrompt:     a.state.systemPrompt,
		Model:            a.state.model,
		ThinkingLevel:    a.state.thinkingLevel,
		Tools:            a.state.tools,
		Messages:         a.state.messages,
		IsStreaming:      a.state.isStreaming,
		StreamingMessage: a.state.streamingMessage,
		PendingToolCalls: pending,
		ErrorMessage:     a.state.errorMessage,
	}
}

// StateSnapshot is a read-only snapshot of the agent's state.
type StateSnapshot struct {
	SystemPrompt     string
	Model            *ai.Model
	ThinkingLevel    ThinkingLevel
	Tools            []*Tool
	Messages         []AgentMessage
	IsStreaming      bool
	StreamingMessage *AgentMessage
	PendingToolCalls map[string]bool
	ErrorMessage     *string
}

// SetSystemPrompt updates the system prompt.
func (a *Agent) SetSystemPrompt(prompt string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state.systemPrompt = prompt
}

// SetModel updates the active model.
func (a *Agent) SetModel(model *ai.Model) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state.model = model
}

// SetThinkingLevel updates the reasoning level.
func (a *Agent) SetThinkingLevel(level ThinkingLevel) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state.thinkingLevel = level
}

// SetTools updates the available tools (copies the slice).
func (a *Agent) SetTools(tools []*Tool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state.tools = append([]*Tool{}, tools...)
}

// Steer queues a message to be injected after the current assistant turn.
func (a *Agent) Steer(msg AgentMessage) {
	a.steeringQueue.Enqueue(msg)
}

// FollowUp queues a message to run only after the agent would otherwise stop.
func (a *Agent) FollowUp(msg AgentMessage) {
	a.followUpQueue.Enqueue(msg)
}

// ClearSteeringQueue removes all queued steering messages.
func (a *Agent) ClearSteeringQueue() { a.steeringQueue.Clear() }

// ClearFollowUpQueue removes all queued follow-up messages.
func (a *Agent) ClearFollowUpQueue() { a.followUpQueue.Clear() }

// ClearAllQueues removes all queued messages.
func (a *Agent) ClearAllQueues() {
	a.ClearSteeringQueue()
	a.ClearFollowUpQueue()
}

// HasQueuedMessages returns true when either queue has pending messages.
func (a *Agent) HasQueuedMessages() bool {
	return a.steeringQueue.HasItems() || a.followUpQueue.HasItems()
}

// SetSteeringMode controls how steering messages are drained.
func (a *Agent) SetSteeringMode(mode QueueMode) { a.steeringQueue.mode = mode }

// SetFollowUpMode controls how follow-up messages are drained.
func (a *Agent) SetFollowUpMode(mode QueueMode) { a.followUpQueue.mode = mode }

// Abort cancels the current run.
func (a *Agent) Abort() {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.activeRun != nil {
		a.activeRun.cancel()
	}
}

// WaitForIdle blocks until the current run completes.
func (a *Agent) WaitForIdle() {
	a.mu.RLock()
	run := a.activeRun
	a.mu.RUnlock()
	if run != nil {
		<-run.done
	}
}

// Reset clears transcript state, runtime state, and queued messages.
func (a *Agent) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state.messages = nil
	a.state.isStreaming = false
	a.state.streamingMessage = nil
	a.state.pendingToolCalls = make(map[string]bool)
	a.state.errorMessage = nil
	a.followUpQueue.Clear()
	a.steeringQueue.Clear()
}

// Prompt sends a text prompt to the agent and runs the loop to completion.
// Returns an error if the agent is already running.
func (a *Agent) Prompt(ctx context.Context, text string) error {
	content := []ai.ContentBlock{ai.NewTextContent(text)}
	msg := ai.NewUserMessageWithContent(content)
	return a.PromptMessages(ctx, []AgentMessage{msg})
}

// PromptMessages sends pre-built messages and runs the loop to completion.
func (a *Agent) PromptMessages(ctx context.Context, messages []AgentMessage) error {
	if !a.tryStartRun() {
		return errorf("agent is already processing a prompt")
	}
	defer a.finishRun()

	return a.runPromptMessages(ctx, messages, false)
}

// Continue resumes from the current transcript.
func (a *Agent) Continue(ctx context.Context) error {
	if !a.tryStartRun() {
		return errorf("agent is already processing")
	}
	defer a.finishRun()

	a.mu.RLock()
	lastIdx := len(a.state.messages) - 1
	a.mu.RUnlock()

	if lastIdx < 0 {
		return errorf("no messages to continue from")
	}

	a.mu.RLock()
	lastRole := a.state.messages[lastIdx].Role
	a.mu.RUnlock()

	if lastRole == "assistant" {
		if drained := a.steeringQueue.Drain(); len(drained) > 0 {
			return a.runPromptMessages(ctx, drained, true)
		}
		if drained := a.followUpQueue.Drain(); len(drained) > 0 {
			return a.runPromptMessages(ctx, drained, false)
		}
		return errorf("cannot continue from message role: assistant")
	}

	return a.runContinuation(ctx)
}


