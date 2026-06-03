package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/chinudotdev/pi-go/agent"
	"github.com/chinudotdev/pi-go/ai"
)

// FindToolOptions configures the find tool.
type FindToolOptions struct {
	Operations FindOperations
}

// FindOperations are pluggable operations for the find tool.
type FindOperations interface {
	Exists(absolutePath string) (bool, error)
	Glob(pattern, cwd string, opts GlobOptions) ([]string, error)
}

// GlobOptions configures glob search.
type GlobOptions struct {
	Ignore []string
	Limit  int
}

type defaultFindOps struct{}

func (d defaultFindOps) Exists(p string) (bool, error) {
	_, err := os.Stat(p)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}
func (d defaultFindOps) Glob(pattern, cwd string, opts GlobOptions) ([]string, error) {
	return nil, nil // placeholder
}

const findDefaultLimit = 1000

// CreateFindTool creates the find tool.
func CreateFindTool(cwd string, opts *FindToolOptions) *agent.Tool {
	if opts == nil {
		opts = &FindToolOptions{}
	}
	ops := opts.Operations
	if ops == nil {
		ops = defaultFindOps{}
	}

	return &agent.Tool{
		Name:        "find",
		Label:       "find",
		Description: ToolDescriptions[ToolFind],
		Parameters:  ToolSchemas[ToolFind],
		Execute: func(ctx context.Context, toolCallID string, params map[string]any, onUpdate agent.ToolUpdateCallback) (*agent.ToolResult, error) {
			pattern, _ := params["pattern"].(string)
			if pattern == "" {
				return nil, fmt.Errorf("pattern is required")
			}

			searchDir, _ := params["path"].(string)
			limit := intVal(params, "limit")
			if limit <= 0 {
				limit = findDefaultLimit
			}

			searchPath := ResolveToCwd(searchDir, cwd)

			// Check if custom operations provide glob
			if customOps, ok := ops.(defaultFindOps); !ok || true {
				_ = customOps
				// Try fd first
				fdPath, _ := exec.LookPath("fd")
				if fdPath != "" {
					return findWithFd(ctx, fdPath, pattern, searchPath, limit)
				}
			}

			// Fallback: Go-based find
			return findWithGo(ctx, pattern, searchPath, limit)
		},
	}
}

func findWithFd(ctx context.Context, fdPath, pattern, searchPath string, limit int) (*agent.ToolResult, error) {
	args := []string{"--glob", "--color=never", "--hidden", "--no-require-git", "--max-results", fmt.Sprintf("%d", limit)}

	effectivePattern := pattern
	if strings.Contains(pattern, "/") {
		args = append(args, "--full-path")
		if !strings.HasPrefix(pattern, "/") && !strings.HasPrefix(pattern, "**/") && pattern != "**" {
			effectivePattern = "**/" + pattern
		}
	}
	args = append(args, "--", effectivePattern, searchPath)

	cmd := exec.CommandContext(ctx, fdPath, args...)
	rawOutputBytes, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if len(exitErr.Stderr) > 0 {
				return nil, fmt.Errorf("fd error: %s", string(exitErr.Stderr))
			}
		}
		// fd returns exit code 1 for no results, that's fine
		if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != 1 {
			return nil, fmt.Errorf("fd error: %w", err)
		}
	}

	outputStr := strings.TrimRight(string(rawOutputBytes), "\n")
	if outputStr == "" {
		return &agent.ToolResult{
			Content: []ai.ContentBlock{newTextBlock("No files found matching pattern")},
		}, nil
	}

	lines := strings.Split(outputStr, "\n")
	relativized := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		hadSlash := strings.HasSuffix(line, "/") || strings.HasSuffix(line, "\\")
		rel := line
		if strings.HasPrefix(line, searchPath) {
			rel = line[len(searchPath)+1:]
		} else {
			r, err := filepath.Rel(searchPath, line)
			if err == nil {
				rel = r
			}
		}
		if hadSlash && !strings.HasSuffix(rel, "/") {
			rel += "/"
		}
		relativized = append(relativized, filepath.ToSlash(rel))
	}

	resultLimitReached := len(relativized) >= limit
	rawOutput := strings.Join(relativized, "\n")
	truncation := TruncateHead(rawOutput, TruncationOptions{MaxLines: DefaultMaxLines * 10})

	output := truncation.Content
	var notices []string
	if resultLimitReached {
		notices = append(notices, fmt.Sprintf("%d results limit reached. Use limit=%d for more, or refine pattern", limit, limit*2))
	}
	if truncation.Truncated {
		notices = append(notices, fmt.Sprintf("%s limit reached", FormatSize(DefaultMaxBytes)))
	}
	if len(notices) > 0 {
		output += "\n\n[" + strings.Join(notices, ". ") + "]"
	}

	return &agent.ToolResult{
		Content: []ai.ContentBlock{newTextBlock(output)},
	}, nil
}

func findWithGo(ctx context.Context, pattern, searchPath string, limit int) (*agent.ToolResult, error) {
	var results []string

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

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Match pattern against filename
		matched, _ := filepath.Match(pattern, d.Name())
		if matched {
			rel, err := filepath.Rel(searchPath, path)
			if err == nil {
				results = append(results, filepath.ToSlash(rel))
			}
			if len(results) >= limit {
				return fmt.Errorf("limit reached")
			}
		}
		return nil
	})

	if len(results) == 0 {
		return &agent.ToolResult{
			Content: []ai.ContentBlock{newTextBlock("No files found matching pattern")},
		}, nil
	}

	return &agent.ToolResult{
		Content: []ai.ContentBlock{newTextBlock(strings.Join(results, "\n"))},
	}, nil
}
