package agent

import (
	"context"
	"sync"

	"github.com/chinudotdev/pi-go/ai"
)

// agentState holds the mutable internal state of an Agent.
type agentState struct {
	systemPrompt     string
	model            *ai.Model
	thinkingLevel    ThinkingLevel
	tools            []*Tool
	messages         []AgentMessage
	isStreaming      bool
	streamingMessage *AgentMessage
	pendingToolCalls map[string]bool
	errorMessage     *string
}

// activeRun tracks an in-progress agent execution.
type activeRun struct {
	cancel context.CancelFunc
	wg     sync.WaitGroup
	done   chan struct{}
}

// ============================================================================
// Run lifecycle
// ============================================================================

func (a *Agent) tryStartRun() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.activeRun != nil {
		return false
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.activeRun = &activeRun{
		cancel: cancel,
		done:   make(chan struct{}),
	}
	a.state.isStreaming = true
	a.state.streamingMessage = nil
	a.state.errorMessage = nil
	_ = ctx // stored in activeRun; loop uses it via a.runCtx()
	return true
}

func (a *Agent) finishRun() {
	a.mu.Lock()
	a.state.isStreaming = false
	a.state.streamingMessage = nil
	a.state.pendingToolCalls = make(map[string]bool)
	if a.activeRun != nil {
		a.activeRun.cancel()
		close(a.activeRun.done)
		a.activeRun = nil
	}
	a.mu.Unlock()
}

func (a *Agent) runCtx() context.Context {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.activeRun == nil {
		return context.Background()
	}
	return context.Background() // TODO: wire up activeRun cancel context
}

func (a *Agent) runPromptMessages(ctx context.Context, messages []AgentMessage, skipInitialSteeringPoll bool) error {
	snapshot := a.contextSnapshot()
	config := a.createLoopConfig(skipInitialSteeringPoll)
	return RunAgentLoop(ctx, messages, snapshot, config, a.emit, a.streamFn)
}

func (a *Agent) runContinuation(ctx context.Context) error {
	snapshot := a.contextSnapshot()
	config := a.createLoopConfig(false)
	return RunAgentLoopContinue(ctx, snapshot, config, a.emit, a.streamFn)
}

func (a *Agent) contextSnapshot() *AgentContext {
	a.mu.RLock()
	defer a.mu.RUnlock()

	messages := make([]AgentMessage, len(a.state.messages))
	copy(messages, a.state.messages)

	tools := make([]*Tool, len(a.state.tools))
	copy(tools, a.state.tools)

	return &AgentContext{
		SystemPrompt: a.state.systemPrompt,
		Messages:     messages,
		Tools:        tools,
	}
}

func (a *Agent) createLoopConfig(skipInitialSteeringPoll bool) *LoopConfig {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return &LoopConfig{
		Model:           a.state.model,
		Reasoning:       a.state.thinkingLevel,
		SessionID:       a.sessionID,
		Transport:       a.transport,
		ThinkingBudgets: a.thinkingBudgets,
		MaxRetryDelayMs: a.maxRetryDelayMs,
		ToolExecution:   a.toolExecution,
		ConvertToLlm:    a.convertToLlm,
		TransformContext: a.transformContext,
		GetApiKey:       a.getApiKey,
		BeforeToolCall:  a.beforeToolCall,
		AfterToolCall:   a.afterToolCall,
		PrepareNextTurn: a.wrapPrepareNextTurn(),
		ShouldStopAfterTurn: nil, // set externally if needed
		GetSteeringMessages: func() ([]AgentMessage, error) {
			if skipInitialSteeringPoll {
				skipInitialSteeringPoll = false
				return nil, nil
			}
			return a.steeringQueue.Drain(), nil
		},
		GetFollowUpMessages: func() ([]AgentMessage, error) {
			return a.followUpQueue.Drain(), nil
		},
	}
}

func (a *Agent) wrapPrepareNextTurn() func(ctx PrepareNextTurnContext) (*AgentLoopTurnUpdate, error) {
	if a.prepareNextTurn == nil {
		return nil
	}
	return func(ctx PrepareNextTurnContext) (*AgentLoopTurnUpdate, error) {
		return a.prepareNextTurn(context.Background())
	}
}

// ============================================================================
// Event processing
// ============================================================================

func (a *Agent) emit(event Event) error {
	a.applyEventToState(event)

	a.listenMu.RLock()
	listeners := make([]func(event Event) error, len(a.listeners))
	copy(listeners, a.listeners)
	a.listenMu.RUnlock()

	for _, listener := range listeners {
		if listener == nil {
			continue
		}
		if err := listener(event); err != nil {
			// Log but don't fail — listeners must not break the loop
			_ = err
		}
	}
	return nil
}

func (a *Agent) applyEventToState(event Event) {
	a.mu.Lock()
	defer a.mu.Unlock()

	switch event.Type {
	case EventMessageStart:
		a.state.streamingMessage = &event.Msg

	case EventMessageUpdate:
		a.state.streamingMessage = &event.Msg

	case EventMessageEnd:
		a.state.streamingMessage = nil
		a.state.messages = append(a.state.messages, event.Msg)

	case EventToolExecutionStart:
		a.state.pendingToolCalls[event.ToolCallID] = true

	case EventToolExecutionEnd:
		delete(a.state.pendingToolCalls, event.ToolCallID)

	case EventTurnEnd:
		if event.Message != nil && event.Message.ErrorMessage != nil {
			a.state.errorMessage = event.Message.ErrorMessage
		}

	case EventAgentEnd:
		a.state.streamingMessage = nil
	}
}
