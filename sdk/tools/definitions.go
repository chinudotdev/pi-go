// Package tools provides built-in tool implementations for the coding agent SDK.
// This file contains the tool factory functions and definitions registry.
package tools

import (
	"fmt"

	"github.com/chinudotdev/pi-go/agent"
	"github.com/chinudotdev/pi-go/ai"
)

// ToolName identifies a built-in tool.
type ToolName string

const (
	ToolRead  ToolName = "read"
	ToolBash  ToolName = "bash"
	ToolEdit  ToolName = "edit"
	ToolWrite ToolName = "write"
	ToolGrep  ToolName = "grep"
	ToolFind  ToolName = "find"
	ToolLs    ToolName = "ls"
)

// AllToolNames is the set of all built-in tool names.
var AllToolNames = map[ToolName]bool{
	ToolRead: true, ToolBash: true, ToolEdit: true,
	ToolWrite: true, ToolGrep: true, ToolFind: true, ToolLs: true,
}

// ToolOptions aggregates per-tool options.
type ToolOptions struct {
	Read  *ReadToolOptions
	Bash  *BashToolOptions
	Write *WriteToolOptions
	Edit  *EditToolOptions
	Grep  *GrepToolOptions
	Find  *FindToolOptions
	Ls    *LsToolOptions
}

// CreateTool creates a single tool by name.
func CreateTool(name ToolName, cwd string, opts *ToolOptions) (*agent.Tool, error) {
	switch name {
	case ToolRead:
		return CreateReadTool(cwd, opts.Read), nil
	case ToolBash:
		return CreateBashTool(cwd, opts.Bash), nil
	case ToolEdit:
		return CreateEditTool(cwd, opts.Edit), nil
	case ToolWrite:
		return CreateWriteTool(cwd, opts.Write), nil
	case ToolGrep:
		return CreateGrepTool(cwd, opts.Grep), nil
	case ToolFind:
		return CreateFindTool(cwd, opts.Find), nil
	case ToolLs:
		return CreateLsTool(cwd, opts.Ls), nil
	default:
		return nil, fmt.Errorf("unknown tool name: %s", name)
	}
}

// CreateCodingTools creates the coding tool set (read, bash, edit, write).
func CreateCodingTools(cwd string, opts *ToolOptions) []*agent.Tool {
	return []*agent.Tool{
		CreateReadTool(cwd, nilIfNil(opts).Read),
		CreateBashTool(cwd, nilIfNil(opts).Bash),
		CreateEditTool(cwd, nilIfNil(opts).Edit),
		CreateWriteTool(cwd, nilIfNil(opts).Write),
	}
}

// CreateReadOnlyTools creates the read-only tool set (read, grep, find, ls).
func CreateReadOnlyTools(cwd string, opts *ToolOptions) []*agent.Tool {
	return []*agent.Tool{
		CreateReadTool(cwd, nilIfNil(opts).Read),
		CreateGrepTool(cwd, nilIfNil(opts).Grep),
		CreateFindTool(cwd, nilIfNil(opts).Find),
		CreateLsTool(cwd, nilIfNil(opts).Ls),
	}
}

// CreateAllTools creates all built-in tools as a map.
func CreateAllTools(cwd string, opts *ToolOptions) map[ToolName]*agent.Tool {
	o := nilIfNil(opts)
	return map[ToolName]*agent.Tool{
		ToolRead:  CreateReadTool(cwd, o.Read),
		ToolBash:  CreateBashTool(cwd, o.Bash),
		ToolEdit:  CreateEditTool(cwd, o.Edit),
		ToolWrite: CreateWriteTool(cwd, o.Write),
		ToolGrep:  CreateGrepTool(cwd, o.Grep),
		ToolFind:  CreateFindTool(cwd, o.Find),
		ToolLs:    CreateLsTool(cwd, o.Ls),
	}
}

// ToolSchemas returns the JSON parameter schemas for each tool.
var ToolSchemas = map[ToolName]map[string]any{
	ToolRead: {
		"type": "object",
		"properties": map[string]any{
			"path":   map[string]any{"type": "string", "description": "Path to the file to read (relative or absolute)"},
			"offset": map[string]any{"type": "number", "description": "Line number to start reading from (1-indexed)"},
			"limit":  map[string]any{"type": "number", "description": "Maximum number of lines to read"},
		},
		"required":             []string{"path"},
		"additionalProperties": false,
	},
	ToolBash: {
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{"type": "string", "description": "Bash command to execute"},
			"timeout": map[string]any{"type": "number", "description": "Timeout in seconds (optional, no default timeout)"},
		},
		"required":             []string{"command"},
		"additionalProperties": false,
	},
	ToolEdit: {
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string", "description": "Path to the file to edit (relative or absolute)"},
			"edits": map[string]any{
				"type": "array",
				"description": "One or more targeted replacements. Each edit is matched against the original file, not incrementally.",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"oldText": map[string]any{"type": "string", "description": "Exact text for one targeted replacement."},
						"newText": map[string]any{"type": "string", "description": "Replacement text for this targeted edit."},
					},
					"required":             []string{"oldText", "newText"},
					"additionalProperties": false,
				},
			},
		},
		"required":             []string{"path", "edits"},
		"additionalProperties": false,
	},
	ToolWrite: {
		"type": "object",
		"properties": map[string]any{
			"path":    map[string]any{"type": "string", "description": "Path to the file to write (relative or absolute)"},
			"content": map[string]any{"type": "string", "description": "Content to write to the file"},
		},
		"required":             []string{"path", "content"},
		"additionalProperties": false,
	},
	ToolGrep: {
		"type": "object",
		"properties": map[string]any{
			"pattern":    map[string]any{"type": "string", "description": "Search pattern (regex or literal string)"},
			"path":       map[string]any{"type": "string", "description": "Directory or file to search (default: current directory)"},
			"glob":       map[string]any{"type": "string", "description": "Filter files by glob pattern"},
			"ignoreCase": map[string]any{"type": "boolean", "description": "Case-insensitive search (default: false)"},
			"literal":    map[string]any{"type": "boolean", "description": "Treat pattern as literal string (default: false)"},
			"context":    map[string]any{"type": "number", "description": "Lines before and after each match (default: 0)"},
			"limit":      map[string]any{"type": "number", "description": "Maximum matches (default: 100)"},
		},
		"required":             []string{"pattern"},
		"additionalProperties": false,
	},
	ToolFind: {
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{"type": "string", "description": "Glob pattern to match files"},
			"path":    map[string]any{"type": "string", "description": "Directory to search in (default: current directory)"},
			"limit":   map[string]any{"type": "number", "description": "Maximum results (default: 1000)"},
		},
		"required":             []string{"pattern"},
		"additionalProperties": false,
	},
	ToolLs: {
		"type": "object",
		"properties": map[string]any{
			"path":  map[string]any{"type": "string", "description": "Directory to list (default: current directory)"},
			"limit": map[string]any{"type": "number", "description": "Maximum entries (default: 500)"},
		},
		"additionalProperties": false,
	},
}

// ToolDescriptions holds human-readable descriptions for each tool.
var ToolDescriptions = map[ToolName]string{
	ToolRead:  fmt.Sprintf("Read the contents of a file. Supports text files and images (jpg, png, gif, webp). Images are sent as attachments. For text files, output is truncated to %d lines or %dKB (whichever is hit first). Use offset/limit for large files. When you need the full file, continue with offset until complete.", DefaultMaxLines, DefaultMaxBytes/1024),
	ToolBash:  fmt.Sprintf("Execute a bash command in the current working directory. Returns stdout and stderr. Output is truncated to last %d lines or %dKB (whichever is hit first). If truncated, full output is saved to a temp file. Optionally provide a timeout in seconds.", DefaultMaxLines, DefaultMaxBytes/1024),
	ToolEdit:  "Edit a single file using exact text replacement. Every edits[].oldText must match a unique, non-overlapping region of the original file. If two changes affect the same block or nearby lines, merge them into one edit instead of emitting overlapping edits. Do not include large unchanged regions just to connect distant changes.",
	ToolWrite: "Write content to a file. Creates the file if it doesn't exist, overwrites if it does. Automatically creates parent directories.",
	ToolGrep:  fmt.Sprintf("Search file contents for a pattern. Returns matching lines with file paths and line numbers. Respects .gitignore. Output is truncated to 100 matches or %dKB. Long lines are truncated to %d chars.", DefaultMaxBytes/1024, GrepMaxLineLen),
	ToolFind:  fmt.Sprintf("Search for files by glob pattern. Returns matching file paths relative to the search directory. Respects .gitignore. Output is truncated to 1000 results or %dKB.", DefaultMaxBytes/1024),
	ToolLs:    fmt.Sprintf("List directory contents. Returns entries sorted alphabetically, with '/' suffix for directories. Includes dotfiles. Output is truncated to 500 entries or %dKB.", DefaultMaxBytes/1024),
}

// ToolPromptSnippets holds prompt snippets for each tool.
var ToolPromptSnippets = map[ToolName]string{
	ToolRead:  "Read file contents",
	ToolBash:  "Execute bash commands (ls, grep, find, etc.)",
	ToolEdit:  "Make precise file edits with exact text replacement, including multiple disjoint edits in one call",
	ToolWrite: "Create or overwrite files",
	ToolGrep:  "Search file contents for patterns (respects .gitignore)",
	ToolFind:  "Find files by glob pattern (respects .gitignore)",
	ToolLs:    "List directory contents",
}

// ToolPromptGuidelines holds prompt guidelines for each tool.
var ToolPromptGuidelines = map[ToolName][]string{
	ToolRead:  {"Use read to examine files instead of cat or sed."},
	ToolWrite: {"Use write only for new files or complete rewrites."},
	ToolEdit: {
		"Use edit for precise changes (edits[].oldText must match exactly)",
		"When changing multiple separate locations in one file, use one edit call with multiple entries in edits[] instead of multiple edit calls",
		"Each edits[].oldText is matched against the original file, not after earlier edits are applied. Do not emit overlapping or nested edits. Merge nearby changes into one edit.",
		"Keep edits[].oldText as small as possible while still being unique in the file. Do not pad with large unchanged regions.",
	},
}

// newTextBlock is a helper to create a text content block.
func newTextBlock(text string) ai.ContentBlock {
	return ai.NewTextContent(text)
}

func nilIfNil(opts *ToolOptions) *ToolOptions {
	if opts == nil {
		return &ToolOptions{}
	}
	return opts
}
