package compaction

import (
	"context"
	"time"

	"github.com/chinudotdev/pi-go/agent/harness"
	"github.com/chinudotdev/pi-go/agent/harness/session"
	"github.com/chinudotdev/pi-go/ai"
)

// BranchSummaryDetails stores file-operation details on branch summary entries.
type BranchSummaryDetails struct {
	ReadFiles     []string `json:"readFiles,omitempty"`
	ModifiedFiles []string `json:"modifiedFiles,omitempty"`
}

// BranchPreparation holds prepared branch content for summarization.
type BranchPreparation struct {
	Messages    []ai.Message    // Messages selected for the branch summary
	FileOps     FileOperations  // File operations extracted from the branch
	TotalTokens int             // Estimated token count for selected messages
}

// CollectEntriesResult holds entries selected for branch summarization.
type CollectEntriesResult struct {
	Entries         []harness.SessionTreeEntry // Entries to summarize in chronological order
	CommonAncestorID *string                   // Deepest common ancestor between old leaf and target
}

// GenerateBranchSummaryOptions holds options for generating a branch summary.
type GenerateBranchSummaryOptions struct {
	Model              ai.Model        // Model used for summarization
	APIKey             string          // API key forwarded to the provider
	Headers            map[string]string // Optional request headers
	CustomInstructions string          // Optional instructions appended to the prompt
	ReplaceInstructions bool           // Replace the default prompt instead of appending
	ReserveTokens      int             // Tokens reserved for prompt and output (default 16384)
}

// CollectEntriesForBranchSummary collects entries that should be summarized before navigating.
func CollectEntriesForBranchSummary(
	sess *session.Session,
	ctx context.Context,
	oldLeafID *string,
	targetID string,
) (*CollectEntriesResult, error) {
	if oldLeafID == nil {
		return &CollectEntriesResult{Entries: nil, CommonAncestorID: nil}, nil
	}

	oldBranch, err := sess.GetBranch(ctx, oldLeafID)
	if err != nil {
		return nil, err
	}
	oldPath := make(map[string]bool)
	for _, e := range oldBranch {
		oldPath[e.ID] = true
	}

	targetPath, err := sess.GetBranch(ctx, &targetID)
	if err != nil {
		return nil, err
	}

	var commonAncestorID *string
	for i := len(targetPath) - 1; i >= 0; i-- {
		if oldPath[targetPath[i].ID] {
			id := targetPath[i].ID
			commonAncestorID = &id
			break
		}
	}

	var entries []harness.SessionTreeEntry
	current := oldLeafID

	for current != nil && *current != derefStr(commonAncestorID, "") {
		entry, err := sess.GetEntry(ctx, *current)
		if err != nil {
			return nil, harness.NewSessionError(harness.SessionErrorInvalidSession, "Entry "+*current+" not found", err)
		}
		if entry == nil {
			break
		}
		entries = append(entries, *entry)
		if entry.ParentID == nil {
			break
		}
		current = entry.ParentID
	}

	// Reverse to get chronological order
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}

	return &CollectEntriesResult{Entries: entries, CommonAncestorID: commonAncestorID}, nil
}

// PrepareBranchEntries prepares branch entries for summarization within an optional token budget.
func PrepareBranchEntries(entries []harness.SessionTreeEntry, tokenBudget int) BranchPreparation {
	var messages []ai.Message
	fileOps := CreateFileOps()
	totalTokens := 0

	// First pass: accumulate file ops from branch summary details
	for _, entry := range entries {
		if entry.Type == "branch_summary" && !entry.FromHook && entry.Details != nil {
			switch d := entry.Details.(type) {
			case BranchSummaryDetails:
				for _, f := range d.ReadFiles {
					fileOps.Read[f] = true
				}
				for _, f := range d.ModifiedFiles {
					fileOps.Edited[f] = true
				}
			case *BranchSummaryDetails:
				for _, f := range d.ReadFiles {
					fileOps.Read[f] = true
				}
				for _, f := range d.ModifiedFiles {
					fileOps.Edited[f] = true
				}
			}
		}
	}

	// Second pass: collect messages from the end, respecting token budget
	for i := len(entries) - 1; i >= 0; i-- {
		msg := getBranchMessageFromEntry(entries[i])
		if msg == nil {
			continue
		}
		ExtractFileOpsFromMessage(*msg, &fileOps)

		tokens := EstimateTokens(*msg)
		if tokenBudget > 0 && totalTokens+tokens > tokenBudget {
			entry := entries[i]
			if entry.Type == "compaction" || entry.Type == "branch_summary" {
				if totalTokens < int(float64(tokenBudget)*0.9) {
					messages = prependMessage(messages, *msg)
					totalTokens += tokens
				}
			}
			break
		}

		messages = prependMessage(messages, *msg)
		totalTokens += tokens
	}

	return BranchPreparation{
		Messages:    messages,
		FileOps:     fileOps,
		TotalTokens: totalTokens,
	}
}

func getBranchMessageFromEntry(entry harness.SessionTreeEntry) *ai.Message {
	switch entry.Type {
	case "message":
		if entry.Message != nil && entry.Message.Role == "toolResult" {
			return nil
		}
		return harness.GetMessageFromEntry(entry)
	case "custom_message", "branch_summary", "compaction":
		return harness.GetMessageFromEntry(entry)
	default:
		return nil
	}
}

func prependMessage(messages []ai.Message, msg ai.Message) []ai.Message {
	return append([]ai.Message{msg}, messages...)
}

const (
	branchSummaryPreamble = "The user explored a different conversation branch before returning here.\nSummary of that exploration:\n\n"

	BranchSummaryPrompt = `Create a structured summary of this conversation branch for context when returning later.

Use this EXACT format:

## Goal
[What was the user trying to accomplish in this branch?]

## Constraints & Preferences
- [Any constraints, preferences, or requirements mentioned]
- [Or "(none)" if none were mentioned]

## Progress
### Done
- [x] [Completed tasks/changes]

### In Progress
- [ ] [Work that was started but not finished]

### Blocked
- [Issues preventing progress, if any]

## Key Decisions
- **[Decision]**: [Brief rationale]

## Next Steps
1. [What should happen next to continue this work]

Keep each section concise. Preserve exact file paths, function names, and error messages.`
)

// GenerateBranchSummary generates a summary for abandoned branch entries.
func GenerateBranchSummary(
	ctx context.Context,
	entries []harness.SessionTreeEntry,
	options GenerateBranchSummaryOptions,
) (*harness.BranchSummaryResult, error) {
	reserveTokens := options.ReserveTokens
	if reserveTokens == 0 {
		reserveTokens = 16384
	}

	contextWindow := options.Model.ContextWindow
	if contextWindow == 0 {
		contextWindow = 128000
	}
	tokenBudget := contextWindow - reserveTokens

	prep := PrepareBranchEntries(entries, tokenBudget)
	if len(prep.Messages) == 0 {
		return &harness.BranchSummaryResult{
			Summary:       "No content to summarize",
			ReadFiles:     nil,
			ModifiedFiles: nil,
		}, nil
	}

	llmMessages := harness.ConvertToLlm(prep.Messages)
	conversationText := harness.SerializeConversation(llmMessages)

	instructions := BranchSummaryPrompt
	if options.ReplaceInstructions && options.CustomInstructions != "" {
		instructions = options.CustomInstructions
	} else if options.CustomInstructions != "" {
		instructions = instructions + "\n\nAdditional focus: " + options.CustomInstructions
	}

	promptText := "<conversation>\n" + conversationText + "\n</conversation>\n\n" + instructions

	summarizationMessages := []ai.Message{
		{
			Role:      "user",
			Content:   promptText,
			Timestamp: time.Now().UnixMilli(),
		},
	}

	maxTokens := 2048
	opts := &ai.SimpleStreamOptions{}
	opts.APIKey = &options.APIKey
	opts.MaxTokens = &maxTokens
	if len(options.Headers) > 0 {
		opts.Headers = options.Headers
	}

	systemPrompt := SummarizationSystemPrompt
	convCtx := &ai.Context{
		SystemPrompt: &systemPrompt,
		Messages:     summarizationMessages,
	}

	response, err := ai.CompleteSimple(ctx, &options.Model, convCtx, opts)
	if err != nil {
		return nil, harness.NewBranchSummaryError(harness.BranchSummaryErrorSummarizationFailed,
			"Branch summary failed: "+err.Error(), err)
	}
	if response.StopReason == "aborted" {
		return nil, harness.NewBranchSummaryError(harness.BranchSummaryErrorAborted,
			derefStr(response.ErrorMessage, "Branch summary aborted"), nil)
	}
	if response.StopReason == "error" {
		return nil, harness.NewBranchSummaryError(harness.BranchSummaryErrorSummarizationFailed,
			"Branch summary failed: "+derefStr(response.ErrorMessage, "Unknown error"), nil)
	}

	var texts []string
	for _, block := range response.Content {
		if block.Type == "text" {
			texts = append(texts, block.Text)
		}
	}
	summary := branchSummaryPreamble + joinTexts(texts)
	readFiles, modifiedFiles := ComputeFileLists(prep.FileOps)
	summary += FormatFileOperations(readFiles, modifiedFiles)

	if summary == branchSummaryPreamble {
		summary = "No summary generated"
	}

	return &harness.BranchSummaryResult{
		Summary:       summary,
		ReadFiles:     readFiles,
		ModifiedFiles: modifiedFiles,
	}, nil
}
