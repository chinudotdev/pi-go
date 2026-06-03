// Package resources provides filesystem-based resource discovery for the coding agent SDK.
// It discovers skills, prompts, context files (AGENTS.md), and system prompts
// from well-known directory locations, without any extension or package manager support.
package resources

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chinudotdev/pi-go/sdk/config"
	"github.com/chinudotdev/pi-go/sdk/skills"
	"github.com/chinudotdev/pi-go/sdk/sourceinfo"
)

// ConfigDirName is the project-local config directory name.
const ConfigDirName = config.ConfigDirName // ".pi"

// Diagnostic represents a warning or error during resource loading.
type Diagnostic struct {
	Type    string `json:"type"` // "warning", "error", "collision"
	Message string `json:"message"`
	Path    string `json:"path"`
}

// ContextFile represents a loaded context file (AGENTS.md / CLAUDE.md).
type ContextFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// PromptTemplate represents a loaded prompt template.
type PromptTemplate struct {
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	ArgumentHint string          `json:"argumentHint,omitempty"`
	Content      string          `json:"content"`
	FilePath     string          `json:"filePath"`
	SourceInfo   sourceinfo.Info `json:"sourceInfo"`
}

// LoadedResources holds all discovered resources.
type LoadedResources struct {
	Skills             []skills.Skill
	SkillDiagnostics   []Diagnostic
	Prompts            []PromptTemplate
	PromptDiagnostics  []Diagnostic
	ContextFiles       []ContextFile
	SystemPrompt       *string
	AppendSystemPrompt []string
}

// LoaderOptions configures resource loading.
type LoaderOptions struct {
	CWD      string // Working directory
	AgentDir string // Agent config directory (~/.pi/agent)

	// Disable specific resource types
	NoSkills       bool
	NoPrompts      bool
	NoContextFiles bool

	// Additional paths
	AdditionalSkillPaths  []string
	AdditionalPromptPaths []string

	// Overrides
	SystemPromptSource       *string  // File path or raw text
	AppendSystemPromptSource []string // File paths or raw text
}

// Loader discovers and loads resources from the filesystem.
type Loader struct {
	opts     LoaderOptions
	cwd      string
	agentDir string
}

// NewLoader creates a new resource loader.
func NewLoader(opts LoaderOptions) *Loader {
	cwd := opts.CWD
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	agentDir := opts.AgentDir
	if agentDir == "" {
		agentDir = config.GetAgentDir()
	}
	return &Loader{opts: opts, cwd: cwd, agentDir: agentDir}
}

// Load discovers and loads all resources from the filesystem.
func (l *Loader) Load() (*LoadedResources, error) {
	result := &LoadedResources{}

	// 1. Load skills
	if !l.opts.NoSkills {
		l.loadSkills(result)
	}

	// 2. Load prompts
	if !l.opts.NoPrompts {
		l.loadPrompts(result)
	}

	// 3. Load context files
	if !l.opts.NoContextFiles {
		l.loadContextFiles(result)
	}

	// 4. Discover system prompt
	l.loadSystemPrompt(result)

	return result, nil
}

func (l *Loader) loadSkills(result *LoadedResources) {
	skillPaths := make([]string, len(l.opts.AdditionalSkillPaths))
	copy(skillPaths, l.opts.AdditionalSkillPaths)

	skillResult := skills.LoadSkills(skills.LoadSkillsOptions{
		CWD:             l.cwd,
		AgentDir:        l.agentDir,
		SkillPaths:      skillPaths,
		IncludeDefaults: true,
	})

	result.Skills = skillResult.Skills
	for _, d := range skillResult.Diagnostics {
		result.SkillDiagnostics = append(result.SkillDiagnostics, Diagnostic{
			Type:    d.Type,
			Message: d.Message,
			Path:    d.Path,
		})
	}
}

func (l *Loader) loadPrompts(result *LoadedResources) {
	promptPaths := make([]string, len(l.opts.AdditionalPromptPaths))
	copy(promptPaths, l.opts.AdditionalPromptPaths)

	// Load from default directories + additional paths
	templates := l.loadPromptTemplates(promptPaths)

	// Deduplicate by name
	seen := make(map[string]*PromptTemplate)
	for _, t := range templates {
		if existing, ok := seen[t.Name]; ok {
			result.PromptDiagnostics = append(result.PromptDiagnostics, Diagnostic{
				Type:    "collision",
				Message: fmt.Sprintf("name \"/%s\" collision", t.Name),
				Path:    t.FilePath,
			})
			_ = existing
			continue
		}
		tCopy := t
		seen[t.Name] = &tCopy
	}

	for _, t := range templates {
		if _, ok := seen[t.Name]; ok {
			result.Prompts = append(result.Prompts, t)
			delete(seen, t.Name)
		}
	}
}

func (l *Loader) loadPromptTemplates(additionalPaths []string) []PromptTemplate {
	var templates []PromptTemplate

	// Global prompts: agentDir/prompts/
	globalDir := filepath.Join(l.agentDir, "prompts")
	templates = append(templates, l.loadPromptsFromDir(globalDir, "user")...)

	// Project prompts: cwd/.agents/prompts/
	projectDir := filepath.Join(l.cwd, ConfigDirName, "prompts")
	templates = append(templates, l.loadPromptsFromDir(projectDir, "project")...)

	// Additional explicit paths
	for _, rawPath := range additionalPaths {
		resolved := resolvePath(rawPath, l.cwd)
		if !fileExists(resolved) {
			continue
		}
		info, err := os.Stat(resolved)
		if err != nil {
			continue
		}
		if info.IsDir() {
			templates = append(templates, l.loadPromptsFromDir(resolved, "path")...)
		} else if strings.HasSuffix(resolved, ".md") {
			if t := l.loadPromptFromFile(resolved, "path"); t != nil {
				templates = append(templates, *t)
			}
		}
	}

	return templates
}

func (l *Loader) loadPromptsFromDir(dir, source string) []PromptTemplate {
	var templates []PromptTemplate

	entries, err := os.ReadDir(dir)
	if err != nil {
		return templates
	}

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		fullPath := filepath.Join(dir, entry.Name())

		isFile := entry.Type().IsRegular()
		if entry.Type()&os.ModeSymlink != 0 {
			realInfo, err := os.Stat(fullPath)
			if err != nil {
				continue
			}
			isFile = realInfo.Mode().IsRegular()
		}

		if !isFile || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		if t := l.loadPromptFromFile(fullPath, source); t != nil {
			templates = append(templates, *t)
		}
	}

	return templates
}

func (l *Loader) loadPromptFromFile(filePath, source string) *PromptTemplate {
	rawContent, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}

	content := string(rawContent)
	fm := parsePromptFrontmatter(content)
	body := extractBody(content)

	name := filepath.Base(filePath)
	name = strings.TrimSuffix(name, ".md")

	description := fm.Description
	if description == "" {
		// Use first non-empty line
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

	return &PromptTemplate{
		Name:         name,
		Description:  description,
		ArgumentHint: fm.ArgumentHint,
		Content:      body,
		FilePath:     filePath,
		SourceInfo:   sourceinfo.Synthetic(filePath, source),
	}
}

func (l *Loader) loadContextFiles(result *LoadedResources) {
	result.ContextFiles = LoadContextFiles(l.cwd, l.agentDir)
}

func (l *Loader) loadSystemPrompt(result *LoadedResources) {
	// System prompt
	if l.opts.SystemPromptSource != nil {
		resolved := resolvePromptInput(*l.opts.SystemPromptSource, "system prompt")
		result.SystemPrompt = &resolved
	} else {
		if sp := l.discoverFile([]string{
			filepath.Join(l.cwd, ConfigDirName, "SYSTEM.md"),
			filepath.Join(l.agentDir, "SYSTEM.md"),
		}); sp != nil {
			result.SystemPrompt = sp
		}
	}

	// Append system prompt
	if len(l.opts.AppendSystemPromptSource) > 0 {
		for _, src := range l.opts.AppendSystemPromptSource {
			resolved := resolvePromptInput(src, "append system prompt")
			if resolved != "" {
				result.AppendSystemPrompt = append(result.AppendSystemPrompt, resolved)
			}
		}
	} else {
		if sp := l.discoverFile([]string{
			filepath.Join(l.cwd, ConfigDirName, "APPEND_SYSTEM.md"),
			filepath.Join(l.agentDir, "APPEND_SYSTEM.md"),
		}); sp != nil {
			result.AppendSystemPrompt = append(result.AppendSystemPrompt, *sp)
		}
	}
}

func (l *Loader) discoverFile(candidates []string) *string {
	for _, p := range candidates {
		if fileExists(p) {
			content, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			s := string(content)
			return &s
		}
	}
	return nil
}

// LoadContextFiles loads AGENTS.md / CLAUDE.md context files.
// It walks from the CWD up to the root, plus checks the global agent dir.
func LoadContextFiles(cwd, agentDir string) []ContextFile {
	var files []ContextFile
	seenPaths := make(map[string]bool)

	// Global context from agent dir
	if ctx := loadContextFileFromDir(agentDir); ctx != nil {
		files = append(files, *ctx)
		seenPaths[ctx.Path] = true
	}

	// Walk from CWD up to root
	var ancestorFiles []ContextFile
	currentDir := cwd
	for {
		if ctx := loadContextFileFromDir(currentDir); ctx != nil && !seenPaths[ctx.Path] {
			ancestorFiles = append([]ContextFile{*ctx}, ancestorFiles...)
			seenPaths[ctx.Path] = true
		}

		parent := filepath.Dir(currentDir)
		if parent == currentDir {
			break
		}
		currentDir = parent
	}

	files = append(files, ancestorFiles...)
	return files
}

func loadContextFileFromDir(dir string) *ContextFile {
	candidates := []string{"AGENTS.md", "AGENTS.MD", "CLAUDE.md", "CLAUDE.MD"}
	for _, name := range candidates {
		fp := filepath.Join(dir, name)
		if fileExists(fp) {
			content, err := os.ReadFile(fp)
			if err != nil {
				continue
			}
			return &ContextFile{Path: fp, Content: string(content)}
		}
	}
	return nil
}

// resolvePromptInput resolves a string that might be a file path or raw text.
func resolvePromptInput(input, desc string) string {
	if input == "" {
		return ""
	}
	if fileExists(input) {
		content, err := os.ReadFile(input)
		if err != nil {
			return input
		}
		return string(content)
	}
	return input
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func resolvePath(p, base string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	if strings.HasPrefix(p, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		return filepath.Join(home, p[1:])
	}
	return filepath.Join(base, p)
}

// promptFrontmatter holds parsed prompt template frontmatter.
type promptFrontmatter struct {
	Description  string `yaml:"description"`
	ArgumentHint string `yaml:"argument-hint"`
}

func parsePromptFrontmatter(content string) promptFrontmatter {
	fm := promptFrontmatter{}
	if !strings.HasPrefix(content, "---") {
		return fm
	}
	end := strings.Index(content[3:], "---")
	if end < 0 {
		return fm
	}
	fmText := strings.TrimSpace(content[3 : end+3])
	for _, line := range strings.Split(fmText, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := parseYAMLLine(line)
		if !ok {
			continue
		}
		switch key {
		case "description":
			fm.Description = unquote(value)
		case "argument-hint":
			fm.ArgumentHint = unquote(value)
		}
	}
	return fm
}

func extractBody(content string) string {
	if !strings.HasPrefix(content, "---") {
		return content
	}
	end := strings.Index(content[3:], "---")
	if end < 0 {
		return content
	}
	body := content[end+6:] // skip closing ---
	return strings.TrimPrefix(body, "\n")
}

func parseYAMLLine(line string) (key, value string, ok bool) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:idx])
	value = strings.TrimSpace(line[idx+1:])
	return key, value, true
}

func unquote(s string) string {
	if len(s) >= 2 && ((s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'')) {
		return s[1 : len(s)-1]
	}
	return s
}
