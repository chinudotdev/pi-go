package tools

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/chinudotdev/pi-go/agent"
	"github.com/chinudotdev/pi-go/ai"
)

// GrepToolOptions configures the grep tool.
type GrepToolOptions struct {
	Operations GrepOperations
}

// GrepOperations are pluggable operations for the grep tool.
type GrepOperations interface {
	IsDirectory(absolutePath string) (bool, error)
	ReadFile(absolutePath string) (string, error)
}

type defaultGrepOps struct{}

func (d defaultGrepOps) IsDirectory(p string) (bool, error) {
	info, err := os.Stat(p)
	if err != nil {
		return false, err
	}
	return info.IsDir(), nil
}
func (d defaultGrepOps) ReadFile(p string) (string, error) {
	data, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

const grepDefaultLimit = 100

// CreateGrepTool creates the grep tool.
func CreateGrepTool(cwd string, opts *GrepToolOptions) *agent.Tool {
	if opts == nil {
		opts = &GrepToolOptions{}
	}
	ops := opts.Operations
	if ops == nil {
		ops = defaultGrepOps{}
	}

	return &agent.Tool{
		Name:        "grep",
		Label:       "grep",
		Description: ToolDescriptions[ToolGrep],
		Parameters:  ToolSchemas[ToolGrep],
		Execute: func(ctx context.Context, toolCallID string, params map[string]any, onUpdate agent.ToolUpdateCallback) (*agent.ToolResult, error) {
			pattern, _ := params["pattern"].(string)
			if pattern == "" {
				return nil, fmt.Errorf("pattern is required")
			}

			searchDir, _ := params["path"].(string)
			glob, _ := params["glob"].(string)
			ignoreCase := boolVal(params, "ignoreCase")
			literal := boolVal(params, "literal")
			contextLines := intVal(params, "context")
			limit := intVal(params, "limit")
			if limit <= 0 {
				limit = grepDefaultLimit
			}

			searchPath := ResolveToCwd(searchDir, cwd)

			// Check if path exists
			isDir, err := ops.IsDirectory(searchPath)
			if err != nil {
				return nil, fmt.Errorf("path not found: %s", searchPath)
			}

			// Try to use rg (ripgrep)
			rgPath, _ := exec.LookPath("rg")
			if rgPath != "" {
				return grepWithRg(ctx, rgPath, pattern, searchPath, isDir, glob, ignoreCase, literal, contextLines, limit, ops)
			}

			// Fallback: Go-based grep
			return grepWithGo(ctx, pattern, searchPath, isDir, glob, ignoreCase, literal, contextLines, limit, ops)
		},
	}
}

func grepWithRg(ctx context.Context, rgPath, pattern, searchPath string, isDir bool, glob string, ignoreCase, literal bool, contextLines, limit int, ops GrepOperations) (*agent.ToolResult, error) {
	args := []string{"--json", "--line-number", "--color=never", "--hidden"}
	if ignoreCase {
		args = append(args, "--ignore-case")
	}
	if literal {
		args = append(args, "--fixed-strings")
	}
	if glob != "" {
		args = append(args, "--glob", glob)
	}
	args = append(args, "--", pattern, searchPath)

	cmd := exec.CommandContext(ctx, rgPath, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to run ripgrep: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to run ripgrep: %w", err)
	}

	type match struct {
		filePath   string
		lineNumber int
		lineText   string
	}

	var matches []match
	linesTruncated := false
	scanner := bufio.NewScanner(stdout)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || len(matches) >= limit {
			continue
		}

		// Parse JSON from rg --json output
		if !strings.HasPrefix(line, "{\"type\":\"match\"") {
			continue
		}

		// Simple JSON extraction for path and line_number
		filePath := extractJSONString(line, `"path":`, `"text":`)
		lineNum := extractJSONInt(line, `"line_number":`)
		lineText := extractJSONString(line, `"lines":`, `"text":`)

		if filePath != "" && lineNum > 0 {
			matches = append(matches, match{filePath, lineNum, lineText})
			if len(matches) >= limit {
				break
			}
		}
	}

	cmd.Wait()

	if len(matches) == 0 {
		return &agent.ToolResult{
			Content: []ai.ContentBlock{newTextBlock("No matches found")},
		}, nil
	}

	// Format output
	var outputLines []string
	formatPath := func(fp string) string {
		if isDir {
			rel, err := filepath.Rel(searchPath, fp)
			if err == nil && !strings.HasPrefix(rel, "..") {
				return filepath.ToSlash(rel)
			}
		}
		return filepath.Base(fp)
	}

	for _, m := range matches {
		relativePath := formatPath(m.filePath)

		if contextLines <= 0 {
			sanitized := strings.ReplaceAll(m.lineText, "\r", "")
			sanitized = strings.TrimRight(sanitized, "\n")
			truncated, wasTruncated := TruncateLine(sanitized)
			if wasTruncated {
				linesTruncated = true
			}
			outputLines = append(outputLines, fmt.Sprintf("%s:%d: %s", relativePath, m.lineNumber, truncated))
		} else {
			// With context lines
			content, err := ops.ReadFile(m.filePath)
			if err != nil {
				outputLines = append(outputLines, fmt.Sprintf("%s:%d: (unable to read file)", relativePath, m.lineNumber))
				continue
			}
			fileLines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")

			start := m.lineNumber - contextLines
			if start < 1 {
				start = 1
			}
			end := m.lineNumber + contextLines
			if end > len(fileLines) {
				end = len(fileLines)
			}

			for i := start; i <= end; i++ {
				lineText := strings.ReplaceAll(fileLines[i-1], "\r", "")
				truncated, wasTruncated := TruncateLine(lineText)
				if wasTruncated {
					linesTruncated = true
				}
				if i == m.lineNumber {
					outputLines = append(outputLines, fmt.Sprintf("%s:%d: %s", relativePath, i, truncated))
				} else {
					outputLines = append(outputLines, fmt.Sprintf("%s-%d- %s", relativePath, i, truncated))
				}
			}
		}
	}

	rawOutput := strings.Join(outputLines, "\n")
	truncation := TruncateHead(rawOutput, TruncationOptions{MaxLines: DefaultMaxLines * 10}) // high line limit, mainly byte limit

	output := truncation.Content
	matchLimitReached := len(matches) >= limit

	var notices []string
	if matchLimitReached {
		notices = append(notices, fmt.Sprintf("%d matches limit reached. Use limit=%d for more, or refine pattern", limit, limit*2))
	}
	if truncation.Truncated {
		notices = append(notices, fmt.Sprintf("%s limit reached", FormatSize(DefaultMaxBytes)))
	}
	if linesTruncated {
		notices = append(notices, fmt.Sprintf("Some lines truncated to %d chars. Use read tool to see full lines", GrepMaxLineLen))
	}
	if len(notices) > 0 {
		output += "\n\n[" + strings.Join(notices, ". ") + "]"
	}

	details := map[string]any{}
	if matchLimitReached {
		details["matchLimitReached"] = limit
	}
	if truncation.Truncated {
		details["truncation"] = truncation
	}
	if linesTruncated {
		details["linesTruncated"] = true
	}

	return &agent.ToolResult{
		Content: []ai.ContentBlock{newTextBlock(output)},
		Details: details,
	}, nil
}

func grepWithGo(ctx context.Context, pattern, searchPath string, isDir bool, glob string, ignoreCase, literal bool, contextLines, limit int, ops GrepOperations) (*agent.ToolResult, error) {
	// Build regex
	pat := pattern
	if literal {
		pat = regexp.QuoteMeta(pat)
	}
	flags := ""
	if ignoreCase {
		flags = "(?i)"
	}
	re, err := regexp.Compile(flags + pat)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern: %w", err)
	}

	var matches []struct {
		filePath   string
		lineNumber int
		lineText   string
	}

	linesTruncated := false

	// Walk the directory or search the file
	searchFile := func(filePath string) error {
		content, err := ops.ReadFile(filePath)
		if err != nil {
			return nil
		}
		fileLines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")

		for i, line := range fileLines {
			line = strings.ReplaceAll(line, "\r", "")
			if re.MatchString(line) {
				truncated, wasTruncated := TruncateLine(line)
				if wasTruncated {
					linesTruncated = true
				}
				matches = append(matches, struct {
					filePath   string
					lineNumber int
					lineText   string
				}{filePath, i + 1, truncated})
				if len(matches) >= limit {
					return nil
				}
			}
		}
		return nil
	}

	if isDir {
		filepath.WalkDir(searchPath, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				name := d.Name()
				if name == ".git" || name == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}

			// Apply glob filter
			if glob != "" {
				matched, _ := filepath.Match(glob, d.Name())
				if !matched {
					return nil
				}
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			return searchFile(path)
		})
	} else {
		searchFile(searchPath)
	}

	if len(matches) == 0 {
		return &agent.ToolResult{
			Content: []ai.ContentBlock{newTextBlock("No matches found")},
		}, nil
	}

	formatPath := func(fp string) string {
		if isDir {
			rel, err := filepath.Rel(searchPath, fp)
			if err == nil && !strings.HasPrefix(rel, "..") {
				return filepath.ToSlash(rel)
			}
		}
		return filepath.Base(fp)
	}

	var outputLines []string
	for _, m := range matches {
		outputLines = append(outputLines, fmt.Sprintf("%s:%d: %s", formatPath(m.filePath), m.lineNumber, m.lineText))
	}

	rawOutput := strings.Join(outputLines, "\n")
	output := rawOutput

	matchLimitReached := len(matches) >= limit
	var notices []string
	if matchLimitReached {
		notices = append(notices, fmt.Sprintf("%d matches limit reached", limit))
	}
	if linesTruncated {
		notices = append(notices, fmt.Sprintf("Some lines truncated to %d chars", GrepMaxLineLen))
	}
	if len(notices) > 0 {
		output += "\n\n[" + strings.Join(notices, ". ") + "]"
	}

	return &agent.ToolResult{
		Content: []ai.ContentBlock{newTextBlock(output)},
		Details: map[string]any{"matchLimitReached": matchLimitReached},
	}, nil
}

// JSON extraction helpers for rg --json output
func extractJSONString(line, key1, key2 string) string {
	idx := strings.Index(line, key1)
	if idx < 0 {
		return ""
	}
	sub := line[idx+len(key1):]
	idx2 := strings.Index(sub, key2)
	if idx2 < 0 {
		return ""
	}
	val := sub[idx2+len(key2):]
	// Extract string value
	start := strings.Index(val, `"`)
	if start < 0 {
		return ""
	}
	val = val[start+1:]
	end := strings.Index(val, `"`)
	if end < 0 {
		return ""
	}
	return val[:end]
}

func extractJSONInt(line, key string) int {
	idx := strings.Index(line, key)
	if idx < 0 {
		return 0
	}
	sub := line[idx+len(key):]
	// Skip whitespace
	sub = strings.TrimLeft(sub, " \t\n\r:")
	num := 0
	for _, c := range sub {
		if c >= '0' && c <= '9' {
			num = num*10 + int(c-'0')
		} else {
			break
		}
	}
	return num
}

func boolVal(params map[string]any, key string) bool {
	v, ok := params[key]
	if !ok {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

func intVal(params map[string]any, key string) int {
	v, ok := params[key]
	if !ok {
		return 0
	}
	if f, ok := v.(float64); ok {
		return int(f)
	}
	return 0
}
