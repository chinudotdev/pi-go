package compaction

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/chinudotdev/pi-go/agent/harness"
	"github.com/chinudotdev/pi-go/ai"
)

// FileOperations is an alias for harness.FileOperations.
type FileOperations = harness.FileOperations

// CompactionDetails stores file-operation details on compaction entries.
type CompactionDetails struct {
	ReadFiles     []string `json:"readFiles,omitempty"`
	ModifiedFiles []string `json:"modifiedFiles,omitempty"`
}

// CreateFileOps creates an empty file-operation accumulator.
func CreateFileOps() FileOperations {
	return FileOperations{
		Read:    make(map[string]bool),
		Written: make(map[string]bool),
		Edited:  make(map[string]bool),
	}
}

// ExtractFileOpsFromMessage adds file operations from assistant tool calls to an accumulator.
func ExtractFileOpsFromMessage(msg ai.Message, fileOps *FileOperations) {
	if msg.Role != "assistant" {
		return
	}

	for _, block := range msg.AssistantContent {
		if block.Type != "toolCall" {
			continue
		}

		path, ok := block.ToolCallArguments["path"].(string)
		if !ok || path == "" {
			continue
		}

		switch block.ToolCallName {
		case "read":
			fileOps.Read[path] = true
		case "write":
			fileOps.Written[path] = true
		case "edit":
			fileOps.Edited[path] = true
		}
	}
}

// ComputeFileLists returns sorted read-only and modified file lists from accumulated operations.
func ComputeFileLists(fileOps FileOperations) (readFiles []string, modifiedFiles []string) {
	modified := make(map[string]bool)
	for f := range fileOps.Edited {
		modified[f] = true
	}
	for f := range fileOps.Written {
		modified[f] = true
	}

	for f := range fileOps.Read {
		if !modified[f] {
			readFiles = append(readFiles, f)
		}
	}
	sort.Strings(readFiles)

	for f := range modified {
		modifiedFiles = append(modifiedFiles, f)
	}
	sort.Strings(modifiedFiles)
	return
}

// FormatFileOperations formats file lists as summary metadata tags.
func FormatFileOperations(readFiles, modifiedFiles []string) string {
	var sections []string
	if len(readFiles) > 0 {
		sections = append(sections, "<read-files>\n"+strings.Join(readFiles, "\n")+"\n</read-files>")
	}
	if len(modifiedFiles) > 0 {
		sections = append(sections, "<modified-files>\n"+strings.Join(modifiedFiles, "\n")+"\n</modified-files>")
	}
	if len(sections) == 0 {
		return ""
	}
	return "\n\n" + strings.Join(sections, "\n\n")
}

// ExtractFileOperations extracts file ops from messages and previous compaction.
func ExtractFileOperations(
	messages []ai.Message,
	entries []harness.SessionTreeEntry,
	prevCompactionIndex int,
) FileOperations {
	fileOps := CreateFileOps()

	if prevCompactionIndex >= 0 {
		entry := entries[prevCompactionIndex]
		if !entry.FromHook && entry.Details != nil {
			switch d := entry.Details.(type) {
			case *CompactionDetails:
				for _, f := range d.ReadFiles {
					fileOps.Read[f] = true
				}
				for _, f := range d.ModifiedFiles {
					fileOps.Edited[f] = true
				}
			case CompactionDetails:
				for _, f := range d.ReadFiles {
					fileOps.Read[f] = true
				}
				for _, f := range d.ModifiedFiles {
					fileOps.Edited[f] = true
				}
			default:
				// Try JSON round-trip for map[string]any
				b, err := json.Marshal(entry.Details)
				if err == nil {
					var raw map[string]any
					if json.Unmarshal(b, &raw) == nil {
						if rf, ok := raw["readFiles"].([]any); ok {
							for _, f := range rf {
								if s, ok := f.(string); ok {
									fileOps.Read[s] = true
								}
							}
						}
						if mf, ok := raw["modifiedFiles"].([]any); ok {
							for _, f := range mf {
								if s, ok := f.(string); ok {
									fileOps.Edited[s] = true
								}
							}
						}
					}
				}
			}
		}
	}

	for _, msg := range messages {
		ExtractFileOpsFromMessage(msg, &fileOps)
	}

	return fileOps
}

// EstimateTokens estimates token count for one message using a conservative character heuristic.
// Uses ~4 chars per token.
func EstimateTokens(message ai.Message) int {
	chars := 0

	switch message.Role {
	case "user":
		chars = estimateContentChars(message.Content)
	case "assistant":
		for _, block := range message.AssistantContent {
			switch block.Type {
			case "text":
				chars += len(block.Text)
			case "thinking":
				chars += len(block.Thinking)
			case "toolCall":
				chars += len(block.ToolCallName) + len(harness.SafeJSONStringify(block.ToolCallArguments))
			}
		}
	case "toolResult":
		if message.ToolResultContent != nil {
			for _, block := range message.ToolResultContent {
				if block.Type == "text" {
					chars += len(block.Text)
				}
			}
		}
		chars += estimateContentChars(message.Content)
	default:
		chars = estimateContentChars(message.Content)
	}

	if chars == 0 {
		return 0
	}
	return (chars + 3) / 4
}

const estimatedImageChars = 4800

func estimateContentChars(content any) int {
	switch c := content.(type) {
	case string:
		return len(c)
	case []any:
		chars := 0
		for _, item := range c {
			if m, ok := item.(map[string]any); ok {
				if m["type"] == "text" {
					if t, ok := m["text"].(string); ok {
						chars += len(t)
					}
				} else if m["type"] == "image" {
					chars += estimatedImageChars
				}
			}
		}
		return chars
	case []ai.ContentBlock:
		chars := 0
		for _, b := range c {
			if b.Type == "text" {
				chars += len(b.Text)
			}
		}
		return chars
	default:
		return 0
	}
}

// ContextUsageEstimate holds the estimated context token usage.
type ContextUsageEstimate struct {
	Tokens         int  // Estimated total context tokens
	UsageTokens    int  // Tokens from last assistant usage block
	TrailingTokens int  // Tokens after the last usage block
	LastUsageIndex *int // Index of message with usage, nil if none
}

// CalculateContextTokens calculates total context tokens from provider usage.
func CalculateContextTokens(usage ai.Usage) int {
	total := usage.TotalTokens
	if total > 0 {
		return total
	}
	return usage.Input + usage.Output + usage.CacheRead + usage.CacheWrite
}

// EstimateContextTokens estimates context tokens for messages using provider usage when available.
func EstimateContextTokens(messages []ai.Message) ContextUsageEstimate {
	usageInfo := getLastAssistantUsageInfo(messages)

	if usageInfo == nil {
		estimated := 0
		for _, msg := range messages {
			estimated += EstimateTokens(msg)
		}
		return ContextUsageEstimate{
			Tokens:         estimated,
			UsageTokens:    0,
			TrailingTokens: estimated,
			LastUsageIndex: nil,
		}
	}

	usageTokens := CalculateContextTokens(usageInfo.usage)
	trailingTokens := 0
	for i := usageInfo.index + 1; i < len(messages); i++ {
		trailingTokens += EstimateTokens(messages[i])
	}

	return ContextUsageEstimate{
		Tokens:         usageTokens + trailingTokens,
		UsageTokens:    usageTokens,
		TrailingTokens: trailingTokens,
		LastUsageIndex: &usageInfo.index,
	}
}

type usageInfo struct {
	usage ai.Usage
	index int
}

func getLastAssistantUsageInfo(messages []ai.Message) *usageInfo {
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role == "assistant" && msg.StopReason != "aborted" && msg.StopReason != "error" {
			usage := msg.Usage
			if usage.TotalTokens > 0 || usage.Input > 0 {
				return &usageInfo{usage: usage, index: i}
			}
		}
	}
	return nil
}

// ShouldCompact returns whether context usage exceeds the compaction threshold.
func ShouldCompact(contextTokens, contextWindow int, settings harness.CompactionSettings) bool {
	if !settings.Enabled {
		return false
	}
	return contextTokens > contextWindow-settings.ReserveTokens
}
