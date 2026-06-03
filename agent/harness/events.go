package harness

import (
	"github.com/chinudotdev/pi-go/ai"
)

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
