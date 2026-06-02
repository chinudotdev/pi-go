package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chinudotdev/pi-go/ai"
)

// ============================================================================
// Helper: create a test model
// ============================================================================

func testModel() *ai.Model {
	return &ai.Model{
		ID:       "test-model",
		Provider: "test",
		API:      "openai-chat-completions",
		BaseURL:  "https://test.example.com/v1",
		Input:    []string{"text"},
	}
}

// ============================================================================
// Mock stream function
// ============================================================================

func mockStreamFn(responses ...string) StreamFn {
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

			stream.Push(ai.AssistantMessageEvent{
				Type:    "start",
				Partial: &output,
			})

			output.Content = append(output.Content, ai.NewTextContent(text))
			stream.Push(ai.AssistantMessageEvent{
				Type:    "text_delta",
				Delta:   &text,
				Partial: &output,
			})

			output.StopReason = ai.StopReasonStop
			output.Timestamp = time.Now().UnixMilli()

			stream.Push(ai.AssistantMessageEvent{
				Type:    "done",
				Reason:  ai.StopReasonStop,
				Message: &output,
			})
			stream.End(output)
		}()
		return stream, nil
	}
}

// ============================================================================
// RunAgentLoop tests
// ============================================================================

func TestRunAgentLoop_SinglePrompt(t *testing.T) {
	var events []Event
	emit := func(e Event) error {
		events = append(events, e)
		return nil
	}

	agentCtx := &AgentContext{
		SystemPrompt: "You are helpful.",
		Tools:        nil,
	}
	config := &LoopConfig{
		Model:        testModel(),
		ConvertToLlm: DefaultConvertToLlm,
	}

	err := RunAgentLoop(context.Background(), []AgentMessage{ai.NewUserMessage("hello")}, agentCtx, config, emit, mockStreamFn("hi there"))
	if err != nil {
		t.Fatal(err)
	}

	// Should have: agent_start, turn_start, message_start(user), message_end(user),
	//              message_start(assistant streaming), message_update, message_end(assistant),
	//              turn_end, agent_end
	types := eventTypes(events)
	if types[0] != EventAgentStart {
		t.Errorf("first event = %q, want agent_start", types[0])
	}
	if types[len(types)-1] != EventAgentEnd {
		t.Errorf("last event = %q, want agent_end", types[len(types)-1])
	}
	assertHasEvent(t, types, EventTurnEnd)
	assertHasEvent(t, types, EventMessageEnd)

	// Check agent_end has messages
	lastEvent := events[len(events)-1]
	if len(lastEvent.Messages) == 0 {
		t.Error("agent_end should have messages")
	}
	t.Logf("Events: %v", types)
	t.Logf("Messages: %d", len(lastEvent.Messages))
}

func TestRunAgentLoop_MultiTurnWithTools(t *testing.T) {
	// Tool that returns a result
	testTool := &Tool{
		Name:        "read_file",
		Description: "Read a file",
		Parameters:  map[string]any{"type": "object"},
		Execute: func(ctx context.Context, toolCallID string, params map[string]any, onUpdate ToolUpdateCallback) (*ToolResult, error) {
			return &ToolResult{
				Content: []ai.ContentBlock{ai.NewTextContent("file contents here")},
				Details: map[string]any{},
			}, nil
		},
	}

	// First response: tool call. Second response: text summary.
	callCount := int64(0)
	streamFn := func(ctx context.Context, model *ai.Model, convCtx *ai.Context, opts *ai.SimpleStreamOptions) (*ai.EventStream, error) {
		i := int(atomic.AddInt64(&callCount, 1)) - 1
		stream := ai.NewEventStream(ctx)
		go func() {
			output := ai.NewAssistantOutput(model.API, model.Provider, model.ID)
			stream.Push(ai.AssistantMessageEvent{Type: "start", Partial: &output})

			if i == 0 {
				// First call: emit a tool call
				tc := ai.NewToolCallContent("tc_1", "read_file", map[string]any{"path": "/tmp/test.txt"})
				output.Content = append(output.Content, tc)
				output.StopReason = ai.StopReasonToolUse
				stream.Push(ai.AssistantMessageEvent{
					Type:    "toolcall_delta",
					Partial: &output,
				})
				stream.Push(ai.AssistantMessageEvent{
					Type:    "done",
					Reason:  ai.StopReasonToolUse,
					Message: &output,
				})
			} else {
				// Second call: text response
				text := "The file contains: file contents here"
				output.Content = append(output.Content, ai.NewTextContent(text))
				output.StopReason = ai.StopReasonStop
				stream.Push(ai.AssistantMessageEvent{
					Type:    "text_delta",
					Delta:   &text,
					Partial: &output,
				})
				stream.Push(ai.AssistantMessageEvent{
					Type:    "done",
					Reason:  ai.StopReasonStop,
					Message: &output,
				})
			}
			stream.End(output)
		}()
		return stream, nil
	}

	var events []Event
	emit := func(e Event) error {
		events = append(events, e)
		return nil
	}

	agentCtx := &AgentContext{
		SystemPrompt: "You are helpful.",
		Tools:        []*Tool{testTool},
	}
	config := &LoopConfig{
		Model:         testModel(),
		ConvertToLlm:  DefaultConvertToLlm,
		ToolExecution: ToolExecutionSequential,
	}

	err := RunAgentLoop(context.Background(), []AgentMessage{ai.NewUserMessage("read the file")}, agentCtx, config, emit, streamFn)
	if err != nil {
		t.Fatal(err)
	}

	types := eventTypes(events)
	assertHasEvent(t, types, EventToolExecutionStart)
	assertHasEvent(t, types, EventToolExecutionEnd)
	assertHasEvent(t, types, EventTurnEnd)

	// Should have 2 turn_end events (tool call turn + summary turn)
	turnEndCount := countEvent(types, EventTurnEnd)
	if turnEndCount != 2 {
		t.Errorf("expected 2 turn_end events, got %d", turnEndCount)
	}

	lastEvent := events[len(events)-1]
	t.Logf("Events: %v", types)
	t.Logf("Total messages: %d", len(lastEvent.Messages))
}

func TestRunAgentLoop_BeforeToolCallBlock(t *testing.T) {
	testTool := &Tool{
		Name:        "dangerous",
		Description: "A dangerous tool",
		Execute: func(ctx context.Context, toolCallID string, params map[string]any, onUpdate ToolUpdateCallback) (*ToolResult, error) {
			t.Error("tool should not have been executed")
			return &ToolResult{}, nil
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
				tc := ai.NewToolCallContent("tc_1", "dangerous", map[string]any{})
				output.Content = append(output.Content, tc)
				output.StopReason = ai.StopReasonToolUse
				stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonToolUse, Message: &output})
			} else {
				text := "ok blocked"
				output.Content = append(output.Content, ai.NewTextContent(text))
				output.StopReason = ai.StopReasonStop
				stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonStop, Message: &output})
			}
			stream.End(output)
		}()
		return stream, nil
	}

	var events []Event
	emit := func(e Event) error {
		events = append(events, e)
		return nil
	}

	agentCtx := &AgentContext{
		Tools: []*Tool{testTool},
	}
	config := &LoopConfig{
		Model:        testModel(),
		ConvertToLlm: DefaultConvertToLlm,
		BeforeToolCall: func(ctx BeforeToolCallContext) (*BeforeToolCallResult, error) {
			return &BeforeToolCallResult{Block: true, Reason: "blocked for safety"}, nil
		},
	}

	err := RunAgentLoop(context.Background(), []AgentMessage{ai.NewUserMessage("do it")}, agentCtx, config, emit, streamFn)
	if err != nil {
		t.Fatal(err)
	}

	// Tool should have been blocked — look for error tool result
	types := eventTypes(events)
	toolEndIdx := indexOf(types, EventToolExecutionEnd)
	if toolEndIdx < 0 {
		t.Fatal("expected tool_execution_end event")
	}
	if !events[toolEndIdx].IsError {
		t.Error("tool_execution_end should have IsError=true since it was blocked")
	}
	t.Logf("Events: %v", types)
}

func TestRunAgentLoop_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	agentCtx := &AgentContext{}
	config := &LoopConfig{
		Model:        testModel(),
		ConvertToLlm: DefaultConvertToLlm,
	}

	emit := func(e Event) error { return nil }
	_ = RunAgentLoop(ctx, []AgentMessage{ai.NewUserMessage("hello")}, agentCtx, config, emit, mockStreamFn("hi"))
	// Should not hang
}

func TestRunAgentLoopContinue_NoMessages(t *testing.T) {
	agentCtx := &AgentContext{}
	config := &LoopConfig{Model: testModel(), ConvertToLlm: DefaultConvertToLlm}
	emit := func(e Event) error { return nil }

	err := RunAgentLoopContinue(context.Background(), agentCtx, config, emit, mockStreamFn())
	if err == nil {
		t.Error("expected error for empty context")
	}
	if !strings.Contains(err.Error(), "no messages") {
		t.Errorf("error = %q, should mention 'no messages'", err.Error())
	}
}

func TestRunAgentLoopContinue_LastMessageAssistant(t *testing.T) {
	agentCtx := &AgentContext{
		Messages: []AgentMessage{
			ai.NewAssistantMessage("test", "test", "test-model", []ai.ContentBlock{ai.NewTextContent("hi")}, ai.Usage{}, ai.StopReasonStop),
		},
	}
	config := &LoopConfig{Model: testModel(), ConvertToLlm: DefaultConvertToLlm}
	emit := func(e Event) error { return nil }

	err := RunAgentLoopContinue(context.Background(), agentCtx, config, emit, mockStreamFn())
	if err == nil {
		t.Error("expected error for last message being assistant")
	}
}

// ============================================================================
// Section 1: Agent Loop Behavioral Tests
// ============================================================================

// 1.1 Event Sequence Verification
func TestRunAgentLoop_FullEventSequence(t *testing.T) {
	var events []Event
	emit := func(e Event) error {
		events = append(events, e)
		return nil
	}

	agentCtx := &AgentContext{SystemPrompt: "You are helpful."}
	config := &LoopConfig{Model: testModel(), ConvertToLlm: DefaultConvertToLlm}

	err := RunAgentLoop(context.Background(), []AgentMessage{ai.NewUserMessage("hello")}, agentCtx, config, emit, mockStreamFn("response"))
	if err != nil {
		t.Fatal(err)
	}

	types := eventTypes(events)

	// Verify exact lifecycle: agent_start → turn_start → message_start → message_end (user)
	// → message_start → message_update → message_end (assistant) → turn_end → agent_end
	if len(types) < 6 {
		t.Fatalf("expected at least 6 events, got %d: %v", len(types), types)
	}
	if types[0] != EventAgentStart {
		t.Errorf("first event = %q, want agent_start", types[0])
	}
	if types[1] != EventTurnStart {
		t.Errorf("second event = %q, want turn_start", types[1])
	}
	// Find user message events
	foundUserStart := false
	foundUserEnd := false
	foundAssistantStart := false
	foundAssistantEnd := false
	foundTurnEnd := false
	foundAgentEnd := false
	for _, typ := range types {
		switch typ {
		case EventMessageStart:
			if !foundUserStart {
				foundUserStart = true
			} else {
				foundAssistantStart = true
			}
		case EventMessageEnd:
			if !foundUserEnd {
				foundUserEnd = true
			} else {
				foundAssistantEnd = true
			}
		case EventTurnEnd:
			foundTurnEnd = true
		case EventAgentEnd:
			foundAgentEnd = true
		}
	}
	if !foundUserStart {
		t.Error("missing user message_start")
	}
	if !foundUserEnd {
		t.Error("missing user message_end")
	}
	if !foundAssistantStart {
		t.Error("missing assistant message_start")
	}
	if !foundAssistantEnd {
		t.Error("missing assistant message_end")
	}
	if !foundTurnEnd {
		t.Error("missing turn_end")
	}
	if !foundAgentEnd {
		t.Error("missing agent_end")
	}
	if types[len(types)-1] != EventAgentEnd {
		t.Errorf("last event = %q, want agent_end", types[len(types)-1])
	}
}

// 1.2 Context Transformation
func TestRunAgentLoop_TransformContext(t *testing.T) {
	var capturedMessages []ai.Message

	convertFn := func(messages []AgentMessage) ([]ai.Message, error) {
		result, err := DefaultConvertToLlm(messages)
		capturedMessages = result
		return result, err
	}

	transformCtx := func(_ context.Context, messages []AgentMessage) ([]AgentMessage, error) {
		// Keep only the last 2 messages
		if len(messages) > 2 {
			return messages[len(messages)-2:], nil
		}
		return messages, nil
	}

	emit := func(e Event) error { return nil }

	agentCtx := &AgentContext{
		SystemPrompt: "test",
		Messages:     []AgentMessage{ai.NewUserMessage("m1"), ai.NewUserMessage("m2")},
	}
	config := &LoopConfig{
		Model:            testModel(),
		ConvertToLlm:     convertFn,
		TransformContext:  transformCtx,
	}

	err := RunAgentLoop(context.Background(), []AgentMessage{ai.NewUserMessage("m3"), ai.NewUserMessage("m4")}, agentCtx, config, emit, mockStreamFn("ok"))
	if err != nil {
		t.Fatal(err)
	}

	// After transformContext (keep last 2), convertToLlm should receive only 2 messages
	if len(capturedMessages) != 2 {
		t.Errorf("convertToLlm received %d messages, want 2 (after pruning)", len(capturedMessages))
	}
}

func TestRunAgentLoop_CustomMessageTypesViaConvertToLlm(t *testing.T) {
	var capturedRoles []string

	convertFn := func(messages []AgentMessage) ([]ai.Message, error) {
		var result []ai.Message
		for _, m := range messages {
			capturedRoles = append(capturedRoles, m.Role)
			if m.Role == "user" || m.Role == "assistant" || m.Role == "toolResult" {
				result = append(result, m)
			}
			// Filter out custom roles like "notification"
		}
		return result, nil
	}

	emit := func(e Event) error { return nil }

	notificationMsg := ai.Message{Role: "notification", Content: "system note", Timestamp: 1}

	agentCtx := &AgentContext{
		Messages: []AgentMessage{notificationMsg},
	}
	config := &LoopConfig{
		Model:        testModel(),
		ConvertToLlm: convertFn,
	}

	err := RunAgentLoop(context.Background(), []AgentMessage{ai.NewUserMessage("hello")}, agentCtx, config, emit, mockStreamFn("ok"))
	if err != nil {
		t.Fatal(err)
	}

	// Should have captured both roles but filtered out notification
	foundNotification := false
	for _, r := range capturedRoles {
		if r == "notification" {
			foundNotification = true
		}
	}
	if !foundNotification {
		t.Error("convertToLlm should have seen the notification message")
	}
}

// 1.3 Tool Execution Modes
func TestRunAgentLoop_ParallelExecution(t *testing.T) {
	started := make(chan int, 2)

	makeTool := func(name string, delay time.Duration) *Tool {
		return &Tool{
			Name:        name,
			Description: name,
			Parameters:  map[string]any{"type": "object"},
			Execute: func(ctx context.Context, toolCallID string, params map[string]any, onUpdate ToolUpdateCallback) (*ToolResult, error) {
				started <- 1
				time.Sleep(delay)
				return &ToolResult{
					Content: []ai.ContentBlock{ai.NewTextContent(name + " done")},
					Details: map[string]any{},
			}, nil
			},
		}
	}

	tool1 := makeTool("slow_tool", 100*time.Millisecond)
	tool2 := makeTool("fast_tool", 10*time.Millisecond)

	callCount := int64(0)
	streamFn := func(ctx context.Context, model *ai.Model, convCtx *ai.Context, opts *ai.SimpleStreamOptions) (*ai.EventStream, error) {
		i := int(atomic.AddInt64(&callCount, 1)) - 1
		stream := ai.NewEventStream(ctx)
		go func() {
			output := ai.NewAssistantOutput(model.API, model.Provider, model.ID)
			stream.Push(ai.AssistantMessageEvent{Type: "start", Partial: &output})

			if i == 0 {
				tc1 := ai.NewToolCallContent("tc_1", "slow_tool", map[string]any{})
				tc2 := ai.NewToolCallContent("tc_2", "fast_tool", map[string]any{})
				output.Content = append(output.Content, tc1, tc2)
				output.StopReason = ai.StopReasonToolUse
				stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonToolUse, Message: &output})
			} else {
				text := "done"
				output.Content = append(output.Content, ai.NewTextContent(text))
				output.StopReason = ai.StopReasonStop
				stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonStop, Message: &output})
			}
			stream.End(output)
		}()
		return stream, nil
	}

	emit := func(e Event) error { return nil }
	agentCtx := &AgentContext{Tools: []*Tool{tool1, tool2}}
	config := &LoopConfig{
		Model:         testModel(),
		ConvertToLlm:  DefaultConvertToLlm,
		ToolExecution: ToolExecutionParallel,
	}

	err := RunAgentLoop(context.Background(), []AgentMessage{ai.NewUserMessage("run both")}, agentCtx, config, emit, streamFn)
	if err != nil {
		t.Fatal(err)
	}

	// Both tools should have started (parallel execution)
	totalStarted := 0
	timeout := time.After(200 * time.Millisecond)
	for totalStarted < 2 {
		select {
		case <-started:
			totalStarted++
		case <-timeout:
			t.Fatalf("only %d/2 tools started in time (expected parallel)", totalStarted)
		}
	}
}

func TestRunAgentLoop_SequentialExecution_ToolOverride(t *testing.T) {
	var executionOrder []string
	var mu sync.Mutex

	makeTool := func(name string, execMode ToolExecutionMode) *Tool {
		return &Tool{
			Name:          name,
			Description:   name,
			Parameters:    map[string]any{"type": "object"},
			ExecutionMode: execMode,
			Execute: func(ctx context.Context, toolCallID string, params map[string]any, onUpdate ToolUpdateCallback) (*ToolResult, error) {
				mu.Lock()
				executionOrder = append(executionOrder, name+"_start")
				mu.Unlock()
				time.Sleep(50 * time.Millisecond)
				mu.Lock()
				executionOrder = append(executionOrder, name+"_end")
				mu.Unlock()
				return &ToolResult{
					Content: []ai.ContentBlock{ai.NewTextContent(name + " done")},
					Details: map[string]any{},
				}, nil
			},
		}
	}

	tool1 := makeTool("tool_a", ToolExecutionSequential)
	tool2 := makeTool("tool_b", ToolExecutionParallel)

	callCount := int64(0)
	streamFn := func(ctx context.Context, model *ai.Model, convCtx *ai.Context, opts *ai.SimpleStreamOptions) (*ai.EventStream, error) {
		i := int(atomic.AddInt64(&callCount, 1)) - 1
		stream := ai.NewEventStream(ctx)
		go func() {
			output := ai.NewAssistantOutput(model.API, model.Provider, model.ID)
			stream.Push(ai.AssistantMessageEvent{Type: "start", Partial: &output})

			if i == 0 {
				tc1 := ai.NewToolCallContent("tc_1", "tool_a", map[string]any{})
				tc2 := ai.NewToolCallContent("tc_2", "tool_b", map[string]any{})
				output.Content = append(output.Content, tc1, tc2)
				output.StopReason = ai.StopReasonToolUse
				stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonToolUse, Message: &output})
			} else {
				text := "done"
				output.Content = append(output.Content, ai.NewTextContent(text))
				output.StopReason = ai.StopReasonStop
				stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonStop, Message: &output})
			}
			stream.End(output)
		}()
		return stream, nil
	}

	emit := func(e Event) error { return nil }
	agentCtx := &AgentContext{Tools: []*Tool{tool1, tool2}}
	config := &LoopConfig{
		Model:         testModel(),
		ConvertToLlm:  DefaultConvertToLlm,
		ToolExecution: ToolExecutionParallel, // default parallel, but tool_a has sequential override
	}

	err := RunAgentLoop(context.Background(), []AgentMessage{ai.NewUserMessage("run both")}, agentCtx, config, emit, streamFn)
	if err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	// tool_a should complete before tool_b starts (sequential)
	if len(executionOrder) < 4 {
		t.Fatalf("expected 4 order entries, got %d: %v", len(executionOrder), executionOrder)
	}
	if executionOrder[0] != "tool_a_start" || executionOrder[1] != "tool_a_end" {
		t.Errorf("expected tool_a to run first sequentially, got: %v", executionOrder)
	}
	if executionOrder[2] != "tool_b_start" {
		t.Errorf("expected tool_b to start after tool_a, got: %v", executionOrder)
	}
}

func TestRunAgentLoop_SequentialExecution_MixedModes(t *testing.T) {
	var executionOrder []string
	var mu sync.Mutex

	slowTool := &Tool{
		Name:          "slow",
		Description:   "slow tool",
		Parameters:    map[string]any{"type": "object"},
		ExecutionMode: ToolExecutionSequential,
		Execute: func(ctx context.Context, toolCallID string, params map[string]any, onUpdate ToolUpdateCallback) (*ToolResult, error) {
			mu.Lock()
			executionOrder = append(executionOrder, "slow_start")
			mu.Unlock()
			time.Sleep(100 * time.Millisecond)
			mu.Lock()
			executionOrder = append(executionOrder, "slow_end")
			mu.Unlock()
			return &ToolResult{Content: []ai.ContentBlock{ai.NewTextContent("slow done")}, Details: map[string]any{}}, nil
		},
	}
	fastTool := &Tool{
		Name:          "fast",
		Description:   "fast tool",
		Parameters:    map[string]any{"type": "object"},
		ExecutionMode: ToolExecutionParallel,
		Execute: func(ctx context.Context, toolCallID string, params map[string]any, onUpdate ToolUpdateCallback) (*ToolResult, error) {
			mu.Lock()
			executionOrder = append(executionOrder, "fast_start")
			mu.Unlock()
			mu.Lock()
			executionOrder = append(executionOrder, "fast_end")
			mu.Unlock()
			return &ToolResult{Content: []ai.ContentBlock{ai.NewTextContent("fast done")}, Details: map[string]any{}}, nil
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
				tc1 := ai.NewToolCallContent("tc_1", "slow", map[string]any{})
				tc2 := ai.NewToolCallContent("tc_2", "fast", map[string]any{})
				output.Content = append(output.Content, tc1, tc2)
				output.StopReason = ai.StopReasonToolUse
				stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonToolUse, Message: &output})
			} else {
				output.Content = append(output.Content, ai.NewTextContent("done"))
				output.StopReason = ai.StopReasonStop
				stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonStop, Message: &output})
			}
			stream.End(output)
		}()
		return stream, nil
	}

	emit := func(e Event) error { return nil }
	agentCtx := &AgentContext{Tools: []*Tool{slowTool, fastTool}}
	config := &LoopConfig{Model: testModel(), ConvertToLlm: DefaultConvertToLlm, ToolExecution: ToolExecutionParallel}

	err := RunAgentLoop(context.Background(), []AgentMessage{ai.NewUserMessage("go")}, agentCtx, config, emit, streamFn)
	if err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	// One sequential tool forces entire batch sequential: slow_start → slow_end → fast_start → fast_end
	if len(executionOrder) < 3 {
		t.Fatalf("expected at least 3 order entries, got %d: %v", len(executionOrder), executionOrder)
	}
	// Key assertion: slow must complete before fast starts
	if executionOrder[0] != "slow_start" || executionOrder[1] != "slow_end" {
		t.Errorf("slow should complete before fast starts, got: %v", executionOrder)
	}
}

// 1.4 Tool Call Hooks
func TestRunAgentLoop_BeforeToolCall_MutateArgs(t *testing.T) {
	var capturedArgs map[string]any

	testTool := &Tool{
		Name:        "mutate_test",
		Description: "test arg mutation",
		Parameters:  map[string]any{"type": "object"},
		Execute: func(ctx context.Context, toolCallID string, params map[string]any, onUpdate ToolUpdateCallback) (*ToolResult, error) {
			capturedArgs = params
			return &ToolResult{
				Content: []ai.ContentBlock{ai.NewTextContent("executed")},
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
				tc := ai.NewToolCallContent("tc_1", "mutate_test", map[string]any{"value": "hello"})
				output.Content = append(output.Content, tc)
				output.StopReason = ai.StopReasonToolUse
				stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonToolUse, Message: &output})
			} else {
				output.Content = append(output.Content, ai.NewTextContent("done"))
				output.StopReason = ai.StopReasonStop
				stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonStop, Message: &output})
			}
			stream.End(output)
		}()
		return stream, nil
	}

	emit := func(e Event) error { return nil }
	agentCtx := &AgentContext{Tools: []*Tool{testTool}}
	config := &LoopConfig{
		Model:        testModel(),
		ConvertToLlm: DefaultConvertToLlm,
		BeforeToolCall: func(ctx BeforeToolCallContext) (*BeforeToolCallResult, error) {
			// Mutate args: change value from string to int
			ctx.Args["value"] = 123
			return nil, nil
		},
	}

	err := RunAgentLoop(context.Background(), []AgentMessage{ai.NewUserMessage("run")}, agentCtx, config, emit, streamFn)
	if err != nil {
		t.Fatal(err)
	}

	if capturedArgs == nil {
		t.Fatal("tool was not executed")
	}
	if capturedArgs["value"] != 123 {
		t.Errorf("expected mutated value 123, got %v", capturedArgs["value"])
	}
}

func TestRunAgentLoop_PrepareArguments(t *testing.T) {
	var capturedArgs map[string]any

	testTool := &Tool{
		Name:        "edit",
		Description: "edit tool",
		Parameters:  map[string]any{"type": "object"},
		PrepareArguments: func(args map[string]any) map[string]any {
			// Wrap oldText/newText into edits array
			return map[string]any{
				"edits": []map[string]any{
					{"oldText": args["oldText"], "newText": args["newText"]},
				},
			}
		},
		Execute: func(ctx context.Context, toolCallID string, params map[string]any, onUpdate ToolUpdateCallback) (*ToolResult, error) {
			capturedArgs = params
			return &ToolResult{
				Content: []ai.ContentBlock{ai.NewTextContent("edited")},
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
				tc := ai.NewToolCallContent("tc_1", "edit", map[string]any{"oldText": "foo", "newText": "bar"})
				output.Content = append(output.Content, tc)
				output.StopReason = ai.StopReasonToolUse
				stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonToolUse, Message: &output})
			} else {
				output.Content = append(output.Content, ai.NewTextContent("done"))
				output.StopReason = ai.StopReasonStop
				stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonStop, Message: &output})
			}
			stream.End(output)
		}()
		return stream, nil
	}

	emit := func(e Event) error { return nil }
	agentCtx := &AgentContext{Tools: []*Tool{testTool}}
	config := &LoopConfig{Model: testModel(), ConvertToLlm: DefaultConvertToLlm}

	err := RunAgentLoop(context.Background(), []AgentMessage{ai.NewUserMessage("edit")}, agentCtx, config, emit, streamFn)
	if err != nil {
		t.Fatal(err)
	}

	if capturedArgs == nil {
		t.Fatal("tool was not executed")
	}
	edits, ok := capturedArgs["edits"]
	if !ok {
		t.Fatal("expected 'edits' key in prepared arguments")
	}
	editsSlice, ok := edits.([]map[string]any)
	if !ok || len(editsSlice) != 1 {
		t.Fatalf("expected edits to be []map[string]any with 1 entry, got %T: %v", edits, edits)
	}
	if editsSlice[0]["oldText"] != "foo" || editsSlice[0]["newText"] != "bar" {
		t.Errorf("unexpected edit content: %v", editsSlice[0])
	}
}

// 1.5 Tool Execution Ordering
func TestRunAgentLoop_ToolExecutionEndCompletionOrder(t *testing.T) {
	testTool1 := &Tool{
		Name:        "slow",
		Description: "slow",
		Parameters:  map[string]any{"type": "object"},
		Execute: func(ctx context.Context, toolCallID string, params map[string]any, onUpdate ToolUpdateCallback) (*ToolResult, error) {
			time.Sleep(100 * time.Millisecond)
			return &ToolResult{Content: []ai.ContentBlock{ai.NewTextContent("slow done")}, Details: map[string]any{}}, nil
		},
	}
	testTool2 := &Tool{
		Name:        "fast",
		Description: "fast",
		Parameters:  map[string]any{"type": "object"},
		Execute: func(ctx context.Context, toolCallID string, params map[string]any, onUpdate ToolUpdateCallback) (*ToolResult, error) {
			return &ToolResult{Content: []ai.ContentBlock{ai.NewTextContent("fast done")}, Details: map[string]any{}}, nil
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
				tc1 := ai.NewToolCallContent("tc_slow", "slow", map[string]any{})
				tc2 := ai.NewToolCallContent("tc_fast", "fast", map[string]any{})
				output.Content = append(output.Content, tc1, tc2)
				output.StopReason = ai.StopReasonToolUse
				stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonToolUse, Message: &output})
			} else {
				output.Content = append(output.Content, ai.NewTextContent("done"))
				output.StopReason = ai.StopReasonStop
				stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonStop, Message: &output})
			}
			stream.End(output)
		}()
		return stream, nil
	}

	var events []Event
	emit := func(e Event) error {
		events = append(events, e)
		return nil
	}

	agentCtx := &AgentContext{Tools: []*Tool{testTool1, testTool2}}
	config := &LoopConfig{Model: testModel(), ConvertToLlm: DefaultConvertToLlm, ToolExecution: ToolExecutionParallel}

	err := RunAgentLoop(context.Background(), []AgentMessage{ai.NewUserMessage("run")}, agentCtx, config, emit, streamFn)
	if err != nil {
		t.Fatal(err)
	}

	// Collect tool_execution_end tool call IDs in order
	var toolEndIDs []string
	// Collect tool result messages in order
	var toolResultNames []string
	for _, e := range events {
		if e.Type == EventToolExecutionEnd {
			toolEndIDs = append(toolEndIDs, e.ToolCallID)
		}
		if e.Type == EventMessageEnd && e.Msg.Role == "toolResult" {
			toolResultNames = append(toolResultNames, e.Msg.ToolCallID)
		}
	}

	// Tool results should be in source order (tc_slow, tc_fast) regardless of completion order
	if len(toolResultNames) != 2 {
		t.Fatalf("expected 2 tool result messages, got %d", len(toolResultNames))
	}
	if toolResultNames[0] != "tc_slow" {
		t.Errorf("first tool result should be tc_slow (source order), got %s", toolResultNames[0])
	}
	if toolResultNames[1] != "tc_fast" {
		t.Errorf("second tool result should be tc_fast (source order), got %s", toolResultNames[1])
	}
}

// 1.6 Steering / Queue Injection
func TestRunAgentLoop_SteeringInjectedAfterToolBatch(t *testing.T) {
	callCount := int64(0)
	var callCtxs [][]ai.Message

	streamFn := func(ctx context.Context, model *ai.Model, convCtx *ai.Context, opts *ai.SimpleStreamOptions) (*ai.EventStream, error) {
		i := int(atomic.AddInt64(&callCount, 1)) - 1
		msgs := make([]ai.Message, len(convCtx.Messages))
		copy(msgs, convCtx.Messages)
		callCtxs = append(callCtxs, msgs)

		stream := ai.NewEventStream(ctx)
		go func() {
			output := ai.NewAssistantOutput(model.API, model.Provider, model.ID)
			stream.Push(ai.AssistantMessageEvent{Type: "start", Partial: &output})
			if i == 0 {
				output.Content = append(output.Content, ai.NewTextContent("first"))
				output.StopReason = ai.StopReasonStop
				stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonStop, Message: &output})
			} else {
				output.Content = append(output.Content, ai.NewTextContent("second"))
				output.StopReason = ai.StopReasonStop
				stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonStop, Message: &output})
			}
			stream.End(output)
		}()
		return stream, nil
	}

	steeringReturned := int64(0)
	emit := func(e Event) error { return nil }

	agentCtx := &AgentContext{}
	config := &LoopConfig{
		Model:        testModel(),
		ConvertToLlm: DefaultConvertToLlm,
		GetSteeringMessages: func() ([]AgentMessage, error) {
			i := int(atomic.AddInt64(&steeringReturned, 1))
			if i == 2 {
				return []AgentMessage{ai.NewUserMessage("steering")}, nil
			}
			return nil, nil
		},
	}

	err := RunAgentLoop(context.Background(), []AgentMessage{ai.NewUserMessage("hello")}, agentCtx, config, emit, streamFn)
	if err != nil {
		t.Fatal(err)
	}

	if atomic.LoadInt64(&callCount) < 2 {
		t.Errorf("expected at least 2 LLM calls, got %d", callCount)
	}
	// Second call should see steering message in context
	if len(callCtxs) >= 2 {
		lastMsgs := callCtxs[1]
		foundSteering := false
		for _, m := range lastMsgs {
			if m.Role == "user" && m.Content == "steering" {
				foundSteering = true
				break
			}
		}
		if !foundSteering {
			t.Error("steering message should be in second LLM call context")
		}
	}
}

// 1.7 Turn Control
func TestRunAgentLoop_PrepareNextTurn(t *testing.T) {
	callCount := int64(0)
	var capturedPrompts []*string

	streamFn := func(ctx context.Context, model *ai.Model, convCtx *ai.Context, opts *ai.SimpleStreamOptions) (*ai.EventStream, error) {
		i := int(atomic.AddInt64(&callCount, 1)) - 1
		capturedPrompts = append(capturedPrompts, convCtx.SystemPrompt)

		stream := ai.NewEventStream(ctx)
		go func() {
			output := ai.NewAssistantOutput(model.API, model.Provider, model.ID)
			stream.Push(ai.AssistantMessageEvent{Type: "start", Partial: &output})
			if i == 0 {
				output.Content = append(output.Content, ai.NewTextContent("response"))
				output.StopReason = ai.StopReasonStop
				stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonStop, Message: &output})
			} else {
				output.Content = append(output.Content, ai.NewTextContent("response2"))
				output.StopReason = ai.StopReasonStop
				stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonStop, Message: &output})
			}
			stream.End(output)
		}()
		return stream, nil
	}

	emit := func(e Event) error { return nil }
	agentCtx := &AgentContext{SystemPrompt: "original"}
	config := &LoopConfig{
		Model:        testModel(),
		ConvertToLlm: DefaultConvertToLlm,
		GetSteeringMessages: func() ([]AgentMessage, error) {
			if atomic.LoadInt64(&callCount) == 1 {
				return []AgentMessage{ai.NewUserMessage("steer")}, nil
			}
			return nil, nil
		},
		PrepareNextTurn: func(ctx PrepareNextTurnContext) (*AgentLoopTurnUpdate, error) {
			newPrompt := "updated system prompt"
			return &AgentLoopTurnUpdate{
				Context: &AgentContext{
					SystemPrompt: newPrompt,
					Messages:     ctx.Context.Messages,
					Tools:        ctx.Context.Tools,
				},
			}, nil
		},
	}

	err := RunAgentLoop(context.Background(), []AgentMessage{ai.NewUserMessage("hello")}, agentCtx, config, emit, streamFn)
	if err != nil {
		t.Fatal(err)
	}

	// Second call should see the updated system prompt from prepareNextTurn
	if len(capturedPrompts) < 2 {
		t.Fatalf("expected at least 2 calls, got %d", len(capturedPrompts))
	}
	if capturedPrompts[0] == nil || *capturedPrompts[0] != "original" {
		t.Errorf("first call prompt = %v, want 'original'", capturedPrompts[0])
	}
	if capturedPrompts[1] == nil || *capturedPrompts[1] != "updated system prompt" {
		t.Errorf("second call prompt = %v, want 'updated system prompt'", capturedPrompts[1])
	}
}

func TestRunAgentLoop_ShouldStopAfterTurn(t *testing.T) {
	callCount := int64(0)

	streamFn := func(ctx context.Context, model *ai.Model, convCtx *ai.Context, opts *ai.SimpleStreamOptions) (*ai.EventStream, error) {
		atomic.AddInt64(&callCount, 1)
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

	var events []Event
	emit := func(e Event) error {
		events = append(events, e)
		return nil
	}

	agentCtx := &AgentContext{}
	config := &LoopConfig{
		Model:        testModel(),
		ConvertToLlm: DefaultConvertToLlm,
		ShouldStopAfterTurn: func(ctx ShouldStopAfterTurnContext) (bool, error) {
			return true, nil // Stop after first turn
		},
	}

	err := RunAgentLoop(context.Background(), []AgentMessage{ai.NewUserMessage("hello")}, agentCtx, config, emit, streamFn)
	if err != nil {
		t.Fatal(err)
	}

	if atomic.LoadInt64(&callCount) != 1 {
		t.Errorf("expected exactly 1 LLM call (should stop after first turn), got %d", callCount)
	}
	types := eventTypes(events)
	if types[len(types)-1] != EventAgentEnd {
		t.Errorf("last event should be agent_end, got %q", types[len(types)-1])
	}
}

func TestRunAgentLoop_TerminateFlag_AllTools(t *testing.T) {
	testTool := &Tool{
		Name:        "terminator",
		Description: "terminates",
		Parameters:  map[string]any{"type": "object"},
		Execute: func(ctx context.Context, toolCallID string, params map[string]any, onUpdate ToolUpdateCallback) (*ToolResult, error) {
			return &ToolResult{
				Content:   []ai.ContentBlock{ai.NewTextContent("done")},
				Details:   map[string]any{},
				Terminate: true,
			}, nil
		},
	}

	callCount := int64(0)
	streamFn := func(ctx context.Context, model *ai.Model, convCtx *ai.Context, opts *ai.SimpleStreamOptions) (*ai.EventStream, error) {
		atomic.AddInt64(&callCount, 1)
		stream := ai.NewEventStream(ctx)
		go func() {
			output := ai.NewAssistantOutput(model.API, model.Provider, model.ID)
			stream.Push(ai.AssistantMessageEvent{Type: "start", Partial: &output})
			tc := ai.NewToolCallContent("tc_1", "terminator", map[string]any{})
			output.Content = append(output.Content, tc)
			output.StopReason = ai.StopReasonToolUse
			stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonToolUse, Message: &output})
			stream.End(output)
		}()
		return stream, nil
	}

	var events []Event
	emit := func(e Event) error { events = append(events, e); return nil }

	agentCtx := &AgentContext{Tools: []*Tool{testTool}}
	config := &LoopConfig{Model: testModel(), ConvertToLlm: DefaultConvertToLlm}

	err := RunAgentLoop(context.Background(), []AgentMessage{ai.NewUserMessage("run")}, agentCtx, config, emit, streamFn)
	if err != nil {
		t.Fatal(err)
	}

	// Should stop after the tool batch (all tools have terminate=true)
	if atomic.LoadInt64(&callCount) != 1 {
		t.Errorf("expected 1 LLM call (terminate after tool), got %d", callCount)
	}
	types := eventTypes(events)
	turnEndCount := countEvent(types, EventTurnEnd)
	if turnEndCount != 1 {
		t.Errorf("expected 1 turn_end, got %d", turnEndCount)
	}
}

func TestRunAgentLoop_TerminateFlag_Partial(t *testing.T) {
	tool1 := &Tool{
		Name: "terminate_tool", Description: "term", Parameters: map[string]any{"type": "object"},
		Execute: func(ctx context.Context, toolCallID string, params map[string]any, onUpdate ToolUpdateCallback) (*ToolResult, error) {
			return &ToolResult{Content: []ai.ContentBlock{ai.NewTextContent("done")}, Details: map[string]any{}, Terminate: true}, nil
		},
	}
	tool2 := &Tool{
		Name: "continue_tool", Description: "cont", Parameters: map[string]any{"type": "object"},
		Execute: func(ctx context.Context, toolCallID string, params map[string]any, onUpdate ToolUpdateCallback) (*ToolResult, error) {
			return &ToolResult{Content: []ai.ContentBlock{ai.NewTextContent("done")}, Details: map[string]any{}}, nil
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
				tc1 := ai.NewToolCallContent("tc_1", "terminate_tool", map[string]any{})
				tc2 := ai.NewToolCallContent("tc_2", "continue_tool", map[string]any{})
				output.Content = append(output.Content, tc1, tc2)
				output.StopReason = ai.StopReasonToolUse
				stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonToolUse, Message: &output})
			} else {
				output.Content = append(output.Content, ai.NewTextContent("summary"))
				output.StopReason = ai.StopReasonStop
				stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonStop, Message: &output})
			}
			stream.End(output)
		}()
		return stream, nil
	}

	emit := func(e Event) error { return nil }
	agentCtx := &AgentContext{Tools: []*Tool{tool1, tool2}}
	config := &LoopConfig{Model: testModel(), ConvertToLlm: DefaultConvertToLlm, ToolExecution: ToolExecutionSequential}

	err := RunAgentLoop(context.Background(), []AgentMessage{ai.NewUserMessage("run")}, agentCtx, config, emit, streamFn)
	if err != nil {
		t.Fatal(err)
	}

	// Mixed terminate: not all tools terminate, so loop continues
	if atomic.LoadInt64(&callCount) != 2 {
		t.Errorf("expected 2 LLM calls (mixed terminate should continue), got %d", callCount)
	}
}

func TestRunAgentLoop_AfterToolCallTerminate(t *testing.T) {
	testTool := &Tool{
		Name: "tool", Description: "tool", Parameters: map[string]any{"type": "object"},
		Execute: func(ctx context.Context, toolCallID string, params map[string]any, onUpdate ToolUpdateCallback) (*ToolResult, error) {
			return &ToolResult{Content: []ai.ContentBlock{ai.NewTextContent("done")}, Details: map[string]any{}}, nil
		},
	}

	callCount := int64(0)
	streamFn := func(ctx context.Context, model *ai.Model, convCtx *ai.Context, opts *ai.SimpleStreamOptions) (*ai.EventStream, error) {
		atomic.AddInt64(&callCount, 1)
		stream := ai.NewEventStream(ctx)
		go func() {
			output := ai.NewAssistantOutput(model.API, model.Provider, model.ID)
			stream.Push(ai.AssistantMessageEvent{Type: "start", Partial: &output})
			tc := ai.NewToolCallContent("tc_1", "tool", map[string]any{})
			output.Content = append(output.Content, tc)
			output.StopReason = ai.StopReasonToolUse
			stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonToolUse, Message: &output})
			stream.End(output)
		}()
		return stream, nil
	}

	emit := func(e Event) error { return nil }
	termTrue := true

	agentCtx := &AgentContext{Tools: []*Tool{testTool}}
	config := &LoopConfig{
		Model:        testModel(),
		ConvertToLlm: DefaultConvertToLlm,
		AfterToolCall: func(ctx AfterToolCallContext) (*AfterToolCallResult, error) {
			return &AfterToolCallResult{Terminate: &termTrue}, nil
		},
	}

	err := RunAgentLoop(context.Background(), []AgentMessage{ai.NewUserMessage("run")}, agentCtx, config, emit, streamFn)
	if err != nil {
		t.Fatal(err)
	}

	if atomic.LoadInt64(&callCount) != 1 {
		t.Errorf("expected 1 LLM call (afterToolCall terminate), got %d", callCount)
	}
}

// 1.8 Continue Mode
func TestRunAgentLoopContinue_NoUserMessageEvents(t *testing.T) {
	var events []Event
	emit := func(e Event) error {
		events = append(events, e)
		return nil
	}

	agentCtx := &AgentContext{
		Messages: []AgentMessage{
			ai.NewAssistantMessage("test", "test", "test-model", []ai.ContentBlock{ai.NewTextContent("hi")}, ai.Usage{}, ai.StopReasonStop),
			ai.NewUserMessage("continue from here"),
		},
	}
	config := &LoopConfig{Model: testModel(), ConvertToLlm: DefaultConvertToLlm}

	err := RunAgentLoopContinue(context.Background(), agentCtx, config, emit, mockStreamFn("continued"))
	if err != nil {
		t.Fatal(err)
	}

	types := eventTypes(events)

	// Continue should not emit user message events
	for i, e := range events {
		if e.Type == EventMessageStart && e.Msg.Role == "user" {
			t.Errorf("continue should not emit message_start for user messages, got user message at index %d", i)
			break
		}
	}

	// Should still have agent_start and turn_start
	if types[0] != EventAgentStart {
		t.Errorf("first event = %q, want agent_start", types[0])
	}

	// Should have assistant message events
	foundAssistantMsg := false
	for _, e := range events {
		if e.Type == EventMessageStart && e.Msg.Role == "assistant" {
			foundAssistantMsg = true
		}
	}
	if !foundAssistantMsg {
		t.Error("expected assistant message events")
	}
}

func TestRunAgentLoopContinue_CustomLastMessage(t *testing.T) {
	var events []Event
	emit := func(e Event) error {
		events = append(events, e)
		return nil
	}

	// Custom message type as last message
	customMsg := ai.Message{Role: "custom_role", Content: "custom data", Timestamp: 1}

	agentCtx := &AgentContext{
		Messages: []AgentMessage{customMsg},
	}

	// Use a convertToLlm that accepts custom_role
	convertFn := func(messages []AgentMessage) ([]ai.Message, error) {
		// Filter to only standard roles
		var result []ai.Message
		for _, m := range messages {
			if m.Role == "user" || m.Role == "assistant" || m.Role == "toolResult" {
				result = append(result, m)
			}
		}
		return result, nil
	}

	config := &LoopConfig{Model: testModel(), ConvertToLlm: convertFn}

	err := RunAgentLoopContinue(context.Background(), agentCtx, config, emit, mockStreamFn("response"))
	if err != nil {
		t.Fatal(err)
	}

	// Should complete successfully with custom message type
	types := eventTypes(events)
	assertHasEvent(t, types, EventAgentStart)
	assertHasEvent(t, types, EventAgentEnd)
}

// ============================================================================
// Steering & follow-up queue tests (existing)
// ============================================================================

func TestRunAgentLoop_SteeringMessages(t *testing.T) {
	steered := int64(0)

	streamFn := func(ctx context.Context, model *ai.Model, convCtx *ai.Context, opts *ai.SimpleStreamOptions) (*ai.EventStream, error) {
		i := int(atomic.AddInt64(&steered, 1)) - 1
		stream := ai.NewEventStream(ctx)
		go func() {
			output := ai.NewAssistantOutput(model.API, model.Provider, model.ID)
			stream.Push(ai.AssistantMessageEvent{Type: "start", Partial: &output})
			text := fmt.Sprintf("response %d", i)
			output.Content = append(output.Content, ai.NewTextContent(text))
			output.StopReason = ai.StopReasonStop
			stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonStop, Message: &output})
			stream.End(output)
		}()
		return stream, nil
	}

	steeringCallCount := int64(0)
	agentCtx := &AgentContext{}
	config := &LoopConfig{
		Model:        testModel(),
		ConvertToLlm: DefaultConvertToLlm,
		GetSteeringMessages: func() ([]AgentMessage, error) {
			i := int(atomic.AddInt64(&steeringCallCount, 1))
			if i == 2 {
				return []AgentMessage{ai.NewUserMessage("steering message")}, nil
			}
			return nil, nil
		},
	}

	var events []Event
	emit := func(e Event) error {
		events = append(events, e)
		return nil
	}

	err := RunAgentLoop(context.Background(), []AgentMessage{ai.NewUserMessage("hello")}, agentCtx, config, emit, streamFn)
	if err != nil {
		t.Fatal(err)
	}

	// Should have 2 turns: initial + steering
	turnEndCount := countEvent(eventTypes(events), EventTurnEnd)
	if turnEndCount < 2 {
		t.Errorf("expected at least 2 turn_end events (initial + steering), got %d", turnEndCount)
	}
	t.Logf("Turn ends: %d, LLM calls: %d", turnEndCount, atomic.LoadInt64(&steered))
}

// ============================================================================
// Agent struct tests
// ============================================================================

func TestAgent_Prompt(t *testing.T) {
	agent := New(Options{
		InitialState: &InitialState{
			Model: testModel(),
		},
		StreamFn: mockStreamFn("hello!"),
	})

	var events []string
	agent.Subscribe(func(e Event) error {
		events = append(events, e.Type)
		return nil
	})

	err := agent.Prompt(context.Background(), "hi")
	if err != nil {
		t.Fatal(err)
	}

	assertHasEvent(t, events, EventAgentStart)
	assertHasEvent(t, events, EventAgentEnd)
	assertHasEvent(t, events, EventMessageEnd)

	state := agent.State()
	if len(state.Messages) == 0 {
		t.Error("expected messages in state after prompt")
	}
	t.Logf("State messages: %d, Events: %v", len(state.Messages), events)
}

func TestAgent_PromptAlreadyRunning(t *testing.T) {
	agent := New(Options{
		InitialState: &InitialState{Model: testModel()},
		StreamFn: func(ctx context.Context, model *ai.Model, convCtx *ai.Context, opts *ai.SimpleStreamOptions) (*ai.EventStream, error) {
			// Slow stream
			stream := ai.NewEventStream(ctx)
			go func() {
				time.Sleep(100 * time.Millisecond)
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

	done := make(chan struct{})
	go func() {
		agent.Prompt(context.Background(), "first")
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	err := agent.Prompt(context.Background(), "second")
	if err == nil {
		t.Error("expected error when prompting already-running agent")
	}
	<-done
}

func TestAgent_SteerQueue(t *testing.T) {
	agent := New(Options{
		InitialState: &InitialState{Model: testModel()},
		StreamFn:     mockStreamFn("ok"),
	})

	agent.Steer(ai.NewUserMessage("steer 1"))
	agent.Steer(ai.NewUserMessage("steer 2"))

	if !agent.HasQueuedMessages() {
		t.Error("expected queued messages after Steer")
	}

	agent.ClearAllQueues()
	if agent.HasQueuedMessages() {
		t.Error("expected no queued messages after ClearAllQueues")
	}
}

func TestAgent_Reset(t *testing.T) {
	agent := New(Options{
		InitialState: &InitialState{Model: testModel()},
		StreamFn:     mockStreamFn("ok"),
	})

	agent.Prompt(context.Background(), "hello")

	state := agent.State()
	if len(state.Messages) == 0 {
		t.Fatal("expected messages before reset")
	}

	agent.Reset()
	state = agent.State()
	if len(state.Messages) != 0 {
		t.Error("expected no messages after reset")
	}
}

func TestAgent_SubscribeUnsubscribe(t *testing.T) {
	agent := New(Options{
		InitialState: &InitialState{Model: testModel()},
		StreamFn:     mockStreamFn("ok"),
	})

	var events []string
	unsub := agent.Subscribe(func(e Event) error {
		events = append(events, e.Type)
		return nil
	})

	agent.Prompt(context.Background(), "hello")
	firstCount := len(events)

	unsub()

	agent.Reset()
	agent.Prompt(context.Background(), "hello again")
	secondCount := len(events)

	if secondCount != firstCount {
		t.Errorf("events should not grow after unsubscribe: first=%d, second=%d", firstCount, secondCount)
	}
}

// ============================================================================
// Queue tests
// ============================================================================

func TestPendingMessageQueue_DrainAll(t *testing.T) {
	q := newPendingMessageQueue(QueueAll)
	q.Enqueue(ai.NewUserMessage("a"))
	q.Enqueue(ai.NewUserMessage("b"))
	q.Enqueue(ai.NewUserMessage("c"))

	drained := q.Drain()
	if len(drained) != 3 {
		t.Fatalf("expected 3, got %d", len(drained))
	}
	if q.HasItems() {
		t.Error("queue should be empty after drain")
	}
}

func TestPendingMessageQueue_DrainOne(t *testing.T) {
	q := newPendingMessageQueue(QueueOneAtATime)
	q.Enqueue(ai.NewUserMessage("a"))
	q.Enqueue(ai.NewUserMessage("b"))

	drained := q.Drain()
	if len(drained) != 1 {
		t.Fatalf("expected 1, got %d", len(drained))
	}
	if !q.HasItems() {
		t.Error("queue should still have items")
	}
}

func TestPendingMessageQueue_Clear(t *testing.T) {
	q := newPendingMessageQueue(QueueAll)
	q.Enqueue(ai.NewUserMessage("a"))
	q.Clear()
	if q.HasItems() {
		t.Error("queue should be empty after clear")
	}
}

// ============================================================================
// Section 2: Agent Lifecycle Tests
// ============================================================================

// 2.1 Construction
func TestAgent_CustomInitialState(t *testing.T) {
	agent := New(Options{
		InitialState: &InitialState{
			Model:         testModel(),
			SystemPrompt:  "You are a coding assistant.",
			ThinkingLevel: ThinkingHigh,
			Tools: []*Tool{
				{Name: "read", Description: "read tool"},
			},
			Messages: []AgentMessage{ai.NewUserMessage("hi")},
		},
		StreamFn: mockStreamFn("ok"),
	})

	state := agent.State()
	if state.SystemPrompt != "You are a coding assistant." {
		t.Errorf("SystemPrompt = %q, want custom prompt", state.SystemPrompt)
	}
	if state.Model == nil || state.Model.ID != "test-model" {
		t.Errorf("Model.ID = %v, want test-model", state.Model)
	}
	if state.ThinkingLevel != ThinkingHigh {
		t.Errorf("ThinkingLevel = %q, want high", state.ThinkingLevel)
	}
	if len(state.Tools) != 1 || state.Tools[0].Name != "read" {
		t.Errorf("Tools = %v, want 1 tool named 'read'", state.Tools)
	}
	if len(state.Messages) != 1 {
		t.Errorf("Messages = %d, want 1", len(state.Messages))
	}
}

// 2.2 Error Propagation
func TestAgent_ThrownRunFailure(t *testing.T) {
	agent := New(Options{
		InitialState: &InitialState{Model: testModel()},
		StreamFn: func(ctx context.Context, model *ai.Model, convCtx *ai.Context, opts *ai.SimpleStreamOptions) (*ai.EventStream, error) {
			return nil, fmt.Errorf("provider exploded")
		},
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

	// Should emit full lifecycle events even for failures
	assertHasEvent(t, types, EventAgentStart)
	assertHasEvent(t, types, EventAgentEnd)
	assertHasEvent(t, types, EventTurnEnd)

	// Find the turn_end event and check error message
	for _, e := range events {
		if e.Type == EventTurnEnd && e.Message != nil {
			if e.Message.StopReason != ai.StopReasonError {
				t.Errorf("turn_end stop reason = %q, want error", e.Message.StopReason)
			}
			if e.Message.ErrorMessage == nil || *e.Message.ErrorMessage != "provider exploded" {
				t.Errorf("turn_end error = %v, want 'provider exploded'", e.Message.ErrorMessage)
			}
		}
	}

	// State should have error
	state := agent.State()
	if state.ErrorMessage == nil || *state.ErrorMessage != "provider exploded" {
		t.Errorf("state error = %v, want 'provider exploded'", state.ErrorMessage)
	}
}

// 2.3 Async Subscriber Semantics
func TestAgent_AwaitAsyncSubscribers(t *testing.T) {
	barrier := make(chan struct{})
	subscriberDone := make(chan struct{})

	agent := New(Options{
		InitialState: &InitialState{Model: testModel()},
		StreamFn:    mockStreamFn("ok"),
	})

	agent.Subscribe(func(e Event) error {
		if e.Type == EventAgentEnd {
			<-barrier // Block until barrier is released
			close(subscriberDone)
		}
		return nil
	})

	promptDone := make(chan error, 1)
	go func() {
		promptDone <- agent.Prompt(context.Background(), "hello")
	}()

	// Give prompt time to reach subscriber
	time.Sleep(50 * time.Millisecond)

	// Subscriber should be blocking prompt
	select {
	case <-promptDone:
		t.Error("prompt should not resolve while subscriber is blocked")
	default:
		// Good - subscriber is still blocking
	}

	// Release barrier
	close(barrier)

	// Now prompt should resolve
	select {
	case err := <-promptDone:
		if err != nil {
			t.Errorf("prompt returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("prompt did not resolve after barrier released")
	}
}

func TestAgent_WaitForIdle(t *testing.T) {
	agent := New(Options{
		InitialState: &InitialState{Model: testModel()},
		StreamFn: func(ctx context.Context, model *ai.Model, convCtx *ai.Context, opts *ai.SimpleStreamOptions) (*ai.EventStream, error) {
			stream := ai.NewEventStream(ctx)
			go func() {
				time.Sleep(100 * time.Millisecond) // Simulate slow streaming
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

	go agent.Prompt(context.Background(), "hello")

	waitDone := make(chan struct{})
	go func() {
		agent.WaitForIdle()
		close(waitDone)
	}()

	select {
	case <-waitDone:
		// Good - WaitForIdle returned after streaming completed
	case <-time.After(3 * time.Second):
		t.Fatal("WaitForIdle did not return after stream completed")
	}
}

// 2.4 Abort Signal
func TestAgent_AbortSignalPropagation(t *testing.T) {
	agent := New(Options{
		InitialState: &InitialState{Model: testModel()},
		StreamFn: func(ctx context.Context, model *ai.Model, convCtx *ai.Context, opts *ai.SimpleStreamOptions) (*ai.EventStream, error) {
			stream := ai.NewEventStream(ctx)
			go func() {
				time.Sleep(200 * time.Millisecond) // Hold open so abort can fire
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

	done := make(chan struct{})
	go func() {
		agent.Prompt(context.Background(), "hello")
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	agent.Abort() // Should not panic

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("abort did not terminate the run")
	}
}

// 2.5 State Mutation
func TestAgent_StateMutators(t *testing.T) {
	agent := New(Options{
		InitialState: &InitialState{Model: testModel()},
		StreamFn:    mockStreamFn("ok"),
	})

	// Test SetSystemPrompt
	agent.SetSystemPrompt("new prompt")
	if agent.State().SystemPrompt != "new prompt" {
		t.Error("SetSystemPrompt did not update")
	}

	// Test SetModel
	newModel := &ai.Model{ID: "new-model", Provider: "test", API: "test"}
	agent.SetModel(newModel)
	if agent.State().Model.ID != "new-model" {
		t.Error("SetModel did not update")
	}

	// Test SetThinkingLevel
	agent.SetThinkingLevel(ThinkingHigh)
	if agent.State().ThinkingLevel != ThinkingHigh {
		t.Error("SetThinkingLevel did not update")
	}

	// Test SetTools
	newTools := []*Tool{{Name: "tool1"}, {Name: "tool2"}}
	agent.SetTools(newTools)
	if len(agent.State().Tools) != 2 {
		t.Error("SetTools did not update")
	}

	// Mutating the original slice should NOT affect agent
	newTools[0] = &Tool{Name: "mutated"}
	if agent.State().Tools[0].Name == "mutated" {
		t.Error("SetTools should copy the slice")
	}
}

// 2.6 Follow-Up Queue
func TestAgent_FollowUpQueue(t *testing.T) {
	agent := New(Options{
		InitialState: &InitialState{Model: testModel()},
		StreamFn:    mockStreamFn("ok"),
	})

	msg := ai.NewUserMessage("follow up")
	agent.FollowUp(msg)

	if !agent.HasQueuedMessages() {
		t.Error("expected queued messages after FollowUp")
	}

	// Message should not be immediately in state
	state := agent.State()
	found := false
	for _, m := range state.Messages {
		if m.Content == "follow up" {
			found = true
		}
	}
	if found {
		t.Error("FollowUp message should not be immediately in state")
	}

	agent.ClearAllQueues()
	if agent.HasQueuedMessages() {
		t.Error("expected no queued messages after ClearAllQueues")
	}
}

// 2.7 Abort
func TestAgent_AbortWhenIdle(t *testing.T) {
	agent := New(Options{
		InitialState: &InitialState{Model: testModel()},
		StreamFn:    mockStreamFn("ok"),
	})

	// Should not panic
	agent.Abort()
}

// 2.8 Continue While Streaming
func TestAgent_ContinueWhileStreaming(t *testing.T) {
	agent := New(Options{
		InitialState: &InitialState{Model: testModel()},
		StreamFn: func(ctx context.Context, model *ai.Model, convCtx *ai.Context, opts *ai.SimpleStreamOptions) (*ai.EventStream, error) {
			stream := ai.NewEventStream(ctx)
			go func() {
				time.Sleep(200 * time.Millisecond)
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

	done := make(chan struct{})
	go func() {
		agent.Prompt(context.Background(), "hello")
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)

	err := agent.Continue(context.Background())
	if err == nil {
		t.Error("expected error when continuing while streaming")
	}

	<-done
}

// 2.9 Continue Follow-Up Processing
func TestAgent_Continue_ProcessesFollowUp(t *testing.T) {
	callCount := int64(0)
	agent := New(Options{
		InitialState: &InitialState{Model: testModel()},
		StreamFn: func(ctx context.Context, model *ai.Model, convCtx *ai.Context, opts *ai.SimpleStreamOptions) (*ai.EventStream, error) {
			i := int(atomic.AddInt64(&callCount, 1)) - 1
			stream := ai.NewEventStream(ctx)
			go func() {
				output := ai.NewAssistantOutput(model.API, model.Provider, model.ID)
				stream.Push(ai.AssistantMessageEvent{Type: "start", Partial: &output})
				text := fmt.Sprintf("response %d", i)
				output.Content = append(output.Content, ai.NewTextContent(text))
				output.StopReason = ai.StopReasonStop
				stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonStop, Message: &output})
				stream.End(output)
			}()
			return stream, nil
		},
		FollowUpMode: QueueOneAtATime,
	})

	// Initial prompt
	err := agent.Prompt(context.Background(), "hello")
	if err != nil {
		t.Fatal(err)
	}

	// Queue follow-up
	agent.FollowUp(ai.NewUserMessage("follow up"))

	// Continue should drain the follow-up
	err = agent.Continue(context.Background())
	if err != nil {
		t.Fatalf("Continue returned error: %v", err)
	}

	// Should have 2 LLM calls
	if atomic.LoadInt64(&callCount) != 2 {
		t.Errorf("expected 2 LLM calls, got %d", callCount)
	}
}

func TestAgent_Continue_SteeringOneAtATime(t *testing.T) {
	callCount := int64(0)
	agent := New(Options{
		InitialState: &InitialState{Model: testModel()},
		StreamFn: func(ctx context.Context, model *ai.Model, convCtx *ai.Context, opts *ai.SimpleStreamOptions) (*ai.EventStream, error) {
			atomic.AddInt64(&callCount, 1)
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
		},
		SteeringMode: QueueOneAtATime,
	})

	// Queue 2 steering messages
	agent.Steer(ai.NewUserMessage("steer1"))
	agent.Steer(ai.NewUserMessage("steer2"))

	// Initial prompt
	err := agent.Prompt(context.Background(), "hello")
	if err != nil {
		t.Fatal(err)
	}

	firstCalls := atomic.LoadInt64(&callCount)
	if firstCalls < 2 {
		t.Errorf("expected at least 2 calls (initial + 1 steering), got %d", firstCalls)
	}
}

// 2.10 Session ID
func TestAgent_SessionIDPropagation(t *testing.T) {
	var capturedSessionID *string

	agent := New(Options{
		InitialState: &InitialState{Model: testModel()},
		StreamFn: func(ctx context.Context, model *ai.Model, convCtx *ai.Context, opts *ai.SimpleStreamOptions) (*ai.EventStream, error) {
			if opts != nil {
				capturedSessionID = opts.SessionID
			}
			return mockStreamFn("ok")(ctx, model, convCtx, opts)
		},
		SessionID: "test-session-123",
	})

	err := agent.Prompt(context.Background(), "hello")
	if err != nil {
		t.Fatal(err)
	}

	if capturedSessionID == nil || *capturedSessionID != "test-session-123" {
		t.Errorf("sessionID = %v, want test-session-123", capturedSessionID)
	}
}

// ============================================================================
// Helpers
// ============================================================================

func eventTypes(events []Event) []string {
	types := make([]string, len(events))
	for i, e := range events {
		types[i] = e.Type
	}
	return types
}

func assertHasEvent(t *testing.T, types []string, want string) {
	t.Helper()
	for _, got := range types {
		if got == want {
			return
		}
	}
	t.Errorf("missing event %q in %v", want, types)
}

func countEvent(types []string, want string) int {
	count := 0
	for _, got := range types {
		if got == want {
			count++
		}
	}
	return count
}

func indexOf(types []string, want string) int {
	for i, got := range types {
		if got == want {
			return i
		}
	}
	return -1
}
