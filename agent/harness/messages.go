package harness

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/chinudotdev/pi-go/ai"
)

// Message role constants for custom messages stored in ai.Message.Role.
const (
	RoleBashExecution     = "bashExecution"
	RoleCustom            = "custom"
	RoleBranchSummary     = "branchSummary"
	RoleCompactionSummary = "compactionSummary"
)

// Compaction/branch summary prefix/suffix constants.
const (
	CompactionSummaryPrefix = "The conversation history before this point was compacted into the following summary:\n\n<summary>\n"
	CompactionSummarySuffix = "\n</summary>"
	BranchSummaryPrefix     = "The following is a summary of a branch that this conversation came back from:\n\n<summary>\n"
	BranchSummarySuffix     = "</summary>"
)

// BashExecutionPayload is stored in ai.Message.Content for bash execution messages.
type BashExecutionPayload struct {
	Command        string `json:"command"`
	Output         string `json:"output"`
	ExitCode       *int   `json:"exitCode,omitempty"`
	Cancelled      bool   `json:"cancelled"`
	Truncated      bool   `json:"truncated"`
	FullOutputPath string `json:"fullOutputPath,omitempty"`
}

// CustomMessagePayload is stored in ai.Message.Details for custom messages.
type CustomMessagePayload struct {
	CustomType string `json:"customType"`
	Display    bool   `json:"display"`
}

// BranchSummaryPayload is stored in ai.Message.Content for branch summary messages.
type BranchSummaryPayload struct {
	Summary string `json:"summary"`
	FromID  string `json:"fromId"`
}

// CompactionSummaryPayload is stored in ai.Message.Content for compaction summary messages.
type CompactionSummaryPayload struct {
	Summary      string `json:"summary"`
	TokensBefore int    `json:"tokensBefore"`
}

// BashExecutionToText converts a bash execution payload to text.
func BashExecutionToText(payload BashExecutionPayload) string {
	text := fmt.Sprintf("Ran `%s`\n", payload.Command)
	if payload.Output != "" {
		text += "```\n" + payload.Output + "\n```"
	} else {
		text += "(no output)"
	}
	if payload.Cancelled {
		text += "\n\n(command cancelled)"
	} else if payload.ExitCode != nil && *payload.ExitCode != 0 {
		text += fmt.Sprintf("\n\nCommand exited with code %d", *payload.ExitCode)
	}
	if payload.Truncated && payload.FullOutputPath != "" {
		text += "\n\n[Output truncated. Full output: " + payload.FullOutputPath + "]"
	}
	return text
}

// NewBashExecutionMessage creates an ai.Message representing a bash execution.
func NewBashExecutionMessage(payload BashExecutionPayload, timestamp int64) ai.Message {
	return ai.Message{
		Role:      RoleBashExecution,
		Content:   payload,
		Timestamp: timestamp,
	}
}

// NewBranchSummaryMessage creates an ai.Message representing a branch summary.
func NewBranchSummaryMessage(summary, fromID string, timestamp int64) ai.Message {
	return ai.Message{
		Role:      RoleBranchSummary,
		Content:   BranchSummaryPayload{Summary: summary, FromID: fromID},
		Timestamp: timestamp,
	}
}

// NewCompactionSummaryMessage creates an ai.Message representing a compaction summary.
func NewCompactionSummaryMessage(summary string, tokensBefore int, timestamp int64) ai.Message {
	return ai.Message{
		Role:      RoleCompactionSummary,
		Content:   CompactionSummaryPayload{Summary: summary, TokensBefore: tokensBefore},
		Timestamp: timestamp,
	}
}

// NewCustomMessage creates an ai.Message representing a custom message.
func NewCustomMessage(customType string, content any, display bool, timestamp int64) ai.Message {
	return ai.Message{
		Role:      RoleCustom,
		Content:   content,
		Details:   CustomMessagePayload{CustomType: customType, Display: display},
		Timestamp: timestamp,
	}
}

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

// SafeJSONStringify converts a value to a JSON string, returning a fallback on error.
func SafeJSONStringify(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "[unserializable]"
	}
	return string(b)
}

// TruncateForSummary truncates text to maxChars with a truncation notice.
func TruncateForSummary(text string, maxChars int) string {
	if len(text) <= maxChars {
		return text
	}
	truncatedChars := len(text) - maxChars
	return fmt.Sprintf("%s\n\n[... %d more characters truncated]", text[:maxChars], truncatedChars)
}

// TimestampFromISO converts an ISO timestamp string to milliseconds.
func TimestampFromISO(ts string) int64 {
	if ts == "" {
		return 0
	}
	for _, f := range []string{
		time.RFC3339Nano,
		"2006-01-02T15:04:05.000Z07:00",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05Z",
		time.RFC3339,
	} {
		if t, err := time.Parse(f, ts); err == nil {
			return t.UnixMilli()
		}
	}
	return 0
}

// SerializeConversation converts LLM messages to plain text for summarization prompts.
func SerializeConversation(messages []ai.Message) string {
	var parts []string

	for _, msg := range messages {
		switch msg.Role {
		case "user":
			content := UserContentToString(msg)
			if content != "" {
				parts = append(parts, "[User]: "+content)
			}
		case "assistant":
			textParts, thinkingParts, toolCalls := ExtractAssistantParts(msg)
			if len(thinkingParts) > 0 {
				parts = append(parts, "[Assistant thinking]: "+strings.Join(thinkingParts, "\n"))
			}
			if len(textParts) > 0 {
				parts = append(parts, "[Assistant]: "+strings.Join(textParts, "\n"))
			}
			if len(toolCalls) > 0 {
				parts = append(parts, "[Assistant tool calls]: "+strings.Join(toolCalls, "; "))
			}
		case "toolResult":
			content := ToolResultContentToString(msg)
			if content != "" {
				parts = append(parts, "[Tool result]: "+TruncateForSummary(content, 2000))
			}
		}
	}

	return strings.Join(parts, "\n\n")
}

// UserContentToString extracts text content from a user message.
func UserContentToString(msg ai.Message) string {
	switch c := msg.Content.(type) {
	case string:
		return c
	case []any:
		var texts []string
		for _, item := range c {
			if m, ok := item.(map[string]any); ok {
				if m["type"] == "text" {
					if t, ok := m["text"].(string); ok {
						texts = append(texts, t)
					}
				}
			}
		}
		return strings.Join(texts, "")
	case []ai.ContentBlock:
		var texts []string
		for _, b := range c {
			if b.Type == "text" {
				texts = append(texts, b.Text)
			}
		}
		return strings.Join(texts, "")
	}
	return ""
}

// ExtractAssistantParts extracts text, thinking, and tool call parts from an assistant message.
func ExtractAssistantParts(msg ai.Message) (texts []string, thinking []string, toolCalls []string) {
	for _, block := range msg.AssistantContent {
		switch block.Type {
		case "text":
			texts = append(texts, block.Text)
		case "thinking":
			thinking = append(thinking, block.Thinking)
		case "toolCall":
			argsStr := FormatArgs(block.ToolCallArguments)
			toolCalls = append(toolCalls, fmt.Sprintf("%s(%s)", block.ToolCallName, argsStr))
		}
	}
	return
}

// FormatArgs formats tool call arguments as key=value pairs.
func FormatArgs(args map[string]any) string {
	pairs := make([]string, 0, len(args))
	for k, v := range args {
		pairs = append(pairs, k+"="+SafeJSONStringify(v))
	}
	return strings.Join(pairs, ", ")
}

// ToolResultContentToString extracts text from a tool result message.
func ToolResultContentToString(msg ai.Message) string {
	if msg.ToolResultContent != nil {
		var texts []string
		for _, block := range msg.ToolResultContent {
			if block.Type == "text" {
				texts = append(texts, block.Text)
			}
		}
		return strings.Join(texts, "")
	}
	return ""
}
