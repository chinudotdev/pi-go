// Package prompt constructs system prompts for the coding agent.
// It composes context files, skills, tool descriptions, and custom prompts
// into the final system prompt text sent to the LLM.
package prompt

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/chinudotdev/pi-go/sdk/resources"
	"github.com/chinudotdev/pi-go/sdk/skills"
)

var (
	positionalRegex = regexp.MustCompile(`\$(\d+)`)
	sliceRegex      = regexp.MustCompile(`\$\{@:(\d+)(?::(\d+))?\}`)
)

// BuildSystemPromptOptions configures system prompt construction.
type BuildSystemPromptOptions struct {
	// CustomPrompt replaces the default system prompt entirely.
	CustomPrompt string

	// SelectedTools lists active tool names. Default: [read, bash, edit, write]
	SelectedTools []string

	// ToolSnippets maps tool name → one-line description.
	ToolSnippets map[string]string

	// PromptGuidelines are additional guideline bullets.
	PromptGuidelines []string

	// AppendSystemPrompt is appended to the system prompt.
	AppendSystemPrompt string

	// CWD is the current working directory.
	CWD string

	// ContextFiles are pre-loaded AGENTS.md / CLAUDE.md files.
	ContextFiles []resources.ContextFile

	// Skills are pre-loaded skills to include.
	Skills []skills.Skill

	// Date override for testing.
	Date string
}

// BuildSystemPrompt constructs the full system prompt.
func BuildSystemPrompt(opts BuildSystemPromptOptions) string {
	promptCwd := strings.ReplaceAll(opts.CWD, "\\", "/")

	date := opts.Date
	if date == "" {
		now := time.Now()
		date = fmt.Sprintf("%d-%02d-%02d", now.Year(), now.Month(), now.Day())
	}

	appendSection := ""
	if opts.AppendSystemPrompt != "" {
		appendSection = "\n\n" + opts.AppendSystemPrompt
	}

	contextFiles := opts.ContextFiles
	sk := opts.Skills

	if opts.CustomPrompt != "" {
		prompt := opts.CustomPrompt
		prompt += appendSection
		prompt += buildContextSection(contextFiles)
		prompt += buildSkillsSection(sk, opts.SelectedTools)
		prompt += fmt.Sprintf("\nCurrent date: %s", date)
		prompt += fmt.Sprintf("\nCurrent working directory: %s", promptCwd)
		return prompt
	}

	// Default system prompt
	tools := opts.SelectedTools
	if len(tools) == 0 {
		tools = []string{"read", "bash", "edit", "write"}
	}

	toolsList := buildToolsList(tools, opts.ToolSnippets)
	guidelines := buildGuidelines(tools, opts.PromptGuidelines)

	prompt := `You are an expert coding assistant operating inside pi, a coding agent harness. You help users by reading files, executing commands, editing code, and writing new files.

Available tools:
` + toolsList + `

In addition to the tools above, you may have access to other custom tools depending on the project.

Guidelines:
` + guidelines

	prompt += appendSection
	prompt += buildContextSection(contextFiles)
	prompt += buildSkillsSection(sk, tools)
	prompt += fmt.Sprintf("\nCurrent date: %s", date)
	prompt += fmt.Sprintf("\nCurrent working directory: %s", promptCwd)

	return prompt
}

func buildToolsList(tools []string, snippets map[string]string) string {
	var visible []string
	for _, name := range tools {
		if snippets != nil && snippets[name] != "" {
			visible = append(visible, fmt.Sprintf("- %s: %s", name, snippets[name]))
		}
	}
	if len(visible) == 0 {
		return "(none)"
	}
	return strings.Join(visible, "\n")
}

func buildGuidelines(tools []string, extra []string) string {
	toolSet := make(map[string]bool)
	for _, t := range tools {
		toolSet[t] = true
	}

	seen := make(map[string]bool)
	var lines []string

	add := func(g string) {
		g = strings.TrimSpace(g)
		if g != "" && !seen[g] {
			seen[g] = true
			lines = append(lines, "- "+g)
		}
	}

	if toolSet["bash"] && !toolSet["grep"] && !toolSet["find"] && !toolSet["ls"] {
		add("Use bash for file operations like ls, rg, find")
	}

	for _, g := range extra {
		add(g)
	}

	add("Be concise in your responses")
	add("Show file paths clearly when working with files")

	return strings.Join(lines, "\n")
}

func buildContextSection(contextFiles []resources.ContextFile) string {
	if len(contextFiles) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n\n<project_context>\n\n")
	sb.WriteString("Project-specific instructions and guidelines:\n\n")
	for _, cf := range contextFiles {
		sb.WriteString(fmt.Sprintf("<project_instructions path=\"%s\">\n%s\n</project_instructions>\n\n", cf.Path, cf.Content))
	}
	sb.WriteString("</project_context>\n")
	return sb.String()
}

func buildSkillsSection(sk []skills.Skill, selectedTools []string) string {
	hasRead := false
	for _, t := range selectedTools {
		if t == "read" {
			hasRead = true
			break
		}
	}
	if !hasRead || len(sk) == 0 {
		return ""
	}
	return skills.FormatSkillsForPrompt(sk)
}

// ============================================================================
// Prompt Template Expansion
// ============================================================================

// ParseCommandArgs parses command arguments respecting quoted strings.
func ParseCommandArgs(argsString string) []string {
	var args []string
	var current string
	var inQuote rune

	for _, ch := range argsString {
		if inQuote != 0 {
			if ch == inQuote {
				inQuote = 0
			} else {
				current += string(ch)
			}
		} else if ch == '"' || ch == '\'' {
			inQuote = ch
		} else if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			if current != "" {
				args = append(args, current)
				current = ""
			}
		} else {
			current += string(ch)
		}
	}

	if current != "" {
		args = append(args, current)
	}

	return args
}

// SubstituteArgs replaces argument placeholders in template content.
// Supports $1, $2 (positional), $@ and $ARGUMENTS (all args),
// ${@:N} and ${@:N:L} (slice).
func SubstituteArgs(content string, args []string) string {
	result := content

	// Positional: $1, $2, etc.
	result = positionalRegex.ReplaceAllStringFunc(result, func(match string) string {
		numStr := positionalRegex.FindStringSubmatch(match)[1]
		var num int
		fmt.Sscanf(numStr, "%d", &num)
		idx := num - 1
		if idx >= 0 && idx < len(args) {
			return args[idx]
		}
		return ""
	})

	// Slice: ${@:start} or ${@:start:length}
	result = sliceRegex.ReplaceAllStringFunc(result, func(match string) string {
		sub := sliceRegex.FindStringSubmatch(match)
		var start int
		fmt.Sscanf(sub[1], "%d", &start)
		start-- // 1-indexed to 0-indexed
		if start < 0 {
			start = 0
		}

		if sub[2] != "" {
			var length int
			fmt.Sscanf(sub[2], "%d", &length)
			end := start + length
			if end > len(args) {
				end = len(args)
			}
			if start >= len(args) {
				return ""
			}
			return strings.Join(args[start:end], " ")
		}
		if start >= len(args) {
			return ""
		}
		return strings.Join(args[start:], " ")
	})

	allArgs := strings.Join(args, " ")
	result = strings.ReplaceAll(result, "$ARGUMENTS", allArgs)
	result = strings.ReplaceAll(result, "$@", allArgs)

	return result
}

// ExpandPromptTemplate expands a slash-command into a prompt template if it matches.
// Returns the expanded content, or the original text if not a template.
func ExpandPromptTemplate(text string, templates []resources.PromptTemplate) string {
	if !strings.HasPrefix(text, "/") {
		return text
	}

	rest := text[1:]
	spaceIdx := strings.IndexAny(rest, " \t\n")
	var templateName, argsString string
	if spaceIdx >= 0 {
		templateName = rest[:spaceIdx]
		argsString = rest[spaceIdx+1:]
	} else {
		templateName = rest
		argsString = ""
	}

	for _, t := range templates {
		if t.Name == templateName {
			args := ParseCommandArgs(argsString)
			return SubstituteArgs(t.Content, args)
		}
	}

	return text
}
