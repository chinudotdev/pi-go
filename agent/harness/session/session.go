package session

import (
	"context"
	"strings"
	"time"

	"github.com/chinudotdev/pi-go/agent/harness"
	"github.com/chinudotdev/pi-go/ai"
)

// Session is a high-level wrapper around SessionStorage that provides
// typed methods for appending entries and building session context.
type Session struct {
	storage SessionStorage
}

// NewSession creates a Session backed by the given storage.
func NewSession(storage SessionStorage) *Session {
	return &Session{storage: storage}
}

// GetStorage returns the underlying storage implementation.
func (s *Session) GetStorage() SessionStorage {
	return s.storage
}

// GetMetadata returns the session metadata.
func (s *Session) GetMetadata(ctx context.Context) (harness.SessionMetadata, error) {
	return s.storage.GetMetadata(ctx)
}

// GetLeafID returns the current leaf entry ID.
func (s *Session) GetLeafID(ctx context.Context) (*string, error) {
	return s.storage.GetLeafID(ctx)
}

// GetEntry returns an entry by ID.
func (s *Session) GetEntry(ctx context.Context, id string) (*harness.SessionTreeEntry, error) {
	return s.storage.GetEntry(ctx, id)
}

// GetEntries returns all entries in chronological order.
func (s *Session) GetEntries(ctx context.Context) ([]harness.SessionTreeEntry, error) {
	return s.storage.GetEntries(ctx)
}

// GetBranch returns entries from the given leaf to the root.
// If fromID is nil, uses the current leaf.
func (s *Session) GetBranch(ctx context.Context, fromID *string) ([]harness.SessionTreeEntry, error) {
	leafID := fromID
	if leafID == nil {
		var err error
		leafID, err = s.storage.GetLeafID(ctx)
		if err != nil {
			return nil, err
		}
	}
	return s.storage.GetPathToRoot(ctx, leafID)
}

// BuildContext reconstructs the session context from the current branch.
func (s *Session) BuildContext(ctx context.Context) (*harness.SessionContext, error) {
	branch, err := s.GetBranch(ctx, nil)
	if err != nil {
		return nil, err
	}
	return BuildSessionContext(branch), nil
}

// GetLabel returns the label for an entry.
func (s *Session) GetLabel(ctx context.Context, id string) (*string, error) {
	return s.storage.GetLabel(ctx, id)
}

// GetSessionName returns the session name from session_info entries.
func (s *Session) GetSessionName(ctx context.Context) (*string, error) {
	entries, err := s.storage.FindEntries(ctx, "session_info")
	if err != nil {
		return nil, err
	}
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Name != nil {
			trimmed := strings.TrimSpace(*entries[i].Name)
			if trimmed != "" {
				return &trimmed, nil
			}
		}
	}
	return nil, nil
}

// ============================================================================
// Append methods
// ============================================================================

func (s *Session) appendEntry(ctx context.Context, entry harness.SessionTreeEntry) (string, error) {
	err := s.storage.AppendEntry(ctx, entry)
	return entry.ID, err
}

func (s *Session) makeBase(ctx context.Context, entryType string) (harness.SessionTreeEntryBase, error) {
	id, err := s.storage.CreateEntryID(ctx)
	if err != nil {
		return harness.SessionTreeEntryBase{}, err
	}
	parentID, err := s.storage.GetLeafID(ctx)
	if err != nil {
		return harness.SessionTreeEntryBase{}, err
	}
	return harness.SessionTreeEntryBase{
		Type:      entryType,
		ID:        id,
		ParentID:  parentID,
		Timestamp: harness.NowISO(),
	}, nil
}

// AppendMessage appends a message entry.
func (s *Session) AppendMessage(ctx context.Context, message ai.Message) (string, error) {
	base, err := s.makeBase(ctx, "message")
	if err != nil {
		return "", err
	}
	return s.appendEntry(ctx, harness.SessionTreeEntry{
		SessionTreeEntryBase: base,
		Message:              &message,
	})
}

// AppendThinkingLevelChange appends a thinking level change entry.
func (s *Session) AppendThinkingLevelChange(ctx context.Context, level string) (string, error) {
	base, err := s.makeBase(ctx, "thinking_level_change")
	if err != nil {
		return "", err
	}
	return s.appendEntry(ctx, harness.SessionTreeEntry{
		SessionTreeEntryBase: base,
		ThinkingLevel:        level,
	})
}

// AppendModelChange appends a model change entry.
func (s *Session) AppendModelChange(ctx context.Context, provider, modelID string) (string, error) {
	base, err := s.makeBase(ctx, "model_change")
	if err != nil {
		return "", err
	}
	return s.appendEntry(ctx, harness.SessionTreeEntry{
		SessionTreeEntryBase: base,
		Provider:             provider,
		ModelID:              modelID,
	})
}

// AppendActiveToolsChange appends an active tools change entry.
func (s *Session) AppendActiveToolsChange(ctx context.Context, activeToolNames []string) (string, error) {
	base, err := s.makeBase(ctx, "active_tools_change")
	if err != nil {
		return "", err
	}
	names := make([]string, len(activeToolNames))
	copy(names, activeToolNames)
	return s.appendEntry(ctx, harness.SessionTreeEntry{
		SessionTreeEntryBase: base,
		ActiveToolNames:      names,
	})
}

// AppendCompaction appends a compaction entry.
func (s *Session) AppendCompaction(ctx context.Context, summary, firstKeptEntryID string, tokensBefore int, details any, fromHook bool) (string, error) {
	base, err := s.makeBase(ctx, "compaction")
	if err != nil {
		return "", err
	}
	return s.appendEntry(ctx, harness.SessionTreeEntry{
		SessionTreeEntryBase: base,
		Summary:              summary,
		FirstKeptEntryID:     firstKeptEntryID,
		TokensBefore:         tokensBefore,
		Details:              details,
		FromHook:             fromHook,
	})
}

// AppendBranchSummary appends a branch summary entry.
func (s *Session) AppendBranchSummary(ctx context.Context, fromID, summary string, details any, fromHook bool) (string, error) {
	base, err := s.makeBase(ctx, "branch_summary")
	if err != nil {
		return "", err
	}
	return s.appendEntry(ctx, harness.SessionTreeEntry{
		SessionTreeEntryBase: base,
		FromID:               fromID,
		Summary:              summary,
		Details:              details,
		FromHook:             fromHook,
	})
}

// AppendCustomEntry appends a custom data entry.
func (s *Session) AppendCustomEntry(ctx context.Context, customType string, data any) (string, error) {
	base, err := s.makeBase(ctx, "custom")
	if err != nil {
		return "", err
	}
	return s.appendEntry(ctx, harness.SessionTreeEntry{
		SessionTreeEntryBase: base,
		CustomType:           customType,
		Data:                 data,
	})
}

// AppendCustomMessageEntry appends a custom message entry.
func (s *Session) AppendCustomMessageEntry(ctx context.Context, customType string, content any, display bool, details any) (string, error) {
	base, err := s.makeBase(ctx, "custom_message")
	if err != nil {
		return "", err
	}
	return s.appendEntry(ctx, harness.SessionTreeEntry{
		SessionTreeEntryBase: base,
		CustomType:           customType,
		Content:              content,
		Display:              display,
		Details:              details,
	})
}

// AppendLabel appends a label entry.
func (s *Session) AppendLabel(ctx context.Context, targetID string, label *string) (string, error) {
	// Validate target exists
	existing, err := s.storage.GetEntry(ctx, targetID)
	if err != nil {
		return "", err
	}
	if existing == nil {
		return "", harness.NewSessionError(harness.SessionErrorNotFound, "Entry "+targetID+" not found", nil)
	}

	base, err := s.makeBase(ctx, "label")
	if err != nil {
		return "", err
	}
	return s.appendEntry(ctx, harness.SessionTreeEntry{
		SessionTreeEntryBase: base,
		TargetID:             &targetID,
		Label:                label,
	})
}

// AppendSessionName appends a session_info entry with a name.
func (s *Session) AppendSessionName(ctx context.Context, name string) (string, error) {
	base, err := s.makeBase(ctx, "session_info")
	if err != nil {
		return "", err
	}
	trimmed := strings.TrimSpace(name)
	return s.appendEntry(ctx, harness.SessionTreeEntry{
		SessionTreeEntryBase: base,
		Name:                 &trimmed,
	})
}

// MoveTo changes the current leaf, optionally appending a branch summary.
func (s *Session) MoveTo(ctx context.Context, entryID *string, summary *harness.BranchSummaryResult) (*string, error) {
	if entryID != nil {
		existing, err := s.storage.GetEntry(ctx, *entryID)
		if err != nil {
			return nil, err
		}
		if existing == nil {
			return nil, harness.NewSessionError(harness.SessionErrorNotFound, "Entry "+*entryID+" not found", nil)
		}
	}

	if err := s.storage.SetLeafID(ctx, entryID); err != nil {
		return nil, err
	}

	if summary == nil {
		return nil, nil
	}

	fromID := "root"
	if entryID != nil {
		fromID = *entryID
	}
	id, err := s.AppendBranchSummary(ctx, fromID, summary.Summary, summary.Details, summary.FromHook)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// ============================================================================
// BuildSessionContext — context reconstruction from tree entries
// ============================================================================

// BuildSessionContext reconstructs a SessionContext from a path of entries.
func BuildSessionContext(pathEntries []harness.SessionTreeEntry) *harness.SessionContext {
	thinkingLevel := "off"
	var model *harness.SessionModelRef
	var activeToolNames []string
	var compaction *harness.SessionTreeEntry

	for i := range pathEntries {
		entry := &pathEntries[i]
		switch entry.Type {
		case "thinking_level_change":
			thinkingLevel = entry.ThinkingLevel
		case "model_change":
			model = &harness.SessionModelRef{Provider: entry.Provider, ModelID: entry.ModelID}
		case "message":
			if entry.Message != nil && entry.Message.Role == "assistant" {
				model = &harness.SessionModelRef{
					Provider: entry.Message.Provider,
					ModelID:  entry.Message.Model,
				}
			}
		case "active_tools_change":
			activeToolNames = make([]string, len(entry.ActiveToolNames))
			copy(activeToolNames, entry.ActiveToolNames)
		case "compaction":
			compaction = entry
		}
	}

	var messages []ai.Message

	appendMessage := func(entry *harness.SessionTreeEntry) {
		switch entry.Type {
		case "message":
			if entry.Message != nil {
				messages = append(messages, *entry.Message)
			}
		case "custom_message":
			// Convert custom message to user message
			text := customMessageToText(entry)
			messages = append(messages, ai.Message{
				Role:      "user",
				Content:   text,
				Timestamp: timestampFromISO(entry.Timestamp),
			})
		case "branch_summary":
			if entry.Summary != "" {
				prefix := "The following is a summary of a branch that this conversation came back from:\n\n<summary>\n"
				suffix := "\n</summary>"
				messages = append(messages, ai.Message{
					Role:    "user",
					Content: prefix + entry.Summary + suffix,
					Timestamp: timestampFromISO(entry.Timestamp),
				})
			}
		}
	}

	if compaction != nil {
		// Compaction summary message
		prefix := "The conversation history before this point was compacted into the following summary:\n\n<summary>\n"
		suffix := "\n</summary>"
		messages = append(messages, ai.Message{
			Role:    "user",
			Content: prefix + compaction.Summary + suffix,
			Timestamp: timestampFromISO(compaction.Timestamp),
		})

		// Find compaction index and first kept entry
		compactionIdx := -1
		for i, e := range pathEntries {
			if e.Type == "compaction" && e.ID == compaction.ID {
				compactionIdx = i
				break
			}
		}

		foundFirstKept := false
		if compactionIdx >= 0 {
			for i := 0; i < compactionIdx; i++ {
				if pathEntries[i].ID == compaction.FirstKeptEntryID {
					foundFirstKept = true
				}
				if foundFirstKept {
					appendMessage(&pathEntries[i])
				}
			}
			for i := compactionIdx + 1; i < len(pathEntries); i++ {
				appendMessage(&pathEntries[i])
			}
		}
	} else {
		for i := range pathEntries {
			appendMessage(&pathEntries[i])
		}
	}

	return &harness.SessionContext{
		Messages:        messages,
		ThinkingLevel:   thinkingLevel,
		Model:           model,
		ActiveToolNames: activeToolNames,
	}
}

func customMessageToText(entry *harness.SessionTreeEntry) string {
	switch v := entry.Content.(type) {
	case string:
		return v
	default:
		return ""
	}
}

func timestampFromISO(iso string) int64 {
	// Best-effort parse; return 0 on failure
	t, err := parseISO(iso)
	if err != nil {
		return 0
	}
	return t
}

// parseISO parses an ISO 8601 timestamp string and returns milliseconds.
func parseISO(s string) (int64, error) {
	// Try common layouts
	for _, layout := range []string{
		"2006-01-02T15:04:05.999Z07:00",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05.999Z",
		"2006-01-02T15:04:05Z",
		time.RFC3339Nano,
		time.RFC3339,
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UnixMilli(), nil
		}
	}
	return 0, nil
}
