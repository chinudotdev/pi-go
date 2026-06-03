package harness

import (
	"encoding/json"

	"github.com/chinudotdev/pi-go/ai"
)

// ConvertToLlm converts agent messages to LLM-compatible ai.Message slice.
// Custom message types (bashExecution, custom, branchSummary, compactionSummary)
// are converted to user messages with appropriate text content.
func ConvertToLlm(messages []ai.Message) []ai.Message {
	result := make([]ai.Message, 0, len(messages))
	for _, m := range messages {
		converted := convertOneToLlm(m)
		if converted != nil {
			result = append(result, *converted)
		}
	}
	return result
}

func convertOneToLlm(m ai.Message) *ai.Message {
	switch m.Role {
	case RoleBashExecution:
		payload := extractBashPayload(m)
		if payload == nil {
			return nil
		}
		return &ai.Message{
			Role:      "user",
			Content:   BashExecutionToText(*payload),
			Timestamp: m.Timestamp,
		}

	case RoleCustom:
		return &ai.Message{
			Role:      "user",
			Content:   m.Content,
			Timestamp: m.Timestamp,
		}

	case RoleBranchSummary:
		summary := extractPayloadSummary(m)
		text := BranchSummaryPrefix + summary + BranchSummarySuffix
		return &ai.Message{
			Role:      "user",
			Content:   text,
			Timestamp: m.Timestamp,
		}

	case RoleCompactionSummary:
		summary := extractPayloadSummary(m)
		text := CompactionSummaryPrefix + summary + CompactionSummarySuffix
		return &ai.Message{
			Role:      "user",
			Content:   text,
			Timestamp: m.Timestamp,
		}

	case "user", "assistant", "toolResult":
		return &m

	default:
		return nil
	}
}

// extractBashPayload extracts a BashExecutionPayload from an ai.Message.
func extractBashPayload(m ai.Message) *BashExecutionPayload {
	switch c := m.Content.(type) {
	case BashExecutionPayload:
		return &c
	case *BashExecutionPayload:
		return c
	case map[string]any:
		return bashPayloadFromMap(c)
	default:
		// Try JSON round-trip
		b, err := json.Marshal(m.Content)
		if err != nil {
			return nil
		}
		var payload BashExecutionPayload
		if json.Unmarshal(b, &payload) != nil {
			return nil
		}
		return &payload
	}
}

func bashPayloadFromMap(m map[string]any) *BashExecutionPayload {
	p := &BashExecutionPayload{}
	if v, ok := m["command"].(string); ok {
		p.Command = v
	}
	if v, ok := m["output"].(string); ok {
		p.Output = v
	}
	if v, ok := m["fullOutputPath"].(string); ok {
		p.FullOutputPath = v
	}
	if v, ok := m["cancelled"].(bool); ok {
		p.Cancelled = v
	}
	if v, ok := m["truncated"].(bool); ok {
		p.Truncated = v
	}
	if v, ok := m["exitCode"]; ok {
		switch n := v.(type) {
		case float64:
			i := int(n)
			p.ExitCode = &i
		case json.Number:
			if i, err := n.Int64(); err == nil {
				ii := int(i)
				p.ExitCode = &ii
			}
		}
	}
	return p
}

// extractPayloadSummary extracts a summary string from message content.
func extractPayloadSummary(m ai.Message) string {
	switch c := m.Content.(type) {
	case string:
		return c
	case map[string]any:
		if s, ok := c["summary"].(string); ok {
			return s
		}
	case BranchSummaryPayload:
		return c.Summary
	case CompactionSummaryPayload:
		return c.Summary
	default:
		// Try JSON round-trip
		b, err := json.Marshal(c)
		if err != nil {
			return ""
		}
		var raw map[string]any
		if json.Unmarshal(b, &raw) != nil {
			return ""
		}
		if s, ok := raw["summary"].(string); ok {
			return s
		}
	}
	return ""
}

// GetMessageFromEntry extracts an ai.Message from a session tree entry.
// Returns nil for non-message entry types.
func GetMessageFromEntry(entry SessionTreeEntry) *ai.Message {
	ts := TimestampFromISO(entry.Timestamp)
	switch entry.Type {
	case "message":
		if entry.Message != nil {
			m := *entry.Message
			return &m
		}
	case "custom_message":
		if entry.Content != nil {
			return &ai.Message{
				Role:      RoleCustom,
				Content:   entry.Content,
				Timestamp: ts,
			}
		}
	case "branch_summary":
		if entry.Summary != "" && entry.FromID != "" {
			msg := NewBranchSummaryMessage(entry.Summary, entry.FromID, ts)
			return &msg
		}
	case "compaction":
		if entry.Summary != "" {
			msg := NewCompactionSummaryMessage(entry.Summary, entry.TokensBefore, ts)
			return &msg
		}
	}
	return nil
}

// GetMessageFromEntryForCompaction is like GetMessageFromEntry but skips compaction entries.
func GetMessageFromEntryForCompaction(entry SessionTreeEntry) *ai.Message {
	if entry.Type == "compaction" {
		return nil
	}
	return GetMessageFromEntry(entry)
}
