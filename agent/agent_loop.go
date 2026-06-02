package agent

import (
	"context"
	"sync"

	"github.com/chinudotdev/pi-go/ai"
)

// RunAgentLoop starts an agent loop with new prompt messages.
func RunAgentLoop(
	ctx context.Context,
	prompts []AgentMessage,
	agentCtx *AgentContext,
	config *LoopConfig,
	emit EventSink,
	streamFn StreamFn,
) error {
	newMessages := make([]AgentMessage, len(prompts))
	copy(newMessages, prompts)

	currentContext := &AgentContext{
		SystemPrompt: agentCtx.SystemPrompt,
		Messages:     make([]AgentMessage, len(agentCtx.Messages)+len(prompts)),
		Tools:        agentCtx.Tools,
	}
	copy(currentContext.Messages, agentCtx.Messages)
	copy(currentContext.Messages[len(agentCtx.Messages):], prompts)

	_ = emit(Event{Type: EventAgentStart})
	_ = emit(Event{Type: EventTurnStart})

	for _, prompt := range prompts {
		_ = emit(Event{Type: EventMessageStart, Msg: prompt})
		_ = emit(Event{Type: EventMessageEnd, Msg: prompt})
	}

	return runLoop(ctx, currentContext, newMessages, config, emit, streamFn)
}

// RunAgentLoopContinue continues from the current context without adding new messages.
func RunAgentLoopContinue(
	ctx context.Context,
	agentCtx *AgentContext,
	config *LoopConfig,
	emit EventSink,
	streamFn StreamFn,
) error {
	if len(agentCtx.Messages) == 0 {
		return errorf("cannot continue: no messages in context")
	}
	if agentCtx.Messages[len(agentCtx.Messages)-1].Role == "assistant" {
		return errorf("cannot continue from message role: assistant")
	}

	newMessages := []AgentMessage{}
	currentContext := &AgentContext{
		SystemPrompt: agentCtx.SystemPrompt,
		Messages:     agentCtx.Messages,
		Tools:        agentCtx.Tools,
	}

	_ = emit(Event{Type: EventAgentStart})
	_ = emit(Event{Type: EventTurnStart})

	return runLoop(ctx, currentContext, newMessages, config, emit, streamFn)
}

// ============================================================================
// Main loop
// ============================================================================

func runLoop(
	ctx context.Context,
	initialContext *AgentContext,
	newMessages []AgentMessage,
	initialConfig *LoopConfig,
	emit EventSink,
	streamFn StreamFn,
) error {
	currentContext := initialContext
	config := initialConfig
	firstTurn := true

	var pendingMessages []AgentMessage
	if config.GetSteeringMessages != nil {
		pendingMessages, _ = config.GetSteeringMessages()
	}

	for {
		hasMoreToolCalls := true

		for hasMoreToolCalls || len(pendingMessages) > 0 {
			if !firstTurn {
				_ = emit(Event{Type: EventTurnStart})
			} else {
				firstTurn = false
			}

			// Inject pending messages
			if len(pendingMessages) > 0 {
				for _, msg := range pendingMessages {
					_ = emit(Event{Type: EventMessageStart, Msg: msg})
					_ = emit(Event{Type: EventMessageEnd, Msg: msg})
					currentContext.Messages = append(currentContext.Messages, msg)
					newMessages = append(newMessages, msg)
				}
				pendingMessages = nil
			}

			// Stream assistant response
			msg, err := streamAssistantResponse(ctx, currentContext, config, emit, streamFn)
			if err != nil {
				errStr := "unknown error"
				if err != nil {
					errStr = err.Error()
				}
				failMsg := ai.MakeErrorAssistantMessage(config.Model, errStr)
				failAsMsg := ai.AssistantMessageToMessage(failMsg)
				_ = emit(Event{Type: EventTurnEnd, Message: &failMsg})
				_ = emit(Event{Type: EventAgentEnd, Messages: append(newMessages, failAsMsg)})
				return nil
			}
			newMessages = append(newMessages, ai.AssistantMessageToMessage(*msg))

			if msg.StopReason == ai.StopReasonError || msg.StopReason == ai.StopReasonAborted {
				_ = emit(Event{Type: EventTurnEnd, Message: msg})
				_ = emit(Event{Type: EventAgentEnd, Messages: newMessages})
				return nil
			}

			// Execute tool calls
			toolCalls := toolCallsFromMessage(msg)
			var toolResults []ai.Message
			hasMoreToolCalls = false

			if len(toolCalls) > 0 {
				batch := executeToolCalls(currentContext, msg, config, ctx, emit)
				toolResults = batch.messages
				hasMoreToolCalls = !batch.terminate

				for _, result := range toolResults {
					currentContext.Messages = append(currentContext.Messages, result)
					newMessages = append(newMessages, result)
				}
			}

			_ = emit(Event{Type: EventTurnEnd, Message: msg, ToolResults: toolResults})

			// Prepare next turn
			if config.PrepareNextTurn != nil {
				update, _ := config.PrepareNextTurn(PrepareNextTurnContext{
					Message:     msg,
					ToolResults: toolResults,
					Context:     currentContext,
					NewMessages: newMessages,
				})
				if update != nil {
					if update.Context != nil {
						currentContext = update.Context
					}
					if update.Model != nil {
						config.Model = update.Model
					}
					if update.ThinkingLevel != "" {
						config.Reasoning = update.ThinkingLevel
					}
				}
			}

			// Should stop after turn?
			if config.ShouldStopAfterTurn != nil {
				stop, _ := config.ShouldStopAfterTurn(ShouldStopAfterTurnContext{
					Message:     msg,
					ToolResults: toolResults,
					Context:     currentContext,
					NewMessages: newMessages,
				})
				if stop {
					_ = emit(Event{Type: EventAgentEnd, Messages: newMessages})
					return nil
				}
			}

			if config.GetSteeringMessages != nil {
				pendingMessages, _ = config.GetSteeringMessages()
			}
		}

		var followUpMessages []AgentMessage
		if config.GetFollowUpMessages != nil {
			followUpMessages, _ = config.GetFollowUpMessages()
		}
		if len(followUpMessages) > 0 {
			pendingMessages = followUpMessages
			continue
		}
		break
	}

	_ = emit(Event{Type: EventAgentEnd, Messages: newMessages})
	return nil
}

// ============================================================================
// Stream assistant response
// ============================================================================

func streamAssistantResponse(
	ctx context.Context,
	agentCtx *AgentContext,
	config *LoopConfig,
	emit EventSink,
	streamFn StreamFn,
) (*ai.AssistantMessage, error) {
	messages := agentCtx.Messages
	if config.TransformContext != nil {
		transformed, err := config.TransformContext(ctx, messages)
		if err == nil && transformed != nil {
			messages = transformed
		}
	}

	llmMessages, err := config.ConvertToLlm(messages)
	if err != nil {
		return nil, err
	}

	llmContext := &ai.Context{
		SystemPrompt: &agentCtx.SystemPrompt,
		Messages:     llmMessages,
		Tools:        agentToolsToAITools(agentCtx.Tools),
	}

	var apiKey string
	if config.GetApiKey != nil {
		if key, err := config.GetApiKey(config.Model.Provider); err == nil && key != "" {
			apiKey = key
		}
	}

	opts := &ai.SimpleStreamOptions{
		StreamOptions: ai.StreamOptions{
			APIKey:    &apiKey,
			Transport: config.Transport,
			SessionID: config.SessionID,
			Headers:   map[string]string{},
		},
		Reasoning:       ai.ThinkingLevel(config.Reasoning),
		ThinkingBudgets: config.ThinkingBudgets,
	}
	if config.MaxRetryDelayMs != nil {
		opts.StreamOptions.MaxRetryDelayMs = config.MaxRetryDelayMs
	}

	fn := streamFn
	if fn == nil {
		fn = defaultStreamFn
	}

	stream, err := fn(ctx, config.Model, llmContext, opts)
	if err != nil {
		return nil, err
	}

	var partialMsg *ai.AssistantMessage
	addedPartial := false

	for ev := range stream.Iterate() {
		switch ev.Type {
		case "start":
			if ev.Partial != nil {
				partialMsg = ev.Partial
				asMsg := ai.AssistantMessageToMessage(*partialMsg)
				agentCtx.Messages = append(agentCtx.Messages, asMsg)
				addedPartial = true
				_ = emit(Event{Type: EventMessageStart, Msg: asMsg})
			}

		case "text_start", "text_delta", "text_end",
			"thinking_start", "thinking_delta", "thinking_end",
			"toolcall_start", "toolcall_delta", "toolcall_end":
			if ev.Partial != nil {
				partialMsg = ev.Partial
				asMsg := ai.AssistantMessageToMessage(*partialMsg)
				if addedPartial && len(agentCtx.Messages) > 0 {
					agentCtx.Messages[len(agentCtx.Messages)-1] = asMsg
				}
				_ = emit(Event{
					Type:                  EventMessageUpdate,
					Msg:          asMsg,
					StreamEvent: &ev,
				})
			}

		case "done":
			finalMsg := <-stream.Result
			asMsg := ai.AssistantMessageToMessage(finalMsg)
			if addedPartial && len(agentCtx.Messages) > 0 {
				agentCtx.Messages[len(agentCtx.Messages)-1] = asMsg
			} else {
				agentCtx.Messages = append(agentCtx.Messages, asMsg)
				_ = emit(Event{Type: EventMessageStart, Msg: asMsg})
			}
			_ = emit(Event{Type: EventMessageEnd, Msg: asMsg})
			return &finalMsg, nil

		case "error":
			if ev.Error != nil {
				asMsg := ai.AssistantMessageToMessage(*ev.Error)
				if addedPartial && len(agentCtx.Messages) > 0 {
					agentCtx.Messages[len(agentCtx.Messages)-1] = asMsg
				} else {
					agentCtx.Messages = append(agentCtx.Messages, asMsg)
				}
				_ = emit(Event{Type: EventMessageEnd, Msg: asMsg})
				return ev.Error, nil
			}
		}
	}

	// Fallback: try result channel
	select {
	case msg := <-stream.Result:
		asMsg := ai.AssistantMessageToMessage(msg)
		if addedPartial && len(agentCtx.Messages) > 0 {
			agentCtx.Messages[len(agentCtx.Messages)-1] = asMsg
		} else {
			agentCtx.Messages = append(agentCtx.Messages, asMsg)
			_ = emit(Event{Type: EventMessageStart, Msg: asMsg})
		}
		_ = emit(Event{Type: EventMessageEnd, Msg: asMsg})
		return &msg, nil
	default:
		if partialMsg != nil {
			return partialMsg, nil
		}
		return nil, errorf("stream ended without result")
	}
}

// defaultStreamFn wraps ai.StreamSimple to match StreamFn signature.
func defaultStreamFn(ctx context.Context, model *ai.Model, convCtx *ai.Context, opts *ai.SimpleStreamOptions) (*ai.EventStream, error) {
	return ai.StreamSimple(ctx, model, convCtx, opts)
}

// ============================================================================
// Tool execution
// ============================================================================

type executedToolCallBatch struct {
	messages  []ai.Message
	terminate bool
}

type finalizedToolCall struct {
	toolCall ai.ContentBlock
	result   *ToolResult
	isError  bool
}

func executeToolCalls(
	currentContext *AgentContext,
	assistantMessage *ai.AssistantMessage,
	config *LoopConfig,
	ctx context.Context,
	emit EventSink,
) *executedToolCallBatch {
	toolCalls := toolCallsFromMessage(assistantMessage)

	hasSequential := false
	for _, tc := range toolCalls {
		for _, t := range currentContext.Tools {
			if t.Name == tc.ToolCallName && t.ExecutionMode == ToolExecutionSequential {
				hasSequential = true
				break
			}
		}
	}

	if config.ToolExecution == ToolExecutionSequential || hasSequential {
		return executeToolCallsSequential(currentContext, assistantMessage, toolCalls, config, ctx, emit)
	}
	return executeToolCallsParallel(currentContext, assistantMessage, toolCalls, config, ctx, emit)
}

func executeToolCallsSequential(
	currentContext *AgentContext,
	assistantMessage *ai.AssistantMessage,
	toolCalls []ai.ContentBlock,
	config *LoopConfig,
	ctx context.Context,
	emit EventSink,
) *executedToolCallBatch {
	var finalized []finalizedToolCall
	var messages []ai.Message

	for _, toolCall := range toolCalls {
		_ = emit(Event{
			Type:       EventToolExecutionStart,
			ToolCallID: toolCall.ToolCallID,
			ToolName:   toolCall.ToolCallName,
			Args:       toolCall.ToolCallArguments,
		})

		preparation := prepareToolCall(currentContext, assistantMessage, toolCall, config, ctx)
		var final finalizedToolCall

		if preparation.immediate {
			final = finalizedToolCall{
				toolCall: toolCall,
				result:   preparation.result,
				isError:  preparation.isError,
			}
		} else {
			executed := executePreparedToolCall(ctx, preparation, emit)
			final = finalizeExecutedToolCall(currentContext, assistantMessage, preparation, executed, config, ctx)
		}

		_ = emit(Event{
			Type:       EventToolExecutionEnd,
			ToolCallID: final.toolCall.ToolCallID,
			ToolName:   final.toolCall.ToolCallName,
			Result:     final.result,
			IsError:    final.isError,
		})

		toolResultMsg := createToolResultMessage(final)
		_ = emit(Event{Type: EventMessageStart, Msg: toolResultMsg})
		_ = emit(Event{Type: EventMessageEnd, Msg: toolResultMsg})
		messages = append(messages, toolResultMsg)
		finalized = append(finalized, final)

		if ctx.Err() != nil {
			break
		}
	}

	return &executedToolCallBatch{
		messages:  messages,
		terminate: shouldTerminateBatch(finalized),
	}
}

func executeToolCallsParallel(
	currentContext *AgentContext,
	assistantMessage *ai.AssistantMessage,
	toolCalls []ai.ContentBlock,
	config *LoopConfig,
	ctx context.Context,
	emit EventSink,
) *executedToolCallBatch {
	type finalizedEntry struct {
		final finalizedToolCall
		ok    bool
	}

	entries := make([]finalizedEntry, len(toolCalls))

	// Phase 1: prepare all tool calls sequentially (includes beforeToolCall)
	for i, toolCall := range toolCalls {
		_ = emit(Event{
			Type:       EventToolExecutionStart,
			ToolCallID: toolCall.ToolCallID,
			ToolName:   toolCall.ToolCallName,
			Args:       toolCall.ToolCallArguments,
		})

		preparation := prepareToolCall(currentContext, assistantMessage, toolCall, config, ctx)
		if preparation.immediate {
			final := finalizedToolCall{
				toolCall: toolCall,
				result:   preparation.result,
				isError:  preparation.isError,
			}
			_ = emit(Event{
				Type:       EventToolExecutionEnd,
				ToolCallID: final.toolCall.ToolCallID,
				ToolName:   final.toolCall.ToolCallName,
				Result:     final.result,
				IsError:    final.isError,
			})
			entries[i] = finalizedEntry{final: final, ok: true}
		}
		// Non-immediate preparations are stored for phase 2
		// (preparation is captured by closure below)

		if ctx.Err() != nil {
			break
		}
	}

	// Phase 2: execute non-immediate tool calls in parallel
	var wg sync.WaitGroup
	for i, toolCall := range toolCalls {
		if entries[i].ok {
			continue
		}
		preparation := prepareToolCall(currentContext, assistantMessage, toolCall, config, ctx)
		if preparation.immediate {
			// Was handled above but didn't get marked — skip
			continue
		}

		wg.Add(1)
		go func(idx int, prep *preparedToolCall) {
			defer wg.Done()
			executed := executePreparedToolCall(ctx, prep, emit)
			final := finalizeExecutedToolCall(currentContext, assistantMessage, prep, executed, config, ctx)
			_ = emit(Event{
				Type:       EventToolExecutionEnd,
				ToolCallID: final.toolCall.ToolCallID,
				ToolName:   final.toolCall.ToolCallName,
				Result:     final.result,
				IsError:    final.isError,
			})
			entries[idx] = finalizedEntry{final: final, ok: true}
		}(i, preparation)
	}
	wg.Wait()

	// Phase 3: emit tool result messages in source order
	var finalized []finalizedToolCall
	var messages []ai.Message
	for _, entry := range entries {
		if !entry.ok {
			continue
		}
		toolResultMsg := createToolResultMessage(entry.final)
		_ = emit(Event{Type: EventMessageStart, Msg: toolResultMsg})
		_ = emit(Event{Type: EventMessageEnd, Msg: toolResultMsg})
		messages = append(messages, toolResultMsg)
		finalized = append(finalized, entry.final)
	}

	return &executedToolCallBatch{
		messages:  messages,
		terminate: shouldTerminateBatch(finalized),
	}
}

// ============================================================================
// Tool call preparation & execution
// ============================================================================

type preparedToolCall struct {
	immediate bool
	toolCall  ai.ContentBlock
	tool      *Tool
	args      map[string]any
	result    *ToolResult
	isError   bool
}

func prepareToolCall(
	currentContext *AgentContext,
	assistantMessage *ai.AssistantMessage,
	toolCall ai.ContentBlock,
	config *LoopConfig,
	ctx context.Context,
) *preparedToolCall {
	var tool *Tool
	for _, t := range currentContext.Tools {
		if t.Name == toolCall.ToolCallName {
			tool = t
			break
		}
	}

	if tool == nil {
		return &preparedToolCall{
			immediate: true,
			toolCall:  toolCall,
			result:    newErrorToolResult("Tool " + toolCall.ToolCallName + " not found"),
			isError:   true,
		}
	}

	args := toolCall.ToolCallArguments
	if args == nil {
		args = map[string]any{}
	}
	if tool.PrepareArguments != nil {
		args = tool.PrepareArguments(args)
	}

	// Validate arguments against tool schema
	validatedArgs, validateErr := ValidateToolArguments(tool, args)
	if validateErr != nil {
		return &preparedToolCall{
			immediate: true,
			toolCall:  toolCall,
			result:    newErrorToolResult(validateErr.Error()),
			isError:   true,
		}
	}
	args = validatedArgs

	if config.BeforeToolCall != nil {
		result, _ := config.BeforeToolCall(BeforeToolCallContext{
			AssistantMessage: assistantMessage,
			ToolCall:         toolCall,
			Args:             args,
			Context:          currentContext,
		})
		if result != nil && result.Block {
			reason := result.Reason
			if reason == "" {
				reason = "Tool execution was blocked"
			}
			return &preparedToolCall{
				immediate: true,
				toolCall:  toolCall,
				result:    newErrorToolResult(reason),
				isError:   true,
			}
		}
	}

	if ctx.Err() != nil {
		return &preparedToolCall{
			immediate: true,
			toolCall:  toolCall,
			result:    newErrorToolResult("operation aborted"),
			isError:   true,
		}
	}

	return &preparedToolCall{
		immediate: false,
		toolCall:  toolCall,
		tool:      tool,
		args:      args,
	}
}

type executedToolOutcome struct {
	result  *ToolResult
	isError bool
}

func executePreparedToolCall(
	ctx context.Context,
	prepared *preparedToolCall,
	emit EventSink,
) *executedToolOutcome {
	if prepared.tool == nil || prepared.tool.Execute == nil {
		return &executedToolOutcome{
			result:  newErrorToolResult("tool has no execute function"),
			isError: true,
		}
	}

	result, err := prepared.tool.Execute(ctx, prepared.toolCall.ToolCallID, prepared.args, func(partial ToolResult) {
		_ = emit(Event{
			Type:          EventToolExecutionUpdate,
			ToolCallID:    prepared.toolCall.ToolCallID,
			ToolName:      prepared.toolCall.ToolCallName,
			Args:          prepared.args,
			PartialResult: partial,
		})
	})

	if err != nil {
		return &executedToolOutcome{
			result:  newErrorToolResult(err.Error()),
			isError: true,
		}
	}

	return &executedToolOutcome{
		result:  result,
		isError: false,
	}
}

func finalizeExecutedToolCall(
	currentContext *AgentContext,
	assistantMessage *ai.AssistantMessage,
	prepared *preparedToolCall,
	executed *executedToolOutcome,
	config *LoopConfig,
	_ context.Context,
) finalizedToolCall {
	result := executed.result
	isError := executed.isError

	if config.AfterToolCall != nil {
		afterResult, _ := config.AfterToolCall(AfterToolCallContext{
			AssistantMessage: assistantMessage,
			ToolCall:         prepared.toolCall,
			Args:             prepared.args,
			Result:           result,
			IsError:          isError,
			Context:          currentContext,
		})
		if afterResult != nil {
			if afterResult.Content != nil {
				result.Content = afterResult.Content
			}
			if afterResult.Details != nil {
				result.Details = afterResult.Details
			}
			if afterResult.IsError != nil {
				isError = *afterResult.IsError
			}
			if afterResult.Terminate != nil {
				result.Terminate = *afterResult.Terminate
			}
		}
	}

	return finalizedToolCall{
		toolCall: prepared.toolCall,
		result:   result,
		isError:  isError,
	}
}

// ============================================================================
// Helpers
// ============================================================================

func shouldTerminateBatch(finalized []finalizedToolCall) bool {
	if len(finalized) == 0 {
		return false
	}
	for _, f := range finalized {
		if f.result == nil || !f.result.Terminate {
			return false
		}
	}
	return true
}

func createToolResultMessage(final finalizedToolCall) ai.Message {
	return ai.NewToolResultMessage(
		final.toolCall.ToolCallID,
		final.toolCall.ToolCallName,
		final.result.Content,
		final.isError,
	)
}
