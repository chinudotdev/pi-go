package agent

import (
	"context"

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


