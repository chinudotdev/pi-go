package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chinudotdev/pi-go/agent"
	"github.com/chinudotdev/pi-go/ai"
)

// ReadToolOptions configures the read tool.
type ReadToolOptions struct {
	// AutoResizeImages enables auto-resizing images. Default: true.
	AutoResizeImages bool
	// Custom operations for file reading.
	Operations ReadOperations
}

// ReadOperations are pluggable operations for the read tool.
type ReadOperations interface {
	ReadFile(absolutePath string) ([]byte, error)
	Access(absolutePath string) error
	DetectImageMimeType(absolutePath string) (string, error)
}

type defaultReadOps struct{}

func (d defaultReadOps) ReadFile(p string) ([]byte, error) { return os.ReadFile(p) }
func (d defaultReadOps) Access(p string) error {
	_, err := os.Stat(p)
	return err
}
func (d defaultReadOps) DetectImageMimeType(p string) (string, error) {
	ext := strings.ToLower(filepath.Ext(p))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg", nil
	case ".png":
		return "image/png", nil
	case ".gif":
		return "image/gif", nil
	case ".webp":
		return "image/webp", nil
	case ".svg":
		return "image/svg+xml", nil
	default:
		return "", nil
	}
}

// CreateReadTool creates the read tool.
func CreateReadTool(cwd string, opts *ReadToolOptions) *agent.Tool {
	if opts == nil {
		opts = &ReadToolOptions{}
	}
	ops := opts.Operations
	if ops == nil {
		ops = defaultReadOps{}
	}

	return &agent.Tool{
		Name:        "read",
		Label:       "read",
		Description: ToolDescriptions[ToolRead],
		Parameters:  ToolSchemas[ToolRead],
		Execute: func(ctx context.Context, toolCallID string, params map[string]any, onUpdate agent.ToolUpdateCallback) (*agent.ToolResult, error) {
			pathVal, _ := params["path"].(string)
			if pathVal == "" {
				return nil, fmt.Errorf("path is required")
			}

			var offset, limit int
			if v, ok := params["offset"]; ok {
				if f, ok := v.(float64); ok {
					offset = int(f)
				}
			}
			if v, ok := params["limit"]; ok {
				if f, ok := v.(float64); ok {
					limit = int(f)
				}
			}

			// Resolve path
			absolutePath := ResolveReadPath(pathVal, cwd)

			// Check accessibility
			if err := ops.Access(absolutePath); err != nil {
				return nil, fmt.Errorf("cannot read file: %s: %w", pathVal, err)
			}

			// Check for context cancellation
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}

			// Try image detection
			mimeType, _ := ops.DetectImageMimeType(absolutePath)
			if mimeType != "" {
				data, err := ops.ReadFile(absolutePath)
				if err != nil {
					return nil, fmt.Errorf("failed to read image: %w", err)
				}
				note := fmt.Sprintf("Read image file [%s]", mimeType)
				return &agent.ToolResult{
					Content: []ai.ContentBlock{
						newTextBlock(note),
						{Type: "image", Text: fmt.Sprintf("[image %s]", mimeType)},
					},
					Details: map[string]any{"mimeType": mimeType, "size": len(data)},
				}, nil
			}

			// Read as text
			data, err := ops.ReadFile(absolutePath)
			if err != nil {
				return nil, fmt.Errorf("failed to read file: %w", err)
			}

			textContent := string(data)
			allLines := strings.Split(textContent, "\n")

			// Remove trailing empty line from trailing newline
			if len(allLines) > 0 && allLines[len(allLines)-1] == "" {
				allLines = allLines[:len(allLines)-1]
			}

			totalFileLines := len(allLines)

			// Apply offset (1-indexed → 0-indexed)
			startLine := 0
			if offset > 0 {
				startLine = offset - 1
				if startLine >= len(allLines) {
					return nil, fmt.Errorf("offset %d is beyond end of file (%d lines total)", offset, totalFileLines)
				}
			}
			startLineDisplay := startLine + 1

			// Apply limit
			var selectedContent string
			var userLimitedLines int
			if limit > 0 {
				endLine := minInt(startLine+limit, len(allLines))
				selectedContent = strings.Join(allLines[startLine:endLine], "\n")
				userLimitedLines = endLine - startLine
			} else {
				selectedContent = strings.Join(allLines[startLine:], "\n")
			}

			// Truncate
			truncation := TruncateHead(selectedContent)
			var outputText string

			if truncation.FirstLineExceeds {
				firstLineSize := FormatSize(len([]byte(allLines[startLine])))
				outputText = fmt.Sprintf("[Line %d is %s, exceeds %s limit. Use bash: sed -n '%dp' %s | head -c %d]",
					startLineDisplay, firstLineSize, FormatSize(DefaultMaxBytes), startLineDisplay, pathVal, DefaultMaxBytes)
			} else if truncation.Truncated {
				endLineDisplay := startLineDisplay + truncation.OutputLines - 1
				nextOffset := endLineDisplay + 1
				outputText = truncation.Content
				if truncation.TruncatedBy == "lines" {
					outputText += fmt.Sprintf("\n\n[Showing lines %d-%d of %d. Use offset=%d to continue.]",
						startLineDisplay, endLineDisplay, totalFileLines, nextOffset)
				} else {
					outputText += fmt.Sprintf("\n\n[Showing lines %d-%d of %d (%s limit). Use offset=%d to continue.]",
						startLineDisplay, endLineDisplay, totalFileLines, FormatSize(DefaultMaxBytes), nextOffset)
				}
			} else if userLimitedLines > 0 && startLine+userLimitedLines < len(allLines) {
				remaining := len(allLines) - (startLine + userLimitedLines)
				nextOffset := startLine + userLimitedLines + 1
				outputText = fmt.Sprintf("%s\n\n[%d more lines in file. Use offset=%d to continue.]",
					truncation.Content, remaining, nextOffset)
			} else {
				outputText = truncation.Content
			}

			return &agent.ToolResult{
				Content: []ai.ContentBlock{newTextBlock(outputText)},
				Details: map[string]any{
					"truncation": truncation,
					"totalLines": totalFileLines,
					"offset":     offset,
					"limit":      limit,
				},
			}, nil
		},
	}
}
