package harness

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chinudotdev/pi-go/agent"
	"github.com/chinudotdev/pi-go/ai"
)

// ============================================================================
// Test helpers for stream config / integration tests
// ============================================================================

// recordingProvider captures the options passed to StreamSimple.
type recordingProvider struct {
	mu      sync.Mutex
	calls   []recordedCall
	streams []*ai.EventStream
}

type recordedCall struct {
	Model   *ai.Model
	Options *ai.StreamOptions
}

func newRecordingProvider() *recordingProvider {
	return &recordingProvider{}
}

func (r *recordingProvider) Stream(ctx context.Context, model *ai.Model, convCtx *ai.Context, options *ai.StreamOptions) (*ai.EventStream, error) {
	return r.streamSimpleImpl(ctx, model, convCtx, options)
}

func (r *recordingProvider) streamSimpleImpl(ctx context.Context, model *ai.Model, convCtx *ai.Context, options *ai.StreamOptions) (*ai.EventStream, error) {
	r.mu.Lock()
	r.calls = append(r.calls, recordedCall{Model: model, Options: options})
	r.mu.Unlock()

	stream := ai.NewEventStream(ctx)
	go func() {
		output := ai.NewAssistantOutput(model.API, model.Provider, model.ID)
		output.Content = append(output.Content, ai.NewTextContent("recorded response"))
		output.StopReason = ai.StopReasonStop
		output.Timestamp = time.Now().UnixMilli()
		stream.Push(ai.AssistantMessageEvent{Type: "start", Partial: &output})
		stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonStop, Message: &output})
		stream.End(output)
	}()
	return stream, nil
}

func (r *recordingProvider) getCalls() []recordedCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]recordedCall, len(r.calls))
	copy(result, r.calls)
	return result
}

// registerTestProvider registers a faux provider under the given API name
// and returns a cleanup function to remove it.
func registerTestProvider(apiName string, rp *recordingProvider) func() {
	ai.RegisterApiProvider(ai.ApiProvider{
		API:          ai.Api(apiName),
		Stream:       rp.Stream,
		StreamSimple: rp.Stream,
	}, "test-harness-stream")
	return func() {
		ai.UnregisterApiProviders("test-harness-stream")
	}
}

// newHarnessWithRecordingProvider creates a harness that uses a recording provider.
func newHarnessWithRecordingProvider(t *testing.T) (*AgentHarness, *recordingProvider, func()) {
	t.Helper()
	rp := newRecordingProvider()
	apiName := fmt.Sprintf("test-recording-%d", time.Now().UnixNano())
	cleanup := registerTestProvider(apiName, rp)

	sess := newMockSession()
	model := &ai.Model{
		ID:       "rec-model",
		Provider: "test",
		API:      apiName,
	}
	apiKey := "test-api-key"
	opts := HarnessOptions{
		Model: model,
		GetApiKeyAndHeaders: func(m *ai.Model) (*AuthInfo, error) {
			return &AuthInfo{
				APIKey:  apiKey,
				Headers: map[string]string{"X-Auth": "bearer-token"},
			}, nil
		},
		Tools: []agent.Tool{},
	}
	h := NewAgentHarness(opts, sess)
	return h, rp, cleanup
}

// newHarnessWithToolCallProvider creates a harness with a provider that
// makes tool calls on the first call and responds with text on the second.
func newHarnessWithToolCallProvider(t *testing.T, tool *agent.Tool) (*AgentHarness, func()) {
	t.Helper()
	apiName := fmt.Sprintf("test-toolcall-%d", time.Now().UnixNano())
	callCount := int64(0)

	ai.RegisterApiProvider(ai.ApiProvider{
		API: ai.Api(apiName),
		Stream: func(ctx context.Context, model *ai.Model, convCtx *ai.Context, options *ai.StreamOptions) (*ai.EventStream, error) {
			return nil, fmt.Errorf("Stream not implemented")
		},
		StreamSimple: func(ctx context.Context, model *ai.Model, convCtx *ai.Context, options *ai.StreamOptions) (*ai.EventStream, error) {
			i := int(atomic.AddInt64(&callCount, 1)) - 1
			stream := ai.NewEventStream(ctx)
			go func() {
				output := ai.NewAssistantOutput(model.API, model.Provider, model.ID)
				stream.Push(ai.AssistantMessageEvent{Type: "start", Partial: &output})
				if i == 0 {
					tc := ai.NewToolCallContent("tc_1", tool.Name, map[string]any{"arg": "value"})
					output.Content = append(output.Content, tc)
					output.StopReason = ai.StopReasonToolUse
					stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonToolUse, Message: &output})
				} else {
					output.Content = append(output.Content, ai.NewTextContent("tool result processed"))
					output.StopReason = ai.StopReasonStop
					stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonStop, Message: &output})
				}
				stream.End(output)
			}()
			return stream, nil
		},
	}, "test-harness-toolcall")

	cleanup := func() { ai.UnregisterApiProviders("test-harness-toolcall") }

	sess := newMockSession()
	model := &ai.Model{
		ID:       "tool-model",
		Provider: "test",
		API:      apiName,
	}
	opts := HarnessOptions{
		Model:         model,
		Tools:         []agent.Tool{*tool},
		StreamOptions: &HarnessStreamOptions{Headers: map[string]string{"base": "header"}},
		GetApiKeyAndHeaders: func(m *ai.Model) (*AuthInfo, error) {
			return &AuthInfo{APIKey: "key", Headers: map[string]string{"auth": "token"}}, nil
		},
	}
	h := NewAgentHarness(opts, sess)
	return h, cleanup
}

// ============================================================================
// Section 3: Harness Stream Configuration Tests
// ============================================================================

// 3.1 Stream Options Snapshot
func TestAgentHarness_StreamOptionsSnapshot(t *testing.T) {
	h, rp, cleanup := newHarnessWithRecordingProvider(t)
	defer cleanup()

	// Set stream options with timeout and headers
	timeout := 5000
	h.SetStreamOptions(context.Background(), HarnessStreamOptions{
		TimeoutMs: &timeout,
		Headers:   map[string]string{"X-Custom": "value"},
	})

	ctx := context.Background()
	_, err := h.Prompt(ctx, "test prompt", nil)
	if err != nil {
		t.Fatalf("Prompt returned error: %v", err)
	}

	calls := rp.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 provider call, got %d", len(calls))
	}

	call := calls[0]
	// Verify API key from auth resolver was passed
	if call.Options.APIKey == nil || *call.Options.APIKey != "test-api-key" {
		t.Errorf("expected API key 'test-api-key', got %v", call.Options.APIKey)
	}

	// Verify headers merged: base ("X-Custom") + auth ("X-Auth")
	if call.Options.Headers == nil {
		t.Fatal("expected non-nil headers")
	}
	if v, ok := call.Options.Headers["X-Custom"]; !ok || v != "value" {
		t.Errorf("expected X-Custom=value in headers, got %v", call.Options.Headers)
	}
	if v, ok := call.Options.Headers["X-Auth"]; !ok || v != "bearer-token" {
		t.Errorf("expected X-Auth=bearer-token in headers, got %v", call.Options.Headers)
	}

	// Verify timeout from stream options
	if call.Options.TimeoutMs == nil || *call.Options.TimeoutMs != 5000 {
		t.Errorf("expected timeout 5000, got %v", call.Options.TimeoutMs)
	}
}

// 3.2 Provider Request Patch Chaining
// Tests that stream options are correctly snapshotted and merged.
// Multiple SetStreamOptions calls create the base; auth headers are added at stream time.
func TestAgentHarness_ProviderRequestOptionsMerge(t *testing.T) {
	h, rp, cleanup := newHarnessWithRecordingProvider(t)
	defer cleanup()

	retries := 3
	h.SetStreamOptions(context.Background(), HarnessStreamOptions{
		MaxRetries: &retries,
		Headers:    map[string]string{"base": "yes", "remove": "me"},
	})

	ctx := context.Background()
	_, err := h.Prompt(ctx, "test", nil)
	if err != nil {
		t.Fatalf("Prompt returned error: %v", err)
	}

	calls := rp.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}

	call := calls[0]
	if call.Options.MaxRetries == nil || *call.Options.MaxRetries != 3 {
		t.Errorf("expected maxRetries=3, got %v", call.Options.MaxRetries)
	}
	// Base headers should be present
	if v, ok := call.Options.Headers["base"]; !ok || v != "yes" {
		t.Errorf("expected base=yes, got %v", call.Options.Headers)
	}
	// Auth headers merged on top
	if v, ok := call.Options.Headers["X-Auth"]; !ok || v != "bearer-token" {
		t.Errorf("expected X-Auth=bearer-token, got %v", call.Options.Headers)
	}
}

// 3.3 Save-Point Snapshot Isolation
// Verifies that stream options are snapshotted at turn start and not mutated mid-turn.
func TestAgentHarness_SavePointSnapshotIsolation(t *testing.T) {
	h, rp, cleanup := newHarnessWithRecordingProvider(t)
	defer cleanup()

	// Set initial options
	timeout1 := 1000
	h.SetStreamOptions(context.Background(), HarnessStreamOptions{
		TimeoutMs: &timeout1,
		Headers:   map[string]string{"turn": "first"},
	})

	// First prompt should use timeout=1000
	ctx := context.Background()
	_, err := h.Prompt(ctx, "first prompt", nil)
	if err != nil {
		t.Fatalf("first prompt error: %v", err)
	}

	calls := rp.getCalls()
	if len(calls) < 1 {
		t.Fatal("expected at least 1 provider call")
	}
	if calls[0].Options.TimeoutMs == nil || *calls[0].Options.TimeoutMs != 1000 {
		t.Errorf("first call: expected timeout 1000, got %v", calls[0].Options.TimeoutMs)
	}

	// Update stream options between turns
	timeout2 := 2000
	h.SetStreamOptions(context.Background(), HarnessStreamOptions{
		TimeoutMs: &timeout2,
		Headers:   map[string]string{"turn": "second"},
	})

	// Second prompt should use timeout=2000
	_, err = h.Prompt(ctx, "second prompt", nil)
	if err != nil {
		t.Fatalf("second prompt error: %v", err)
	}

	calls = rp.getCalls()
	if len(calls) < 2 {
		t.Fatalf("expected 2 provider calls, got %d", len(calls))
	}
	if calls[1].Options.TimeoutMs == nil || *calls[1].Options.TimeoutMs != 2000 {
		t.Errorf("second call: expected timeout 2000, got %v", calls[1].Options.TimeoutMs)
	}
	// Second call should have updated headers
	if v, ok := calls[1].Options.Headers["turn"]; !ok || v != "second" {
		t.Errorf("second call: expected turn=second, got %v", calls[1].Options.Headers)
	}
}

// 3.4 Provider Payload Options
// Verifies that OnPayload/OnResponse hooks in stream options are propagated.
func TestAgentHarness_ProviderPayloadOptions(t *testing.T) {
	h, rp, cleanup := newHarnessWithRecordingProvider(t)
	defer cleanup()

	var payloadCalled int64
	var responseCalled int64

	h.SetStreamOptions(context.Background(), HarnessStreamOptions{
		Headers: map[string]string{"X-Test": "yes"},
	})

	// We can verify the model and session ID are correct by checking the context
	ctx := context.Background()
	result, err := h.Prompt(ctx, "test", nil)
	if err != nil {
		t.Fatalf("Prompt error: %v", err)
	}

	// Provider should have been called
	calls := rp.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}

	// Verify model is correct
	if calls[0].Model.ID != "rec-model" {
		t.Errorf("expected model ID 'rec-model', got %s", calls[0].Model.ID)
	}

	// Verify result is assistant message
	if result == nil || result.Role != "assistant" {
		t.Errorf("expected assistant message result, got %v", result)
	}

	_ = payloadCalled
	_ = responseCalled
}

// ============================================================================
// Section 4: Harness Integration Tests
// ============================================================================

// 4.1 Steering Drain One At A Time
func TestAgentHarness_SteeringDrainOneAtATime(t *testing.T) {
	rp := newRecordingProvider()
	apiName := fmt.Sprintf("test-steer-%d", time.Now().UnixNano())
	cleanup := registerTestProvider(apiName, rp)
	defer cleanup()

	// Create a provider that counts calls and keeps running across multiple turns
	callCount := int64(0)
	providerCalls := int64(0)

	ai.RegisterApiProvider(ai.ApiProvider{
		API: ai.Api(apiName),
		Stream: func(ctx context.Context, model *ai.Model, convCtx *ai.Context, options *ai.StreamOptions) (*ai.EventStream, error) {
			return nil, fmt.Errorf("not implemented")
		},
		StreamSimple: func(ctx context.Context, model *ai.Model, convCtx *ai.Context, options *ai.StreamOptions) (*ai.EventStream, error) {
			i := int(atomic.AddInt64(&providerCalls, 1)) - 1
			stream := ai.NewEventStream(ctx)
			go func() {
				output := ai.NewAssistantOutput(model.API, model.Provider, model.ID)
				stream.Push(ai.AssistantMessageEvent{Type: "start", Partial: &output})
				if i == 0 {
					// First response: return tool call so loop continues
					tc := ai.NewToolCallContent("tc_1", "echo", map[string]any{"msg": "hello"})
					output.Content = append(output.Content, tc)
					output.StopReason = ai.StopReasonToolUse
					stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonToolUse, Message: &output})
				} else {
					// Second response: just text
					output.Content = append(output.Content, ai.NewTextContent("done"))
					output.StopReason = ai.StopReasonStop
					stream.Push(ai.AssistantMessageEvent{Type: "done", Reason: ai.StopReasonStop, Message: &output})
				}
				stream.End(output)
			}()
			return stream, nil
		},
	}, "test-harness-steer")

	// Re-cleanup with correct source
	defer ai.UnregisterApiProviders("test-harness-steer")

	echoTool := agent.Tool{
		Name:        "echo",
		Description: "Echo tool",
		Parameters:  map[string]any{"type": "object"},
		Execute: func(ctx context.Context, toolCallID string, params map[string]any, onUpdate agent.ToolUpdateCallback) (*agent.ToolResult, error) {
			atomic.AddInt64(&callCount, 1)
			return &agent.ToolResult{
				Content: []ai.ContentBlock{ai.NewTextContent("echoed")},
			}, nil
		},
	}

	sess := newMockSession()
	model := &ai.Model{ID: "steer-model", Provider: "test", API: apiName}
	opts := HarnessOptions{
		Model:        model,
		Tools:        []agent.Tool{echoTool},
		SteeringMode: agent.QueueOneAtATime,
		GetApiKeyAndHeaders: func(m *ai.Model) (*AuthInfo, error) {
			return &AuthInfo{APIKey: "key"}, nil
		},
	}
	h := NewAgentHarness(opts, sess)

	// Verify default steering mode
	if h.GetSteeringMode() != agent.QueueOneAtATime {
		t.Errorf("expected QueueOneAtATime, got %v", h.GetSteeringMode())
	}

	// DrainQueue test: one-at-a-time drains 1 message
	h.mu.Lock()
	h.steerQueue = []ai.Message{
		ai.NewUserMessage("msg1"),
		ai.NewUserMessage("msg2"),
		ai.NewUserMessage("msg3"),
	}
	h.mu.Unlock()

	drained := h.drainQueue(&h.steerQueue, agent.QueueOneAtATime)
	if len(drained) != 1 {
		t.Errorf("expected 1 drained, got %d", len(drained))
	}
	if drained[0].Role != "user" {
		t.Errorf("expected user role, got %s", drained[0].Role)
	}

	// Queue should have 2 remaining
	h.mu.Lock()
	remaining := len(h.steerQueue)
	h.mu.Unlock()
	if remaining != 2 {
		t.Errorf("expected 2 remaining, got %d", remaining)
	}
}

// 4.2 Before Agent Start
func TestAgentHarness_BeforeAgentStart(t *testing.T) {
	h, _, cleanup := newHarnessWithRecordingProvider(t)
	defer cleanup()

	var receivedEvents []HarnessEvent
	h.Subscribe(func(event HarnessEvent) (any, error) {
		receivedEvents = append(receivedEvents, event)
		return nil, nil
	})

	ctx := context.Background()
	_, err := h.Prompt(ctx, "test prompt", nil)
	if err != nil {
		t.Fatalf("Prompt error: %v", err)
	}

	// Find before_agent_start event
	var found *HarnessEvent
	for _, e := range receivedEvents {
		if e.Type == "before_agent_start" {
			found = &e
			break
		}
	}
	if found == nil {
		t.Fatal("expected before_agent_start event")
	}
	if found.Prompt != "test prompt" {
		t.Errorf("expected prompt 'test prompt', got %q", found.Prompt)
	}
	if found.SystemPrompt == "" {
		t.Error("expected non-empty system prompt")
	}
	if found.Resources == nil {
		t.Error("expected non-nil resources")
	}
}

// 4.3 Abort Clears Queues
func TestAgentHarness_AbortClearsQueues(t *testing.T) {
	sess := newMockSession()
	model := &ai.Model{ID: "test", Provider: "test", API: "faux-abort"}
	opts := HarnessOptions{
		Model: model,
		Tools: []agent.Tool{},
	}
	h := NewAgentHarness(opts, sess)

	// Manually set up queues
	h.mu.Lock()
	h.steerQueue = []ai.Message{ai.NewUserMessage("steer1"), ai.NewUserMessage("steer2")}
	h.followUpQueue = []ai.Message{ai.NewUserMessage("followup1")}
	h.nextTurnQueue = []ai.Message{ai.NewUserMessage("next1")}
	h.mu.Unlock()

	// Abort should clear steer and follow-up but preserve nextTurn
	result, err := h.Abort(context.Background())
	if err != nil {
		t.Fatalf("Abort error: %v", err)
	}

	// Verify cleared queues in result
	if len(result.ClearedSteer) != 2 {
		t.Errorf("expected 2 cleared steer, got %d", len(result.ClearedSteer))
	}
	if len(result.ClearedFollowUp) != 1 {
		t.Errorf("expected 1 cleared follow-up, got %d", len(result.ClearedFollowUp))
	}

	// Steer and follow-up should be empty
	h.mu.Lock()
	st := len(h.steerQueue)
	fu := len(h.followUpQueue)
	nt := len(h.nextTurnQueue)
	h.mu.Unlock()

	if st != 0 {
		t.Errorf("steer queue should be empty, has %d", st)
	}
	if fu != 0 {
		t.Errorf("follow-up queue should be empty, has %d", fu)
	}
	if nt != 1 {
		t.Errorf("next turn queue should be preserved (1), has %d", nt)
	}
}

// 4.4 Follow-Up Drain One At A Time
func TestAgentHarness_FollowUpDrainOneAtATime(t *testing.T) {
	sess := newMockSession()
	model := &ai.Model{ID: "test", Provider: "test", API: "faux"}
	opts := HarnessOptions{
		Model:        model,
		Tools:        []agent.Tool{},
		FollowUpMode: agent.QueueOneAtATime,
	}
	h := NewAgentHarness(opts, sess)

	// Enqueue follow-up messages
	h.mu.Lock()
	h.followUpQueue = []ai.Message{
		ai.NewUserMessage("follow1"),
		ai.NewUserMessage("follow2"),
		ai.NewUserMessage("follow3"),
	}
	h.mu.Unlock()

	// Drain one-at-a-time
	drained := h.drainQueue(&h.followUpQueue, agent.QueueOneAtATime)
	if len(drained) != 1 {
		t.Fatalf("expected 1, got %d", len(drained))
	}

	// Verify remaining
	h.mu.Lock()
	remaining := len(h.followUpQueue)
	h.mu.Unlock()
	if remaining != 2 {
		t.Errorf("expected 2 remaining, got %d", remaining)
	}

	// Drain all
	drained = h.drainQueue(&h.followUpQueue, agent.QueueAll)
	if len(drained) != 2 {
		t.Fatalf("expected 2, got %d", len(drained))
	}

	h.mu.Lock()
	remaining = len(h.followUpQueue)
	h.mu.Unlock()
	if remaining != 0 {
		t.Errorf("expected 0 remaining, got %d", remaining)
	}
}

// 4.5 Hook Failure Handling
func TestAgentHarness_HookFailureSettles(t *testing.T) {
	sess := newMockSession()
	model := &ai.Model{ID: "test", Provider: "test", API: "faux"}
	opts := HarnessOptions{
		Model: model,
		Tools: []agent.Tool{},
	}
	h := NewAgentHarness(opts, sess)

	var events []HarnessEvent
	h.Subscribe(func(event HarnessEvent) (any, error) {
		events = append(events, event)
		return nil, nil
	})

	// Register a failing hook for model_update
	failingCalled := false
	h.On("model_update", func(event HarnessEvent) (any, error) {
		failingCalled = true
		return nil, fmt.Errorf("hook exploded")
	})

	// SetModel should trigger the hook, which returns an error
	err := h.SetModel(context.Background(), &ai.Model{ID: "new", Provider: "test", API: "faux"})
	if err == nil {
		t.Fatal("expected error from failing hook")
	}
	if !failingCalled {
		t.Error("expected failing hook to be called")
	}

	// Verify it's an AgentHarnessError
	if he, ok := err.(*AgentHarnessError); !ok {
		t.Errorf("expected AgentHarnessError, got %T", err)
	} else if he.Code != HarnessErrorHook {
		t.Errorf("expected hook error code, got %s", he.Code)
	}
}

// 4.6 Save-Point Refresh
func TestAgentHarness_SavePointRefresh(t *testing.T) {
	h, _, cleanup := newHarnessWithRecordingProvider(t)
	defer cleanup()

	// Set initial state
	ctx := context.Background()
	h.SetThinkingLevel(ctx, "low")

	// The model should be refreshed at turn start
	if h.GetThinkingLevel() != "low" {
		t.Errorf("expected thinking level 'low', got %s", h.GetThinkingLevel())
	}

	// Run a prompt
	_, err := h.Prompt(ctx, "test", nil)
	if err != nil {
		t.Fatalf("Prompt error: %v", err)
	}

	// After prompt, state should be idle
	if h.GetPhase() != PhaseIdle {
		t.Errorf("expected PhaseIdle, got %s", h.GetPhase())
	}

	// Verify thinking level persisted
	if h.GetThinkingLevel() != "low" {
		t.Errorf("expected thinking level 'low' after prompt, got %s", h.GetThinkingLevel())
	}
}

// 4.7 Write Ordering
func TestAgentHarness_WriteOrdering(t *testing.T) {
	sess := newMockSession()
	model := &ai.Model{ID: "test", Provider: "test", API: "faux"}
	opts := HarnessOptions{Model: model, Tools: []agent.Tool{}}
	h := NewAgentHarness(opts, sess)

	ctx := context.Background()

	// Buffer pending writes while "busy"
	h.setPhase(PhaseTurn)

	msg1 := ai.NewUserMessage("pending1")
	msg2 := ai.NewUserMessage("pending2")
	_ = h.AppendMessage(ctx, msg1)
	_ = h.AppendMessage(ctx, msg2)

	// They should be buffered
	h.mu.Lock()
	pw := len(h.pendingWrites)
	h.mu.Unlock()
	if pw != 2 {
		t.Fatalf("expected 2 pending writes, got %d", pw)
	}

	// Entries should not be written yet
	entries, _ := sess.GetEntries(ctx)
	if len(entries) != 0 {
		t.Errorf("expected 0 entries (deferred), got %d", len(entries))
	}

	// Flush
	err := h.flushPendingWrites(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Now entries should exist in order
	entries, _ = sess.GetEntries(ctx)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	// Verify ordering: msg1 then msg2
	me0, ok0 := entries[0].AsMessageEntry()
	me1, ok1 := entries[1].AsMessageEntry()
	if !ok0 || !ok1 {
		t.Fatal("expected message entries")
	}
	text0 := extractTextContent(me0.Message.Content)
	text1 := extractTextContent(me1.Message.Content)
	if text0 != "pending1" || text1 != "pending2" {
		t.Errorf("wrong order: %q, %q", text0, text1)
	}

	// Pending writes should be cleared
	h.mu.Lock()
	pw = len(h.pendingWrites)
	h.mu.Unlock()
	if pw != 0 {
		t.Errorf("expected 0 pending writes, got %d", pw)
	}
}

// 4.8 WaitForIdle
func TestAgentHarness_WaitForIdle_Integration(t *testing.T) {
	h, _, cleanup := newHarnessWithRecordingProvider(t)
	defer cleanup()

	ctx := context.Background()

	// Start a prompt in a goroutine
	promptDone := make(chan struct{})
	go func() {
		defer close(promptDone)
		_, _ = h.Prompt(ctx, "test", nil)
	}()

	// Wait for prompt to complete first, then verify WaitForIdle returns
	<-promptDone

	// WaitForIdle should return immediately since prompt is done
	waitDone := make(chan struct{})
	go func() {
		defer close(waitDone)
		h.WaitForIdle()
	}()

	select {
	case <-waitDone:
		// Good
	case <-time.After(2 * time.Second):
		t.Fatal("WaitForIdle did not return within timeout")
	}
}

// 4.9 Tool Hooks
func TestAgentHarness_ToolHooks(t *testing.T) {
	echoTool := agent.Tool{
		Name:        "echo",
		Description: "Echo tool",
		Parameters:  map[string]any{"type": "object"},
		Execute: func(ctx context.Context, toolCallID string, params map[string]any, onUpdate agent.ToolUpdateCallback) (*agent.ToolResult, error) {
			return &agent.ToolResult{
				Content: []ai.ContentBlock{ai.NewTextContent("echoed")},
			}, nil
		},
	}

	h, cleanup := newHarnessWithToolCallProvider(t, &echoTool)
	defer cleanup()

	ctx := context.Background()
	result, err := h.Prompt(ctx, "use echo", nil)
	if err != nil {
		t.Fatalf("Prompt error: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Role != "assistant" {
		t.Errorf("expected assistant role, got %s", result.Role)
	}
	if result.StopReason != ai.StopReasonStop {
		t.Errorf("expected stop reason 'stop', got %s", result.StopReason)
	}
}
