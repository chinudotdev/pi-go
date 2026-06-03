// Package messages provides coding-agent-specific message conversions.
// It provides a ConvertToLlm function that handles the coding-agent custom
// message types alongside standard ai.Messages.
package messages

import (
	"fmt"
	"strings"

	"github.com/chinudotdev/pi-go/ai"
)

// Compaction/branch summary delimiters.
const (
	CompactionSummaryPrefix = "The conversation history before this point was compacted into the following summary:\n\n<summary>\n"
	CompactionSummarySuffix = "\n</summary>"
	BranchSummaryPrefix    = "The following is a summary of a branch that this conversation came back from:\n\n<summary>\n"
	BranchSummarySuffix    = "</summary>"
)

// Custom role types for coding-agent messages.
// These are stored in ai.Message.Role alongside standard roles.
const (
	RoleBashExecution     = "bashExecution"
	RoleCustom            = "custom"
	RoleBranchSummary     = "branchSummary"
	RoleCompactionSummary = "compactionSummary"
)

// BashExecutionDetails holds bash execution metadata stored in ai.Message.Details.
type BashExecutionDetails struct {
	Command        string `json:"command"`
	Output         string `json:"output"`
	ExitCode       int    `json:"exitCode"`
	Cancelled      bool   `json:"cancelled"`
	Truncated      bool   `json:"truncated"`
	FullOutputPath string `json:"fullOutputPath,omitempty"`
	ExcludeFromCtx bool   `json:"excludeFromContext,omitempty"`
}

// BranchSummaryDetails holds branch summary metadata stored in ai.Message.Details.
type BranchSummaryDetails struct {
	Summary string `json:"summary"`
	FromID  string `json:"fromId"`
}

// CompactionSummaryDetails holds compaction summary metadata.
type CompactionSummaryDetails struct {
	Summary      string `json:"summary"`
	TokensBefore int    `json:"tokensBefore"`
}

// CustomMessageDetails holds custom message metadata.
type CustomMessageDetails struct {
	CustomType string `json:"customType"`
	Display    bool   `json:"display"`
}

// ConvertToLlm converts coding-agent messages to LLM-compatible messages.
// Messages with custom roles (bashExecution, custom, branchSummary, compactionSummary)
// are converted to user messages with appropriate formatting.
// Standard messages (user, assistant, toolResult) are passed through.
func ConvertToLlm(messages []ai.Message) []ai.Message {
	var result []ai.Message
	for _, m := range messages {
		msg := convertOne(m)
		if msg != nil {
			result = append(result, *msg)
		}
	}
	return result
}

func convertOne(m ai.Message) *ai.Message {
	switch m.Role {
	case RoleBashExecution:
		details, ok := m.Details.(*BashExecutionDetails)
		if !ok {
			return nil
		}
		if details.ExcludeFromCtx {
			return nil
		}
		return &ai.Message{
			Role:      "user",
			Content:   []ai.ContentBlock{ai.NewTextContent(bashExecutionToText(details))},
			Timestamp: m.Timestamp,
		}

	case RoleCustom:
		return &ai.Message{
			Role:      "user",
			Content:   m.Content,
			Timestamp: m.Timestamp,
		}

	case RoleBranchSummary:
		details, ok := m.Details.(*BranchSummaryDetails)
		if !ok {
			return nil
		}
		text := BranchSummaryPrefix + details.Summary + BranchSummarySuffix
		return &ai.Message{
			Role:      "user",
			Content:   []ai.ContentBlock{ai.NewTextContent(text)},
			Timestamp: m.Timestamp,
		}

	case RoleCompactionSummary:
		details, ok := m.Details.(*CompactionSummaryDetails)
		if !ok {
			return nil
		}
		text := CompactionSummaryPrefix + details.Summary + CompactionSummarySuffix
		return &ai.Message{
			Role:      "user",
			Content:   []ai.ContentBlock{ai.NewTextContent(text)},
			Timestamp: m.Timestamp,
		}

	case "user", "assistant", "toolResult":
		return &m

	default:
		return nil
	}
}

// bashExecutionToText converts a BashExecutionDetails to user message text.
func bashExecutionToText(details *BashExecutionDetails) string {
	var text strings.Builder
	fmt.Fprintf(&text, "Ran `%s`\n", details.Command)
	if details.Output != "" {
		fmt.Fprintf(&text, "```\n%s\n```", details.Output)
	} else {
		text.WriteString("(no output)")
	}
	if details.Cancelled {
		text.WriteString("\n\n(command cancelled)")
	} else if details.ExitCode != 0 {
		fmt.Fprintf(&text, "\n\nCommand exited with code %d", details.ExitCode)
	}
	if details.Truncated && details.FullOutputPath != "" {
		fmt.Fprintf(&text, "\n\n[Output truncated. Full output: %s]", details.FullOutputPath)
	}
	return text.String()
}

// NewBashExecutionMessage creates a new bash execution ai.Message.
func NewBashExecutionMessage(command, output string, exitCode int, cancelled, truncated bool, fullPath string, ts int64) ai.Message {
	return ai.Message{
		Role: RoleBashExecution,
		Details: &BashExecutionDetails{
			Command:        command,
			Output:         output,
			ExitCode:       exitCode,
			Cancelled:      cancelled,
			Truncated:      truncated,
			FullOutputPath: fullPath,
		},
		Timestamp: ts,
	}
}

// NewBranchSummaryMessage creates a new branch summary ai.Message.
func NewBranchSummaryMessage(summary, fromID string, ts int64) ai.Message {
	return ai.Message{
		Role: RoleBranchSummary,
		Details: &BranchSummaryDetails{
			Summary: summary,
			FromID:  fromID,
		},
		Timestamp: ts,
	}
}

// NewCompactionSummaryMessage creates a new compaction summary ai.Message.
func NewCompactionSummaryMessage(summary string, tokensBefore int, ts int64) ai.Message {
	return ai.Message{
		Role: RoleCompactionSummary,
		Details: &CompactionSummaryDetails{
			Summary:      summary,
			TokensBefore: tokensBefore,
		},
		Timestamp: ts,
	}
}

// NewCustomMessage creates a new custom ai.Message.
func NewCustomMessage(customType string, content interface{}, display bool, ts int64) ai.Message {
	return ai.Message{
		Role:    RoleCustom,
		Content: content,
		Details: &CustomMessageDetails{
			CustomType: customType,
			Display:    display,
		},
		Timestamp: ts,
	}
}
