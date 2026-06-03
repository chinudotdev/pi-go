package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"unicode"

	"github.com/chinudotdev/pi-go/agent"
	"github.com/chinudotdev/pi-go/ai"
)

// EditToolOptions configures the edit tool.
type EditToolOptions struct {
	Operations EditOperations
}

// EditOperations are pluggable operations for the edit tool.
type EditOperations interface {
	ReadFile(absolutePath string) ([]byte, error)
	WriteFile(absolutePath, content string) error
	Access(absolutePath string) error
}

type defaultEditOps struct{}

func (d defaultEditOps) ReadFile(p string) ([]byte, error) { return os.ReadFile(p) }
func (d defaultEditOps) WriteFile(p, content string) error {
	return os.WriteFile(p, []byte(content), 0644)
}
func (d defaultEditOps) Access(p string) error {
	_, err := os.Stat(p)
	return err
}

// Edit represents a single text replacement.
type Edit struct {
	OldText string `json:"oldText"`
	NewText string `json:"newText"`
}

// CreateEditTool creates the edit tool.
func CreateEditTool(cwd string, opts *EditToolOptions) *agent.Tool {
	if opts == nil {
		opts = &EditToolOptions{}
	}
	ops := opts.Operations
	if ops == nil {
		ops = defaultEditOps{}
	}

	return &agent.Tool{
		Name:        "edit",
		Label:       "edit",
		Description: ToolDescriptions[ToolEdit],
		Parameters:  ToolSchemas[ToolEdit],
		PrepareArguments: func(args map[string]any) map[string]any {
			return prepareEditArguments(args)
		},
		Execute: func(ctx context.Context, toolCallID string, params map[string]any, onUpdate agent.ToolUpdateCallback) (*agent.ToolResult, error) {
			pathVal, _ := params["path"].(string)
			if pathVal == "" {
				return nil, fmt.Errorf("path is required")
			}

			edits := parseEdits(params)
			if len(edits) == 0 {
				return nil, fmt.Errorf("edits must contain at least one replacement")
			}

			absolutePath := ResolveToCwd(pathVal, cwd)

			var result *agent.ToolResult
			err := WithFileMutationQueue(absolutePath, func() error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}

				// Check file exists
				if err := ops.Access(absolutePath); err != nil {
					return fmt.Errorf("could not edit file: %s: %w", pathVal, err)
				}

				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}

				// Read file
				data, err := ops.ReadFile(absolutePath)
				if err != nil {
					return fmt.Errorf("failed to read file: %w", err)
				}

				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}

				rawContent := string(data)
				bom, content := stripBOM(rawContent)
				originalEnding := detectLineEnding(content)
				normalizedContent := normalizeToLF(content)

				baseContent, newContent, err := applyEditsToNormalizedContent(normalizedContent, edits, pathVal)
				if err != nil {
					return err
				}

				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}

				finalContent := bom + restoreLineEndings(newContent, originalEnding)
				if err := ops.WriteFile(absolutePath, finalContent); err != nil {
					return fmt.Errorf("failed to write file: %w", err)
				}

				diff := generateDiffString(baseContent, newContent)
				patch := generateUnifiedPatch(pathVal, baseContent, newContent)

				result = &agent.ToolResult{
					Content: []ai.ContentBlock{
						newTextBlock(fmt.Sprintf("Successfully replaced %d block(s) in %s.", len(edits), pathVal)),
					},
					Details: map[string]any{
						"diff":  diff,
						"patch": patch,
					},
				}
				return nil
			})

			if err != nil {
				return nil, err
			}
			return result, nil
		},
	}
}

func parseEdits(params map[string]any) []Edit {
	// Try array form first
	if rawEdits, ok := params["edits"]; ok {
		switch v := rawEdits.(type) {
		case []any:
			var edits []Edit
			for _, item := range v {
				if m, ok := item.(map[string]any); ok {
					edit := Edit{}
					if old, ok := m["oldText"].(string); ok {
						edit.OldText = old
					}
					if new, ok := m["newText"].(string); ok {
						edit.NewText = new
					}
					if edit.OldText != "" {
						edits = append(edits, edit)
					}
				}
			}
			return edits
		case string:
			// Some models send edits as a JSON string
			var parsed []Edit
			if err := parseJSON(v, &parsed); err == nil && len(parsed) > 0 {
				return parsed
			}
		}
	}

	// Legacy single-edit form
	oldText, _ := params["oldText"].(string)
	newText, _ := params["newText"].(string)
	if oldText != "" {
		return []Edit{{OldText: oldText, NewText: newText}}
	}
	return nil
}

func prepareEditArguments(args map[string]any) map[string]any {
	if args == nil {
		return nil
	}

	// Some models send edits as a JSON string
	if raw, ok := args["edits"].(string); ok {
		var parsed []any
		if err := parseJSON(raw, &parsed); err == nil {
			args["edits"] = parsed
		}
	}

	// Legacy single-edit form
	oldText, hasOld := args["oldText"].(string)
	newText, hasNew := args["newText"].(string)
	if hasOld && hasNew {
		existing, ok := args["edits"].([]any)
		if !ok {
			existing = nil
		}
		existing = append(existing, map[string]any{"oldText": oldText, "newText": newText})
		args["edits"] = existing
		delete(args, "oldText")
		delete(args, "newText")
	}

	return args
}

// ============================================================================
// Edit-diff utilities (ported from edit-diff.ts)
// ============================================================================

func stripBOM(content string) (bom, text string) {
	if strings.HasPrefix(content, "\uFEFF") {
		return "\uFEFF", content[1:]
	}
	return "", content
}

func detectLineEnding(content string) string {
	crlfIdx := strings.Index(content, "\r\n")
	lfIdx := strings.Index(content, "\n")
	if lfIdx < 0 || crlfIdx < 0 || crlfIdx >= lfIdx {
		return "\n"
	}
	return "\r\n"
}

func normalizeToLF(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n")
}

func restoreLineEndings(text, ending string) string {
	if ending == "\r\n" {
		return strings.ReplaceAll(text, "\n", "\r\n")
	}
	return text
}

// normalizeForFuzzyMatch normalizes text for fuzzy matching.
func normalizeForFuzzyMatch(text string) string {
	// NFKC normalization - Go's unicode.NFKC
	result := strings.ToLower(text) // approximate: Go doesn't have NFKC in stdlib

	// Strip trailing whitespace per line
	lines := strings.Split(result, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRightFunc(line, unicode.IsSpace)
	}
	return strings.Join(lines, "\n")
}

type fuzzyMatchResult struct {
	found                 bool
	index                 int
	matchLength           int
	usedFuzzyMatch        bool
	contentForReplacement string
}

func fuzzyFindText(content, oldText string) fuzzyMatchResult {
	// Exact match first
	idx := strings.Index(content, oldText)
	if idx >= 0 {
		return fuzzyMatchResult{found: true, index: idx, matchLength: len(oldText), contentForReplacement: content}
	}

	// Fuzzy match
	fuzzyContent := normalizeForFuzzyMatch(content)
	fuzzyOldText := normalizeForFuzzyMatch(oldText)
	fuzzyIdx := strings.Index(fuzzyContent, fuzzyOldText)
	if fuzzyIdx < 0 {
		return fuzzyMatchResult{contentForReplacement: content}
	}

	return fuzzyMatchResult{
		found:                 true,
		index:                 fuzzyIdx,
		matchLength:           len(fuzzyOldText),
		usedFuzzyMatch:        true,
		contentForReplacement: fuzzyContent,
	}
}

type matchedEdit struct {
	editIndex   int
	matchIndex  int
	matchLength int
	newText     string
}

func applyEditsToNormalizedContent(normalizedContent string, edits []Edit, path string) (baseContent, newContent string, err error) {
	// Normalize edits
	normalizedEdits := make([]Edit, len(edits))
	for i, e := range edits {
		normalizedEdits[i] = Edit{
			OldText: normalizeToLF(e.OldText),
			NewText: normalizeToLF(e.NewText),
		}
	}

	// Validate
	for i, e := range normalizedEdits {
		if e.OldText == "" {
			if len(normalizedEdits) == 1 {
				return "", "", fmt.Errorf("oldText must not be empty in %s", path)
			}
			return "", "", fmt.Errorf("edits[%d].oldText must not be empty in %s", i, path)
		}
	}

	// Check for fuzzy match needs
	initialMatches := make([]fuzzyMatchResult, len(normalizedEdits))
	needsFuzzy := false
	for i, e := range normalizedEdits {
		initialMatches[i] = fuzzyFindText(normalizedContent, e.OldText)
		if initialMatches[i].usedFuzzyMatch {
			needsFuzzy = true
		}
	}

	base := normalizedContent
	if needsFuzzy {
		base = normalizeForFuzzyMatch(normalizedContent)
	}

	// Match all edits
	var matched []matchedEdit
	for i, e := range normalizedEdits {
		match := fuzzyFindText(base, e.OldText)
		if !match.found {
			if len(normalizedEdits) == 1 {
				return "", "", fmt.Errorf("could not find the exact text in %s. The old text must match exactly including all whitespace and newlines", path)
			}
			return "", "", fmt.Errorf("could not find edits[%d] in %s. The oldText must match exactly including all whitespace and newlines", i, path)
		}

		// Check uniqueness
		count := strings.Count(base, e.OldText)
		if count > 1 {
			if len(normalizedEdits) == 1 {
				return "", "", fmt.Errorf("found %d occurrences of the text in %s. The text must be unique", count, path)
			}
			return "", "", fmt.Errorf("found %d occurrences of edits[%d] in %s. Each oldText must be unique", count, i, path)
		}

		matched = append(matched, matchedEdit{
			editIndex:   i,
			matchIndex:  match.index,
			matchLength: match.matchLength,
			newText:     e.NewText,
		})
	}

	// Sort by position
	sort.Slice(matched, func(i, j int) bool {
		return matched[i].matchIndex < matched[j].matchIndex
	})

	// Check for overlaps
	for i := 1; i < len(matched); i++ {
		prev := matched[i-1]
		curr := matched[i]
		if prev.matchIndex+prev.matchLength > curr.matchIndex {
			return "", "", fmt.Errorf("edits[%d] and edits[%d] overlap in %s. Merge them into one edit or target disjoint regions", prev.editIndex, curr.editIndex, path)
		}
	}

	// Apply in reverse order
	result := base
	for i := len(matched) - 1; i >= 0; i-- {
		e := matched[i]
		result = result[:e.matchIndex] + e.newText + result[e.matchIndex+e.matchLength:]
	}

	if base == result {
		return "", "", fmt.Errorf("no changes made to %s. The replacement produced identical content", path)
	}

	return base, result, nil
}

// generateDiffString creates a display-oriented diff with line numbers.
func generateDiffString(oldContent, newContent string) string {
	// Simple line-based diff
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")

	if len(oldLines) > 0 && oldLines[len(oldLines)-1] == "" {
		oldLines = oldLines[:len(oldLines)-1]
	}
	if len(newLines) > 0 && newLines[len(newLines)-1] == "" {
		newLines = newLines[:len(newLines)-1]
	}

	// Compute a simple diff using LCS-like approach
	ops := computeDiffOps(oldLines, newLines)

	maxLineNum := len(oldLines)
	if len(newLines) > maxLineNum {
		maxLineNum = len(newLines)
	}
	lineNumWidth := len(fmt.Sprintf("%d", maxLineNum))

	var output []string
	oldLineNum := 1
	newLineNum := 1
	lastWasChange := false
	const contextLines = 4

	for i, op := range ops {
		switch op.kind {
		case "equal":
			nextIsChange := i+1 < len(ops) && ops[i+1].kind != "equal"
			hasLeading := lastWasChange
			hasTrailing := nextIsChange

			if hasLeading && hasTrailing {
				for _, line := range op.lines {
					output = append(output, fmt.Sprintf(" %*s %s", lineNumWidth, fmt.Sprintf("%d", oldLineNum), line))
					oldLineNum++
					newLineNum++
				}
			} else if hasLeading {
				shown := op.lines
				if len(shown) > contextLines {
					shown = shown[:contextLines]
				}
				for _, line := range shown {
					output = append(output, fmt.Sprintf(" %*s %s", lineNumWidth, fmt.Sprintf("%d", oldLineNum), line))
					oldLineNum++
					newLineNum++
				}
				skipped := len(op.lines) - len(shown)
				if skipped > 0 {
					output = append(output, fmt.Sprintf(" %*s ...", lineNumWidth, ""))
					oldLineNum += skipped
					newLineNum += skipped
				}
			} else if hasTrailing {
				skipped := len(op.lines) - contextLines
				if skipped > 0 {
					output = append(output, fmt.Sprintf(" %*s ...", lineNumWidth, ""))
					oldLineNum += skipped
					newLineNum += skipped
				}
				start := 0
				if skipped > 0 {
					start = skipped
				}
				for _, line := range op.lines[start:] {
					output = append(output, fmt.Sprintf(" %*s %s", lineNumWidth, fmt.Sprintf("%d", oldLineNum), line))
					oldLineNum++
					newLineNum++
				}
			} else {
				oldLineNum += len(op.lines)
				newLineNum += len(op.lines)
			}
			lastWasChange = false

		case "delete":
			for _, line := range op.lines {
				output = append(output, fmt.Sprintf("-%*s %s", lineNumWidth, fmt.Sprintf("%d", oldLineNum), line))
				oldLineNum++
			}
			lastWasChange = true

		case "insert":
			for _, line := range op.lines {
				output = append(output, fmt.Sprintf("+%*s %s", lineNumWidth, fmt.Sprintf("%d", newLineNum), line))
				newLineNum++
			}
			lastWasChange = true
		}
	}

	return strings.Join(output, "\n")
}

type diffOp struct {
	kind  string // "equal", "delete", "insert"
	lines []string
}

func computeDiffOps(oldLines, newLines []string) []diffOp {
	// Simple Myers-like diff using edit distance with path tracing
	// For practical file sizes this is fine
	m, n := len(oldLines), len(newLines)

	if m == 0 && n == 0 {
		return nil
	}
	if m == 0 {
		return []diffOp{{kind: "insert", lines: newLines}}
	}
	if n == 0 {
		return []diffOp{{kind: "delete", lines: oldLines}}
	}

	// Use a simple LCS-based approach
	lcs := computeLCS(oldLines, newLines)

	var ops []diffOp
	var equalBuf []string
	var flushEqual = func() {
		if len(equalBuf) > 0 {
			ops = append(ops, diffOp{kind: "equal", lines: append([]string{}, equalBuf...)})
			equalBuf = equalBuf[:0]
		}
	}

	oi, ni := 0, 0
	for _, lcsLine := range lcs {
		// Collect deletes
		var dels []string
		for oi < m && oldLines[oi] != lcsLine {
			dels = append(dels, oldLines[oi])
			oi++
		}
		// Collect inserts
		var ins []string
		for ni < n && newLines[ni] != lcsLine {
			ins = append(ins, newLines[ni])
			ni++
		}

		if len(dels) > 0 || len(ins) > 0 {
			flushEqual()
			if len(dels) > 0 {
				ops = append(ops, diffOp{kind: "delete", lines: dels})
			}
			if len(ins) > 0 {
				ops = append(ops, diffOp{kind: "insert", lines: ins})
			}
		}

		equalBuf = append(equalBuf, lcsLine)
		oi++
		ni++
	}

	// Remaining after LCS
	var dels []string
	for oi < m {
		dels = append(dels, oldLines[oi])
		oi++
	}
	var ins []string
	for ni < n {
		ins = append(ins, newLines[ni])
		ni++
	}
	if len(dels) > 0 || len(ins) > 0 {
		flushEqual()
		if len(dels) > 0 {
			ops = append(ops, diffOp{kind: "delete", lines: dels})
		}
		if len(ins) > 0 {
			ops = append(ops, diffOp{kind: "insert", lines: ins})
		}
	}
	flushEqual()

	return ops
}

func computeLCS(a, b []string) []string {
	m, n := len(a), len(b)
	if m == 0 || n == 0 {
		return nil
	}

	// DP table
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}

	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] > dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}

	// Backtrack
	var result []string
	i, j := m, n
	for i > 0 && j > 0 {
		if a[i-1] == b[j-1] {
			result = append([]string{a[i-1]}, result...)
			i--
			j--
		} else if dp[i-1][j] > dp[i][j-1] {
			i--
		} else {
			j--
		}
	}

	return result
}

func generateUnifiedPatch(path, oldContent, newContent string) string {
	diff := generateDiffString(oldContent, newContent)
	return fmt.Sprintf("--- %s\n+++ %s\n%s", path, path, diff)
}

// parseJSON parses a JSON string into v.
func parseJSON(s string, v any) error {
	return json.Unmarshal([]byte(s), v)
}
