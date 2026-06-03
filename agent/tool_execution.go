package agent

import (
	"context"
	"sync"

	"github.com/chinudotdev/pi-go/ai"
)

// ============================================================================
// Tool execution types
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

// ============================================================================
// Tool call dispatch
// ============================================================================

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
// Tool call lifecycle: prepare → execute → finalize
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
