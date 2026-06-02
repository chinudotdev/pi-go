package harness

import (
	"context"
	"fmt"
	"time"

	"github.com/chinudotdev/pi-go/agent"
	"github.com/chinudotdev/pi-go/ai"
)

// ============================================================================
// Result type
// ============================================================================

// Result is a discriminated union representing a fallible operation.
// Expected failures are returned as Err != nil instead of panicking.
type Result[T any] struct {
	Value T
	Err   error
	OK    bool
}

// OkResult creates a successful Result.
func OkResult[T any](value T) Result[T] {
	return Result[T]{Value: value, OK: true}
}

// ErrResult creates a failed Result.
func ErrResult[T any](err error) Result[T] {
	return Result[T]{Err: err, OK: false}
}

// GetOrThrow returns the success value or panics with the error.
func GetOrThrow[T any](r Result[T]) T {
	if !r.OK {
		panic(r.Err)
	}
	return r.Value
}

// GetOrZero returns the success value or the zero value of T.
func GetOrZero[T any](r Result[T]) T {
	if !r.OK {
		var zero T
		return zero
	}
	return r.Value
}

// ============================================================================
// Typed errors
// ============================================================================

// FileKind describes the type of a filesystem object.
type FileKind string

const (
	FileKindFile      FileKind = "file"
	FileKindDirectory FileKind = "directory"
	FileKindSymlink   FileKind = "symlink"
)

// FileErrorCode is a stable, backend-independent error code for filesystem operations.
type FileErrorCode string

const (
	FileErrorAborted         FileErrorCode = "aborted"
	FileErrorNotFound        FileErrorCode = "not_found"
	FileErrorPermissionDenied FileErrorCode = "permission_denied"
	FileErrorNotDirectory    FileErrorCode = "not_directory"
	FileErrorIsDirectory     FileErrorCode = "is_directory"
	FileErrorInvalid         FileErrorCode = "invalid"
	FileErrorNotSupported    FileErrorCode = "not_supported"
	FileErrorUnknown         FileErrorCode = "unknown"
)

// FileError is returned by FileSystem operations.
type FileError struct {
	Code    FileErrorCode
	Message string
	Path    string
	Cause   error
}

func (e *FileError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("FileError(%s): %s (path: %s)", e.Code, e.Message, e.Path)
	}
	return fmt.Sprintf("FileError(%s): %s", e.Code, e.Message)
}

func (e *FileError) Unwrap() error { return e.Cause }

func NewFileError(code FileErrorCode, message string, path string, cause error) *FileError {
	return &FileError{Code: code, Message: message, Path: path, Cause: cause}
}

// ExecutionErrorCode is a stable error code for execution environments.
type ExecutionErrorCode string

const (
	ExecErrorAborted        ExecutionErrorCode = "aborted"
	ExecErrorTimeout        ExecutionErrorCode = "timeout"
	ExecErrorShellUnavailable ExecutionErrorCode = "shell_unavailable"
	ExecErrorSpawn          ExecutionErrorCode = "spawn_error"
	ExecErrorCallback       ExecutionErrorCode = "callback_error"
	ExecErrorUnknown        ExecutionErrorCode = "unknown"
)

// ExecutionError is returned by Shell.Exec.
type ExecutionError struct {
	Code    ExecutionErrorCode
	Message string
	Cause   error
}

func (e *ExecutionError) Error() string {
	return fmt.Sprintf("ExecutionError(%s): %s", e.Code, e.Message)
}

func (e *ExecutionError) Unwrap() error { return e.Cause }

func NewExecutionError(code ExecutionErrorCode, message string, cause error) *ExecutionError {
	return &ExecutionError{Code: code, Message: message, Cause: cause}
}

// CompactionErrorCode is a stable error code for compaction operations.
type CompactionErrorCode string

const (
	CompactionErrorAborted           CompactionErrorCode = "aborted"
	CompactionErrorSummarizationFailed CompactionErrorCode = "summarization_failed"
	CompactionErrorInvalidSession    CompactionErrorCode = "invalid_session"
	CompactionErrorUnknown           CompactionErrorCode = "unknown"
)

// CompactionError is returned by compaction helpers.
type CompactionError struct {
	Code    CompactionErrorCode
	Message string
	Cause   error
}

func (e *CompactionError) Error() string {
	return fmt.Sprintf("CompactionError(%s): %s", e.Code, e.Message)
}

func (e *CompactionError) Unwrap() error { return e.Cause }

func NewCompactionError(code CompactionErrorCode, message string, cause error) *CompactionError {
	return &CompactionError{Code: code, Message: message, Cause: cause}
}

// BranchSummaryErrorCode is a stable error code for branch summarization.
type BranchSummaryErrorCode string

const (
	BranchSummaryErrorAborted           BranchSummaryErrorCode = "aborted"
	BranchSummaryErrorSummarizationFailed BranchSummaryErrorCode = "summarization_failed"
	BranchSummaryErrorInvalidSession    BranchSummaryErrorCode = "invalid_session"
)

// BranchSummaryError is returned by branch summarization helpers.
type BranchSummaryError struct {
	Code    BranchSummaryErrorCode
	Message string
	Cause   error
}

func (e *BranchSummaryError) Error() string {
	return fmt.Sprintf("BranchSummaryError(%s): %s", e.Code, e.Message)
}

func (e *BranchSummaryError) Unwrap() error { return e.Cause }

func NewBranchSummaryError(code BranchSummaryErrorCode, message string, cause error) *BranchSummaryError {
	return &BranchSummaryError{Code: code, Message: message, Cause: cause}
}

// SessionErrorCode is a stable error code for session operations.
type SessionErrorCode string

const (
	SessionErrorNotFound          SessionErrorCode = "not_found"
	SessionErrorInvalidSession    SessionErrorCode = "invalid_session"
	SessionErrorInvalidEntry      SessionErrorCode = "invalid_entry"
	SessionErrorInvalidForkTarget SessionErrorCode = "invalid_fork_target"
	SessionErrorStorage           SessionErrorCode = "storage"
	SessionErrorUnknown           SessionErrorCode = "unknown"
)

// SessionError is thrown by session storage, repositories, and tree operations.
type SessionError struct {
	Code    SessionErrorCode
	Message string
	Cause   error
}

func (e *SessionError) Error() string {
	return fmt.Sprintf("SessionError(%s): %s", e.Code, e.Message)
}

func (e *SessionError) Unwrap() error { return e.Cause }

func NewSessionError(code SessionErrorCode, message string, cause error) *SessionError {
	return &SessionError{Code: code, Message: message, Cause: cause}
}

// AgentHarnessErrorCode is a stable top-level error code for the harness.
type AgentHarnessErrorCode string

const (
	HarnessErrorBusy          AgentHarnessErrorCode = "busy"
	HarnessErrorInvalidState  AgentHarnessErrorCode = "invalid_state"
	HarnessErrorInvalidArg    AgentHarnessErrorCode = "invalid_argument"
	HarnessErrorSession       AgentHarnessErrorCode = "session"
	HarnessErrorHook          AgentHarnessErrorCode = "hook"
	HarnessErrorAuth          AgentHarnessErrorCode = "auth"
	HarnessErrorCompaction    AgentHarnessErrorCode = "compaction"
	HarnessErrorBranchSummary AgentHarnessErrorCode = "branch_summary"
	HarnessErrorUnknown       AgentHarnessErrorCode = "unknown"
)

// AgentHarnessError is the public error type for harness failures.
type AgentHarnessError struct {
	Code    AgentHarnessErrorCode
	Message string
	Cause   error
}

func (e *AgentHarnessError) Error() string {
	return fmt.Sprintf("AgentHarnessError(%s): %s", e.Code, e.Message)
}

func (e *AgentHarnessError) Unwrap() error { return e.Cause }

func NewAgentHarnessError(code AgentHarnessErrorCode, message string, cause error) *AgentHarnessError {
	return &AgentHarnessError{Code: code, Message: message, Cause: cause}
}

// NormalizeHarnessError converts unknown errors into AgentHarnessError.
func NormalizeHarnessError(err error, fallback AgentHarnessErrorCode) *AgentHarnessError {
	if he, ok := err.(*AgentHarnessError); ok {
		return he
	}
	cause := ToError(err)
	if se, ok := cause.(*SessionError); ok {
		return NewAgentHarnessError(HarnessErrorSession, se.Message, se)
	}
	if ce, ok := cause.(*CompactionError); ok {
		return NewAgentHarnessError(HarnessErrorCompaction, ce.Message, ce)
	}
	if be, ok := cause.(*BranchSummaryError); ok {
		return NewAgentHarnessError(HarnessErrorBranchSummary, be.Message, be)
	}
	return NewAgentHarnessError(fallback, cause.Error(), cause)
}

// ToError normalizes unknown values into Error instances.
func ToError(err any) error {
	if e, ok := err.(error); ok {
		return e
	}
	if s, ok := err.(string); ok {
		return fmt.Errorf("%s", s)
	}
	return fmt.Errorf("%v", err)
}

// ============================================================================
// FileInfo, FileSystem, Shell, ExecutionEnv
// ============================================================================

// FileInfo holds metadata for one filesystem object.
type FileInfo struct {
	Name    string   `json:"name"`
	Path    string   `json:"path"`
	Kind    FileKind `json:"kind"`
	Size    int64    `json:"size"`
	MtimeMs int64    `json:"mtimeMs"`
}

// ExecOptions controls shell command execution.
type ExecOptions struct {
	Cwd     string
	Env     map[string]string
	Timeout int // seconds
	OnStdout func(chunk string)
	OnStderr func(chunk string)
}

// ExecResult holds the result of a shell command.
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// FileSystem is the filesystem capability used by the harness.
// All methods receive context.Context for cancellation.
// All methods return Result[T, *FileError] instead of panicking.
type FileSystem interface {
	// Cwd returns the current working directory.
	Cwd() string

	// AbsolutePath returns an absolute path without requiring it to exist.
	AbsolutePath(ctx context.Context, path string) Result[string]
	// JoinPath joins path segments without requiring the result to exist.
	JoinPath(ctx context.Context, parts ...string) Result[string]
	// ReadTextFile reads a UTF-8 text file.
	ReadTextFile(ctx context.Context, path string) Result[string]
	// ReadTextLines reads UTF-8 text lines, stopping after maxLines if > 0.
	ReadTextLines(ctx context.Context, path string, maxLines int) Result[[]string]
	// ReadBinaryFile reads a binary file.
	ReadBinaryFile(ctx context.Context, path string) Result[[]byte]
	// WriteFile creates or overwrites a file, creating parent directories.
	WriteFile(ctx context.Context, path string, content []byte) Result[struct{}]
	// AppendFile creates or appends to a file, creating parent directories.
	AppendFile(ctx context.Context, path string, content []byte) Result[struct{}]
	// FileInfo returns metadata for the addressed path.
	FileInfo(ctx context.Context, path string) Result[FileInfo]
	// ListDir lists direct children of a directory.
	ListDir(ctx context.Context, path string) Result[[]FileInfo]
	// CanonicalPath returns the canonical path, resolving symlinks.
	CanonicalPath(ctx context.Context, path string) Result[string]
	// Exists returns false for missing paths. Other errors return FileError.
	Exists(ctx context.Context, path string) Result[bool]
	// CreateDir creates a directory. Use recursive=true for mkdir -p.
	CreateDir(ctx context.Context, path string, recursive bool) Result[struct{}]
	// Remove removes a file or directory.
	Remove(ctx context.Context, path string, recursive bool, force bool) Result[struct{}]
	// CreateTempDir creates a temporary directory and returns its absolute path.
	CreateTempDir(ctx context.Context, prefix string) Result[string]
	// CreateTempFile creates a temporary file and returns its absolute path.
	CreateTempFile(ctx context.Context, prefix, suffix string) Result[string]
	// Cleanup releases filesystem resources.
	Cleanup(ctx context.Context)
}

// Shell is the shell execution capability used by the harness.
type Shell interface {
	// Exec executes a shell command.
	Exec(ctx context.Context, command string, opts *ExecOptions) Result[ExecResult]
	// Cleanup releases shell resources.
	Cleanup(ctx context.Context)
}

// ExecutionEnv combines FileSystem and Shell.
type ExecutionEnv interface {
	FileSystem
	Shell
}

// ============================================================================
// Session tree entries
// ============================================================================

// SessionTreeEntryBase contains fields shared by all session tree entries.
type SessionTreeEntryBase struct {
	Type      string  `json:"type"`
	ID        string  `json:"id"`
	ParentID  *string `json:"parentId"`
	Timestamp string  `json:"timestamp"`
}

// MessageEntry records an AgentMessage in the session tree.
type MessageEntry struct {
	SessionTreeEntryBase
	Message ai.Message `json:"message"`
}

// ThinkingLevelChangeEntry records a thinking level change.
type ThinkingLevelChangeEntry struct {
	SessionTreeEntryBase
	ThinkingLevel string `json:"thinkingLevel"`
}

// ModelChangeEntry records a model change.
type ModelChangeEntry struct {
	SessionTreeEntryBase
	Provider string `json:"provider"`
	ModelID  string `json:"modelId"`
}

// ActiveToolsChangeEntry records an active tools change.
type ActiveToolsChangeEntry struct {
	SessionTreeEntryBase
	ActiveToolNames []string `json:"activeToolNames"`
}

// CompactionEntry records a compaction with its summary.
type CompactionEntry struct {
	SessionTreeEntryBase
	Summary         string `json:"summary"`
	FirstKeptEntryID string `json:"firstKeptEntryId"`
	TokensBefore    int    `json:"tokensBefore"`
	Details         any    `json:"details,omitempty"`
	FromHook        bool   `json:"fromHook,omitempty"`
}

// BranchSummaryEntry records a branch summary.
type BranchSummaryEntry struct {
	SessionTreeEntryBase
	FromID   string `json:"fromId"`
	Summary  string `json:"summary"`
	Details  any    `json:"details,omitempty"`
	FromHook bool   `json:"fromHook,omitempty"`
}

// CustomEntry records custom application data.
type CustomEntry struct {
	SessionTreeEntryBase
	CustomType string `json:"customType"`
	Data       any    `json:"data,omitempty"`
}

// CustomMessageEntry records a custom message in the session tree.
type CustomMessageEntry struct {
	SessionTreeEntryBase
	CustomType string                    `json:"customType"`
	Content    any                       `json:"content"` // string or []ContentBlock
	Display    bool                      `json:"display"`
	Details    any                       `json:"details,omitempty"`
}

// LabelEntry records a label applied to an entry.
type LabelEntry struct {
	SessionTreeEntryBase
	TargetID string  `json:"targetId"`
	Label    *string `json:"label"`
}

// SessionInfoEntry records session metadata (name, etc.).
type SessionInfoEntry struct {
	SessionTreeEntryBase
	Name *string `json:"name,omitempty"`
}

// LeafEntry records the current leaf of the session tree.
type LeafEntry struct {
	SessionTreeEntryBase
	TargetID *string `json:"targetId"`
}

// SessionTreeEntry is a union of all entry types. The Type field
// discriminates which concrete type the entry is.
type SessionTreeEntry struct {
	SessionTreeEntryBase

	// Message entry fields
	Message *ai.Message `json:"message,omitempty"`

	// ThinkingLevelChange fields
	ThinkingLevel string `json:"thinkingLevel,omitempty"`

	// ModelChange fields
	Provider string `json:"provider,omitempty"`
	ModelID  string `json:"modelId,omitempty"`

	// ActiveToolsChange fields
	ActiveToolNames []string `json:"activeToolNames,omitempty"`

	// Compaction fields
	Summary          string `json:"summary,omitempty"`
	FirstKeptEntryID string `json:"firstKeptEntryId,omitempty"`
	TokensBefore     int    `json:"tokensBefore,omitempty"`
	Details          any    `json:"details,omitempty"`
	FromHook         bool   `json:"fromHook,omitempty"`

	// BranchSummary fields
	FromID string `json:"fromId,omitempty"`

	// Custom fields
	CustomType string `json:"customType,omitempty"`
	Data       any    `json:"data,omitempty"`

	// CustomMessage fields
	Content any  `json:"content,omitempty"`
	Display bool `json:"display,omitempty"`

	// Label fields
	TargetID *string `json:"targetId,omitempty"`
	Label    *string `json:"label,omitempty"`

	// SessionInfo fields
	Name *string `json:"name,omitempty"`
}

// AsMessageEntry returns the entry as a typed MessageEntry if type matches.
func (e SessionTreeEntry) AsMessageEntry() (*MessageEntry, bool) {
	if e.Type != "message" || e.Message == nil {
		return nil, false
	}
	return &MessageEntry{SessionTreeEntryBase: e.SessionTreeEntryBase, Message: *e.Message}, true
}

// ============================================================================
// Session metadata & context
// ============================================================================

// SessionMetadata holds basic session identification.
type SessionMetadata struct {
	ID        string `json:"id"`
	CreatedAt string `json:"createdAt"`
}

// JsonlSessionMetadata extends SessionMetadata with JSONL-specific fields.
type JsonlSessionMetadata struct {
	SessionMetadata
	Cwd              string  `json:"cwd"`
	Path             string  `json:"path"`
	ParentSessionPath *string `json:"parentSessionPath,omitempty"`
}

// SessionContext holds the reconstructed session state: messages, settings.
type SessionContext struct {
	Messages        []ai.Message
	ThinkingLevel   string
	Model           *SessionModelRef
	ActiveToolNames []string
}

// SessionModelRef is a lightweight model reference stored in session context.
type SessionModelRef struct {
	Provider string `json:"provider"`
	ModelID  string `json:"modelId"`
}

// ============================================================================
// Skill and PromptTemplate
// ============================================================================

// Skill represents a loaded skill from a SKILL.md file.
type Skill struct {
	Name                string `json:"name"`
	Description         string `json:"description"`
	Content             string `json:"content"`
	FilePath            string `json:"filePath"`
	DisableModelInvocation bool  `json:"disableModelInvocation,omitempty"`
}

// PromptTemplate is a named template for generating prompts.
type PromptTemplate struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Content     string `json:"content"`
}

// HarnessResources holds skills and prompt templates available to the harness.
type HarnessResources struct {
	PromptTemplates []PromptTemplate `json:"promptTemplates,omitempty"`
	Skills          []Skill          `json:"skills,omitempty"`
}

// ============================================================================
// Stream options
// ============================================================================

// HarnessStreamOptions holds curated provider request options.
type HarnessStreamOptions struct {
	Transport       ai.Transport        `json:"transport,omitempty"`
	TimeoutMs       *int                `json:"timeoutMs,omitempty"`
	MaxRetries      *int                `json:"maxRetries,omitempty"`
	MaxRetryDelayMs *int                `json:"maxRetryDelayMs,omitempty"`
	Headers         map[string]string   `json:"headers,omitempty"`
	Metadata        map[string]any      `json:"metadata,omitempty"`
	CacheRetention  *ai.CacheRetention  `json:"cacheRetention,omitempty"`
}

// Clone returns a deep copy of the stream options.
func (o HarnessStreamOptions) Clone() HarnessStreamOptions {
	clone := o
	if o.Headers != nil {
		clone.Headers = make(map[string]string, len(o.Headers))
		for k, v := range o.Headers {
			clone.Headers[k] = v
		}
	}
	if o.Metadata != nil {
		clone.Metadata = make(map[string]any, len(o.Metadata))
		for k, v := range o.Metadata {
			clone.Metadata[k] = v
		}
	}
	return clone
}

// HarnessStreamOptionsPatch is a per-request patch for stream options.
type HarnessStreamOptionsPatch struct {
	Transport       *ai.Transport      `json:"transport,omitempty"`
	TimeoutMs       *int               `json:"timeoutMs,omitempty"`
	MaxRetries      *int               `json:"maxRetries,omitempty"`
	MaxRetryDelayMs *int               `json:"maxRetryDelayMs,omitempty"`
	CacheRetention  *ai.CacheRetention `json:"cacheRetention,omitempty"`
	Headers         map[string]*string `json:"headers,omitempty"`
	Metadata        map[string]any     `json:"metadata,omitempty"`
}

// ApplyStreamOptionsPatch applies a patch to base stream options.
func ApplyStreamOptionsPatch(base HarnessStreamOptions, patch *HarnessStreamOptionsPatch) HarnessStreamOptions {
	result := base.Clone()
	if patch == nil {
		return result
	}

	if patch.Transport != nil {
		result.Transport = *patch.Transport
	}
	if patch.TimeoutMs != nil {
		result.TimeoutMs = patch.TimeoutMs
	}
	if patch.MaxRetries != nil {
		result.MaxRetries = patch.MaxRetries
	}
	if patch.MaxRetryDelayMs != nil {
		result.MaxRetryDelayMs = patch.MaxRetryDelayMs
	}
	if patch.CacheRetention != nil {
		result.CacheRetention = patch.CacheRetention
	}

	if patch.Headers != nil {
		if result.Headers == nil {
			result.Headers = make(map[string]string)
		}
		for k, v := range patch.Headers {
			if v == nil {
				delete(result.Headers, k)
			} else {
				result.Headers[k] = *v
			}
		}
		if len(result.Headers) == 0 {
			result.Headers = nil
		}
	}

	if patch.Metadata != nil {
		if result.Metadata == nil {
			result.Metadata = make(map[string]any)
		}
		for k, v := range patch.Metadata {
			if v == nil {
				delete(result.Metadata, k)
			} else {
				result.Metadata[k] = v
			}
		}
		if len(result.Metadata) == 0 {
			result.Metadata = nil
		}
	}

	return result
}

// ============================================================================
// Harness events & hook results
// ============================================================================

// HarnessPhase tracks the current state of the harness.
type HarnessPhase string

const (
	PhaseIdle          HarnessPhase = "idle"
	PhaseTurn          HarnessPhase = "turn"
	PhaseCompaction    HarnessPhase = "compaction"
	PhaseBranchSummary HarnessPhase = "branch_summary"
	PhaseRetry         HarnessPhase = "retry"
)

// HarnessEvent is a discriminated union of all harness-level events.
// The Type field determines which fields are populated.
type HarnessEvent struct {
	Type string `json:"type"`

	// queue_update
	Steer    []ai.Message `json:"steer,omitempty"`
	FollowUp []ai.Message `json:"followUp,omitempty"`
	NextTurn []ai.Message `json:"nextTurn,omitempty"`

	// save_point
	HadPendingMutations bool `json:"hadPendingMutations,omitempty"`

	// abort
	ClearedSteer    []ai.Message `json:"clearedSteer,omitempty"`
	ClearedFollowUp []ai.Message `json:"clearedFollowUp,omitempty"`

	// settled
	NextTurnCount int `json:"nextTurnCount,omitempty"`

	// before_agent_start
	Prompt       string             `json:"prompt,omitempty"`
	Images       []ai.ContentBlock  `json:"images,omitempty"`
	SystemPrompt string             `json:"systemPrompt,omitempty"`
	Resources    *HarnessResources  `json:"resources,omitempty"`

	// context
	Messages []ai.Message `json:"messages,omitempty"`

	// before_provider_request
	Model         *ai.Model             `json:"model,omitempty"`
	SessionID     string                `json:"sessionId,omitempty"`
	StreamOptions *HarnessStreamOptions `json:"streamOptions,omitempty"`

	// before_provider_payload
	Payload any `json:"payload,omitempty"`

	// after_provider_response
	Status int               `json:"status,omitempty"`
	Headers map[string]string `json:"responseHeaders,omitempty"`

	// tool_call, tool_result
	ToolCallID string         `json:"toolCallId,omitempty"`
	ToolName   string         `json:"toolName,omitempty"`
	Input      map[string]any `json:"input,omitempty"`

	// tool_result
	Content []ai.ContentBlock `json:"content,omitempty"`
	IsError bool              `json:"isError,omitempty"`

	// session_before_compact, session_compact
	CompactionEntry   *CompactionEntry `json:"compactionEntry,omitempty"`
	Preparation       any              `json:"preparation,omitempty"`
	BranchEntries     []SessionTreeEntry `json:"branchEntries,omitempty"`
	CustomInstructions *string         `json:"customInstructions,omitempty"`

	// session_before_tree, session_tree
	NewLeafID    *string             `json:"newLeafId,omitempty"`
	OldLeafID    *string             `json:"oldLeafId,omitempty"`
	SummaryEntry *BranchSummaryEntry `json:"summaryEntry,omitempty"`

	// model_update
	PreviousModel *ai.Model `json:"previousModel,omitempty"`
	Source        string    `json:"source,omitempty"`

	// thinking_level_update
	Level         string `json:"level,omitempty"`
	PreviousLevel string `json:"previousLevel,omitempty"`

	// tools_update
	ToolNames             []string `json:"toolNames,omitempty"`
	PreviousToolNames     []string `json:"previousToolNames,omitempty"`
	ActiveToolNamesEvt    []string `json:"activeToolNamesEvt,omitempty"`
	PreviousActiveToolNames []string `json:"previousActiveToolNames,omitempty"`

	// resources_update
	PreviousResources *HarnessResources `json:"previousResources,omitempty"`
}

// Hook result types — returned by event handlers to modify behavior.

// BeforeAgentStartResult allows modifying the initial prompt/system.
type BeforeAgentStartResult struct {
	Messages     []ai.Message `json:"messages,omitempty"`
	SystemPrompt *string      `json:"systemPrompt,omitempty"`
}

// ContextResult allows modifying the context messages.
type ContextResult struct {
	Messages []ai.Message `json:"messages"`
}

// BeforeProviderRequestResult allows patching stream options.
type BeforeProviderRequestResult struct {
	StreamOptions *HarnessStreamOptionsPatch `json:"streamOptions,omitempty"`
}

// BeforeProviderPayloadResult allows modifying the provider payload.
type BeforeProviderPayloadResult struct {
	Payload any `json:"payload"`
}

// ToolCallResult allows blocking a tool call.
type ToolCallResult struct {
	Block  bool   `json:"block,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// ToolResultPatch allows modifying a tool result.
type ToolResultPatch struct {
	Content   []ai.ContentBlock `json:"content,omitempty"`
	Details   any               `json:"details,omitempty"`
	IsError   *bool             `json:"isError,omitempty"`
	Terminate *bool             `json:"terminate,omitempty"`
}

// SessionBeforeCompactResult allows cancelling or providing compaction.
type SessionBeforeCompactResult struct {
	Cancel     bool          `json:"cancel,omitempty"`
	Compaction *CompactResult `json:"compaction,omitempty"`
}

// SessionBeforeTreeResult allows cancelling or providing a branch summary.
type SessionBeforeTreeResult struct {
	Cancel           bool   `json:"cancel,omitempty"`
	Summary          *string `json:"summary,omitempty"`
	Details          any    `json:"details,omitempty"`
	CustomInstructions  *string `json:"customInstructions,omitempty"`
	ReplaceInstructions *bool `json:"replaceInstructions,omitempty"`
	Label            *string `json:"label,omitempty"`
}

// CompactResult holds the output of a compaction operation.
type CompactResult struct {
	Summary         string `json:"summary"`
	FirstKeptEntryID string `json:"firstKeptEntryId"`
	TokensBefore    int    `json:"tokensBefore"`
	Details         any    `json:"details,omitempty"`
}

// FileOperations tracks file reads/writes during a turn.
type FileOperations struct {
	Read    map[string]bool `json:"read"`
	Written map[string]bool `json:"written"`
	Edited  map[string]bool `json:"edited"`
}

// CompactionSettings configures compaction behavior.
type CompactionSettings struct {
	Enabled        bool `json:"enabled"`
	ReserveTokens  int  `json:"reserveTokens"`
	KeepRecentTokens int `json:"keepRecentTokens"`
}

// DefaultCompactionSettings returns the default compaction settings.
func DefaultCompactionSettings() CompactionSettings {
	return CompactionSettings{
		Enabled:        true,
		ReserveTokens:  8000,
		KeepRecentTokens: 4000,
	}
}

// CompactionPreparation holds the computed state before compaction.
type CompactionPreparation struct {
	FirstKeptEntryID    string
	MessagesToSummarize []ai.Message
	TurnPrefixMessages  []ai.Message
	IsSplitTurn         bool
	TokensBefore        int
	PreviousSummary     string
	FileOps             FileOperations
	Settings            CompactionSettings
}

// TreePreparation holds the computed state before tree navigation.
type TreePreparation struct {
	TargetID           string
	OldLeafID          *string
	CommonAncestorID   *string
	EntriesToSummarize []SessionTreeEntry
	UserWantsSummary   bool
	CustomInstructions *string
	ReplaceInstructions *bool
	Label              *string
}

// NavigateTreeResult holds the output of a tree navigation.
// BranchSummaryResult holds the output of a branch summary generation.
type BranchSummaryResult struct {
	Summary       string   `json:"summary"`
	ReadFiles     []string `json:"readFiles,omitempty"`
	ModifiedFiles []string `json:"modifiedFiles,omitempty"`
	Details       any      `json:"details,omitempty"`
	FromHook      bool     `json:"fromHook,omitempty"`
}

// NavigateTreeResult holds the output of a tree navigation operation.
type NavigateTreeResult struct {
	Cancelled    bool
	EditorText   string
	SummaryEntry *BranchSummaryEntry
}

// CompactionResult holds the output of a compaction operation.
type CompactionResult struct {
	Summary          string `json:"summary"`
	FirstKeptEntryID string `json:"firstKeptEntryId"`
	TokensBefore     int    `json:"tokensBefore"`
	Details          any    `json:"details,omitempty"`
}

// AbortResult holds the output of an abort operation.
type AbortResult struct {
	ClearedSteer    []ai.Message
	ClearedFollowUp []ai.Message
}

// ============================================================================
// Harness Options & Config
// ============================================================================

// PendingSessionWrite represents a deferred session mutation that is flushed
// when the harness transitions back to idle.
type PendingSessionWrite struct {
	Type string `json:"type"`

	// message
	Message ai.Message `json:"message,omitempty"`

	// model_change
	Provider string `json:"provider,omitempty"`
	ModelID  string `json:"modelId,omitempty"`

	// thinking_level_change
	ThinkingLevel string `json:"thinkingLevel,omitempty"`

	// active_tools_change
	ActiveToolNames []string `json:"activeToolNames,omitempty"`

	// custom
	CustomType string `json:"customType,omitempty"`
	Data       any    `json:"data,omitempty"`

	// custom_message
	Content any  `json:"content,omitempty"`
	Display bool `json:"display,omitempty"`
	Details any  `json:"details,omitempty"`

	// label
	TargetID string  `json:"targetId,omitempty"`
	Label    *string `json:"label,omitempty"`

	// session_info
	Name string `json:"name,omitempty"`
}

// AuthInfo contains resolved API key and headers.
type AuthInfo struct {
	APIKey  string
	Headers map[string]string
}

// SystemPromptFn resolves the system prompt string.
type SystemPromptFn func(env ExecutionEnv, model *ai.Model, thinkingLevel string, activeTools []agent.Tool, resources HarnessResources) (string, error)

// GetApiKeyAndHeadersFn resolves API authentication for a model.
type GetApiKeyAndHeadersFn func(model *ai.Model) (*AuthInfo, error)

// HarnessEventHandler processes a harness event. May return a result to
// modify harness behavior.
type HarnessEventHandler func(event HarnessEvent) (any, error)

// SessionProvider is the interface the harness needs from a session.
// *session.Session satisfies this interface.
type SessionProvider interface {
	GetMetadata(ctx context.Context) (SessionMetadata, error)
	GetLeafID(ctx context.Context) (*string, error)
	GetEntry(ctx context.Context, id string) (*SessionTreeEntry, error)
	GetEntries(ctx context.Context) ([]SessionTreeEntry, error)
	GetBranch(ctx context.Context, fromID *string) ([]SessionTreeEntry, error)
	BuildContext(ctx context.Context) (*SessionContext, error)
	AppendMessage(ctx context.Context, message ai.Message) (string, error)
	AppendModelChange(ctx context.Context, provider, modelID string) (string, error)
	AppendThinkingLevelChange(ctx context.Context, level string) (string, error)
	AppendActiveToolsChange(ctx context.Context, activeToolNames []string) (string, error)
	AppendCompaction(ctx context.Context, summary, firstKeptEntryID string, tokensBefore int, details any, fromHook bool) (string, error)
	AppendBranchSummary(ctx context.Context, fromID, summary string, details any, fromHook bool) (string, error)
	AppendCustomEntry(ctx context.Context, customType string, data any) (string, error)
	AppendCustomMessageEntry(ctx context.Context, customType string, content any, display bool, details any) (string, error)
	AppendLabel(ctx context.Context, targetID string, label *string) (string, error)
	AppendSessionName(ctx context.Context, name string) (string, error)
	MoveTo(ctx context.Context, entryID *string, summary *BranchSummaryResult) (*string, error)
}

// CompactionFunc performs compaction and returns the result.
type CompactionFunc func(ctx context.Context, preparation any, model *ai.Model, apiKey string, headers map[string]string, customInstructions string, thinkingLevel string) (any, error)

// PrepareCompactionFunc prepares compaction from branch entries.
type PrepareCompactionFunc func(entries []SessionTreeEntry, settings any) (any, error)

// CollectBranchEntriesFunc collects entries for branch summarization.
type CollectBranchEntriesFunc func(sess SessionProvider, oldLeafID, targetID string) (entries []SessionTreeEntry, commonAncestorID string, err error)

// GenerateBranchSummaryFunc generates a summary for a branch.
type GenerateBranchSummaryFunc func(ctx context.Context, entries []SessionTreeEntry, opts any) (any, error)

// HarnessOptions configures an AgentHarness.
type HarnessOptions struct {
	Env              ExecutionEnv
	Resources        *HarnessResources
	StreamOptions    *HarnessStreamOptions
	SystemPrompt     any // string or SystemPromptFn
	GetApiKeyAndHeaders GetApiKeyAndHeadersFn
	Tools            []agent.Tool
	ActiveToolNames  []string
	Model            *ai.Model
	ThinkingLevel    string
	SteeringMode     agent.QueueMode
	FollowUpMode     agent.QueueMode

	// Compaction/branch functions (injected to avoid import cycles).
	// Wire these from the compaction package at the call site.
	CompactFn              CompactionFunc
	PrepareCompactionFn    PrepareCompactionFunc
	DefaultCompactionSettingsFn func() any
	CollectBranchEntriesFn CollectBranchEntriesFunc
	GenerateBranchSummaryFn GenerateBranchSummaryFunc
}

// ============================================================================
// Utility functions
// ============================================================================

// NowISO returns the current time as an ISO 8601 string.
var NowISO = func() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

// MergeHeaders merges multiple header maps, later maps override earlier ones.
func MergeHeaders(headers ...map[string]string) map[string]string {
	merged := make(map[string]string)
	for _, h := range headers {
		for k, v := range h {
			merged[k] = v
		}
	}
	return merged
}
