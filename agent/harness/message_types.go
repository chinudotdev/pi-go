package harness

import (
	"encoding/json"
	"fmt"
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
