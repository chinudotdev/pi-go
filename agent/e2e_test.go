package agent

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chinudotdev/pi-go/ai"
)

// ============================================================================
// E2E Agent Tests: Full Agent + Mock Stream pipeline
// ============================================================================

// fauxStreamFn creates a mock stream function that returns the given text.
// This simulates a complete streaming response from a faux provider.
func fauxStreamFn(responses ...string) StreamFn {
	idx := int64(0)
	return func(ctx context.Context, model *ai.Model, convCtx *ai.Context, opts *ai.SimpleStreamOptions) (*ai.EventStream, error) {
		i := int(atomic.AddInt64(&idx, 1)) - 1
		text := "default response"
		if i < len(responses) {
			text = responses[i]
		}

		stream := ai.NewEventStream(ctx)
		go func() {
			output := ai.NewAssistantOutput(model.API, model.Provider, model.ID)
			stream.Push(ai.AssistantMessageEvent{Type: "start", Partial: &output})
			output.Content = append(output.Content, ai.NewTextContent(text))
			output.StopReason = ai.StopReasonStop
			output.Timestamp = time.Now().UnixMilli()
			stream.Push(ai.AssistantMessageEvent{Type: "text_delta", Delta: &text, Partial: &output})
			stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonStop, Message: &output})
			stream.End(output)
		}()
		return stream, nil
	}
}

// 6.1 Basic Pipeline
func TestE2E_BasicTextPrompt(t *testing.T) {
	agent := New(Options{
		InitialState: &InitialState{
			Model:        testModel(),
			SystemPrompt: "You are helpful.",
		},
		StreamFn: fauxStreamFn("Hello! I am a test response."),
	})

	err := agent.Prompt(context.Background(), "Hi there")
	if err != nil {
		t.Fatalf("Prompt returned error: %v", err)
	}

	state := agent.State()
	if len(state.Messages) != 2 { // user + assistant
		t.Errorf("expected 2 messages, got %d", len(state.Messages))
	}

	// First message should be user
	if state.Messages[0].Role != "user" {
		t.Errorf("first message role = %q, want user", state.Messages[0].Role)
	}
	// Second message should be assistant
	if state.Messages[1].Role != "assistant" {
		t.Errorf("second message role = %q, want assistant", state.Messages[1].Role)
	}
	if state.Messages[1].StopReason != ai.StopReasonStop {
		t.Errorf("stop reason = %q, want stop", state.Messages[1].StopReason)
	}
}

// 6.2 Tool Execution
func TestE2E_ToolExecutionAndPendingCalls(t *testing.T) {
	toolExecuted := int64(0)

	testTool := &Tool{
		Name:        "get_weather",
		Description: "Get weather",
		Parameters:  map[string]any{"type": "object"},
		Execute: func(ctx context.Context, toolCallID string, params map[string]any, onUpdate ToolUpdateCallback) (*ToolResult, error) {
			atomic.AddInt64(&toolExecuted, 1)
			return &ToolResult{
				Content: []ai.ContentBlock{ai.NewTextContent("sunny, 72°F")},
				Details: map[string]any{},
			}, nil
		},
	}

	callCount := int64(0)
	streamFn := func(ctx context.Context, model *ai.Model, convCtx *ai.Context, opts *ai.SimpleStreamOptions) (*ai.EventStream, error) {
		i := int(atomic.AddInt64(&callCount, 1)) - 1
		stream := ai.NewEventStream(ctx)
		go func() {
			output := ai.NewAssistantOutput(model.API, model.Provider, model.ID)
			stream.Push(ai.AssistantMessageEvent{Type: "start", Partial: &output})
			if i == 0 {
				tc := ai.NewToolCallContent("tc_1", "get_weather", map[string]any{"city": "SF"})
				output.Content = append(output.Content, tc)
				output.StopReason = ai.StopReasonToolUse
				stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonToolUse, Message: &output})
			} else {
				text := "The weather in SF is sunny and 72°F"
				output.Content = append(output.Content, ai.NewTextContent(text))
				output.StopReason = ai.StopReasonStop
				stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonStop, Message: &output})
			}
			stream.End(output)
		}()
		return stream, nil
	}

	agent := New(Options{
		InitialState: &InitialState{
			Model: testModel(),
			Tools: []*Tool{testTool},
		},
		StreamFn:      streamFn,
		ToolExecution: ToolExecutionSequential,
	})

	var events []Event
	agent.Subscribe(func(e Event) error {
		events = append(events, e)
		return nil
	})

	err := agent.Prompt(context.Background(), "What's the weather in SF?")
	if err != nil {
		t.Fatal(err)
	}

	// Tool should have executed
	if atomic.LoadInt64(&toolExecuted) != 1 {
		t.Errorf("tool executed %d times, want 1", toolExecuted)
	}

	// Should have tool_execution events
	types := eventTypes(events)
	assertHasEvent(t, types, EventToolExecutionStart)
	assertHasEvent(t, types, EventToolExecutionEnd)

	// State should have: user, assistant (tool call), tool result, assistant (summary)
	state := agent.State()
	if len(state.Messages) < 4 {
		t.Errorf("expected at least 4 messages, got %d", len(state.Messages))
	}

	// Verify no pending tool calls remain
	if len(state.PendingToolCalls) != 0 {
		t.Errorf("pending tool calls = %d, want 0", len(state.PendingToolCalls))
	}
}

// 6.3 Abort
func TestE2E_AbortDuringStreaming(t *testing.T) {
	agent := New(Options{
		InitialState: &InitialState{Model: testModel()},
		StreamFn: func(ctx context.Context, model *ai.Model, convCtx *ai.Context, opts *ai.SimpleStreamOptions) (*ai.EventStream, error) {
			stream := ai.NewEventStream(ctx)
			go func() {
				time.Sleep(200 * time.Millisecond) // Hold open
				output := ai.NewAssistantOutput(model.API, model.Provider, model.ID)
				output.Content = append(output.Content, ai.NewTextContent("done"))
				output.StopReason = ai.StopReasonStop
				stream.Push(ai.AssistantMessageEvent{Type: "start", Partial: &output})
				stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonStop, Message: &output})
				stream.End(output)
			}()
			return stream, nil
		},
	})

	done := make(chan error, 1)
	go func() {
		done <- agent.Prompt(context.Background(), "hello")
	}()

	time.Sleep(20 * time.Millisecond)
	agent.Abort()

	select {
	case err := <-done:
		if err != nil {
			t.Logf("Prompt returned error after abort: %v (acceptable)", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("prompt did not resolve after abort")
	}
}

// 6.4 Lifecycle Events
func TestE2E_LifecycleEventsDuringStreaming(t *testing.T) {
	agent := New(Options{
		InitialState: &InitialState{Model: testModel()},
		StreamFn:    fauxStreamFn("response"),
	})

	var events []Event
	agent.Subscribe(func(e Event) error {
		events = append(events, e)
		return nil
	})

	err := agent.Prompt(context.Background(), "hello")
	if err != nil {
		t.Fatal(err)
	}

	types := eventTypes(events)

	// Should have complete lifecycle
	assertHasEvent(t, types, EventAgentStart)
	assertHasEvent(t, types, EventTurnStart)
	assertHasEvent(t, types, EventMessageStart)
	assertHasEvent(t, types, EventMessageEnd)
	assertHasEvent(t, types, EventTurnEnd)
	assertHasEvent(t, types, EventAgentEnd)

	// agent_start should be first
	if types[0] != EventAgentStart {
		t.Errorf("first event = %q, want agent_start", types[0])
	}
	// agent_end should be last
	if types[len(types)-1] != EventAgentEnd {
		t.Errorf("last event = %q, want agent_end", types[len(types)-1])
	}
}

// 6.5 Multi-Turn
func TestE2E_MultiTurnContext(t *testing.T) {
	var callCtxs [][]ai.Message

	streamFn := func(ctx context.Context, model *ai.Model, convCtx *ai.Context, opts *ai.SimpleStreamOptions) (*ai.EventStream, error) {
		msgs := make([]ai.Message, len(convCtx.Messages))
		copy(msgs, convCtx.Messages)
		callCtxs = append(callCtxs, msgs)

		stream := ai.NewEventStream(ctx)
		go func() {
			output := ai.NewAssistantOutput(model.API, model.Provider, model.ID)
			stream.Push(ai.AssistantMessageEvent{Type: "start", Partial: &output})
			output.Content = append(output.Content, ai.NewTextContent("response"))
			output.StopReason = ai.StopReasonStop
			stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonStop, Message: &output})
			stream.End(output)
		}()
		return stream, nil
	}

	agent := New(Options{
		InitialState: &InitialState{Model: testModel()},
		StreamFn:    streamFn,
	})

	// Turn 1
	agent.Prompt(context.Background(), "message 1")
	// Turn 2
	agent.Prompt(context.Background(), "message 2")

	// Second call should see both user messages + first assistant response
	if len(callCtxs) < 2 {
		t.Fatalf("expected 2 calls, got %d", len(callCtxs))
	}
	secondCtx := callCtxs[1]
	// Should have: user1, assistant1, user2
	if len(secondCtx) < 3 {
		t.Errorf("second call context = %d messages, want at least 3", len(secondCtx))
	}
}

// 6.6 Thinking Blocks
func TestE2E_ThinkingContentBlocks(t *testing.T) {
	agent := New(Options{
		InitialState: &InitialState{Model: testModel()},
		StreamFn: func(ctx context.Context, model *ai.Model, convCtx *ai.Context, opts *ai.SimpleStreamOptions) (*ai.EventStream, error) {
			stream := ai.NewEventStream(ctx)
			go func() {
				output := ai.NewAssistantOutput(model.API, model.Provider, model.ID)
				stream.Push(ai.AssistantMessageEvent{Type: "start", Partial: &output})

				// Add thinking block
				thinking := ai.NewThinkingContent("Let me think about this...")
				output.Content = append(output.Content, thinking)
				stream.Push(ai.AssistantMessageEvent{
					Type:    "thinking_delta",
					Partial: &output,
				})

				// Add text block
				text := "Here is my answer."
				output.Content = append(output.Content, ai.NewTextContent(text))
				stream.Push(ai.AssistantMessageEvent{
					Type:    "text_delta",
					Delta:   &text,
					Partial: &output,
				})

				output.StopReason = ai.StopReasonStop
				output.Timestamp = time.Now().UnixMilli()
				stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonStop, Message: &output})
				stream.End(output)
			}()
			return stream, nil
		},
	})

	err := agent.Prompt(context.Background(), "think and answer")
	if err != nil {
		t.Fatal(err)
	}

	state := agent.State()
	// Last message should be assistant with thinking + text
	if len(state.Messages) < 2 {
		t.Fatal("expected at least 2 messages")
	}
	lastMsg := state.Messages[len(state.Messages)-1]
	if lastMsg.Role != "assistant" {
		t.Fatalf("last message role = %q, want assistant", lastMsg.Role)
	}
	if len(lastMsg.AssistantContent) < 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(lastMsg.AssistantContent))
	}
	if lastMsg.AssistantContent[0].Type != "thinking" {
		t.Errorf("first content = %q, want thinking", lastMsg.AssistantContent[0].Type)
	}
	if lastMsg.AssistantContent[1].Type != "text" {
		t.Errorf("second content = %q, want text", lastMsg.AssistantContent[1].Type)
	}
}

// 6.7 Continue Validation - empty context
func TestE2E_Continue_ThrowsWhenNoMessages(t *testing.T) {
	agent := New(Options{
		InitialState: &InitialState{Model: testModel()},
		StreamFn:    fauxStreamFn("ok"),
	})

	err := agent.Continue(context.Background())
	if err == nil {
		t.Error("expected error when continuing with no messages")
	}
}

// 6.8 Continue when last is assistant
func TestE2E_Continue_ThrowsWhenLastIsAssistant(t *testing.T) {
	agent := New(Options{
		InitialState: &InitialState{
			Model: testModel(),
			Messages: []AgentMessage{
				ai.NewUserMessage("hello"),
				ai.NewAssistantMessage("test", "test", "test-model",
					[]ai.ContentBlock{ai.NewTextContent("hi")},
					ai.Usage{}, ai.StopReasonStop),
			},
		},
		StreamFn: fauxStreamFn("ok"),
	})

	err := agent.Continue(context.Background())
	if err == nil {
		t.Error("expected error when continuing with last message as assistant (no queued messages)")
	}
}

// 6.9 Continue when last is user
func TestE2E_Continue_RespondsWhenLastIsUser(t *testing.T) {
	agent := New(Options{
		InitialState: &InitialState{
			Model: testModel(),
			Messages: []AgentMessage{
				ai.NewUserMessage("hello"),
			},
		},
		StreamFn: fauxStreamFn("response"),
	})

	err := agent.Continue(context.Background())
	if err != nil {
		t.Fatalf("Continue returned error: %v", err)
	}

	state := agent.State()
	// Should have: user (initial) + assistant (from continue)
	if len(state.Messages) < 2 {
		t.Errorf("expected at least 2 messages after continue, got %d", len(state.Messages))
	}
}

// 6.10 Continue processes tool results
func TestE2E_Continue_ProcessesToolResults(t *testing.T) {
	testTool := &Tool{
		Name:        "calc",
		Description: "calculator",
		Parameters:  map[string]any{"type": "object"},
		Execute: func(ctx context.Context, toolCallID string, params map[string]any, onUpdate ToolUpdateCallback) (*ToolResult, error) {
			return &ToolResult{
				Content: []ai.ContentBlock{ai.NewTextContent("42")},
				Details: map[string]any{},
			}, nil
		},
	}

	callCount := int64(0)
	streamFn := func(ctx context.Context, model *ai.Model, convCtx *ai.Context, opts *ai.SimpleStreamOptions) (*ai.EventStream, error) {
		i := int(atomic.AddInt64(&callCount, 1)) - 1
		stream := ai.NewEventStream(ctx)
		go func() {
			output := ai.NewAssistantOutput(model.API, model.Provider, model.ID)
			stream.Push(ai.AssistantMessageEvent{Type: "start", Partial: &output})
			if i == 0 {
				tc := ai.NewToolCallContent("tc_1", "calc", map[string]any{"expr": "6*7"})
				output.Content = append(output.Content, tc)
				output.StopReason = ai.StopReasonToolUse
				stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonToolUse, Message: &output})
			} else {
				text := "The answer is 42"
				output.Content = append(output.Content, ai.NewTextContent(text))
				output.StopReason = ai.StopReasonStop
				stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonStop, Message: &output})
			}
			stream.End(output)
		}()
		return stream, nil
	}

	agent := New(Options{
		InitialState: &InitialState{
			Model: testModel(),
			Messages: []AgentMessage{
				ai.NewUserMessage("what is 6*7?"),
			},
			Tools: []*Tool{testTool},
		},
		StreamFn:      streamFn,
		ToolExecution: ToolExecutionSequential,
	})

	err := agent.Continue(context.Background())
	if err != nil {
		t.Fatalf("Continue returned error: %v", err)
	}

	// Should have 2 LLM calls: tool call + summary
	if atomic.LoadInt64(&callCount) != 2 {
		t.Errorf("expected 2 LLM calls, got %d", callCount)
	}

	state := agent.State()
	// user + assistant(tool_call) + tool_result + assistant(summary) = 4
	if len(state.Messages) < 4 {
		t.Errorf("expected at least 4 messages, got %d", len(state.Messages))
	}
}
