package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chinudotdev/pi-go/agent"
	"github.com/chinudotdev/pi-go/ai"
)

// LsToolOptions configures the ls tool.
type LsToolOptions struct {
	Operations LsOperations
}

// LsOperations are pluggable operations for the ls tool.
type LsOperations interface {
	Exists(absolutePath string) (bool, error)
	Stat(absolutePath string) (LsFileInfo, error)
	ReadDir(absolutePath string) ([]string, error)
}

// LsFileInfo holds file info.
type LsFileInfo interface {
	IsDir() bool
}

type osLsFileInfo struct {
	os.FileInfo
}

type defaultLsOps struct{}

func (d defaultLsOps) Exists(p string) (bool, error) {
	_, err := os.Stat(p)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}
func (d defaultLsOps) Stat(p string) (LsFileInfo, error) {
	info, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	return osLsFileInfo{info}, nil
}
func (d defaultLsOps) ReadDir(p string) ([]string, error) {
	entries, err := os.ReadDir(p)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names, nil
}

const lsDefaultLimit = 500

// CreateLsTool creates the ls tool.
func CreateLsTool(cwd string, opts *LsToolOptions) *agent.Tool {
	if opts == nil {
		opts = &LsToolOptions{}
	}
	ops := opts.Operations
	if ops == nil {
		ops = defaultLsOps{}
	}

	return &agent.Tool{
		Name:        "ls",
		Label:       "ls",
		Description: ToolDescriptions[ToolLs],
		Parameters:  ToolSchemas[ToolLs],
		Execute: func(ctx context.Context, toolCallID string, params map[string]any, onUpdate agent.ToolUpdateCallback) (*agent.ToolResult, error) {
			pathVal, _ := params["path"].(string)
			limit := intVal(params, "limit")
			if limit <= 0 {
				limit = lsDefaultLimit
			}

			dirPath := ResolveToCwd(pathVal, cwd)

			// Check exists
			exists, err := ops.Exists(dirPath)
			if err != nil {
				return nil, fmt.Errorf("path not found: %s", dirPath)
			}
			if !exists {
				return nil, fmt.Errorf("path not found: %s", dirPath)
			}

			// Check is directory
			stat, err := ops.Stat(dirPath)
			if err != nil {
				return nil, fmt.Errorf("cannot stat path: %s: %w", dirPath, err)
			}
			if !stat.IsDir() {
				return nil, fmt.Errorf("not a directory: %s", dirPath)
			}

			// Read entries
			entries, err := ops.ReadDir(dirPath)
			if err != nil {
				return nil, fmt.Errorf("cannot read directory: %w", err)
			}

			// Sort alphabetically, case-insensitive
			sort.Slice(entries, func(i, j int) bool {
				return strings.ToLower(entries[i]) < strings.ToLower(entries[j])
			})

			// Format entries with directory indicators
			var results []string
			entryLimitReached := false
			for _, entry := range entries {
				if len(results) >= limit {
					entryLimitReached = true
					break
				}

				fullPath := filepath.Join(dirPath, entry)
				entryStat, err := ops.Stat(fullPath)
				if err != nil {
					continue // skip entries we cannot stat
				}
				if entryStat.IsDir() {
					results = append(results, entry+"/")
				} else {
					results = append(results, entry)
				}
			}

			if len(results) == 0 {
				return &agent.ToolResult{
					Content: []ai.ContentBlock{newTextBlock("(empty directory)")},
				}, nil
			}

			rawOutput := strings.Join(results, "\n")
			truncation := TruncateHead(rawOutput, TruncationOptions{MaxLines: DefaultMaxLines * 10})

			output := truncation.Content
			var notices []string
			if entryLimitReached {
				notices = append(notices, fmt.Sprintf("%d entries limit reached. Use limit=%d for more", limit, limit*2))
			}
			if truncation.Truncated {
				notices = append(notices, fmt.Sprintf("%s limit reached", FormatSize(DefaultMaxBytes)))
			}
			if len(notices) > 0 {
				output += "\n\n[" + strings.Join(notices, ". ") + "]"
			}

			details := map[string]any{}
			if entryLimitReached {
				details["entryLimitReached"] = limit
			}
			if truncation.Truncated {
				details["truncation"] = truncation
			}

			return &agent.ToolResult{
				Content: []ai.ContentBlock{newTextBlock(output)},
				Details: details,
			}, nil
		},
	}
}
