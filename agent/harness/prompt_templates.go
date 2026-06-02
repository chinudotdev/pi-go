package harness

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// PromptTemplateDiagnosticCode is a stable code for prompt template loading diagnostics.
type PromptTemplateDiagnosticCode string

const (
	PTDiagFileInfoFailed PromptTemplateDiagnosticCode = "file_info_failed"
	PTDiagListFailed     PromptTemplateDiagnosticCode = "list_failed"
	PTDiagReadFailed     PromptTemplateDiagnosticCode = "read_failed"
	PTDiagParseFailed    PromptTemplateDiagnosticCode = "parse_failed"
)

// PromptTemplateDiagnostic is a warning produced while loading prompt templates.
type PromptTemplateDiagnostic struct {
	Type    PromptTemplateDiagnosticCode `json:"type"`
	Code    PromptTemplateDiagnosticCode `json:"code"`
	Message string                       `json:"message"`
	Path    string                       `json:"path"`
}

// LoadPromptTemplates loads prompt templates from one or more paths.
// Directory inputs load direct .md children non-recursively.
// File inputs load explicit .md files.
// Missing paths and non-markdown files are skipped.
func LoadPromptTemplates(ctx context.Context, env ExecutionEnv, paths []string) ([]PromptTemplate, []PromptTemplateDiagnostic) {
	var templates []PromptTemplate
	var diagnostics []PromptTemplateDiagnostic

	for _, path := range paths {
		infoResult := env.FileInfo(ctx, path)
		if !infoResult.OK {
			fe, ok := infoResult.Err.(*FileError)
			if !ok || fe.Code != FileErrorNotFound {
				diagnostics = append(diagnostics, PromptTemplateDiagnostic{
					Type: PTDiagFileInfoFailed, Code: PTDiagFileInfoFailed,
					Message: errorString(infoResult.Err), Path: path,
				})
			}
			continue
		}

		var diags []SkillDiagnostic
		kind := resolveKindSync(env, ctx, infoResult.Value, &diags)
		for _, d := range diags {
			diagnostics = append(diagnostics, PromptTemplateDiagnostic{
				Type: PTDiagFileInfoFailed, Code: PTDiagFileInfoFailed,
				Message: d.Message, Path: d.Path,
			})
		}

		if kind == "directory" {
			subTemplates, subDiags := loadTemplatesFromDir(ctx, env, infoResult.Value.Path)
			templates = append(templates, subTemplates...)
			diagnostics = append(diagnostics, subDiags...)
		} else if kind == "file" && strings.HasSuffix(infoResult.Value.Name, ".md") {
			t, diags := loadTemplateFromFile(ctx, env, infoResult.Value.Path)
			if t != nil {
				templates = append(templates, *t)
			}
			diagnostics = append(diagnostics, diags...)
		}
	}

	return templates, diagnostics
}

func loadTemplatesFromDir(ctx context.Context, env ExecutionEnv, dir string) ([]PromptTemplate, []PromptTemplateDiagnostic) {
	var templates []PromptTemplate
	var diagnostics []PromptTemplateDiagnostic

	entriesResult := env.ListDir(ctx, dir)
	if !entriesResult.OK {
		diagnostics = append(diagnostics, PromptTemplateDiagnostic{
			Type: PTDiagListFailed, Code: PTDiagListFailed,
			Message: errorString(entriesResult.Err), Path: dir,
		})
		return templates, diagnostics
	}

	sorted := make([]FileInfo, len(entriesResult.Value))
	copy(sorted, entriesResult.Value)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	for _, entry := range sorted {
		var skillDiags []SkillDiagnostic
		kind := resolveKindSync(env, ctx, entry, &skillDiags)
		for _, d := range skillDiags {
			diagnostics = append(diagnostics, PromptTemplateDiagnostic{
				Type: PTDiagFileInfoFailed, Code: PTDiagFileInfoFailed,
				Message: d.Message, Path: d.Path,
			})
		}
		if kind != "file" || !strings.HasSuffix(entry.Name, ".md") {
			continue
		}
		t, diags := loadTemplateFromFile(ctx, env, entry.Path)
		if t != nil {
			templates = append(templates, *t)
		}
		diagnostics = append(diagnostics, diags...)
	}

	return templates, diagnostics
}

func loadTemplateFromFile(ctx context.Context, env ExecutionEnv, filePath string) (*PromptTemplate, []PromptTemplateDiagnostic) {
	var diagnostics []PromptTemplateDiagnostic

	rawResult := env.ReadTextFile(ctx, filePath)
	if !rawResult.OK {
		diagnostics = append(diagnostics, PromptTemplateDiagnostic{
			Type: PTDiagReadFailed, Code: PTDiagReadFailed,
			Message: errorString(rawResult.Err), Path: filePath,
		})
		return nil, diagnostics
	}

	frontmatter, body, err := parseFrontmatter(rawResult.Value)
	if err != nil {
		diagnostics = append(diagnostics, PromptTemplateDiagnostic{
			Type: PTDiagParseFailed, Code: PTDiagParseFailed,
			Message: err.Error(), Path: filePath,
		})
		return nil, diagnostics
	}

	description := ""
	if desc, ok := frontmatter["description"].(string); ok {
		description = desc
	}
	if description == "" {
		// Use first non-empty line, capped at 60 chars
		for _, line := range strings.Split(body, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				description = trimmed
				if len(description) > 60 {
					description = description[:60] + "..."
				}
				break
			}
		}
	}

	name := strings.TrimSuffix(basenameEnvPath(filePath), ".md")

	return &PromptTemplate{
		Name:        name,
		Description: description,
		Content:     body,
	}, diagnostics
}

// ParseCommandArgs parses an argument string using simple shell-style single and double quotes.
func ParseCommandArgs(argsString string) []string {
	var args []string
	var current strings.Builder
	inQuote := rune(0)

	for _, char := range argsString {
		if inQuote != 0 {
			if char == inQuote {
				inQuote = 0
			} else {
				current.WriteRune(char)
			}
		} else if char == '"' || char == '\'' {
			inQuote = char
		} else if char == ' ' || char == '\t' {
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		} else {
			current.WriteRune(char)
		}
	}
	if current.Len() > 0 {
		args = append(args, current.String())
	}
	return args
}

// SubstituteArgs substitutes prompt template placeholders ($1, $@, $ARGUMENTS, ${@:N}, ${@:N:L})
// with command arguments.
func SubstituteArgs(content string, args []string) string {
	result := content

	// $N → positional args
	result = regexpDollarNum.ReplaceAllStringFunc(result, func(match string) string {
		numStr := match[1:]
		num, err := strconv.Atoi(numStr)
		if err != nil || num < 1 || num > len(args) {
			return ""
		}
		return args[num-1]
	})

	// ${@:N} and ${@:N:L}
	result = regexpDollarAtRange.ReplaceAllStringFunc(result, func(match string) string {
		parts := rangeSplit.FindStringSubmatch(match)
		if len(parts) < 3 {
			return ""
		}
		start, err := strconv.Atoi(parts[1])
		if err != nil {
			return ""
		}
		start-- // 1-indexed to 0-indexed
		if start < 0 {
			start = 0
		}
		if parts[2] != "" {
			length, err := strconv.Atoi(parts[2])
			if err != nil {
				return ""
			}
			end := start + length
			if end > len(args) {
				end = len(args)
			}
			return strings.Join(args[start:end], " ")
		}
		return strings.Join(args[start:], " ")
	})

	allArgs := strings.Join(args, " ")
	result = strings.ReplaceAll(result, "$ARGUMENTS", allArgs)
	result = strings.ReplaceAll(result, "$@", allArgs)

	return result
}

// FormatPromptTemplateInvocation formats a prompt template with positional arguments.
func FormatPromptTemplateInvocation(template PromptTemplate, args []string) string {
	return SubstituteArgs(template.Content, args)
}

var (
	regexpDollarNum     = regexp.MustCompile(`\$(\d+)`)
	regexpDollarAtRange = regexp.MustCompile(`\$\{@:(\d+)(?::(\d+))?\}`)
	rangeSplit          = regexp.MustCompile(`\$\{@:(\d+)(?::(\d+))?\}`)
)

// FormatPromptTemplateList formats the list of templates for display.
func FormatPromptTemplateList(templates []PromptTemplate) string {
	if len(templates) == 0 {
		return "No prompt templates available."
	}
	var lines []string
	for _, t := range templates {
		if t.Description != "" {
			lines = append(lines, fmt.Sprintf("  %s - %s", t.Name, t.Description))
		} else {
			lines = append(lines, fmt.Sprintf("  %s", t.Name))
		}
	}
	return "Available prompt templates:\n" + strings.Join(lines, "\n")
}
