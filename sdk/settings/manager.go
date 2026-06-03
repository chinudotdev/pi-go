// Package settings manages application settings from global and project-level
// settings.json files, with deep merge support.
package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/chinudotdev/pi-go/ai"
	"github.com/chinudotdev/pi-go/sdk/config"
	"github.com/chinudotdev/pi-go/sdk/internal/paths"
)

// ============================================================================
// Types
// ============================================================================

// Scope indicates whether settings are global or project-scoped.
type Scope string

const (
	ScopeGlobal  Scope = "global"
	ScopeProject Scope = "project"
)

// CompactionSettings controls context compaction behavior.
type CompactionSettings struct {
	Enabled         *bool `json:"enabled,omitempty"`
	ReserveTokens   *int  `json:"reserveTokens,omitempty"`
	KeepRecentTokens *int `json:"keepRecentTokens,omitempty"`
}

// BranchSummarySettings controls branch summarization.
type BranchSummarySettings struct {
	ReserveTokens *int  `json:"reserveTokens,omitempty"`
	SkipPrompt    *bool `json:"skipPrompt,omitempty"`
}

// ProviderRetrySettings controls provider-level retry behavior.
type ProviderRetrySettings struct {
	TimeoutMs      *int `json:"timeoutMs,omitempty"`
	MaxRetries     *int `json:"maxRetries,omitempty"`
	MaxRetryDelayMs *int `json:"maxRetryDelayMs,omitempty"`
}

// RetrySettings controls auto-retry behavior.
type RetrySettings struct {
	Enabled    *bool                 `json:"enabled,omitempty"`
	MaxRetries *int                  `json:"maxRetries,omitempty"`
	BaseDelayMs *int                 `json:"baseDelayMs,omitempty"`
	Provider   *ProviderRetrySettings `json:"provider,omitempty"`
}

// TerminalSettings controls terminal display options.
type TerminalSettings struct {
	ShowImages          *bool `json:"showImages,omitempty"`
	ImageWidthCells     *int  `json:"imageWidthCells,omitempty"`
	ClearOnShrink       *bool `json:"clearOnShrink,omitempty"`
	ShowTerminalProgress *bool `json:"showTerminalProgress,omitempty"`
}

// ImageSettings controls image handling.
type ImageSettings struct {
	AutoResize  *bool `json:"autoResize,omitempty"`
	BlockImages *bool `json:"blockImages,omitempty"`
}

// ThinkingBudgetsSettings configures token budgets per thinking level.
type ThinkingBudgetsSettings struct {
	Minimal *int `json:"minimal,omitempty"`
	Low     *int `json:"low,omitempty"`
	Medium  *int `json:"medium,omitempty"`
	High    *int `json:"high,omitempty"`
}

// MarkdownSettings controls markdown rendering.
type MarkdownSettings struct {
	CodeBlockIndent *string `json:"codeBlockIndent,omitempty"`
}

// WarningSettings controls warning display.
type WarningSettings struct {
	AnthropicExtraUsage *bool `json:"anthropicExtraUsage,omitempty"`
}

// Settings is the full settings structure.
type Settings struct {
	LastChangelogVersion      *string                  `json:"lastChangelogVersion,omitempty"`
	DefaultProvider           *string                  `json:"defaultProvider,omitempty"`
	DefaultModel              *string                  `json:"defaultModel,omitempty"`
	DefaultThinkingLevel      *string                  `json:"defaultThinkingLevel,omitempty"`
	Transport                 *ai.Transport            `json:"transport,omitempty"`
	SteeringMode              *string                  `json:"steeringMode,omitempty"`
	FollowUpMode              *string                  `json:"followUpMode,omitempty"`
	Theme                     *string                  `json:"theme,omitempty"`
	Compaction                *CompactionSettings      `json:"compaction,omitempty"`
	BranchSummary             *BranchSummarySettings   `json:"branchSummary,omitempty"`
	Retry                     *RetrySettings           `json:"retry,omitempty"`
	HideThinkingBlock         *bool                    `json:"hideThinkingBlock,omitempty"`
	ShellPath                 *string                  `json:"shellPath,omitempty"`
	QuietStartup              *bool                    `json:"quietStartup,omitempty"`
	ShellCommandPrefix        *string                  `json:"shellCommandPrefix,omitempty"`
	CollapseChangelog         *bool                    `json:"collapseChangelog,omitempty"`
	EnableInstallTelemetry    *bool                    `json:"enableInstallTelemetry,omitempty"`
	Skills                    []string                 `json:"skills,omitempty"`
	Prompts                   []string                 `json:"prompts,omitempty"`
	Extensions                []string                 `json:"extensions,omitempty"`
	EnableSkillCommands       *bool                    `json:"enableSkillCommands,omitempty"`
	Terminal                  *TerminalSettings        `json:"terminal,omitempty"`
	Images                    *ImageSettings           `json:"images,omitempty"`
	EnabledModels             []string                 `json:"enabledModels,omitempty"`
	DoubleEscapeAction        *string                  `json:"doubleEscapeAction,omitempty"`
	TreeFilterMode            *string                  `json:"treeFilterMode,omitempty"`
	ThinkingBudgets           *ThinkingBudgetsSettings `json:"thinkingBudgets,omitempty"`
	EditorPaddingX            *int                     `json:"editorPaddingX,omitempty"`
	AutocompleteMaxVisible    *int                     `json:"autocompleteMaxVisible,omitempty"`
	Markdown                  *MarkdownSettings        `json:"markdown,omitempty"`
	Warnings                  *WarningSettings         `json:"warnings,omitempty"`
	SessionDir                *string                  `json:"sessionDir,omitempty"`
	HTTPIdleTimeoutMs         *int                     `json:"httpIdleTimeoutMs,omitempty"`
	WebSocketConnectTimeoutMs *int                     `json:"websocketConnectTimeoutMs,omitempty"`
}

// Error wraps a settings-related error with scope info.
type Error struct {
	Scope Scope
	Err   error
}

func (e *Error) Error() string { return fmt.Sprintf("settings (%s): %v", e.Scope, e.Err) }

// ============================================================================
// Storage interface
// ============================================================================

// StorageBackend provides read/write access to settings files.
type StorageBackend interface {
	WithLock(scope Scope, fn func(current []byte) *[]byte)
}

// ============================================================================
// File storage backend
// ============================================================================

// FileStorageBackend stores settings in global and project JSON files.
type FileStorageBackend struct {
	globalPath  string
	projectPath string
	mu          sync.Mutex
}

// NewFileStorageBackend creates a file-backed settings storage.
func NewFileStorageBackend(cwd string, agentDir string) *FileStorageBackend {
	resolvedCwd := paths.ResolvePath(cwd, cwd)
	resolvedAgentDir := paths.ResolvePath(agentDir, agentDir)
	return &FileStorageBackend{
		globalPath:  filepath.Join(resolvedAgentDir, "settings.json"),
		projectPath: filepath.Join(resolvedCwd, config.ConfigDirName, "settings.json"),
	}
}

func (fsb *FileStorageBackend) WithLock(scope Scope, fn func(current []byte) *[]byte) {
	fsb.mu.Lock()
	defer fsb.mu.Unlock()

	path := fsb.globalPath
	if scope == ScopeProject {
		path = fsb.projectPath
	}

	var current []byte
	if data, err := os.ReadFile(path); err == nil {
		current = data
	}

	result := fn(current)
	if result != nil {
		dir := filepath.Dir(path)
		os.MkdirAll(dir, 0o755)
		os.WriteFile(path, *result, 0o644)
	}
}

// ============================================================================
// In-memory storage backend
// ============================================================================

// MemoryStorageBackend stores settings in memory.
type MemoryStorageBackend struct {
	mu      sync.Mutex
	global  []byte
	project []byte
}

// NewMemoryStorageBackend creates an in-memory settings backend.
func NewMemoryStorageBackend() *MemoryStorageBackend {
	return &MemoryStorageBackend{}
}

func (msb *MemoryStorageBackend) WithLock(scope Scope, fn func(current []byte) *[]byte) {
	msb.mu.Lock()
	defer msb.mu.Unlock()

	var current []byte
	if scope == ScopeGlobal {
		current = msb.global
	} else {
		current = msb.project
	}

	result := fn(current)
	if result != nil {
		if scope == ScopeGlobal {
			msb.global = *result
		} else {
			msb.project = *result
		}
	}
}

// ============================================================================
// Manager
// ============================================================================

// Manager loads, merges, and persists settings from global + project scopes.
type Manager struct {
	storage          StorageBackend
	globalSettings   Settings
	projectSettings  Settings
	settings         Settings // merged result
	modifiedFields   map[string]bool
	errors           []Error
	mu               sync.RWMutex
}

// NewManager creates a Manager from a storage backend.
func NewManager(storage StorageBackend) *Manager {
	m := &Manager{
		storage:        storage,
		modifiedFields: make(map[string]bool),
	}
	m.loadFromStorage()
	return m
}

// Create creates a Manager that loads from files.
func Create(cwd string, agentDir ...string) *Manager {
	ad := config.GetAgentDir()
	if len(agentDir) > 0 && agentDir[0] != "" {
		ad = agentDir[0]
	}
	return NewManager(NewFileStorageBackend(cwd, ad))
}

// InMemory creates an in-memory Manager with optional initial settings.
func InMemory(initial ...Settings) *Manager {
	backend := NewMemoryStorageBackend()
	if len(initial) > 0 {
		data, _ := json.MarshalIndent(initial[0], "", "  ")
		backend.WithLock(ScopeGlobal, func(current []byte) *[]byte {
			return &data
		})
	}
	return NewManager(backend)
}

func (m *Manager) loadFromStorage() {
	globalSettings, globalErr := loadScope(m.storage, ScopeGlobal)
	projectSettings, projectErr := loadScope(m.storage, ScopeProject)

	if globalErr != nil {
		m.errors = append(m.errors, Error{Scope: ScopeGlobal, Err: globalErr})
	}
	if projectErr != nil {
		m.errors = append(m.errors, Error{Scope: ScopeProject, Err: projectErr})
	}

	m.globalSettings = globalSettings
	m.projectSettings = projectSettings
	m.settings = deepMerge(m.globalSettings, m.projectSettings)
}

func loadScope(storage StorageBackend, scope Scope) (Settings, error) {
	var content []byte
	storage.WithLock(scope, func(current []byte) *[]byte {
		content = current
		return nil
	})

	if content == nil {
		return Settings{}, nil
	}

	var s Settings
	if err := json.Unmarshal(content, &s); err != nil {
		return Settings{}, err
	}
	return s, nil
}

// deepMerge merges override on top of base. Nested objects merge recursively;
// arrays and primitives are replaced.
func deepMerge(base, override Settings) Settings {
	result := base

	// Simple pointer fields: override wins if non-nil
	if override.LastChangelogVersion != nil {
		result.LastChangelogVersion = override.LastChangelogVersion
	}
	if override.DefaultProvider != nil {
		result.DefaultProvider = override.DefaultProvider
	}
	if override.DefaultModel != nil {
		result.DefaultModel = override.DefaultModel
	}
	if override.DefaultThinkingLevel != nil {
		result.DefaultThinkingLevel = override.DefaultThinkingLevel
	}
	if override.Transport != nil {
		result.Transport = override.Transport
	}
	if override.SteeringMode != nil {
		result.SteeringMode = override.SteeringMode
	}
	if override.FollowUpMode != nil {
		result.FollowUpMode = override.FollowUpMode
	}
	if override.Theme != nil {
		result.Theme = override.Theme
	}
	if override.HideThinkingBlock != nil {
		result.HideThinkingBlock = override.HideThinkingBlock
	}
	if override.ShellPath != nil {
		result.ShellPath = override.ShellPath
	}
	if override.QuietStartup != nil {
		result.QuietStartup = override.QuietStartup
	}
	if override.ShellCommandPrefix != nil {
		result.ShellCommandPrefix = override.ShellCommandPrefix
	}
	if override.CollapseChangelog != nil {
		result.CollapseChangelog = override.CollapseChangelog
	}
	if override.EnableInstallTelemetry != nil {
		result.EnableInstallTelemetry = override.EnableInstallTelemetry
	}
	if override.EnableSkillCommands != nil {
		result.EnableSkillCommands = override.EnableSkillCommands
	}
	if override.DoubleEscapeAction != nil {
		result.DoubleEscapeAction = override.DoubleEscapeAction
	}
	if override.TreeFilterMode != nil {
		result.TreeFilterMode = override.TreeFilterMode
	}
	if override.EditorPaddingX != nil {
		result.EditorPaddingX = override.EditorPaddingX
	}
	if override.AutocompleteMaxVisible != nil {
		result.AutocompleteMaxVisible = override.AutocompleteMaxVisible
	}
	if override.SessionDir != nil {
		result.SessionDir = override.SessionDir
	}
	if override.HTTPIdleTimeoutMs != nil {
		result.HTTPIdleTimeoutMs = override.HTTPIdleTimeoutMs
	}
	if override.WebSocketConnectTimeoutMs != nil {
		result.WebSocketConnectTimeoutMs = override.WebSocketConnectTimeoutMs
	}

	// Slice fields
	if override.Skills != nil {
		result.Skills = override.Skills
	}
	if override.Prompts != nil {
		result.Prompts = override.Prompts
	}
	if override.Extensions != nil {
		result.Extensions = override.Extensions
	}
	if override.EnabledModels != nil {
		result.EnabledModels = override.EnabledModels
	}

	// Nested structs: merge if both exist
	if override.Compaction != nil {
		if result.Compaction == nil {
			result.Compaction = &CompactionSettings{}
		}
		mergeCompaction(result.Compaction, override.Compaction)
	}
	if override.BranchSummary != nil {
		if result.BranchSummary == nil {
			result.BranchSummary = &BranchSummarySettings{}
		}
		mergeBranchSummary(result.BranchSummary, override.BranchSummary)
	}
	if override.Retry != nil {
		if result.Retry == nil {
			result.Retry = &RetrySettings{}
		}
		mergeRetry(result.Retry, override.Retry)
	}
	if override.Terminal != nil {
		if result.Terminal == nil {
			result.Terminal = &TerminalSettings{}
		}
		mergeTerminal(result.Terminal, override.Terminal)
	}
	if override.Images != nil {
		if result.Images == nil {
			result.Images = &ImageSettings{}
		}
		mergeImages(result.Images, override.Images)
	}
	if override.ThinkingBudgets != nil {
		result.ThinkingBudgets = override.ThinkingBudgets
	}
	if override.Markdown != nil {
		result.Markdown = override.Markdown
	}
	if override.Warnings != nil {
		result.Warnings = override.Warnings
	}

	return result
}

func ptrBool(v bool) *bool       { return &v }
func ptrInt(v int) *int          { return &v }
func ptrString(v string) *string { return &v }

func mergeCompaction(base, override *CompactionSettings) {
	if override.Enabled != nil {
		base.Enabled = override.Enabled
	}
	if override.ReserveTokens != nil {
		base.ReserveTokens = override.ReserveTokens
	}
	if override.KeepRecentTokens != nil {
		base.KeepRecentTokens = override.KeepRecentTokens
	}
}

func mergeBranchSummary(base, override *BranchSummarySettings) {
	if override.ReserveTokens != nil {
		base.ReserveTokens = override.ReserveTokens
	}
	if override.SkipPrompt != nil {
		base.SkipPrompt = override.SkipPrompt
	}
}

func mergeRetry(base, override *RetrySettings) {
	if override.Enabled != nil {
		base.Enabled = override.Enabled
	}
	if override.MaxRetries != nil {
		base.MaxRetries = override.MaxRetries
	}
	if override.BaseDelayMs != nil {
		base.BaseDelayMs = override.BaseDelayMs
	}
	if override.Provider != nil {
		if base.Provider == nil {
			base.Provider = &ProviderRetrySettings{}
		}
		if override.Provider.TimeoutMs != nil {
			base.Provider.TimeoutMs = override.Provider.TimeoutMs
		}
		if override.Provider.MaxRetries != nil {
			base.Provider.MaxRetries = override.Provider.MaxRetries
		}
		if override.Provider.MaxRetryDelayMs != nil {
			base.Provider.MaxRetryDelayMs = override.Provider.MaxRetryDelayMs
		}
	}
}

func mergeTerminal(base, override *TerminalSettings) {
	if override.ShowImages != nil {
		base.ShowImages = override.ShowImages
	}
	if override.ImageWidthCells != nil {
		base.ImageWidthCells = override.ImageWidthCells
	}
	if override.ClearOnShrink != nil {
		base.ClearOnShrink = override.ClearOnShrink
	}
	if override.ShowTerminalProgress != nil {
		base.ShowTerminalProgress = override.ShowTerminalProgress
	}
}

func mergeImages(base, override *ImageSettings) {
	if override.AutoResize != nil {
		base.AutoResize = override.AutoResize
	}
	if override.BlockImages != nil {
		base.BlockImages = override.BlockImages
	}
}

// ============================================================================
// Getters
// ============================================================================

func (m *Manager) get() Settings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.settings
}

// Reload re-reads settings from storage.
func (m *Manager) Reload() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.loadFromStorage()
	m.modifiedFields = make(map[string]bool)
}

// DrainErrors returns and clears accumulated errors.
func (m *Manager) DrainErrors() []Error {
	m.mu.Lock()
	defer m.mu.Unlock()
	errs := m.errors
	m.errors = nil
	return errs
}

// GetDefaultProvider returns the configured default provider.
func (m *Manager) GetDefaultProvider() string {
	if v := m.get().DefaultProvider; v != nil {
		return *v
	}
	return ""
}

// GetDefaultModel returns the configured default model ID.
func (m *Manager) GetDefaultModel() string {
	if v := m.get().DefaultModel; v != nil {
		return *v
	}
	return ""
}

// SetDefaultProvider persists the default provider.
func (m *Manager) SetDefaultProvider(provider string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.globalSettings.DefaultProvider = &provider
	m.modifiedFields["defaultProvider"] = true
	m.save()
}

// SetDefaultModel persists the default model.
func (m *Manager) SetDefaultModel(modelID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.globalSettings.DefaultModel = &modelID
	m.modifiedFields["defaultModel"] = true
	m.save()
}

// SetDefaultModelAndProvider persists both default provider and model.
func (m *Manager) SetDefaultModelAndProvider(provider, modelID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.globalSettings.DefaultProvider = &provider
	m.globalSettings.DefaultModel = &modelID
	m.modifiedFields["defaultProvider"] = true
	m.modifiedFields["defaultModel"] = true
	m.save()
}

// GetDefaultThinkingLevel returns the configured thinking level.
func (m *Manager) GetDefaultThinkingLevel() string {
	if v := m.get().DefaultThinkingLevel; v != nil {
		return *v
	}
	return ""
}

// SetDefaultThinkingLevel persists the thinking level.
func (m *Manager) SetDefaultThinkingLevel(level string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.globalSettings.DefaultThinkingLevel = &level
	m.modifiedFields["defaultThinkingLevel"] = true
	m.save()
}

// GetTransport returns the configured transport.
func (m *Manager) GetTransport() ai.Transport {
	if v := m.get().Transport; v != nil {
		return *v
	}
	return ai.TransportAuto
}

// GetSteeringMode returns the steering queue mode.
func (m *Manager) GetSteeringMode() string {
	if v := m.get().SteeringMode; v != nil && *v != "" {
		return *v
	}
	return "one-at-a-time"
}

// GetFollowUpMode returns the follow-up queue mode.
func (m *Manager) GetFollowUpMode() string {
	if v := m.get().FollowUpMode; v != nil && *v != "" {
		return *v
	}
	return "one-at-a-time"
}

// GetCompactionEnabled returns whether compaction is enabled.
func (m *Manager) GetCompactionEnabled() bool {
	if s := m.get().Compaction; s != nil && s.Enabled != nil {
		return *s.Enabled
	}
	return true
}

// GetCompactionReserveTokens returns the token reserve for compaction.
func (m *Manager) GetCompactionReserveTokens() int {
	if s := m.get().Compaction; s != nil && s.ReserveTokens != nil {
		return *s.ReserveTokens
	}
	return 16384
}

// GetCompactionKeepRecentTokens returns how many recent tokens to keep.
func (m *Manager) GetCompactionKeepRecentTokens() int {
	if s := m.get().Compaction; s != nil && s.KeepRecentTokens != nil {
		return *s.KeepRecentTokens
	}
	return 20000
}

// GetRetryEnabled returns whether auto-retry is enabled.
func (m *Manager) GetRetryEnabled() bool {
	if s := m.get().Retry; s != nil && s.Enabled != nil {
		return *s.Enabled
	}
	return true
}

// GetRetryMaxRetries returns the max retry attempts.
func (m *Manager) GetRetryMaxRetries() int {
	if s := m.get().Retry; s != nil && s.MaxRetries != nil {
		return *s.MaxRetries
	}
	return 3
}

// GetRetryBaseDelayMs returns the base delay for exponential backoff.
func (m *Manager) GetRetryBaseDelayMs() int {
	if s := m.get().Retry; s != nil && s.BaseDelayMs != nil {
		return *s.BaseDelayMs
	}
	return 2000
}

// GetProviderRetrySettings returns provider-level retry config.
func (m *Manager) GetProviderRetrySettings() (timeoutMs int, maxRetries int, maxRetryDelayMs int) {
	maxRetryDelayMs = 60000
	s := m.get()
	if s.Retry == nil || s.Retry.Provider == nil {
		return 0, 0, maxRetryDelayMs
	}
	if s.Retry.Provider.TimeoutMs != nil {
		timeoutMs = *s.Retry.Provider.TimeoutMs
	}
	if s.Retry.Provider.MaxRetries != nil {
		maxRetries = *s.Retry.Provider.MaxRetries
	}
	if s.Retry.Provider.MaxRetryDelayMs != nil {
		maxRetryDelayMs = *s.Retry.Provider.MaxRetryDelayMs
	}
	return
}

// GetHideThinkingBlock returns whether to hide thinking blocks.
func (m *Manager) GetHideThinkingBlock() bool {
	if v := m.get().HideThinkingBlock; v != nil {
		return *v
	}
	return false
}

// GetShellPath returns the custom shell path.
func (m *Manager) GetShellPath() string {
	if v := m.get().ShellPath; v != nil {
		return *v
	}
	return ""
}

// GetShellCommandPrefix returns the shell command prefix.
func (m *Manager) GetShellCommandPrefix() string {
	if v := m.get().ShellCommandPrefix; v != nil {
		return *v
	}
	return ""
}

// GetSkillPaths returns configured skill paths.
func (m *Manager) GetSkillPaths() []string {
	if v := m.get().Skills; v != nil {
		return append([]string{}, v...)
	}
	return nil
}

// GetPromptTemplatePaths returns configured prompt template paths.
func (m *Manager) GetPromptTemplatePaths() []string {
	if v := m.get().Prompts; v != nil {
		return append([]string{}, v...)
	}
	return nil
}

// GetSessionDir returns the custom session directory.
func (m *Manager) GetSessionDir() string {
	if v := m.get().SessionDir; v != nil {
		return paths.NormalizePath(*v)
	}
	return ""
}

// GetHTTPIdleTimeoutMs returns the HTTP idle timeout in ms.
func (m *Manager) GetHTTPIdleTimeoutMs() int {
	if v := m.get().HTTPIdleTimeoutMs; v != nil {
		return *v
	}
	return 300_000
}

// GetImageAutoResize returns whether image auto-resize is enabled.
func (m *Manager) GetImageAutoResize() bool {
	if s := m.get().Images; s != nil && s.AutoResize != nil {
		return *s.AutoResize
	}
	return true
}

// GetBlockImages returns whether images should be blocked.
func (m *Manager) GetBlockImages() bool {
	if s := m.get().Images; s != nil && s.BlockImages != nil {
		return *s.BlockImages
	}
	return false
}

// GetEnableSkillCommands returns whether skill commands are enabled.
func (m *Manager) GetEnableSkillCommands() bool {
	if v := m.get().EnableSkillCommands; v != nil {
		return *v
	}
	return true
}

// GetEnabledModels returns the enabled model patterns.
func (m *Manager) GetEnabledModels() []string {
	if v := m.get().EnabledModels; v != nil {
		return append([]string{}, v...)
	}
	return nil
}

// GetCodeBlockIndent returns the markdown code block indent string.
func (m *Manager) GetCodeBlockIndent() string {
	if s := m.get().Markdown; s != nil && s.CodeBlockIndent != nil {
		return *s.CodeBlockIndent
	}
	return "  "
}

// GetThinkingBudgets returns the configured thinking budgets.
func (m *Manager) GetThinkingBudgets() *ThinkingBudgetsSettings {
	return m.get().ThinkingBudgets
}

// GetTheme returns the configured theme name.
func (m *Manager) GetTheme() string {
	if v := m.get().Theme; v != nil {
		return *v
	}
	return ""
}

// GetDoubleEscapeAction returns the double-escape action.
func (m *Manager) GetDoubleEscapeAction() string {
	if v := m.get().DoubleEscapeAction; v != nil {
		return *v
	}
	return "tree"
}

// GetTreeFilterMode returns the tree filter mode.
func (m *Manager) GetTreeFilterMode() string {
	if v := m.get().TreeFilterMode; v != nil {
		return *v
	}
	return "default"
}

// GetWarnings returns warning settings.
func (m *Manager) GetWarnings() WarningSettings {
	if s := m.get().Warnings; s != nil {
		return *s
	}
	return WarningSettings{}
}

// ============================================================================
// Persistence
// ============================================================================

func (m *Manager) save() {
	m.settings = deepMerge(m.globalSettings, m.projectSettings)

	// Snapshot what to persist
	snapshot := m.globalSettings
	modifiedFields := make(map[string]bool)
	for k, v := range m.modifiedFields {
		modifiedFields[k] = v
	}

	go func() {
		m.storage.WithLock(ScopeGlobal, func(current []byte) *[]byte {
			var fileSettings Settings
			if current != nil {
				json.Unmarshal(current, &fileSettings)
			}

			// Only write modified fields
			merged := fileSettings
			if modifiedFields["defaultProvider"] {
				merged.DefaultProvider = snapshot.DefaultProvider
			}
			if modifiedFields["defaultModel"] {
				merged.DefaultModel = snapshot.DefaultModel
			}
			if modifiedFields["defaultThinkingLevel"] {
				merged.DefaultThinkingLevel = snapshot.DefaultThinkingLevel
			}
			if modifiedFields["steeringMode"] {
				merged.SteeringMode = snapshot.SteeringMode
			}
			if modifiedFields["followUpMode"] {
				merged.FollowUpMode = snapshot.FollowUpMode
			}
			if modifiedFields["hideThinkingBlock"] {
				merged.HideThinkingBlock = snapshot.HideThinkingBlock
			}
			if modifiedFields["shellPath"] {
				merged.ShellPath = snapshot.ShellPath
			}
			if modifiedFields["quietStartup"] {
				merged.QuietStartup = snapshot.QuietStartup
			}
			if modifiedFields["shellCommandPrefix"] {
				merged.ShellCommandPrefix = snapshot.ShellCommandPrefix
			}
			if modifiedFields["theme"] {
				merged.Theme = snapshot.Theme
			}
			if modifiedFields["lastChangelogVersion"] {
				merged.LastChangelogVersion = snapshot.LastChangelogVersion
			}
			if modifiedFields["enableSkillCommands"] {
				merged.EnableSkillCommands = snapshot.EnableSkillCommands
			}
			if modifiedFields["enabledModels"] {
				merged.EnabledModels = snapshot.EnabledModels
			}

			data, _ := json.MarshalIndent(merged, "", "  ")
			return &data
		})
	}()
}
