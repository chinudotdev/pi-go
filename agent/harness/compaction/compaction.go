package compaction

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/chinudotdev/pi-go/agent/harness"
	"github.com/chinudotdev/pi-go/agent/harness/session"
	"github.com/chinudotdev/pi-go/ai"
)

// CompactionResult holds generated compaction data ready to be persisted.
type CompactionResult struct {
	Summary          string `json:"summary"`
	FirstKeptEntryID string `json:"firstKeptEntryId"`
	TokensBefore     int    `json:"tokensBefore"`
	Details          any    `json:"details,omitempty"`
}

// CutPointResult describes the selected compaction cut point.
type CutPointResult struct {
	FirstKeptEntryIndex int  // Index of first entry retained after compaction
	TurnStartIndex      int  // Index of turn-start entry when cut splits a turn, else -1
	IsSplitTurn         bool // Whether the cut point splits an in-progress turn
}

// FindValidCutPoints returns indices of entries that are valid compaction cut points.
func FindValidCutPoints(entries []harness.SessionTreeEntry, startIndex, endIndex int) []int {
	var cutPoints []int
	for i := startIndex; i < endIndex; i++ {
		entry := entries[i]
		switch entry.Type {
		case "message":
			role := ""
			if entry.Message != nil {
				role = entry.Message.Role
			}
			switch role {
			case "user", "assistant", "bashExecution", "custom", "branchSummary", "compactionSummary":
				cutPoints = append(cutPoints, i)
			}
		case "branch_summary", "custom_message":
			cutPoints = append(cutPoints, i)
		}
	}
	return cutPoints
}

// FindTurnStartIndex finds the user-visible message that starts the turn containing an entry.
func FindTurnStartIndex(entries []harness.SessionTreeEntry, entryIndex, startIndex int) int {
	for i := entryIndex; i >= startIndex; i-- {
		entry := entries[i]
		if entry.Type == "branch_summary" || entry.Type == "custom_message" {
			return i
		}
		if entry.Type == "message" && entry.Message != nil {
			role := entry.Message.Role
			if role == "user" || role == "bashExecution" {
				return i
			}
		}
	}
	return -1
}

// FindCutPoint finds the compaction cut point that keeps approximately the requested recent-token budget.
func FindCutPoint(
	entries []harness.SessionTreeEntry,
	startIndex, endIndex int,
	keepRecentTokens int,
) CutPointResult {
	cutPoints := FindValidCutPoints(entries, startIndex, endIndex)
	if len(cutPoints) == 0 {
		return CutPointResult{FirstKeptEntryIndex: startIndex, TurnStartIndex: -1, IsSplitTurn: false}
	}

	accumulatedTokens := 0
	cutIndex := cutPoints[0]

	for i := endIndex - 1; i >= startIndex; i-- {
		entry := entries[i]
		if entry.Type != "message" || entry.Message == nil {
			continue
		}
		messageTokens := EstimateTokens(*entry.Message)
		accumulatedTokens += messageTokens
		if accumulatedTokens >= keepRecentTokens {
			for _, cp := range cutPoints {
				if cp >= i {
					cutIndex = cp
					break
				}
			}
			break
		}
	}

	// Walk back past non-message, non-compaction entries
	for cutIndex > startIndex {
		prevEntry := entries[cutIndex-1]
		if prevEntry.Type == "compaction" || prevEntry.Type == "message" {
			break
		}
		cutIndex--
	}

	cutEntry := entries[cutIndex]
	isUserMessage := cutEntry.Type == "message" && cutEntry.Message != nil && cutEntry.Message.Role == "user"
	turnStartIndex := -1
	if !isUserMessage {
		turnStartIndex = FindTurnStartIndex(entries, cutIndex, startIndex)
	}

	return CutPointResult{
		FirstKeptEntryIndex: cutIndex,
		TurnStartIndex:      turnStartIndex,
		IsSplitTurn:         !isUserMessage && turnStartIndex != -1,
	}
}

// Summarization prompts.
const (
	SummarizationSystemPrompt = `You are a context summarization assistant. Your task is to read a conversation between a user and an AI coding assistant, then produce a structured summary following the exact format specified.

Do NOT continue the conversation. Do NOT respond to any questions in the conversation. ONLY output the structured summary.`

	SummarizationPrompt = `The messages above are a conversation to summarize. Create a structured context checkpoint summary that another LLM will use to continue the work.

Use this EXACT format:

## Goal
[What is the user trying to accomplish? Can be multiple items if the session covers different tasks.]

## Constraints & Preferences
- [Any constraints, preferences, or requirements mentioned by user]
- [Or "(none)" if none were mentioned]

## Progress
### Done
- [x] [Completed tasks/changes]

### In Progress
- [ ] [Current work]

### Blocked
- [Issues preventing progress, if any]

## Key Decisions
- **[Decision]**: [Brief rationale]

## Next Steps
1. [Ordered list of what should happen next]

## Critical Context
- [Any data, examples, or references needed to continue]
- [Or "(none)" if not applicable]

Keep each section concise. Preserve exact file paths, function names, and error messages.`

	UpdateSummarizationPrompt = `The messages above are NEW conversation messages to incorporate into the existing summary provided in <previous-summary> tags.

Update the existing structured summary with new information. RULES:
- PRESERVE all existing information from the previous summary
- ADD new progress, decisions, and context from the new messages
- UPDATE the Progress section: move items from "In Progress" to "Done" when completed
- UPDATE "Next Steps" based on what was accomplished
- PRESERVE exact file paths, function names, and error messages
- If something is no longer relevant, you may remove it

Use this EXACT format:

## Goal
[Preserve existing goals, add new ones if the task expanded]

## Constraints & Preferences
- [Preserve existing, add new ones discovered]

## Progress
### Done
- [x] [Include previously done items AND newly completed items]

### In Progress
- [ ] [Current work - update based on progress]

### Blocked
- [Current blockers - remove if resolved]

## Key Decisions
- **[Decision]**: [Brief rationale] (preserve all previous, add new)

## Next Steps
1. [Update based on current state]

## Critical Context
- [Preserve important context, add new if needed]

Keep each section concise. Preserve exact file paths, function names, and error messages.`

	TurnPrefixSummarizationPrompt = `This is the PREFIX of a turn that was too large to keep. The SUFFIX (recent work) is retained.

Summarize the prefix to provide context for the retained suffix:

## Original Request
[What did the user ask for in this turn?]

## Early Progress
- [Key decisions and work done in the prefix]

## Context for Suffix
- [Information needed to understand the retained recent work]

Be concise. Focus on what's needed to understand the kept suffix.`
)

// GenerateSummary generates or updates a conversation summary for compaction.
func GenerateSummary(
	ctx context.Context,
	currentMessages []ai.Message,
	model ai.Model,
	reserveTokens int,
	apiKey string,
	headers map[string]string,
	customInstructions string,
	previousSummary string,
	thinkingLevel string,
) (string, error) {
	maxTokens := int(math.Min(
		math.Floor(0.8*float64(reserveTokens)),
		float64(safeMaxTokens(model)),
	))

	basePrompt := SummarizationPrompt
	if previousSummary != "" {
		basePrompt = UpdateSummarizationPrompt
	}
	if customInstructions != "" {
		basePrompt = basePrompt + "\n\nAdditional focus: " + customInstructions
	}

	llmMessages := harness.ConvertToLlm(currentMessages)
	conversationText := harness.SerializeConversation(llmMessages)

	promptText := "<conversation>\n" + conversationText + "\n</conversation>\n\n"
	if previousSummary != "" {
		promptText += "<previous-summary>\n" + previousSummary + "\n</previous-summary>\n\n"
	}
	promptText += basePrompt

	summarizationMessages := []ai.Message{
		{
			Role:      "user",
			Content:   promptText,
			Timestamp: time.Now().UnixMilli(),
		},
	}

	opts := &ai.SimpleStreamOptions{}
	opts.APIKey = &apiKey
	if len(headers) > 0 {
		opts.Headers = headers
	}
	if maxTokens > 0 {
		opts.MaxTokens = &maxTokens
	}
	if model.Reasoning && thinkingLevel != "" && thinkingLevel != "off" {
		opts.Reasoning = ai.ThinkingLevel(thinkingLevel)
	}

	convCtx := &ai.Context{
		SystemPrompt: &[]string{SummarizationSystemPrompt}[0],
		Messages:     summarizationMessages,
	}

	response, err := ai.CompleteSimple(ctx, &model, convCtx, opts)
	if err != nil {
		return "", harness.NewCompactionError(harness.CompactionErrorSummarizationFailed, "Summarization failed: "+err.Error(), nil)
	}

	if response.StopReason == "aborted" {
		return "", harness.NewCompactionError(harness.CompactionErrorAborted, derefStr(response.ErrorMessage, "Summarization aborted"), nil)
	}
	if response.StopReason == "error" {
		return "", harness.NewCompactionError(harness.CompactionErrorSummarizationFailed, "Summarization failed: "+derefStr(response.ErrorMessage, "Unknown error"), nil)
	}

	var texts []string
	for _, block := range response.Content {
		if block.Type == "text" {
			texts = append(texts, block.Text)
		}
	}
	return joinTexts(texts), nil
}

// PrepareCompaction prepares session entries for compaction.
// Returns nil if compaction is not applicable (empty entries or last entry is already a compaction).
func PrepareCompaction(
	pathEntries []harness.SessionTreeEntry,
	settings harness.CompactionSettings,
) (*harness.CompactionPreparation, error) {
	if len(pathEntries) == 0 || pathEntries[len(pathEntries)-1].Type == "compaction" {
		return nil, nil
	}

	prevCompactionIndex := -1
	for i := len(pathEntries) - 1; i >= 0; i-- {
		if pathEntries[i].Type == "compaction" {
			prevCompactionIndex = i
			break
		}
	}

	var previousSummary string
	boundaryStart := 0
	if prevCompactionIndex >= 0 {
		previousSummary = pathEntries[prevCompactionIndex].Summary
		firstKeptEntryIndex := -1
		for i, entry := range pathEntries {
			if entry.ID == pathEntries[prevCompactionIndex].FirstKeptEntryID {
				firstKeptEntryIndex = i
				break
			}
		}
		if firstKeptEntryIndex >= 0 {
			boundaryStart = firstKeptEntryIndex
		} else {
			boundaryStart = prevCompactionIndex + 1
		}
	}
	boundaryEnd := len(pathEntries)

	sctx := session.BuildSessionContext(pathEntries)
	tokensBefore := EstimateContextTokens(sctx.Messages).Tokens

	cutPoint := FindCutPoint(pathEntries, boundaryStart, boundaryEnd, settings.KeepRecentTokens)
	firstKeptEntry := pathEntries[cutPoint.FirstKeptEntryIndex]
	if firstKeptEntry.ID == "" {
		return nil, harness.NewCompactionError(harness.CompactionErrorInvalidSession, "First kept entry has no UUID - session may need migration", nil)
	}
	firstKeptEntryID := firstKeptEntry.ID

	historyEnd := cutPoint.FirstKeptEntryIndex
	if cutPoint.IsSplitTurn {
		historyEnd = cutPoint.TurnStartIndex
	}

	var messagesToSummarize []ai.Message
	for i := boundaryStart; i < historyEnd; i++ {
		msg := harness.GetMessageFromEntryForCompaction(pathEntries[i])
		if msg != nil {
			messagesToSummarize = append(messagesToSummarize, *msg)
		}
	}

	var turnPrefixMessages []ai.Message
	if cutPoint.IsSplitTurn {
		for i := cutPoint.TurnStartIndex; i < cutPoint.FirstKeptEntryIndex; i++ {
			msg := harness.GetMessageFromEntryForCompaction(pathEntries[i])
			if msg != nil {
				turnPrefixMessages = append(turnPrefixMessages, *msg)
			}
		}
	}

	fileOps := ExtractFileOperations(messagesToSummarize, pathEntries, prevCompactionIndex)
	if cutPoint.IsSplitTurn {
		for _, msg := range turnPrefixMessages {
			ExtractFileOpsFromMessage(msg, &fileOps)
		}
	}

	return &harness.CompactionPreparation{
		FirstKeptEntryID:    firstKeptEntryID,
		MessagesToSummarize: messagesToSummarize,
		TurnPrefixMessages:  turnPrefixMessages,
		IsSplitTurn:         cutPoint.IsSplitTurn,
		TokensBefore:        tokensBefore,
		PreviousSummary:     previousSummary,
		FileOps:             fileOps,
		Settings:            settings,
	}, nil
}

// Compact generates compaction summary data from prepared session history.
func Compact(
	ctx context.Context,
	preparation harness.CompactionPreparation,
	model ai.Model,
	apiKey string,
	headers map[string]string,
	customInstructions string,
	thinkingLevel string,
) (*CompactionResult, error) {
	firstKeptEntryID := preparation.FirstKeptEntryID
	if firstKeptEntryID == "" {
		return nil, harness.NewCompactionError(harness.CompactionErrorInvalidSession, "First kept entry has no UUID", nil)
	}

	var summary string

	if preparation.IsSplitTurn && len(preparation.TurnPrefixMessages) > 0 {
		// Generate history summary
		var histText string
		var err error
		if len(preparation.MessagesToSummarize) > 0 {
			histText, err = GenerateSummary(ctx, preparation.MessagesToSummarize, model,
				preparation.Settings.ReserveTokens, apiKey, headers,
				customInstructions, preparation.PreviousSummary, thinkingLevel)
			if err != nil {
				return nil, err
			}
		} else {
			histText = "No prior history."
		}

		// Generate turn prefix summary
		turnText, err := generateTurnPrefixSummary(ctx, preparation.TurnPrefixMessages, model,
			preparation.Settings.ReserveTokens, apiKey, headers, thinkingLevel)
		if err != nil {
			return nil, err
		}

		summary = histText + "\n\n---\n\n**Turn Context (split turn):**\n\n" + turnText
	} else {
		text, err := GenerateSummary(ctx, preparation.MessagesToSummarize, model,
			preparation.Settings.ReserveTokens, apiKey, headers,
			customInstructions, preparation.PreviousSummary, thinkingLevel)
		if err != nil {
			return nil, err
		}
		summary = text
	}

	readFiles, modifiedFiles := ComputeFileLists(preparation.FileOps)
	summary += FormatFileOperations(readFiles, modifiedFiles)

	return &CompactionResult{
		Summary:          summary,
		FirstKeptEntryID: firstKeptEntryID,
		TokensBefore:     preparation.TokensBefore,
		Details:          CompactionDetails{ReadFiles: readFiles, ModifiedFiles: modifiedFiles},
	}, nil
}

func generateTurnPrefixSummary(
	ctx context.Context,
	messages []ai.Message,
	model ai.Model,
	reserveTokens int,
	apiKey string,
	headers map[string]string,
	thinkingLevel string,
) (string, error) {
	maxTokens := int(math.Min(
		math.Floor(0.5*float64(reserveTokens)),
		float64(safeMaxTokens(model)),
	))

	llmMessages := harness.ConvertToLlm(messages)
	conversationText := harness.SerializeConversation(llmMessages)
	promptText := "<conversation>\n" + conversationText + "\n</conversation>\n\n" + TurnPrefixSummarizationPrompt

	summarizationMessages := []ai.Message{
		{
			Role:      "user",
			Content:   promptText,
			Timestamp: time.Now().UnixMilli(),
		},
	}

	opts := &ai.SimpleStreamOptions{}
	opts.APIKey = &apiKey
	if len(headers) > 0 {
		opts.Headers = headers
	}
	if maxTokens > 0 {
		opts.MaxTokens = &maxTokens
	}
	if model.Reasoning && thinkingLevel != "" && thinkingLevel != "off" {
		opts.Reasoning = ai.ThinkingLevel(thinkingLevel)
	}

	convCtx := &ai.Context{
		SystemPrompt: &[]string{SummarizationSystemPrompt}[0],
		Messages:     summarizationMessages,
	}

	response, err := ai.CompleteSimple(ctx, &model, convCtx, opts)
	if err != nil {
		return "", harness.NewCompactionError(harness.CompactionErrorSummarizationFailed, "Turn prefix summarization failed: "+err.Error(), nil)
	}
	if response.StopReason == "aborted" {
		return "", harness.NewCompactionError(harness.CompactionErrorAborted, derefStr(response.ErrorMessage, "Turn prefix summarization aborted"), nil)
	}
	if response.StopReason == "error" {
		return "", harness.NewCompactionError(harness.CompactionErrorSummarizationFailed, "Turn prefix summarization failed: "+derefStr(response.ErrorMessage, "Unknown error"), nil)
	}

	var texts []string
	for _, block := range response.Content {
		if block.Type == "text" {
			texts = append(texts, block.Text)
		}
	}
	return joinTexts(texts), nil
}

// helpers

func safeMaxTokens(model ai.Model) int {
	if model.MaxTokens > 0 {
		return model.MaxTokens
	}
	return 16384
}

func derefStr(s *string, fallback string) string {
	if s != nil {
		return *s
	}
	return fallback
}

func joinTexts(texts []string) string {
	result := ""
	for i, t := range texts {
		if i > 0 {
			result += "\n"
		}
		result += t
	}
	return result
}

// GetLastAssistantUsage returns usage from the last successful assistant message.
func GetLastAssistantUsage(entries []harness.SessionTreeEntry) *ai.Usage {
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if entry.Type == "message" && entry.Message != nil {
			msg := entry.Message
			if msg.Role == "assistant" && msg.StopReason != "aborted" && msg.StopReason != "error" {
				usage := msg.Usage
				if usage.TotalTokens > 0 || usage.Input > 0 {
					return &usage
				}
			}
		}
	}
	return nil
}

// GetModel returns the model info from the last assistant or model_change entry.
func GetModel(entries []harness.SessionTreeEntry) *harness.SessionModelRef {
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if entry.Type == "model_change" && entry.Provider != "" {
			return &harness.SessionModelRef{Provider: entry.Provider, ModelID: entry.ModelID}
		}
		if entry.Type == "message" && entry.Message != nil {
			msg := entry.Message
			if msg.Role == "assistant" && msg.Provider != "" {
				return &harness.SessionModelRef{Provider: string(msg.Provider), ModelID: msg.Model}
			}
		}
	}
	return nil
}

// FormatCompactionDetails creates a CompactionDetails from file lists.
func FormatCompactionDetails(fileOps FileOperations) CompactionDetails {
	readFiles, modifiedFiles := ComputeFileLists(fileOps)
	return CompactionDetails{ReadFiles: readFiles, ModifiedFiles: modifiedFiles}
}

// BuildSummaryWithFileOps appends file operation metadata to a summary string.
func BuildSummaryWithFileOps(summary string, fileOps FileOperations) string {
	readFiles, modifiedFiles := ComputeFileLists(fileOps)
	return summary + FormatFileOperations(readFiles, modifiedFiles)
}

// Extra safeguard for math
var _ = fmt.Sprintf
