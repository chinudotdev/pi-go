package harness

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chinudotdev/pi-go/ai"
)

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
		pairs = append(pairs, k+"="+jsonStringify(v))
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

// jsonStringify is a local helper for FormatArgs.
func jsonStringify(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "[unserializable]"
	}
	return string(b)
}
