// Package sdk provides the top-level entry point for the pi coding agent SDK.
// It wires together auth, settings, model registry, resource loading,
// tools, and the agent harness into a cohesive session lifecycle.
package sdk

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/chinudotdev/pi-go/agent"
	"github.com/chinudotdev/pi-go/agent/harness"
	"github.com/chinudotdev/pi-go/agent/harness/compaction"
	"github.com/chinudotdev/pi-go/agent/harness/env"
	"github.com/chinudotdev/pi-go/agent/harness/session"
	"github.com/chinudotdev/pi-go/ai"
	"github.com/chinudotdev/pi-go/ai/providers"
	"github.com/chinudotdev/pi-go/sdk/auth"
	"github.com/chinudotdev/pi-go/sdk/config"
	"github.com/chinudotdev/pi-go/sdk/models"
	"github.com/chinudotdev/pi-go/sdk/prompt"
	"github.com/chinudotdev/pi-go/sdk/resources"
	"github.com/chinudotdev/pi-go/sdk/settings"
	sdkskills "github.com/chinudotdev/pi-go/sdk/skills"
	sdktools "github.com/chinudotdev/pi-go/sdk/tools"
)

func init() {
	providers.RegisterBuiltInApiProviders()
}

// ============================================================================
// Types
// ============================================================================

// SessionEvent represents events emitted during agent session operation.
type SessionEvent struct {
	Type string `json:"type"`

	// Common fields (present based on Type)
	Message      *ai.Message `json:"message,omitempty"`
	ToolName     string      `json:"toolName,omitempty"`
	ToolCallID   string      `json:"toolCallId,omitempty"`
	ErrorMessage string      `json:"errorMessage,omitempty"`
	Attempt      int         `json:"attempt,omitempty"`
	MaxAttempts  int         `json:"maxAttempts,omitempty"`
	DelayMs      int         `json:"delayMs,omitempty"`
	Level        string      `json:"level,omitempty"`
	Model        *ai.Model   `json:"model,omitempty"`

	// Compaction
	Reason    string `json:"reason,omitempty"`
	Aborted   bool   `json:"aborted,omitempty"`
	WillRetry bool   `json:"willRetry,omitempty"`

	// Queue
	Steering []string `json:"steering,omitempty"`
	FollowUp []string `json:"followUp,omitempty"`
}

// SessionEventListener processes session events.
type SessionEventListener func(event SessionEvent)

// ModelCycleResult holds the result of cycling to a different model.
type ModelCycleResult struct {
	Model         *ai.Model             `json:"model"`
	ThinkingLevel ai.ModelThinkingLevel `json:"thinkingLevel"`
	IsScoped      bool                  `json:"isScoped"`
}

// SessionStats holds statistics about the current session.
type SessionStats struct {
	SessionID         string    `json:"sessionId"`
	StartedAt         time.Time `json:"startedAt"`
	UserMessages      int       `json:"userMessages"`
	AssistantMessages int       `json:"assistantMessages"`
	ToolCalls         int       `json:"toolCalls"`
	ToolResults       int       `json:"toolResults"`
	TotalMessages     int       `json:"totalMessages"`
	InputTokens       int64     `json:"inputTokens"`
	OutputTokens      int64     `json:"outputTokens"`
	Cost              float64   `json:"cost"`
}

// Duration returns the time elapsed since the session started.
func (s *SessionStats) Duration() time.Duration {
	return time.Since(s.StartedAt)
}

// CreateSessionOptions configures session creation.
type CreateSessionOptions struct {
	// Working directory. Default: os.Getwd()
	CWD string

	// Agent config directory. Default: config.GetAgentDir()
	AgentDir string

	// Model selection
	Model         *ai.Model
	ThinkingLevel ai.ModelThinkingLevel // "off", "minimal", "low", "medium", "high"

	// Tool configuration
	NoTools      bool     // Disable all tools
	ToolList     []string // Allowlist (nil = use defaults)
	ExcludeTools []string // Denylist

	// Resource overrides (skip auto-discovery when provided)
	ResourceLoader *resources.Loader

	// Auth/Settings/Models overrides
	AuthStorage   *auth.Storage
	SettingsMgr   *settings.Manager
	ModelRegistry *models.Registry

	// Session storage directory override
	SessionDir string
}

// CreateSessionResult holds the created session and metadata.
type CreateSessionResult struct {
	Session              *AgentSession
	ModelFallbackMessage string
}

// ============================================================================
// AgentSession
// ============================================================================

// AgentSession manages the lifecycle of a coding agent session.
// It wraps the AgentHarness with SDK-level concerns: resource loading,
// model management, compaction, and event handling.
type AgentSession struct {
	harness     *harness.AgentHarness
	sess        *session.Session
	modelReg    *models.Registry
	settingsMgr *settings.Manager
	resLoader   *resources.Loader

	mu               sync.Mutex
	cwd              string
	agentDir         string
	model            *ai.Model
	thinkingLevel    ai.ModelThinkingLevel
	baseSystemPrompt string
	startedAt        time.Time

	listenerPtrs []*SessionEventListener
	scopedModels []ScopedModelEntry
}

// ScopedModelEntry pairs a model with a thinking level for scoped model cycling.
type ScopedModelEntry struct {
	Model         *ai.Model
	ThinkingLevel ai.ModelThinkingLevel
}

// CreateSession creates a new agent session with the given options.
func CreateSession(ctx context.Context, opts CreateSessionOptions) (*CreateSessionResult, error) {
	// Validate tool configuration
	if opts.NoTools && (len(opts.ToolList) > 0 || len(opts.ExcludeTools) > 0) {
		return nil, fmt.Errorf("CreateSessionOptions: NoTools cannot be used with ToolList or ExcludeTools")
	}
	if len(opts.ToolList) > 0 && len(opts.ExcludeTools) > 0 {
		return nil, fmt.Errorf("CreateSessionOptions: ToolList and ExcludeTools are mutually exclusive")
	}
	cwd := opts.CWD
	if cwd == "" {
		if wd, err := os.Getwd(); err == nil {
			cwd = wd
		}
	}

	agentDir := opts.AgentDir
	if agentDir == "" {
		agentDir = config.GetAgentDir()
	}

	// Initialize auth storage
	authStorage := opts.AuthStorage
	if authStorage == nil {
		authPath := config.GetAuthPath()
		backend := auth.NewFileBackend(authPath)
		authStorage = auth.NewStorage(backend)
	}

	// Initialize settings
	settingsMgr := opts.SettingsMgr
	if settingsMgr == nil {
		settingsMgr = settings.Create(cwd, agentDir)
	}

	// Initialize model registry
	modelReg := opts.ModelRegistry
	if modelReg == nil {
		modelReg = models.NewRegistry(authStorage, config.GetModelsPath())
	}

	// Load resources
	resLoader := opts.ResourceLoader
	if resLoader == nil {
		resLoader = resources.NewLoader(resources.LoaderOptions{
			CWD:      cwd,
			AgentDir: agentDir,
		})
	}
	loadedResources, err := resLoader.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load resources: %w", err)
	}

	// Resolve model
	model := opts.Model
	var modelFallbackMessage string

	if model == nil {
		defaultProvider := settingsMgr.GetDefaultProvider()
		defaultModelID := settingsMgr.GetDefaultModel()
		if defaultProvider != "" && defaultModelID != "" {
			model = modelReg.Find(defaultProvider, defaultModelID)
		}
		if model == nil {
			available := modelReg.GetAvailable()
			if len(available) > 0 {
				model = available[0]
			}
		}
		if model == nil {
			modelFallbackMessage = "No models available. Configure API keys or models.json."
		}
	}

	// Resolve thinking level
	thinkingLevel := opts.ThinkingLevel
	if thinkingLevel == "" {
		thinkingLevel = ai.ModelThinkingLevel(settingsMgr.GetDefaultThinkingLevel())
	}
	if thinkingLevel == "" {
		thinkingLevel = ai.ModelThinkingLevel(config.DefaultThinkingLevel)
	}
	if model != nil {
		thinkingLevel = ai.ClampThinkingLevel(model, thinkingLevel)
	} else {
		thinkingLevel = ai.ThinkingOff
	}

	// Build tool set
	toolConfigs := resolveTools(opts, settingsMgr)

	// Build system prompt
	sysPrompt := buildSystemPromptFromResources(cwd, loadedResources, toolConfigs)

	// Create session storage
	sessDir := opts.SessionDir
	if sessDir == "" {
		sessDir = config.GetSessionsDir()
	}
	localFs := env.NewLocalEnv(sessDir)
	sessionID := generateSessionID()
	jsonlStorage, err := session.CreateJsonlSession(ctx, localFs, sessDir+"/session.jsonl", cwd, sessionID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create session storage: %w", err)
	}
	sess := session.NewSession(jsonlStorage)

	// Build harness options
	harnessOpts := harness.HarnessOptions{
		Env:             localFs,
		Model:           model,
		ThinkingLevel:   string(thinkingLevel),
		SystemPrompt:    sysPrompt,
		Tools:           toolConfigs.Tools,
		ActiveToolNames: toolConfigs.ActiveNames,
		SteeringMode:    agent.QueueAll,
		FollowUpMode:    agent.QueueAll,
		GetApiKeyAndHeaders: func(m *ai.Model) (*harness.AuthInfo, error) {
			return resolveAuth(modelReg, m)
		},
		CompactFn: func(ctx context.Context, preparation any, m *ai.Model, apiKey string, headers map[string]string, customInstructions string, tl string) (any, error) {
			prep := preparation.(*harness.CompactionPreparation)
			return compaction.Compact(ctx, *prep, *m, apiKey, headers, customInstructions, tl)
		},
		PrepareCompactionFn: func(entries []harness.SessionTreeEntry, settings any) (any, error) {
			return compaction.PrepareCompaction(entries, settings.(harness.CompactionSettings))
		},
		DefaultCompactionSettingsFn: func() any {
			return compactionSettingsFromManager(settingsMgr)
		},
	}

	h := harness.NewAgentHarness(harnessOpts, sess)

	// Save initial model and thinking level
	if model != nil {
		sess.AppendModelChange(ctx, model.Provider, model.ID)
	}
	sess.AppendThinkingLevelChange(ctx, string(thinkingLevel))

	as := &AgentSession{
		harness:          h,
		sess:             sess,
		modelReg:         modelReg,
		settingsMgr:      settingsMgr,
		resLoader:        resLoader,
		cwd:              cwd,
		agentDir:         agentDir,
		model:            model,
		thinkingLevel:    thinkingLevel,
		baseSystemPrompt: sysPrompt,
		startedAt:        time.Now(),
	}

	return &CreateSessionResult{
		Session:              as,
		ModelFallbackMessage: modelFallbackMessage,
	}, nil
}

// ============================================================================
// Session Operations
// ============================================================================

// Prompt sends a user message to the agent and runs the agent loop.
func (s *AgentSession) Prompt(ctx context.Context, text string, images []ai.ContentBlock) (*ai.Message, error) {
	loadedRes, _ := s.resLoader.Load()
	expanded := prompt.ExpandPromptTemplate(text, loadedRes.Prompts)
	return s.harness.Prompt(ctx, expanded, images)
}

// Steer queues a steering message during streaming.
func (s *AgentSession) Steer(ctx context.Context, text string, images []ai.ContentBlock) error {
	loadedRes, _ := s.resLoader.Load()
	expanded := prompt.ExpandPromptTemplate(text, loadedRes.Prompts)
	return s.harness.Steer(ctx, expanded, images)
}

// FollowUp queues a follow-up message for after the agent finishes.
func (s *AgentSession) FollowUp(ctx context.Context, text string, images []ai.ContentBlock) error {
	loadedRes, _ := s.resLoader.Load()
	expanded := prompt.ExpandPromptTemplate(text, loadedRes.Prompts)
	return s.harness.FollowUp(ctx, expanded, images)
}

// NextTurn queues a message for the next turn.
func (s *AgentSession) NextTurn(ctx context.Context, text string, images []ai.ContentBlock) error {
	return s.harness.NextTurn(ctx, text, images)
}

// Abort cancels the current agent operation.
func (s *AgentSession) Abort(ctx context.Context) error {
	_, err := s.harness.Abort(ctx)
	return err
}

// WaitForIdle blocks until the harness is idle.
func (s *AgentSession) WaitForIdle() {
	s.harness.WaitForIdle()
}

// Subscribe registers a session event listener.
func (s *AgentSession) Subscribe(listener SessionEventListener) func() {
	s.mu.Lock()
	defer s.mu.Unlock()
	listenerPtr := &listener
	s.listenerPtrs = append(s.listenerPtrs, listenerPtr)
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		for i, p := range s.listenerPtrs {
			if p == listenerPtr {
				s.listenerPtrs = append(s.listenerPtrs[:i], s.listenerPtrs[i+1:]...)
				break
			}
		}
	}
}

// Dispose cleans up all session resources.
func (s *AgentSession) Dispose(ctx context.Context) {
	s.Abort(ctx)
	s.harness.WaitForIdle()
	s.mu.Lock()
	s.listenerPtrs = nil
	s.mu.Unlock()
}

// ============================================================================
// State Access
// ============================================================================

// Model returns the current model.
func (s *AgentSession) Model() *ai.Model {
	return s.model
}

// ThinkingLevel returns the current thinking level.
func (s *AgentSession) ThinkingLevel() ai.ModelThinkingLevel {
	return s.thinkingLevel
}

// IsStreaming returns whether the agent is currently processing.
func (s *AgentSession) IsStreaming() bool {
	return s.harness.GetPhase() == harness.PhaseTurn
}

// SystemPrompt returns the current system prompt.
func (s *AgentSession) SystemPrompt() string {
	return s.baseSystemPrompt
}

// CWD returns the working directory.
func (s *AgentSession) CWD() string {
	return s.cwd
}

// ModelRegistry returns the model registry.
func (s *AgentSession) ModelRegistry() *models.Registry {
	return s.modelReg
}

// Settings returns the settings manager.
func (s *AgentSession) Settings() *settings.Manager {
	return s.settingsMgr
}

// Session returns the underlying session.
// Deprecated: This exposes internal implementation details. File a feature request
// if you need session-level access through the SDK.
func (s *AgentSession) Session() *session.Session {
	return s.sess
}

// Harness returns the underlying agent harness.
// Deprecated: This exposes internal implementation details. File a feature request
// if you need harness-level access through the SDK.
func (s *AgentSession) Harness() *harness.AgentHarness {
	return s.harness
}

// GetActiveToolNames returns currently active tool names.
func (s *AgentSession) GetActiveToolNames() []string {
	tools := s.harness.GetActiveTools()
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name
	}
	return names
}

// SetActiveToolsByName enables only the named tools.
func (s *AgentSession) SetActiveToolsByName(ctx context.Context, names []string) error {
	return s.harness.SetActiveTools(ctx, names)
}

// GetSkills returns the loaded skills.
func (s *AgentSession) GetSkills() []sdkskills.Skill {
	res, _ := s.resLoader.Load()
	return res.Skills
}

// GetContextFiles returns loaded context files (AGENTS.md / CLAUDE.md).
func (s *AgentSession) GetContextFiles() []resources.ContextFile {
	res, _ := s.resLoader.Load()
	return res.ContextFiles
}

// GetPromptTemplates returns loaded prompt templates.
func (s *AgentSession) GetPromptTemplates() []resources.PromptTemplate {
	res, _ := s.resLoader.Load()
	return res.Prompts
}

// ============================================================================
// Model Management
// ============================================================================

// SetModel changes the active model.
func (s *AgentSession) SetModel(ctx context.Context, model *ai.Model) error {
	if !s.modelReg.HasConfiguredAuth(model) {
		return fmt.Errorf("no API key for %s/%s", model.Provider, model.ID)
	}

	s.mu.Lock()
	s.model = model
	s.mu.Unlock()

	tl := ai.ClampThinkingLevel(model, s.thinkingLevel)
	s.thinkingLevel = tl

	if err := s.harness.SetModel(ctx, model); err != nil {
		return err
	}
	s.harness.SetThinkingLevel(ctx, string(tl))
	s.sess.AppendModelChange(ctx, model.Provider, model.ID)
	s.settingsMgr.SetDefaultModelAndProvider(model.Provider, model.ID)

	return nil
}

// CycleModel cycles to the next or previous available model.
func (s *AgentSession) CycleModel(ctx context.Context, direction string) (*ModelCycleResult, error) {
	if len(s.scopedModels) > 0 {
		return s.cycleScopedModel(direction)
	}

	available := s.modelReg.GetAvailable()
	if len(available) <= 1 {
		return nil, nil
	}

	currentModel := s.model
	idx := -1
	for i, m := range available {
		if m.Provider == currentModel.Provider && m.ID == currentModel.ID {
			idx = i
			break
		}
	}
	if idx == -1 {
		idx = 0
	}

	var nextIdx int
	if direction == "backward" {
		nextIdx = (idx - 1 + len(available)) % len(available)
	} else {
		nextIdx = (idx + 1) % len(available)
	}

	nextModel := available[nextIdx]
	if err := s.SetModel(ctx, nextModel); err != nil {
		return nil, err
	}

	return &ModelCycleResult{
		Model:         nextModel,
		ThinkingLevel: s.thinkingLevel,
		IsScoped:      false,
	}, nil
}

func (s *AgentSession) cycleScopedModel(direction string) (*ModelCycleResult, error) {
	filtered := make([]ScopedModelEntry, 0, len(s.scopedModels))
	for _, se := range s.scopedModels {
		if s.modelReg.HasConfiguredAuth(se.Model) {
			filtered = append(filtered, se)
		}
	}
	if len(filtered) <= 1 {
		return nil, nil
	}

	idx := 0
	for i, se := range filtered {
		if se.Model.Provider == s.model.Provider && se.Model.ID == s.model.ID {
			idx = i
			break
		}
	}

	var nextIdx int
	if direction == "backward" {
		nextIdx = (idx - 1 + len(filtered)) % len(filtered)
	} else {
		nextIdx = (idx + 1) % len(filtered)
	}

	next := filtered[nextIdx]
	return &ModelCycleResult{
		Model:         next.Model,
		ThinkingLevel: s.thinkingLevel,
		IsScoped:      true,
	}, nil
}

// SetScopedModels configures models for cycling.
// SetScopedModels configures models for cycling.
func (s *AgentSession) SetScopedModels(entries []ScopedModelEntry) {
	s.scopedModels = entries
}

// ============================================================================
// Thinking Level Management
// ============================================================================

// SetThinkingLevel changes the thinking level, clamped to model capabilities.
func (s *AgentSession) SetThinkingLevel(ctx context.Context, level ai.ModelThinkingLevel) {
	effective := ai.ClampThinkingLevel(s.model, level)
	s.mu.Lock()
	s.thinkingLevel = effective
	s.mu.Unlock()
	s.harness.SetThinkingLevel(ctx, string(effective))
	s.sess.AppendThinkingLevelChange(ctx, string(effective))
	s.settingsMgr.SetDefaultThinkingLevel(string(effective))
}

// CycleThinkingLevel cycles to the next available thinking level for the current model.
// Returns empty string if the model does not support reasoning.
func (s *AgentSession) CycleThinkingLevel(ctx context.Context) ai.ModelThinkingLevel {
	if s.model == nil || !s.model.Reasoning {
		return ""
	}

	levels := ai.GetSupportedThinkingLevels(s.model)
	if len(levels) == 0 {
		return ""
	}

	idx := 0
	for i, l := range levels {
		if l == s.thinkingLevel {
			idx = i
			break
		}
	}
	next := levels[(idx+1)%len(levels)]
	s.SetThinkingLevel(ctx, next)
	return next
}

// ============================================================================
// Compaction
// ============================================================================

// CompactionResult holds the output of a compaction operation.
type CompactionResult struct {
	Summary          string `json:"summary"`
	FirstKeptEntryID string `json:"firstKeptEntryId"`
	TokensBefore     int    `json:"tokensBefore"`
}

// Compact manually compacts the session context.
func (s *AgentSession) Compact(ctx context.Context, customInstructions string) (*CompactionResult, error) {
	res, err := s.harness.Compact(ctx, customInstructions)
	if err != nil {
		return nil, err
	}
	return &CompactionResult{
		Summary:          res.Summary,
		FirstKeptEntryID: res.FirstKeptEntryID,
		TokensBefore:     res.TokensBefore,
	}, nil
}

// ============================================================================
// Session Stats
// ============================================================================

// GetSessionStats returns statistics about the current session.
func (s *AgentSession) GetSessionStats(ctx context.Context) (*SessionStats, error) {
	sessCtx, err := s.sess.BuildContext(ctx)
	if err != nil {
		return nil, err
	}

	stats := &SessionStats{
		StartedAt: s.startedAt,
	}
	meta, err := s.sess.GetMetadata(ctx)
	if err == nil {
		stats.SessionID = meta.ID
	}

	if sessCtx != nil && sessCtx.Messages != nil {
		for _, msg := range sessCtx.Messages {
			switch msg.Role {
			case "user":
				stats.UserMessages++
			case "assistant":
				stats.AssistantMessages++
				stats.InputTokens += int64(msg.Usage.Input)
				stats.OutputTokens += int64(msg.Usage.Output)
				stats.Cost += msg.Usage.Cost.Total
				if msg.AssistantContent != nil {
					for _, block := range msg.AssistantContent {
						if block.Type == "toolCall" {
							stats.ToolCalls++
						}
					}
				}
			case "toolResult":
				stats.ToolResults++
			}
		}
		if stats.SessionID == "" {
			stats.TotalMessages = len(sessCtx.Messages)
		}
	}

	return stats, nil
}

// GetLastAssistantText returns the text of the last assistant message.
func (s *AgentSession) GetLastAssistantText(ctx context.Context) string {
	sessCtx, err := s.sess.BuildContext(ctx)
	if err != nil || sessCtx == nil || sessCtx.Messages == nil {
		return ""
	}

	msgs := sessCtx.Messages
	for i := len(msgs) - 1; i >= 0; i-- {
		msg := msgs[i]
		if msg.Role != "assistant" {
			continue
		}
		if msg.StopReason == ai.StopReasonAborted && len(msg.AssistantContent) == 0 {
			continue
		}
		var sb strings.Builder
		if msg.AssistantContent != nil {
			for _, block := range msg.AssistantContent {
				if block.Type == "text" {
					sb.WriteString(block.Text)
				}
			}
		}
		return strings.TrimSpace(sb.String())
	}
	return ""
}

// ============================================================================
// Harness Events
// ============================================================================

// SessionEventType identifies the type of a session event.
type SessionEventType string

const (
	EventTypeQueueUpdate       SessionEventType = "queue_update"
	EventTypeSavePoint         SessionEventType = "save_point"
	EventTypeAbort             SessionEventType = "abort"
	EventTypeSettled           SessionEventType = "settled"
	EventTypeBeforeAgentStart  SessionEventType = "before_agent_start"
	EventTypeContext           SessionEventType = "context"
	EventTypeBeforeRequest     SessionEventType = "before_provider_request"
	EventTypeBeforePayload     SessionEventType = "before_provider_payload"
	EventTypeAfterResponse    SessionEventType = "after_provider_response"
	EventTypeToolCall         SessionEventType = "tool_call"
	EventTypeToolResult       SessionEventType = "tool_result"
	EventTypeModelUpdate      SessionEventType = "model_update"
	EventTypeThinkingUpdate   SessionEventType = "thinking_level_update"
	EventTypeToolsUpdate      SessionEventType = "tools_update"
	EventTypeResourcesUpdate  SessionEventType = "resources_update"
	EventTypeBeforeCompact    SessionEventType = "session_before_compact"
	EventTypeCompact          SessionEventType = "session_compact"
	EventTypeBeforeTree       SessionEventType = "session_before_tree"
	EventTypeTree             SessionEventType = "session_tree"
)

// SessionEventHandler processes a session event.
// Return nil to continue, or a hook result to modify behavior.
type SessionEventHandler func(event SessionEvent) (any, error)

// On registers a handler for a specific session event type.
// This is an advanced API for fine-grained event handling.
func (s *AgentSession) On(eventType string, handler SessionEventHandler) {
	s.harness.On(eventType, func(event harness.HarnessEvent) (any, error) {
		return handler(sdkEventFromHarness(event))
	})
}

func sdkEventFromHarness(he harness.HarnessEvent) SessionEvent {
	return SessionEvent{
		Type:         he.Type,
		ToolName:     he.ToolName,
		ToolCallID:   he.ToolCallID,
		ErrorMessage: "",
		Model:        he.Model,
		Reason:       "",
		Steering:     nil,
		FollowUp:     nil,
	}
}

// ============================================================================
// Internal helpers
// ============================================================================

type toolResolution struct {
	Tools       []agent.Tool
	ActiveNames []string
}

func resolveTools(opts CreateSessionOptions, mgr *settings.Manager) *toolResolution {
	if opts.NoTools {
		return &toolResolution{}
	}

	// Create all coding tools
	cwd := opts.CWD
	allToolsMap := sdktools.CreateAllTools(cwd, &sdktools.ToolOptions{
		Bash: &sdktools.BashToolOptions{
			CommandPrefix: mgr.GetShellCommandPrefix(),
			ShellPath:     mgr.GetShellPath(),
		},
	})

	// Convert map to slice
	var allTools []agent.Tool
	names := make([]string, 0, len(allToolsMap))
	for name, t := range allToolsMap {
		allTools = append(allTools, *t)
		names = append(names, string(name))
	}

	// Apply allowlist
	if len(opts.ToolList) > 0 {
		filtered := make([]agent.Tool, 0, len(opts.ToolList))
		filteredNames := make([]string, 0, len(opts.ToolList))
		toolMap := make(map[string]agent.Tool)
		for _, t := range allTools {
			toolMap[t.Name] = t
		}
		for _, name := range opts.ToolList {
			if t, ok := toolMap[name]; ok {
				filtered = append(filtered, t)
				filteredNames = append(filteredNames, name)
			}
		}
		return &toolResolution{Tools: filtered, ActiveNames: filteredNames}
	}

	// Apply denylist
	if len(opts.ExcludeTools) > 0 {
		exclude := make(map[string]bool)
		for _, name := range opts.ExcludeTools {
			exclude[name] = true
		}
		filtered := make([]agent.Tool, 0, len(allTools))
		filteredNames := make([]string, 0, len(allTools))
		for _, t := range allTools {
			if !exclude[t.Name] {
				filtered = append(filtered, t)
				for _, n := range names {
					if n == t.Name {
						filteredNames = append(filteredNames, n)
						break
					}
				}
			}
		}
		return &toolResolution{Tools: filtered, ActiveNames: filteredNames}
	}

	return &toolResolution{Tools: allTools, ActiveNames: names}
}

func buildSystemPromptFromResources(cwd string, loadedRes *resources.LoadedResources, toolRes *toolResolution) string {
	// Build tool snippets from tool definitions
	snippets := make(map[string]string)
	for _, name := range toolRes.ActiveNames {
		for _, t := range toolRes.Tools {
			if t.Name == name {
				snippets[name] = t.Description
				break
			}
		}
	}

	appendText := ""
	if len(loadedRes.AppendSystemPrompt) > 0 {
		appendText = strings.Join(loadedRes.AppendSystemPrompt, "\n\n")
	}

	var customPrompt string
	if loadedRes.SystemPrompt != nil {
		customPrompt = *loadedRes.SystemPrompt
	}

	return prompt.BuildSystemPrompt(prompt.BuildSystemPromptOptions{
		CWD:                cwd,
		CustomPrompt:       customPrompt,
		SelectedTools:      toolRes.ActiveNames,
		ToolSnippets:       snippets,
		AppendSystemPrompt: appendText,
		ContextFiles:       loadedRes.ContextFiles,
		Skills:             loadedRes.Skills,
	})
}

func resolveAuth(modelReg *models.Registry, model *ai.Model) (*harness.AuthInfo, error) {
	result := modelReg.GetAPIKeyAndHeaders(model)
	if !result.OK {
		return nil, fmt.Errorf("failed to resolve API key for %s/%s: %s", model.Provider, model.ID, result.Error)
	}
	return &harness.AuthInfo{
		APIKey:  result.APIKey,
		Headers: result.Headers,
	}, nil
}

func compactionSettingsFromManager(mgr *settings.Manager) harness.CompactionSettings {
	return harness.CompactionSettings{
		Enabled:          mgr.GetCompactionEnabled(),
		ReserveTokens:    mgr.GetCompactionReserveTokens(),
		KeepRecentTokens: mgr.GetCompactionKeepRecentTokens(),
	}
}

func generateSessionID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based if crypto/rand fails
		for i := range b {
			b[i] = byte(time.Now().UnixNano())
		}
	}
	return fmt.Sprintf("sess_%d_%x", time.Now().UnixMilli(), b)
}
