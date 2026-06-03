package harness

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/chinudotdev/pi-go/agent"
	"github.com/chinudotdev/pi-go/ai"
)

// ============================================================================
// Helper functions
// ============================================================================

func createUserMessage(text string, images []ai.ContentBlock) ai.Message {
	content := []ai.ContentBlock{ai.NewTextContent(text)}
	content = append(content, images...)
	return ai.Message{
		Role:      "user",
		Content:   content,
		Timestamp: time.Now().UnixMilli(),
	}
}

func createFailureMessage(model *ai.Model, errMsg string, aborted bool) ai.Message {
	stopReason := ai.StopReasonError
	if aborted {
		stopReason = ai.StopReasonAborted
	}
	return ai.Message{
		Role:             "assistant",
		AssistantContent: []ai.ContentBlock{ai.NewTextContent("")},
		API:              model.API,
		Provider:         model.Provider,
		Model:            model.ID,
		StopReason:       stopReason,
		ErrorMessage:     &errMsg,
		Timestamp:        time.Now().UnixMilli(),
		Usage:            ai.Usage{},
	}
}

func cloneStreamOptions(opts *HarnessStreamOptions) HarnessStreamOptions {
	if opts == nil {
		return HarnessStreamOptions{}
	}
	clone := *opts
	if opts.Headers != nil {
		clone.Headers = make(map[string]string)
		for k, v := range opts.Headers {
			clone.Headers[k] = v
		}
	}
	if opts.Metadata != nil {
		clone.Metadata = make(map[string]any)
		for k, v := range opts.Metadata {
			clone.Metadata[k] = v
		}
	}
	return clone
}

func findDuplicateNames(names []string) []string {
	seen := make(map[string]bool)
	dups := make(map[string]bool)
	for _, n := range names {
		if seen[n] {
			dups[n] = true
		}
		seen[n] = true
	}
	result := make([]string, 0, len(dups))
	for n := range dups {
		result = append(result, n)
	}
	return result
}

// ============================================================================
// AgentHarness — orchestrates agent turns with session persistence
// ============================================================================

// turnState holds the snapshot of state for a single agent turn.
type turnState struct {
	messages      []agent.AgentMessage
	resources     HarnessResources
	streamOptions HarnessStreamOptions
	sessionID     string
	systemPrompt  string
	model         *ai.Model
	thinkingLevel string
	tools         []agent.Tool
	activeTools   []agent.Tool
}

// AgentHarness orchestrates multi-turn agent interactions with session
// persistence, event hooks, compaction, and tree navigation.
type AgentHarness struct {
	env  ExecutionEnv
	sess SessionProvider

	mu    sync.Mutex
	phase HarnessPhase

	cancelFunc context.CancelFunc
	runDone    chan struct{}

	pendingWrites []PendingSessionWrite

	model         *ai.Model
	thinkingLevel string
	systemPrompt  any // string or SystemPromptFn
	streamOptions HarnessStreamOptions
	getAuth       GetApiKeyAndHeadersFn
	resources     HarnessResources

	tools           map[string]agent.Tool
	activeToolNames []string

	steerQueue    []ai.Message
	steeringMode  agent.QueueMode
	followUpQueue []ai.Message
	followUpMode  agent.QueueMode
	nextTurnQueue []ai.Message

	// Compaction functions (injected)
	compactFn                   CompactionFunc
	prepareCompactionFn         PrepareCompactionFunc
	defaultCompactionSettingsFn func() any
	collectBranchEntriesFn      CollectBranchEntriesFunc
	generateBranchSummaryFn     GenerateBranchSummaryFunc

	handlers    map[string][]HarnessEventHandler
	allHandlers []HarnessEventHandler
}

// NewAgentHarness creates a new harness from the given options.
func NewAgentHarness(opts HarnessOptions, sess SessionProvider) *AgentHarness {
	h := &AgentHarness{
		env:             opts.Env,
		sess:            sess,
		resources:       derefResources(opts.Resources),
		streamOptions:   cloneStreamOptions(opts.StreamOptions),
		systemPrompt:    opts.SystemPrompt,
		getAuth:         opts.GetApiKeyAndHeaders,
		model:           opts.Model,
		thinkingLevel:   opts.ThinkingLevel,
		tools:           make(map[string]agent.Tool),
		activeToolNames: make([]string, 0),
		steeringMode:    opts.SteeringMode,
		followUpMode:    opts.FollowUpMode,
		handlers:        make(map[string][]HarnessEventHandler),
		allHandlers:     make([]HarnessEventHandler, 0),

		compactFn:                   opts.CompactFn,
		prepareCompactionFn:         opts.PrepareCompactionFn,
		defaultCompactionSettingsFn: opts.DefaultCompactionSettingsFn,
		collectBranchEntriesFn:      opts.CollectBranchEntriesFn,
		generateBranchSummaryFn:     opts.GenerateBranchSummaryFn,
	}

	// Validate and register tools
	if len(opts.Tools) > 0 {
		names := make([]string, len(opts.Tools))
		for i, t := range opts.Tools {
			names[i] = t.Name
		}
		dups := findDuplicateNames(names)
		if len(dups) > 0 {
			panic(fmt.Sprintf("Duplicate tool name(s): %v", dups))
		}
		for _, t := range opts.Tools {
			h.tools[t.Name] = t
		}
	}

	// Active tools
	if len(opts.ActiveToolNames) > 0 {
		h.activeToolNames = make([]string, len(opts.ActiveToolNames))
		copy(h.activeToolNames, opts.ActiveToolNames)
	} else {
		for _, t := range opts.Tools {
			h.activeToolNames = append(h.activeToolNames, t.Name)
		}
	}
	h.validateToolNames(h.activeToolNames)

	if h.thinkingLevel == "" {
		h.thinkingLevel = "off"
	}
	if h.steeringMode == "" {
		h.steeringMode = agent.QueueOneAtATime
	}
	if h.followUpMode == "" {
		h.followUpMode = agent.QueueOneAtATime
	}
	h.phase = PhaseIdle

	return h
}

func derefResources(r *HarnessResources) HarnessResources {
	if r == nil {
		return HarnessResources{}
	}
	return *r
}

// ============================================================================
// Public API — Prompt / Skill / Template
// ============================================================================

// Prompt sends a user message and runs the agent loop to completion.
// Returns the final assistant message.
func (h *AgentHarness) Prompt(ctx context.Context, text string, images []ai.ContentBlock) (*ai.Message, error) {
	if err := h.requirePhase(PhaseIdle); err != nil {
		return nil, err
	}
	h.setPhase(PhaseTurn)
	defer h.ensureIdle()

	ts, err := h.createTurnState(ctx)
	if err != nil {
		return nil, h.wrapError(err, "unknown")
	}
	return h.executeTurn(ctx, ts, text, images)
}

// Skill invokes a named skill and runs the agent loop.
func (h *AgentHarness) Skill(ctx context.Context, name string, additionalInstructions string) (*ai.Message, error) {
	if err := h.requirePhase(PhaseIdle); err != nil {
		return nil, err
	}
	h.setPhase(PhaseTurn)
	defer h.ensureIdle()

	ts, err := h.createTurnState(ctx)
	if err != nil {
		return nil, h.wrapError(err, "unknown")
	}
	skill := h.findSkill(ts.resources.Skills, name)
	if skill == nil {
		return nil, NewAgentHarnessError("invalid_argument", "Unknown skill: "+name, nil)
	}
	prompt := FormatSkillInvocation(*skill, additionalInstructions)
	return h.executeTurn(ctx, ts, prompt, nil)
}

// PromptFromTemplate renders a prompt template and runs the agent loop.
func (h *AgentHarness) PromptFromTemplate(ctx context.Context, name string, args []string) (*ai.Message, error) {
	if err := h.requirePhase(PhaseIdle); err != nil {
		return nil, err
	}
	h.setPhase(PhaseTurn)
	defer h.ensureIdle()

	ts, err := h.createTurnState(ctx)
	if err != nil {
		return nil, h.wrapError(err, "unknown")
	}
	tmpl := h.findTemplate(ts.resources.PromptTemplates, name)
	if tmpl == nil {
		return nil, NewAgentHarnessError("invalid_argument", "Unknown prompt template: "+name, nil)
	}
	prompt := FormatPromptTemplateInvocation(*tmpl, args)
	return h.executeTurn(ctx, ts, prompt, nil)
}

// ============================================================================
// Public API — Queues
// ============================================================================

// Steer enqueues a steering message (injected mid-run).
func (h *AgentHarness) Steer(ctx context.Context, text string, images []ai.ContentBlock) error {
	if h.getPhase() == PhaseIdle {
		return NewAgentHarnessError("invalid_state", "Cannot steer while idle", nil)
	}
	h.mu.Lock()
	h.steerQueue = append(h.steerQueue, createUserMessage(text, images))
	h.mu.Unlock()
	return h.emitOwn(ctx, HarnessEvent{Type: "queue_update",
		Steer:    h.steerQueueCopy(),
		FollowUp: h.followUpQueueCopy(),
		NextTurn: h.nextTurnQueueCopy(),
	})
}

// FollowUp enqueues a follow-up message (processed after agent would stop).
func (h *AgentHarness) FollowUp(ctx context.Context, text string, images []ai.ContentBlock) error {
	if h.getPhase() == PhaseIdle {
		return NewAgentHarnessError("invalid_state", "Cannot follow up while idle", nil)
	}
	h.mu.Lock()
	h.followUpQueue = append(h.followUpQueue, createUserMessage(text, images))
	h.mu.Unlock()
	return h.emitOwn(ctx, HarnessEvent{Type: "queue_update",
		Steer:    h.steerQueueCopy(),
		FollowUp: h.followUpQueueCopy(),
		NextTurn: h.nextTurnQueueCopy(),
	})
}

// NextTurn enqueues a message for the next turn.
func (h *AgentHarness) NextTurn(ctx context.Context, text string, images []ai.ContentBlock) error {
	h.mu.Lock()
	h.nextTurnQueue = append(h.nextTurnQueue, createUserMessage(text, images))
	h.mu.Unlock()
	return h.emitOwn(ctx, HarnessEvent{Type: "queue_update",
		Steer:    h.steerQueueCopy(),
		FollowUp: h.followUpQueueCopy(),
		NextTurn: h.nextTurnQueueCopy(),
	})
}

// AppendMessage appends a message to the session (directly if idle, deferred if busy).
func (h *AgentHarness) AppendMessage(ctx context.Context, msg ai.Message) error {
	h.mu.Lock()
	isIdle := h.phase == PhaseIdle
	if isIdle {
		h.mu.Unlock()
		_, err := h.sess.AppendMessage(ctx, msg)
		return h.wrapError(err, "session")
	}
	h.pendingWrites = append(h.pendingWrites, PendingSessionWrite{Type: "message", Message: msg})
	h.mu.Unlock()
	return nil
}

// ============================================================================
// Public API — Compaction & Tree Navigation
// ============================================================================

// Compact runs compaction on the current session branch.
func (h *AgentHarness) Compact(ctx context.Context, customInstructions string) (*CompactionResult, error) {
	if err := h.requirePhase(PhaseIdle); err != nil {
		return nil, err
	}
	h.setPhase(PhaseCompaction)
	defer h.ensureIdle()

	if h.model == nil {
		return nil, NewAgentHarnessError("invalid_state", "No model set for compaction", nil)
	}
	auth, err := h.resolveAuth(h.model)
	if err != nil {
		return nil, err
	}

	branchEntries, err := h.sess.GetBranch(ctx, nil)
	if err != nil {
		return nil, h.wrapError(err, "session")
	}

	if h.prepareCompactionFn == nil || h.defaultCompactionSettingsFn == nil {
		return nil, NewAgentHarnessError("invalid_state", "Compaction functions not configured", nil)
	}

	settings := h.defaultCompactionSettingsFn()
	prepResult, prepErr := h.prepareCompactionFn(branchEntries, settings)
	if prepErr != nil {
		return nil, h.wrapError(prepErr, "compaction")
	}
	if prepResult == nil {
		return nil, NewAgentHarnessError("compaction", "Nothing to compact", nil)
	}

	// Run compaction via AI
	compactResult, compactErr := h.compactFn(ctx, prepResult, h.model, auth.APIKey, auth.Headers, customInstructions, h.thinkingLevel)
	if compactErr != nil {
		return nil, h.wrapError(compactErr, "compaction")
	}

	// Type-assert result
	result, ok := compactResult.(*CompactionResult)
	if !ok {
		// Try map-based result
		return nil, NewAgentHarnessError("compaction", "Unexpected compaction result type", nil)
	}

	// Append compaction entry
	_, err = h.sess.AppendCompaction(ctx, result.Summary, result.FirstKeptEntryID, result.TokensBefore, nil, false)
	if err != nil {
		return nil, h.wrapError(err, "session")
	}

	return result, nil
}

// NavigateTree moves the session leaf to a different entry, optionally generating a branch summary.
func (h *AgentHarness) NavigateTree(ctx context.Context, targetID string, summarize bool, customInstructions string) (*NavigateTreeResult, error) {
	if err := h.requirePhase(PhaseIdle); err != nil {
		return nil, err
	}
	h.setPhase(PhaseBranchSummary)
	defer h.ensureIdle()

	oldLeafID, err := h.sess.GetLeafID(ctx)
	if err != nil {
		return nil, h.wrapError(err, "session")
	}
	oldLeaf := ""
	if oldLeafID != nil {
		oldLeaf = *oldLeafID
	}
	if oldLeaf == targetID {
		return &NavigateTreeResult{Cancelled: false}, nil
	}

	targetEntry, err := h.sess.GetEntry(ctx, targetID)
	if err != nil {
		return nil, h.wrapError(err, "session")
	}
	if targetEntry == nil {
		return nil, NewAgentHarnessError("invalid_argument", "Entry "+targetID+" not found", nil)
	}

	// Collect branch entries for summarization
	if h.collectBranchEntriesFn == nil {
		return nil, NewAgentHarnessError("invalid_state", "Branch entry collection not configured", nil)
	}
	entries, commonAncestorID, err := h.collectBranchEntriesFn(h.sess, oldLeaf, targetID)
	if err != nil {
		return nil, h.wrapError(err, "branch_summary")
	}
	_ = commonAncestorID

	// Generate branch summary if requested
	var summaryText string
	var summaryDetails any
	if summarize && len(entries) > 0 {
		if h.model == nil {
			return nil, NewAgentHarnessError("invalid_state", "No model set for branch summary", nil)
		}
		if h.generateBranchSummaryFn == nil {
			return nil, NewAgentHarnessError("invalid_state", "Branch summary generation not configured", nil)
		}
		bsResult, bsErr := h.generateBranchSummaryFn(ctx, entries, nil)
		if bsErr != nil {
			return nil, NewAgentHarnessError("branch_summary", bsErr.Error(), bsErr)
		}
		if bsResult == nil {
			return &NavigateTreeResult{Cancelled: true}, nil
		}
		if bsr, ok := bsResult.(*BranchSummaryResult); ok {
			summaryText = bsr.Summary
			summaryDetails = bsr
		}
	}

	// Determine new leaf
	var newLeafID *string
	var editorText string
	if targetEntry.Type == "message" && targetEntry.Message.Role == "user" {
		pid := targetEntry.ParentID
		newLeafID = pid
		editorText = extractTextContent(targetEntry.Message.Content)
	} else {
		id := targetID
		newLeafID = &id
	}

	// Move
	var summaryForMove *BranchSummaryResult
	if summaryText != "" {
		summaryForMove = &BranchSummaryResult{
			Summary: summaryText,
			Details: summaryDetails,
		}
	}
	_, err = h.sess.MoveTo(ctx, newLeafID, summaryForMove)
	if err != nil {
		return nil, h.wrapError(err, "session")
	}

	newLeaf, _ := h.sess.GetLeafID(ctx)
	newLeafStr := ""
	if newLeaf != nil {
		newLeafStr = *newLeaf
	}
	_ = h.emitOwn(ctx, HarnessEvent{
		Type:      "session_tree",
		NewLeafID: &newLeafStr,
		OldLeafID: &oldLeaf,
	})

	return &NavigateTreeResult{
		Cancelled:  false,
		EditorText: editorText,
	}, nil
}

// ============================================================================
// Public API — Model / Tools / Settings
// ============================================================================

// GetModel returns the current model.
func (h *AgentHarness) GetModel() *ai.Model { return h.model }

// SetModel changes the model and persists the change to the session.
func (h *AgentHarness) SetModel(ctx context.Context, model *ai.Model) error {
	prev := h.model
	h.mu.Lock()
	isIdle := h.phase == PhaseIdle
	if isIdle {
		h.mu.Unlock()
		_, err := h.sess.AppendModelChange(ctx, model.Provider, model.ID)
		if err != nil {
			return h.wrapError(err, "session")
		}
	} else {
		h.pendingWrites = append(h.pendingWrites, PendingSessionWrite{
			Type:     "model_change",
			Provider: model.Provider,
			ModelID:  model.ID,
		})
		h.mu.Unlock()
	}
	h.model = model
	return h.emitOwn(ctx, HarnessEvent{
		Type:          "model_update",
		Model:         model,
		PreviousModel: prev,
		Source:        "set",
	})
}

// GetThinkingLevel returns the current thinking level.
func (h *AgentHarness) GetThinkingLevel() string { return h.thinkingLevel }

// SetThinkingLevel changes the thinking level and persists the change.
func (h *AgentHarness) SetThinkingLevel(ctx context.Context, level string) error {
	prev := h.thinkingLevel
	h.mu.Lock()
	isIdle := h.phase == PhaseIdle
	if isIdle {
		h.mu.Unlock()
		_, err := h.sess.AppendThinkingLevelChange(ctx, level)
		if err != nil {
			return h.wrapError(err, "session")
		}
	} else {
		h.pendingWrites = append(h.pendingWrites, PendingSessionWrite{
			Type:          "thinking_level_change",
			ThinkingLevel: level,
		})
		h.mu.Unlock()
	}
	h.thinkingLevel = level
	return h.emitOwn(ctx, HarnessEvent{
		Type:          "thinking_level_update",
		Level:         level,
		PreviousLevel: prev,
	})
}

// GetTools returns all registered tools.
func (h *AgentHarness) GetTools() []agent.Tool {
	h.mu.Lock()
	defer h.mu.Unlock()
	result := make([]agent.Tool, 0, len(h.tools))
	for _, t := range h.tools {
		result = append(result, t)
	}
	return result
}

// SetTools replaces the tool set and optionally the active tool names.
func (h *AgentHarness) SetTools(ctx context.Context, tools []agent.Tool, activeToolNames []string) error {
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name
	}
	if dups := findDuplicateNames(names); len(dups) > 0 {
		return NewAgentHarnessError("invalid_argument", fmt.Sprintf("Duplicate tool name(s): %v", dups), nil)
	}

	newMap := make(map[string]agent.Tool, len(tools))
	for _, t := range tools {
		newMap[t.Name] = t
	}

	nextActive := activeToolNames
	if nextActive == nil {
		nextActive = h.activeToolNames
	}
	h.validateToolNamesWith(nextActive, newMap)

	h.mu.Lock()
	prevNames := h.toolNames()
	prevActive := make([]string, len(h.activeToolNames))
	copy(prevActive, h.activeToolNames)

	isIdle := h.phase == PhaseIdle
	if isIdle {
		h.mu.Unlock()
		_, err := h.sess.AppendActiveToolsChange(ctx, nextActive)
		if err != nil {
			return h.wrapError(err, "session")
		}
	} else {
		h.pendingWrites = append(h.pendingWrites, PendingSessionWrite{
			Type:            "active_tools_change",
			ActiveToolNames: make([]string, len(nextActive)),
		})
		copy(h.pendingWrites[len(h.pendingWrites)-1].ActiveToolNames, nextActive)
		h.mu.Unlock()
	}

	h.mu.Lock()
	h.tools = newMap
	h.activeToolNames = make([]string, len(nextActive))
	copy(h.activeToolNames, nextActive)
	h.mu.Unlock()

	return h.emitOwn(ctx, HarnessEvent{
		Type:                    "tools_update",
		ToolNames:               h.toolNames(),
		PreviousToolNames:       prevNames,
		ActiveToolNamesEvt:      make([]string, len(nextActive)),
		PreviousActiveToolNames: prevActive,
		Source:                  "set",
	})
}

// GetActiveTools returns the currently active tools.
func (h *AgentHarness) GetActiveTools() []agent.Tool {
	h.mu.Lock()
	defer h.mu.Unlock()
	result := make([]agent.Tool, 0, len(h.activeToolNames))
	for _, name := range h.activeToolNames {
		if t, ok := h.tools[name]; ok {
			result = append(result, t)
		}
	}
	return result
}

// SetActiveTools changes which tools are active.
func (h *AgentHarness) SetActiveTools(ctx context.Context, toolNames []string) error {
	h.validateToolNames(toolNames)

	h.mu.Lock()
	prevNames := h.toolNames()
	prevActive := make([]string, len(h.activeToolNames))
	copy(prevActive, h.activeToolNames)

	isIdle := h.phase == PhaseIdle
	if isIdle {
		h.mu.Unlock()
		_, err := h.sess.AppendActiveToolsChange(ctx, toolNames)
		if err != nil {
			return h.wrapError(err, "session")
		}
	} else {
		h.pendingWrites = append(h.pendingWrites, PendingSessionWrite{
			Type:            "active_tools_change",
			ActiveToolNames: make([]string, len(toolNames)),
		})
		copy(h.pendingWrites[len(h.pendingWrites)-1].ActiveToolNames, toolNames)
		h.mu.Unlock()
	}

	h.mu.Lock()
	h.activeToolNames = make([]string, len(toolNames))
	copy(h.activeToolNames, toolNames)
	h.mu.Unlock()

	return h.emitOwn(ctx, HarnessEvent{
		Type:                    "tools_update",
		ToolNames:               h.toolNames(),
		PreviousToolNames:       prevNames,
		ActiveToolNamesEvt:      make([]string, len(toolNames)),
		PreviousActiveToolNames: prevActive,
		Source:                  "set",
	})
}

// GetResources returns the current resources.
func (h *AgentHarness) GetResources() HarnessResources {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.copyResources()
}

// SetResources replaces the current resources.
func (h *AgentHarness) SetResources(ctx context.Context, resources HarnessResources) error {
	h.mu.Lock()
	prev := h.copyResources()
	h.resources = resources
	h.mu.Unlock()
	return h.emitOwn(ctx, HarnessEvent{
		Type:              "resources_update",
		Resources:         &resources,
		PreviousResources: &prev,
	})
}

// GetStreamOptions returns a copy of the current stream options.
func (h *AgentHarness) GetStreamOptions() HarnessStreamOptions {
	return cloneStreamOptions(&h.streamOptions)
}

// SetStreamOptions replaces stream options.
func (h *AgentHarness) SetStreamOptions(_ context.Context, opts HarnessStreamOptions) {
	h.streamOptions = cloneStreamOptions(&opts)
}

// GetSteeringMode returns the current steering mode.
func (h *AgentHarness) GetSteeringMode() agent.QueueMode { return h.steeringMode }

// SetSteeringMode changes the steering mode.
func (h *AgentHarness) SetSteeringMode(_ context.Context, mode agent.QueueMode) {
	h.steeringMode = mode
}

// GetFollowUpMode returns the current follow-up mode.
func (h *AgentHarness) GetFollowUpMode() agent.QueueMode { return h.followUpMode }

// SetFollowUpMode changes the follow-up mode.
func (h *AgentHarness) SetFollowUpMode(_ context.Context, mode agent.QueueMode) {
	h.followUpMode = mode
}

// ============================================================================
// Public API — Lifecycle
// ============================================================================

// Abort cancels the current run and clears queues.
func (h *AgentHarness) Abort(ctx context.Context) (*AbortResult, error) {
	h.mu.Lock()
	clearedSteer := h.steerQueue
	clearedFollowUp := h.followUpQueue
	h.steerQueue = nil
	h.followUpQueue = nil
	if h.cancelFunc != nil {
		h.cancelFunc()
	}
	h.mu.Unlock()

	var errs []error
	if err := h.emitOwn(ctx, HarnessEvent{Type: "queue_update"}); err != nil {
		errs = append(errs, err)
	}
	h.waitForIdle()
	if err := h.emitOwn(ctx, HarnessEvent{
		Type:            "abort",
		ClearedSteer:    clearedSteer,
		ClearedFollowUp: clearedFollowUp,
	}); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return nil, h.wrapError(errs[0], "hook")
	}
	return &AbortResult{
		ClearedSteer:    clearedSteer,
		ClearedFollowUp: clearedFollowUp,
	}, nil
}

// WaitForIdle blocks until the harness is idle.
func (h *AgentHarness) WaitForIdle() { h.waitForIdle() }

// Subscribe registers a catch-all event handler.
func (h *AgentHarness) Subscribe(handler HarnessEventHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.allHandlers = append(h.allHandlers, handler)
}

// On registers a handler for a specific event type.
func (h *AgentHarness) On(eventType string, handler HarnessEventHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.handlers[eventType] = append(h.handlers[eventType], handler)
}

// GetPhase returns the current harness phase.
func (h *AgentHarness) GetPhase() HarnessPhase {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.phase
}

// ============================================================================
// Internal — turn execution
// ============================================================================

func (h *AgentHarness) executeTurn(ctx context.Context, ts *turnState, text string, images []ai.ContentBlock) (*ai.Message, error) {
	msgs := []agent.AgentMessage{createUserMessage(text, images)}

	// Drain nextTurn queue
	h.mu.Lock()
	if len(h.nextTurnQueue) > 0 {
		queued := h.nextTurnQueue
		h.nextTurnQueue = nil
		msgs = append(queued, msgs...)
	}
	h.mu.Unlock()

	// before_agent_start hook
	_ = h.emitOwn(ctx, HarnessEvent{
		Type:         "before_agent_start",
		Prompt:       text,
		Images:       images,
		SystemPrompt: ts.systemPrompt,
		Resources:    &ts.resources,
	})

	// Create cancellable context
	turnCtx, cancel := context.WithCancel(ctx)
	h.mu.Lock()
	h.cancelFunc = cancel
	h.mu.Unlock()

	// Build agent context
	agentCtx := &agent.AgentContext{
		SystemPrompt: ts.systemPrompt,
		Messages:     ts.messages,
		Tools:        toolsToPtrs(ts.activeTools),
	}

	// Collect events to find the last assistant message
	var lastAssistant *ai.Message
	emit := func(event agent.Event) error {
		if event.Type == agent.EventMessageEnd {
			if event.Msg.Role == "assistant" {
				msg := event.Msg
				lastAssistant = &msg
			}
		}
		h.handleAgentEvent(turnCtx, event)
		return nil
	}

	// Build loop config
	loopConfig := h.createLoopConfig(turnCtx, ts, cancel)

	// Build stream function
	streamFn := h.createStreamFn(ts)

	// Run the agent loop
	loopErr := agent.RunAgentLoop(turnCtx, msgs, agentCtx, loopConfig, emit, streamFn)

	// Flush pending writes
	_ = h.flushPendingWrites(turnCtx)

	h.mu.Lock()
	h.cancelFunc = nil
	h.mu.Unlock()

	if loopErr != nil {
		aborted := turnCtx.Err() != nil
		failureMsg := createFailureMessage(ts.model, loopErr.Error(), aborted)
		lastAssistant = &failureMsg
	}

	h.ensureIdle()
	if lastAssistant == nil {
		return nil, NewAgentHarnessError("invalid_state", "AgentHarness prompt completed without an assistant message", nil)
	}
	return lastAssistant, nil
}

// ============================================================================
// Internal — event handling
// ============================================================================

func (h *AgentHarness) handleAgentEvent(ctx context.Context, event agent.Event) {
	switch event.Type {
	case agent.EventMessageEnd:
		_ = h.flushSingleWrite(ctx, event.Msg)

	case agent.EventTurnEnd:
		_ = h.flushPendingWrites(ctx)
		_ = h.emitOwn(ctx, HarnessEvent{Type: "save_point", HadPendingMutations: len(h.pendingWrites) > 0})

	case agent.EventAgentEnd:
		_ = h.flushPendingWrites(ctx)
		h.setPhase(PhaseIdle)
		_ = h.emitOwn(ctx, HarnessEvent{Type: "settled"})
	}

	// Forward to all handlers
	_ = h.emitAny(ctx, HarnessEvent{
		Type:     event.Type,
		Messages: event.Messages,
	})
}

func (h *AgentHarness) flushSingleWrite(ctx context.Context, msg agent.AgentMessage) error {
	// Direct write since we're in a turn
	_, err := h.sess.AppendMessage(ctx, msg)
	return err
}

func (h *AgentHarness) flushPendingWrites(ctx context.Context) error {
	h.mu.Lock()
	writes := h.pendingWrites
	h.pendingWrites = nil
	h.mu.Unlock()

	for _, w := range writes {
		var err error
		switch w.Type {
		case "message":
			_, err = h.sess.AppendMessage(ctx, w.Message)
		case "model_change":
			_, err = h.sess.AppendModelChange(ctx, w.Provider, w.ModelID)
		case "thinking_level_change":
			_, err = h.sess.AppendThinkingLevelChange(ctx, w.ThinkingLevel)
		case "active_tools_change":
			_, err = h.sess.AppendActiveToolsChange(ctx, w.ActiveToolNames)
		case "custom":
			_, err = h.sess.AppendCustomEntry(ctx, w.CustomType, w.Data)
		case "custom_message":
			_, err = h.sess.AppendCustomMessageEntry(ctx, w.CustomType, w.Content, w.Display, w.Details)
		case "label":
			_, err = h.sess.AppendLabel(ctx, w.TargetID, w.Label)
		case "session_info":
			_, err = h.sess.AppendSessionName(ctx, w.Name)
		}
		if err != nil {
			// Re-enqueue remaining
			h.mu.Lock()
			h.pendingWrites = append(writes, h.pendingWrites...)
			h.mu.Unlock()
			return err
		}
	}
	return nil
}

// ============================================================================
// Internal — loop config & stream fn
// ============================================================================

func (h *AgentHarness) createLoopConfig(ctx context.Context, ts *turnState, cancel context.CancelFunc) *agent.LoopConfig {
	return &agent.LoopConfig{
		Model:     ts.model,
		Reasoning: agent.ThinkingLevel(ts.thinkingLevel),
		ConvertToLlm: func(msgs []agent.AgentMessage) ([]ai.Message, error) {
			return ConvertToLlm(msgs), nil
		},
		TransformContext: func(_ context.Context, msgs []agent.AgentMessage) ([]agent.AgentMessage, error) {
			return msgs, nil
		},
		BeforeToolCall: func(tctx agent.BeforeToolCallContext) (*agent.BeforeToolCallResult, error) {
			return nil, nil
		},
		AfterToolCall: func(tctx agent.AfterToolCallContext) (*agent.AfterToolCallResult, error) {
			return nil, nil
		},
		PrepareNextTurn: func(_ agent.PrepareNextTurnContext) (*agent.AgentLoopTurnUpdate, error) {
			return &agent.AgentLoopTurnUpdate{
				Context: &agent.AgentContext{
					SystemPrompt: ts.systemPrompt,
					Messages:     ts.messages,
					Tools:        toolsToPtrs(ts.activeTools),
				},
				Model:         ts.model,
				ThinkingLevel: agent.ThinkingLevel(ts.thinkingLevel),
			}, nil
		},
		GetSteeringMessages: func() ([]agent.AgentMessage, error) {
			return h.drainQueue(&h.steerQueue, h.steeringMode), nil
		},
		GetFollowUpMessages: func() ([]agent.AgentMessage, error) {
			return h.drainQueue(&h.followUpQueue, h.followUpMode), nil
		},
	}
}

func (h *AgentHarness) createStreamFn(ts *turnState) agent.StreamFn {
	return func(ctx context.Context, model *ai.Model, convCtx *ai.Context, options *ai.SimpleStreamOptions) (*ai.EventStream, error) {
		// Resolve auth
		var apiKey *string
		var headers map[string]string
		if h.getAuth != nil {
			auth, err := h.getAuth(model)
			if err != nil {
				return nil, err
			}
			apiKey = &auth.APIKey
			headers = auth.Headers
		}

		// Merge headers
		mergedHeaders := headers
		if ts.streamOptions.Headers != nil {
			mergedHeaders = MergeHeaders(ts.streamOptions.Headers, mergedHeaders)
		}

		streamOpts := &ai.SimpleStreamOptions{
			StreamOptions: ai.StreamOptions{
				APIKey:  apiKey,
				Headers: mergedHeaders,
			},
			Reasoning: ai.ThinkingLevel(ts.thinkingLevel),
		}

		if ts.streamOptions.TimeoutMs != nil {
			streamOpts.StreamOptions.TimeoutMs = ts.streamOptions.TimeoutMs
		}
		if ts.streamOptions.MaxRetries != nil {
			streamOpts.StreamOptions.MaxRetries = ts.streamOptions.MaxRetries
		}
		if ts.streamOptions.CacheRetention != nil {
			streamOpts.StreamOptions.CacheRetention = *ts.streamOptions.CacheRetention
		}

		return ai.StreamSimple(ctx, model, convCtx, streamOpts)
	}
}

// ============================================================================
// Internal — turn state
// ============================================================================

func (h *AgentHarness) createTurnState(ctx context.Context) (*turnState, error) {
	sessCtx, err := h.sess.BuildContext(ctx)
	if err != nil {
		return nil, err
	}
	meta, err := h.sess.GetMetadata(ctx)
	if err != nil {
		return nil, err
	}

	var msgs []agent.AgentMessage
	if sessCtx != nil && sessCtx.Messages != nil {
		msgs = sessCtx.Messages
	}

	resources := h.copyResources()

	tools := h.GetTools()
	activeTools := h.GetActiveTools()

	// Resolve system prompt
	sysPrompt := "You are a helpful assistant."
	if h.systemPrompt != nil {
		switch sp := h.systemPrompt.(type) {
		case string:
			sysPrompt = sp
		case SystemPromptFn:
			resolved, err := sp(h.env, h.model, h.thinkingLevel, activeTools, resources)
			if err != nil {
				return nil, err
			}
			sysPrompt = resolved
		}
	}

	return &turnState{
		messages:      msgs,
		resources:     resources,
		streamOptions: cloneStreamOptions(&h.streamOptions),
		sessionID:     meta.ID,
		systemPrompt:  sysPrompt,
		model:         h.model,
		thinkingLevel: h.thinkingLevel,
		tools:         tools,
		activeTools:   activeTools,
	}, nil
}

// ============================================================================
// Internal — event emission
// ============================================================================

func (h *AgentHarness) emitOwn(ctx context.Context, event HarnessEvent) error {
	// Call wildcard handlers
	for _, handler := range h.allHandlers {
		if _, err := handler(event); err != nil {
			return h.wrapError(err, "hook")
		}
	}
	// Call per-type handlers
	if typed, ok := h.handlers[event.Type]; ok {
		for _, handler := range typed {
			if _, err := handler(event); err != nil {
				return h.wrapError(err, "hook")
			}
		}
	}
	return nil
}

func (h *AgentHarness) emitAny(ctx context.Context, event HarnessEvent) error {
	return h.emitOwn(ctx, event)
}

func (h *AgentHarness) emitHook(ctx context.Context, event HarnessEvent) (any, error) {
	handlers := h.handlers[event.Type]
	var lastResult any
	for _, handler := range handlers {
		result, err := handler(event)
		if err != nil {
			return nil, h.wrapError(err, "hook")
		}
		if result != nil {
			lastResult = result
		}
	}
	return lastResult, nil
}

// ============================================================================
// Internal — validation & helpers
// ============================================================================

func (h *AgentHarness) requirePhase(want HarnessPhase) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.phase != want {
		if h.phase == PhaseTurn {
			return NewAgentHarnessError("busy", "AgentHarness is busy", nil)
		}
		return NewAgentHarnessError("invalid_state", "AgentHarness is in phase "+string(h.phase), nil)
	}
	return nil
}

func (h *AgentHarness) setPhase(p HarnessPhase) {
	h.mu.Lock()
	h.phase = p
	h.mu.Unlock()
}

func (h *AgentHarness) ensureIdle() {
	h.mu.Lock()
	h.phase = PhaseIdle
	h.mu.Unlock()
}

func (h *AgentHarness) getPhase() HarnessPhase {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.phase
}

func (h *AgentHarness) waitForIdle() {
	h.mu.Lock()
	done := h.runDone
	h.mu.Unlock()
	if done != nil {
		<-done
	}
}

func (h *AgentHarness) validateToolNames(names []string) {
	h.validateToolNamesWith(names, h.tools)
}

func (h *AgentHarness) validateToolNamesWith(names []string, tools map[string]agent.Tool) {
	if dups := findDuplicateNames(names); len(dups) > 0 {
		panic(fmt.Sprintf("Duplicate active tool name(s): %v", dups))
	}
	for _, n := range names {
		if _, ok := tools[n]; !ok {
			panic(fmt.Sprintf("Unknown tool: %s", n))
		}
	}
}

func (h *AgentHarness) wrapError(err error, code string) error {
	if err == nil {
		return nil
	}
	if _, ok := err.(*AgentHarnessError); ok {
		return err
	}
	return NewAgentHarnessError(AgentHarnessErrorCode(code), err.Error(), err)
}

func (h *AgentHarness) resolveAuth(model *ai.Model) (*AuthInfo, error) {
	if h.getAuth == nil {
		return nil, NewAgentHarnessError("auth", "No auth provider configured", nil)
	}
	return h.getAuth(model)
}

func (h *AgentHarness) drainQueue(queue *[]ai.Message, mode agent.QueueMode) []ai.Message {
	h.mu.Lock()
	defer h.mu.Unlock()
	if mode == agent.QueueAll {
		drained := *queue
		*queue = nil
		return drained
	}
	if len(*queue) == 0 {
		return nil
	}
	first := (*queue)[0]
	*queue = (*queue)[1:]
	return []ai.Message{first}
}

func (h *AgentHarness) toolNames() []string {
	names := make([]string, 0, len(h.tools))
	for n := range h.tools {
		names = append(names, n)
	}
	return names
}

func (h *AgentHarness) copyResources() HarnessResources {
	r := HarnessResources{}
	if h.resources.Skills != nil {
		r.Skills = make([]Skill, len(h.resources.Skills))
		copy(r.Skills, h.resources.Skills)
	}
	if h.resources.PromptTemplates != nil {
		r.PromptTemplates = make([]PromptTemplate, len(h.resources.PromptTemplates))
		copy(r.PromptTemplates, h.resources.PromptTemplates)
	}
	return r
}

func (h *AgentHarness) steerQueueCopy() []ai.Message    { return copyMessages(h.steerQueue) }
func (h *AgentHarness) followUpQueueCopy() []ai.Message { return copyMessages(h.followUpQueue) }
func (h *AgentHarness) nextTurnQueueCopy() []ai.Message { return copyMessages(h.nextTurnQueue) }

func copyMessages(src []ai.Message) []ai.Message {
	if src == nil {
		return nil
	}
	dst := make([]ai.Message, len(src))
	copy(dst, src)
	return dst
}

func (h *AgentHarness) findSkill(skills []Skill, name string) *Skill {
	for i := range skills {
		if skills[i].Name == name {
			return &skills[i]
		}
	}
	return nil
}

func (h *AgentHarness) findTemplate(templates []PromptTemplate, name string) *PromptTemplate {
	for i := range templates {
		if templates[i].Name == name {
			return &templates[i]
		}
	}
	return nil
}

func extractTextContent(content any) string {
	switch c := content.(type) {
	case string:
		return c
	case []ai.ContentBlock:
		var text string
		for _, block := range c {
			if block.Type == "text" {
				text += block.Text
			}
		}
		return text
	}
	return ""
}

func toolsToPtrs(tools []agent.Tool) []*agent.Tool {
	ptrs := make([]*agent.Tool, len(tools))
	for i := range tools {
		ptrs[i] = &tools[i]
	}
	return ptrs
}
