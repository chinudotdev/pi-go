package ai

import "strings"

const (
	nonVisionUserImagePlaceholder = "(image omitted: model does not support images)"
	nonVisionToolImagePlaceholder = "(tool image omitted: model does not support images)"
)

// TransformMessages prepares messages for sending to a specific model's API.
// It handles:
//   - Downgrading images for non-vision models
//   - Normalizing thinking blocks across model switches
//   - Normalizing tool call IDs for cross-provider compatibility
//   - Inserting synthetic tool results for orphaned tool calls
func TransformMessages(messages []Message, model *Model, normalizeToolCallID func(id string, model *Model, source *AssistantMessage) string) []Message {
	toolCallIDMap := make(map[string]string)
	imageAware := downgradeUnsupportedImages(messages, model)

	// First pass: transform messages
	transformed := make([]Message, 0, len(imageAware))
	for _, msg := range imageAware {
		switch msg.Role {
		case "user":
			transformed = append(transformed, msg)

		case "toolResult":
			if normalizedID, ok := toolCallIDMap[msg.ToolCallID]; ok && normalizedID != msg.ToolCallID {
				clone := msg
				clone.ToolCallID = normalizedID
				transformed = append(transformed, clone)
			} else {
				transformed = append(transformed, msg)
			}

		case "assistant":
			t := transformAssistantMessage(msg, model, toolCallIDMap, normalizeToolCallID)
			transformed = append(transformed, t)
		}
	}

	// Second pass: insert synthetic empty tool results for orphaned tool calls
	return insertSyntheticToolResults(transformed)
}

func downgradeUnsupportedImages(messages []Message, model *Model) []Message {
	supportsImages := false
	for _, input := range model.Input {
		if input == "image" {
			supportsImages = true
			break
		}
	}
	if supportsImages {
		return messages
	}

	result := make([]Message, len(messages))
	for i, msg := range messages {
		switch msg.Role {
		case "user":
			if blocks, ok := msg.Content.([]ContentBlock); ok {
				result[i] = msg
				result[i].Content = replaceImagesWithPlaceholder(blocks, nonVisionUserImagePlaceholder)
			} else {
				result[i] = msg
			}
		case "toolResult":
			result[i] = msg
			result[i].ToolResultContent = replaceImagesWithPlaceholder(msg.ToolResultContent, nonVisionToolImagePlaceholder)
		default:
			result[i] = msg
		}
	}
	return result
}

func replaceImagesWithPlaceholder(blocks []ContentBlock, placeholder string) []ContentBlock {
	var result []ContentBlock
	previousWasPlaceholder := false

	for _, block := range blocks {
		if block.Type == "image" {
			if !previousWasPlaceholder {
				result = append(result, NewTextContent(placeholder))
			}
			previousWasPlaceholder = true
			continue
		}
		result = append(result, block)
		previousWasPlaceholder = block.Type == "text" && block.Text == placeholder
	}
	return result
}

func transformAssistantMessage(
	msg Message,
	model *Model,
	toolCallIDMap map[string]string,
	normalizeToolCallID func(id string, model *Model, source *AssistantMessage) string,
) Message {
	isSameModel := msg.Provider == model.Provider && msg.API == model.API && msg.Model == model.ID

	var newContent []ContentBlock
	for _, block := range msg.AssistantContent {
		switch block.Type {
		case "thinking":
			if block.Redacted && !isSameModel {
				continue
			}
			if isSameModel && block.ThinkingSignature != nil {
				newContent = append(newContent, block)
				continue
			}
			if strings.TrimSpace(block.Thinking) == "" {
				continue
			}
			if isSameModel {
				newContent = append(newContent, block)
				continue
			}
			newContent = append(newContent, NewTextContent(block.Thinking))

		case "text":
			newContent = append(newContent, block)

		case "toolCall":
			tc := block
			if !isSameModel && tc.ThoughtSignature != nil {
				tc.ThoughtSignature = nil
			}
			if !isSameModel && normalizeToolCallID != nil {
				asm := &AssistantMessage{Provider: msg.Provider, API: msg.API, Model: msg.Model}
				normalizedID := normalizeToolCallID(tc.ToolCallID, model, asm)
				if normalizedID != tc.ToolCallID {
					toolCallIDMap[tc.ToolCallID] = normalizedID
					tc.ToolCallID = normalizedID
				}
			}
			newContent = append(newContent, tc)
		}
	}

	clone := msg
	clone.AssistantContent = newContent
	return clone
}

func insertSyntheticToolResults(messages []Message) []Message {
	var result []Message
	var pendingToolCalls []ContentBlock
	existingToolResultIDs := make(map[string]bool)

	insertSynthetic := func() {
		for _, tc := range pendingToolCalls {
			if !existingToolResultIDs[tc.ToolCallID] {
				result = append(result, NewToolResultMessage(
					tc.ToolCallID,
					tc.ToolCallName,
					[]ContentBlock{NewTextContent("No result provided")},
					true,
				))
			}
		}
		pendingToolCalls = nil
		existingToolResultIDs = make(map[string]bool)
	}

	for _, msg := range messages {
		switch msg.Role {
		case "assistant":
			if msg.StopReason == StopReasonError || msg.StopReason == StopReasonAborted {
				continue
			}
			insertSynthetic()

			pendingToolCalls = nil
			for _, block := range msg.AssistantContent {
				if block.Type == "toolCall" {
					pendingToolCalls = append(pendingToolCalls, block)
				}
			}
			existingToolResultIDs = make(map[string]bool)
			result = append(result, msg)

		case "toolResult":
			existingToolResultIDs[msg.ToolCallID] = true
			result = append(result, msg)

		case "user":
			insertSynthetic()
			result = append(result, msg)
		}
	}

	insertSynthetic()
	return result
}
