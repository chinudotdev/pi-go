package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/chinudotdev/pi-go/agent"
	"github.com/chinudotdev/pi-go/ai"
)

// WriteToolOptions configures the write tool.
type WriteToolOptions struct {
	Operations WriteOperations
}

// WriteOperations are pluggable operations for the write tool.
type WriteOperations interface {
	WriteFile(absolutePath, content string) error
	Mkdir(dir string) error
}

type defaultWriteOps struct{}

func (d defaultWriteOps) WriteFile(p, content string) error {
	return os.WriteFile(p, []byte(content), 0644)
}
func (d defaultWriteOps) Mkdir(dir string) error {
	return os.MkdirAll(dir, 0755)
}

// CreateWriteTool creates the write tool.
func CreateWriteTool(cwd string, opts *WriteToolOptions) *agent.Tool {
	if opts == nil {
		opts = &WriteToolOptions{}
	}
	ops := opts.Operations
	if ops == nil {
		ops = defaultWriteOps{}
	}

	return &agent.Tool{
		Name:        "write",
		Label:       "write",
		Description: ToolDescriptions[ToolWrite],
		Parameters:  ToolSchemas[ToolWrite],
		Execute: func(ctx context.Context, toolCallID string, params map[string]any, onUpdate agent.ToolUpdateCallback) (*agent.ToolResult, error) {
			pathVal, _ := params["path"].(string)
			content, _ := params["content"].(string)
			if pathVal == "" {
				return nil, fmt.Errorf("path is required")
			}

			absolutePath := ResolveToCwd(pathVal, cwd)

			err := WithFileMutationQueue(absolutePath, func() error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}

				dir := filepath.Dir(absolutePath)
				if err := ops.Mkdir(dir); err != nil {
					return fmt.Errorf("failed to create directory: %w", err)
				}

				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}

				if err := ops.WriteFile(absolutePath, content); err != nil {
					return fmt.Errorf("failed to write file: %w", err)
				}

				return nil
			})

			if err != nil {
				return nil, err
			}

			return &agent.ToolResult{
				Content: []ai.ContentBlock{
					newTextBlock(fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), pathVal)),
				},
			}, nil
		},
	}
}
