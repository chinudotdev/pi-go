package harness

import "github.com/chinudotdev/pi-go/ai"

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
