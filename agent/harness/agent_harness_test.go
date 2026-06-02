package harness

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/chinudotdev/pi-go/agent"
	"github.com/chinudotdev/pi-go/ai"
)

// mockSession implements SessionProvider for testing.
type mockSession struct {
	metadata   SessionMetadata
	leafID     *string
	entries    []SessionTreeEntry
	labels     map[string]string
}

func newMockSession() *mockSession {
	return &mockSession{
		metadata: SessionMetadata{ID: "test-session"},
		labels:   make(map[string]string),
	}
}

func (m *mockSession) GetMetadata(_ context.Context) (SessionMetadata, error) {
	return m.metadata, nil
}
func (m *mockSession) GetLeafID(_ context.Context) (*string, error) {
	return m.leafID, nil
}
func (m *mockSession) GetEntry(_ context.Context, id string) (*SessionTreeEntry, error) {
	for i := range m.entries {
		if m.entries[i].ID == id {
			return &m.entries[i], nil
		}
	}
	return nil, nil
}
func (m *mockSession) GetEntries(_ context.Context) ([]SessionTreeEntry, error) {
	return m.entries, nil
}
func (m *mockSession) GetBranch(_ context.Context, fromID *string) ([]SessionTreeEntry, error) {
	return m.entries, nil
}
func (m *mockSession) BuildContext(_ context.Context) (*SessionContext, error) {
	return &SessionContext{Messages: nil}, nil
}
func (m *mockSession) AppendMessage(_ context.Context, msg ai.Message) (string, error) {
	id := fmt.Sprintf("msg-%d", len(m.entries))
	m.entries = append(m.entries, SessionTreeEntry{
		SessionTreeEntryBase: SessionTreeEntryBase{ID: id, Type: "message"},
		Message:              &msg,
	})
	m.leafID = &id
	return id, nil
}
func (m *mockSession) AppendModelChange(_ context.Context, provider, modelID string) (string, error) {
	id := fmt.Sprintf("mc-%d", len(m.entries))
	m.entries = append(m.entries, SessionTreeEntry{
		SessionTreeEntryBase: SessionTreeEntryBase{ID: id, Type: "model_change"},
		Provider:             provider,
		ModelID:              modelID,
	})
	m.leafID = &id
	return id, nil
}
func (m *mockSession) AppendThinkingLevelChange(_ context.Context, level string) (string, error) {
	id := fmt.Sprintf("tlc-%d", len(m.entries))
	m.entries = append(m.entries, SessionTreeEntry{
		SessionTreeEntryBase: SessionTreeEntryBase{ID: id, Type: "thinking_level_change"},
		ThinkingLevel:        level,
	})
	m.leafID = &id
	return id, nil
}
func (m *mockSession) AppendActiveToolsChange(_ context.Context, activeToolNames []string) (string, error) {
	id := fmt.Sprintf("atc-%d", len(m.entries))
	m.entries = append(m.entries, SessionTreeEntry{
		SessionTreeEntryBase: SessionTreeEntryBase{ID: id, Type: "active_tools_change"},
		ActiveToolNames:      activeToolNames,
	})
	m.leafID = &id
	return id, nil
}
func (m *mockSession) AppendCompaction(_ context.Context, _, _ string, _ int, _ any, _ bool) (string, error) {
	id := fmt.Sprintf("comp-%d", len(m.entries))
	m.leafID = &id
	return id, nil
}
func (m *mockSession) AppendBranchSummary(_ context.Context, _, _ string, _ any, _ bool) (string, error) {
	id := fmt.Sprintf("bs-%d", len(m.entries))
	m.leafID = &id
	return id, nil
}
func (m *mockSession) AppendCustomEntry(_ context.Context, _ string, _ any) (string, error) {
	return "custom", nil
}
func (m *mockSession) AppendCustomMessageEntry(_ context.Context, _ string, _ any, _ bool, _ any) (string, error) {
	return "custom-msg", nil
}
func (m *mockSession) AppendLabel(_ context.Context, targetID string, label *string) (string, error) {
	id := fmt.Sprintf("label-%d", len(m.entries))
	if label != nil {
		m.labels[targetID] = *label
	}
	return id, nil
}
func (m *mockSession) AppendSessionName(_ context.Context, name string) (string, error) {
	return "session-name", nil
}
func (m *mockSession) MoveTo(_ context.Context, entryID *string, summary *BranchSummaryResult) (*string, error) {
	m.leafID = entryID
	return nil, nil
}

// Need fmt in test
var _ = fmt.Sprintf

func newTestHarness(t *testing.T) *AgentHarness {
	t.Helper()
	sess := newMockSession()
	model := &ai.Model{
		ID:       "test-model",
		Provider: "faux",
		API:      "faux",
	}
	opts := HarnessOptions{
		Model:         model,
		ThinkingLevel: "off",
		Tools:         []agent.Tool{},
	}
	return NewAgentHarness(opts, sess)
}

func TestNewAgentHarness_BasicConstruction(t *testing.T) {
	h := newTestHarness(t)
	if h.GetPhase() != PhaseIdle {
		t.Errorf("expected PhaseIdle, got %s", h.GetPhase())
	}
	if h.GetThinkingLevel() != "off" {
		t.Errorf("expected off, got %s", h.GetThinkingLevel())
	}
	if h.GetModel().ID != "test-model" {
		t.Errorf("expected test-model, got %s", h.GetModel().ID)
	}
}

func TestNewAgentHarness_WithTools(t *testing.T) {
	tool1 := agent.Tool{Name: "read_file", Description: "Read a file"}
	tool2 := agent.Tool{Name: "write_file", Description: "Write a file"}

	sess := newMockSession()
	model := &ai.Model{ID: "test", Provider: "faux", API: "faux"}

	opts := HarnessOptions{
		Model:          model,
		Tools:          []agent.Tool{tool1, tool2},
		ActiveToolNames: []string{"read_file"},
	}
	h := NewAgentHarness(opts, sess)

	tools := h.GetTools()
	if len(tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(tools))
	}
	active := h.GetActiveTools()
	if len(active) != 1 || active[0].Name != "read_file" {
		t.Errorf("expected 1 active tool (read_file), got %v", active)
	}
}

func TestAgentHarness_RequirePhaseIdle(t *testing.T) {
	h := newTestHarness(t)
	if err := h.requirePhase(PhaseIdle); err != nil {
		t.Errorf("expected no error for idle, got %v", err)
	}
	h.setPhase(PhaseTurn)
	if err := h.requirePhase(PhaseIdle); err == nil {
		t.Error("expected error when not idle")
	}
	h.setPhase(PhaseIdle)
}

func TestAgentHarness_SetModel(t *testing.T) {
	h := newTestHarness(t)
	newModel := &ai.Model{ID: "new-model", Provider: "faux", API: "faux"}
	ctx := context.Background()

	err := h.SetModel(ctx, newModel)
	if err != nil {
		t.Fatal(err)
	}
	if h.GetModel().ID != "new-model" {
		t.Errorf("expected new-model, got %s", h.GetModel().ID)
	}
}

func TestAgentHarness_SetThinkingLevel(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	err := h.SetThinkingLevel(ctx, "high")
	if err != nil {
		t.Fatal(err)
	}
	if h.GetThinkingLevel() != "high" {
		t.Errorf("expected high, got %s", h.GetThinkingLevel())
	}
}

func TestAgentHarness_SetActiveTools(t *testing.T) {
	tool1 := agent.Tool{Name: "tool1", Description: "Tool 1"}
	tool2 := agent.Tool{Name: "tool2", Description: "Tool 2"}

	sess := newMockSession()
	model := &ai.Model{ID: "test", Provider: "faux", API: "faux"}

	opts := HarnessOptions{
		Model: model,
		Tools: []agent.Tool{tool1, tool2},
	}
	h := NewAgentHarness(opts, sess)
	ctx := context.Background()

	err := h.SetActiveTools(ctx, []string{"tool2"})
	if err != nil {
		t.Fatal(err)
	}
	active := h.GetActiveTools()
	if len(active) != 1 || active[0].Name != "tool2" {
		t.Errorf("expected [tool2], got %v", active)
	}
}

func TestAgentHarness_QueueOperations(t *testing.T) {
	h := newTestHarness(t)
	h.setPhase(PhaseTurn) // steer/followUp require non-idle

	ctx := context.Background()

	err := h.Steer(ctx, "steer message", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.steerQueue) != 1 {
		t.Errorf("expected 1 steer message, got %d", len(h.steerQueue))
	}

	err = h.FollowUp(ctx, "followup message", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.followUpQueue) != 1 {
		t.Errorf("expected 1 followup message, got %d", len(h.followUpQueue))
	}

	// NextTurn can be called in any phase
	h.setPhase(PhaseIdle)
	err = h.NextTurn(ctx, "next turn message", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.nextTurnQueue) != 1 {
		t.Errorf("expected 1 next turn message, got %d", len(h.nextTurnQueue))
	}
}

func TestAgentHarness_SteerRequiresNonIdle(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()
	err := h.Steer(ctx, "test", nil)
	if err == nil {
		t.Error("expected error when steering while idle")
	}
	if _, ok := err.(*AgentHarnessError); !ok {
		t.Errorf("expected AgentHarnessError, got %T", err)
	}
}

func TestAgentHarness_Subscribe(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	var received []HarnessEvent
	h.Subscribe(func(event HarnessEvent) (any, error) {
		received = append(received, event)
		return nil, nil
	})

	// SetModel triggers a model_update event
	_ = h.SetModel(ctx, &ai.Model{ID: "new", Provider: "faux", API: "faux"})

	// Give handlers time to run
	time.Sleep(10 * time.Millisecond)

	if len(received) == 0 {
		t.Error("expected to receive events")
	}
	found := false
	for _, e := range received {
		if e.Type == "model_update" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected model_update event")
	}
}

func TestAgentHarness_On(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	var received HarnessEvent
	h.On("model_update", func(event HarnessEvent) (any, error) {
		received = event
		return nil, nil
	})

	_ = h.SetModel(ctx, &ai.Model{ID: "new", Provider: "faux", API: "faux"})
	time.Sleep(10 * time.Millisecond)

	if received.Type != "model_update" {
		t.Errorf("expected model_update, got %s", received.Type)
	}
}

func TestAgentHarness_AppendMessage_Idle(t *testing.T) {
	sess := newMockSession()
	model := &ai.Model{ID: "test", Provider: "faux", API: "faux"}
	opts := HarnessOptions{Model: model}
	h := NewAgentHarness(opts, sess)

	ctx := context.Background()
	msg := ai.NewUserMessage("hello")
	err := h.AppendMessage(ctx, msg)
	if err != nil {
		t.Fatal(err)
	}

	// Verify message was appended
	entries, _ := sess.GetEntries(ctx)
	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}
}

func TestAgentHarness_AppendMessage_Busy(t *testing.T) {
	sess := newMockSession()
	model := &ai.Model{ID: "test", Provider: "faux", API: "faux"}
	opts := HarnessOptions{Model: model}
	h := NewAgentHarness(opts, sess)

	ctx := context.Background()
	h.setPhase(PhaseTurn)

	msg := ai.NewUserMessage("hello")
	err := h.AppendMessage(ctx, msg)
	if err != nil {
		t.Fatal(err)
	}

	// Should be buffered
	if len(h.pendingWrites) != 1 {
		t.Errorf("expected 1 pending write, got %d", len(h.pendingWrites))
	}

	// Entries should not have the message yet
	entries, _ := sess.GetEntries(ctx)
	if len(entries) != 0 {
		t.Errorf("expected 0 entries (deferred), got %d", len(entries))
	}
}

func TestAgentHarness_SetResources(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	skills := []Skill{{Name: "test-skill", FilePath: "/test"}}
	resources := HarnessResources{Skills: skills}

	err := h.SetResources(ctx, resources)
	if err != nil {
		t.Fatal(err)
	}

	got := h.GetResources()
	if len(got.Skills) != 1 || got.Skills[0].Name != "test-skill" {
		t.Errorf("expected 1 skill, got %v", got.Skills)
	}
}

func TestAgentHarness_SetStreamOptions(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	timeout := 5000
	opts := HarnessStreamOptions{TimeoutMs: &timeout}
	h.SetStreamOptions(ctx, opts)

	got := h.GetStreamOptions()
	if got.TimeoutMs == nil || *got.TimeoutMs != 5000 {
		t.Error("expected TimeoutMs=5000")
	}
}

func TestAgentHarness_DrainQueue(t *testing.T) {
	h := newTestHarness(t)

	// One-at-a-time mode
	h.steeringMode = agent.QueueOneAtATime
	h.steerQueue = []ai.Message{
		ai.NewUserMessage("msg1"),
		ai.NewUserMessage("msg2"),
	}

	drained := h.drainQueue(&h.steerQueue, h.steeringMode)
	if len(drained) != 1 {
		t.Errorf("expected 1 message, got %d", len(drained))
	}
	if len(h.steerQueue) != 1 {
		t.Errorf("expected 1 remaining, got %d", len(h.steerQueue))
	}

	// All mode
	h.followUpMode = agent.QueueAll
	h.followUpQueue = []ai.Message{
		ai.NewUserMessage("msg3"),
		ai.NewUserMessage("msg4"),
	}
	drained = h.drainQueue(&h.followUpQueue, h.followUpMode)
	if len(drained) != 2 {
		t.Errorf("expected 2 messages, got %d", len(drained))
	}
	if len(h.followUpQueue) != 0 {
		t.Errorf("expected 0 remaining, got %d", len(h.followUpQueue))
	}
}

func TestAgentHarness_FlushPendingWrites(t *testing.T) {
	sess := newMockSession()
	model := &ai.Model{ID: "test", Provider: "faux", API: "faux"}
	opts := HarnessOptions{Model: model}
	h := NewAgentHarness(opts, sess)
	ctx := context.Background()

	// Add pending writes
	h.mu.Lock()
	h.pendingWrites = []PendingSessionWrite{
		{Type: "message", Message: ai.NewUserMessage("hello")},
		{Type: "thinking_level_change", ThinkingLevel: "high"},
	}
	h.mu.Unlock()

	err := h.flushPendingWrites(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Verify writes were applied
	entries, _ := sess.GetEntries(ctx)
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}

	// Verify pending writes were cleared
	h.mu.Lock()
	writes := h.pendingWrites
	h.mu.Unlock()
	if len(writes) != 0 {
		t.Errorf("expected 0 pending writes, got %d", len(writes))
	}
}

func TestAgentHarness_ConcurrentAccess(t *testing.T) {
	sess := newMockSession()
	model := &ai.Model{ID: "test", Provider: "faux", API: "faux"}
	opts := HarnessOptions{Model: model}
	h := NewAgentHarness(opts, sess)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = h.SetThinkingLevel(ctx, "high")
			_ = h.SetModel(ctx, &ai.Model{ID: "model", Provider: "faux", API: "faux"})
			_ = h.GetTools()
			_ = h.GetActiveTools()
			_ = h.GetResources()
		}(i)
	}
	wg.Wait()
}

func TestCreateUserMessage(t *testing.T) {
	msg := createUserMessage("hello world", nil)
	if msg.Role != "user" {
		t.Errorf("expected user role, got %s", msg.Role)
	}
	if msg.Timestamp == 0 {
		t.Error("expected non-zero timestamp")
	}
}

func TestCreateFailureMessage(t *testing.T) {
	model := &ai.Model{ID: "test", Provider: "faux", API: "faux"}
	msg := createFailureMessage(model, "something failed", false)
	if msg.Role != "assistant" {
		t.Errorf("expected assistant role, got %s", msg.Role)
	}
	if msg.StopReason != ai.StopReasonError {
		t.Errorf("expected error stop reason")
	}
	if msg.ErrorMessage == nil || *msg.ErrorMessage != "something failed" {
		t.Error("expected error message")
	}

	// Aborted variant
	msg = createFailureMessage(model, "aborted!", true)
	if msg.StopReason != ai.StopReasonAborted {
		t.Errorf("expected aborted stop reason")
	}
}

func TestFindDuplicateNames(t *testing.T) {
	tests := []struct {
		names []string
		want  int
	}{
		{[]string{"a", "b", "c"}, 0},
		{[]string{"a", "b", "a"}, 1},
		{[]string{"a", "a", "a"}, 1},
		{[]string{}, 0},
	}
	for _, tt := range tests {
		got := findDuplicateNames(tt.names)
		if len(got) != tt.want {
			t.Errorf("findDuplicateNames(%v) = %d, want %d", tt.names, len(got), tt.want)
		}
	}
}

func TestExtractTextContent(t *testing.T) {
	if got := extractTextContent("plain text"); got != "plain text" {
		t.Errorf("expected 'plain text', got %q", got)
	}
	if got := extractTextContent(42); got != "" {
		t.Errorf("expected empty for non-string, got %q", got)
	}
	if got := extractTextContent([]ai.ContentBlock{
		ai.NewTextContent("hello "),
		ai.NewTextContent("world"),
	}); got != "hello world" {
		t.Errorf("expected 'hello world', got %q", got)
	}
}
