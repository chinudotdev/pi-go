// Package skills provides skill discovery and loading for the coding agent SDK.
// Skills are markdown files with frontmatter that provide specialized instructions
// for the agent. They are discovered from multiple locations on the filesystem.
package skills

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/chinudotdev/pi-go/sdk/config"
	"github.com/chinudotdev/pi-go/sdk/sourceinfo"
)

const maxNameLength = 64
const maxDescriptionLength = 1024

// Skill represents a loaded skill.
type Skill struct {
	Name                 string
	Description          string
	FilePath             string
	BaseDir              string
	SourceInfo           sourceinfo.Info
	DisableModelInvocation bool
}

// LoadSkillsResult holds the result of loading skills.
type LoadSkillsResult struct {
	Skills      []Skill
	Diagnostics []Diagnostic
}

// Diagnostic represents a warning or error during skill loading.
type Diagnostic struct {
	Type    string // "warning", "error"
	Message string
	Path    string
}

// SkillFrontmatter holds parsed frontmatter from a skill file.
type SkillFrontmatter struct {
	Name                  string `yaml:"name"`
	Description           string `yaml:"description"`
	DisableModelInvocation bool  `yaml:"disable-model-invocation"`
}

// LoadSkillsOptions configures skill loading.
type LoadSkillsOptions struct {
	CWD            string   // Working directory for project-local skills
	AgentDir       string   // Agent config directory for global skills
	SkillPaths     []string // Explicit skill paths
	IncludeDefaults bool    // Include default skill directories
}

// LoadSkills loads skills from all configured locations.
func LoadSkills(opts LoadSkillsOptions) LoadSkillsResult {
	skillMap := make(map[string]*Skill)
	realPathSet := make(map[string]bool)
	var diagnostics []Diagnostic

	addSkills := func(result LoadSkillsResult) {
		diagnostics = append(diagnostics, result.Diagnostics...)
		for i := range result.Skills {
			skill := result.Skills[i]
			realPath := canonicalizePath(skill.FilePath)

			if realPathSet[realPath] {
				continue
			}

			if _, exists := skillMap[skill.Name]; exists {
				diagnostics = append(diagnostics, Diagnostic{
					Type:    "collision",
					Message: fmt.Sprintf("name %q collision", skill.Name),
					Path:    skill.FilePath,
				})
				continue
			}

			skillMap[skill.Name] = &skill
			realPathSet[realPath] = true
		}
	}

	if opts.IncludeDefaults {
		userSkillsDir := filepath.Join(opts.AgentDir, "skills")
		addSkills(LoadSkillsFromDir(userSkillsDir, "user"))

		projectSkillsDir := filepath.Join(opts.CWD, config.ConfigDirName, "skills")
		addSkills(LoadSkillsFromDir(projectSkillsDir, "project"))
	}

	for _, rawPath := range opts.SkillPaths {
		resolved := resolvePath(rawPath, opts.CWD)
		if !fileExists(resolved) {
			diagnostics = append(diagnostics, Diagnostic{
				Type:    "warning",
				Message: "skill path does not exist",
				Path:    resolved,
			})
			continue
		}

		info, err := os.Stat(resolved)
		if err != nil {
			continue
		}

		if info.IsDir() {
			addSkills(LoadSkillsFromDir(resolved, "path"))
		} else if strings.HasSuffix(resolved, ".md") {
			result := loadSkillFromFile(resolved, "path")
			if result.Skill != nil {
				addSkills(LoadSkillsResult{Skills: []Skill{*result.Skill}, Diagnostics: result.Diagnostics})
			} else {
				diagnostics = append(diagnostics, result.Diagnostics...)
			}
		} else {
			diagnostics = append(diagnostics, Diagnostic{
				Type:    "warning",
				Message: "skill path is not a markdown file",
				Path:    resolved,
			})
		}
	}

	skills := make([]Skill, 0, len(skillMap))
	for _, s := range skillMap {
		skills = append(skills, *s)
	}
	sort.Slice(skills, func(i, j int) bool {
		return skills[i].Name < skills[j].Name
	})

	return LoadSkillsResult{Skills: skills, Diagnostics: diagnostics}
}

// LoadSkillsFromDir loads skills from a directory.
func LoadSkillsFromDir(dir, source string) LoadSkillsResult {
	return loadSkillsFromDirInternal(dir, source, true)
}

func loadSkillsFromDirInternal(dir, source string, includeRootFiles bool) LoadSkillsResult {
	var skills []Skill
	var diagnostics []Diagnostic

	if !fileExists(dir) {
		return LoadSkillsResult{Skills: skills, Diagnostics: diagnostics}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return LoadSkillsResult{Skills: skills, Diagnostics: diagnostics}
	}

	// Check for SKILL.md at this level first
	for _, entry := range entries {
		if entry.Name() != "SKILL.md" {
			continue
		}
		fullPath := filepath.Join(dir, entry.Name())

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if entry.Type()&os.ModeSymlink != 0 {
			realInfo, err := os.Stat(fullPath)
			if err != nil {
				continue
			}
			info = realInfo
		}

		if !info.Mode().IsRegular() {
			continue
		}

		result := loadSkillFromFile(fullPath, source)
		if result.Skill != nil {
			skills = append(skills, *result.Skill)
		}
		diagnostics = append(diagnostics, result.Diagnostics...)
		return LoadSkillsResult{Skills: skills, Diagnostics: diagnostics}
	}

	// Recurse into subdirectories and check root .md files
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if entry.Name() == "node_modules" {
			continue
		}

		fullPath := filepath.Join(dir, entry.Name())

		info, err := entry.Info()
		if err != nil {
			continue
		}

		isDir := info.IsDir()
		isFile := info.Mode().IsRegular()

		if entry.Type()&os.ModeSymlink != 0 {
			realInfo, err := os.Stat(fullPath)
			if err != nil {
				continue
			}
			isDir = realInfo.IsDir()
			isFile = realInfo.Mode().IsRegular()
		}

		if isDir {
			subResult := loadSkillsFromDirInternal(fullPath, source, false)
			skills = append(skills, subResult.Skills...)
			diagnostics = append(diagnostics, subResult.Diagnostics...)
			continue
		}

		if !isFile || !includeRootFiles || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		result := loadSkillFromFile(fullPath, source)
		if result.Skill != nil {
			skills = append(skills, *result.Skill)
		}
		diagnostics = append(diagnostics, result.Diagnostics...)
	}

	return LoadSkillsResult{Skills: skills, Diagnostics: diagnostics}
}

type singleSkillResult struct {
	Skill       *Skill
	Diagnostics []Diagnostic
}

func loadSkillFromFile(filePath, source string) singleSkillResult {
	var diagnostics []Diagnostic

	rawContent, err := os.ReadFile(filePath)
	if err != nil {
		return singleSkillResult{Diagnostics: []Diagnostic{{
			Type:    "warning",
			Message: fmt.Sprintf("failed to read skill file: %v", err),
			Path:    filePath,
		}}}
	}

	frontmatter := parseFrontmatter(string(rawContent))
	skillDir := filepath.Dir(filePath)
	parentDirName := filepath.Base(skillDir)

	// Validate description
	if frontmatter.Description == "" || strings.TrimSpace(frontmatter.Description) == "" {
		diagnostics = append(diagnostics, Diagnostic{
			Type: "warning", Message: "description is required", Path: filePath,
		})
		return singleSkillResult{Diagnostics: diagnostics}
	}
	if len(frontmatter.Description) > maxDescriptionLength {
		diagnostics = append(diagnostics, Diagnostic{
			Type:    "warning",
			Message: fmt.Sprintf("description exceeds %d characters (%d)", maxDescriptionLength, len(frontmatter.Description)),
			Path:    filePath,
		})
	}

	// Name: from frontmatter or parent directory
	name := frontmatter.Name
	if name == "" {
		name = parentDirName
	}

	// Validate name
	nameErrors := validateName(name)
	for _, e := range nameErrors {
		diagnostics = append(diagnostics, Diagnostic{
			Type: "warning", Message: e, Path: filePath,
		})
	}

	return singleSkillResult{
		Skill: &Skill{
			Name:                 name,
			Description:          frontmatter.Description,
			FilePath:             filePath,
			BaseDir:              skillDir,
			SourceInfo:           sourceinfo.Synthetic(filePath, source),
			DisableModelInvocation: frontmatter.DisableModelInvocation,
		},
		Diagnostics: diagnostics,
	}
}

func validateName(name string) []string {
	var errors []string
	if len(name) > maxNameLength {
		errors = append(errors, fmt.Sprintf("name exceeds %d characters (%d)", maxNameLength, len(name)))
	}
	if !isValidName(name) {
		errors = append(errors, "name contains invalid characters (must be lowercase a-z, 0-9, hyphens only)")
	}
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		errors = append(errors, "name must not start or end with a hyphen")
	}
	if strings.Contains(name, "--") {
		errors = append(errors, "name must not contain consecutive hyphens")
	}
	return errors
}

var validNameRegex = regexp.MustCompile(`^[a-z0-9-]+$`)

func isValidName(name string) bool {
	return validNameRegex.MatchString(name)
}

// FormatSkillsForPrompt formats skills for inclusion in a system prompt.
// Skills with DisableModelInvocation=true are excluded.
func FormatSkillsForPrompt(skills []Skill) string {
	var visible []Skill
	for _, s := range skills {
		if !s.DisableModelInvocation {
			visible = append(visible, s)
		}
	}

	if len(visible) == 0 {
		return ""
	}

	var lines []string
	lines = append(lines,
		"\n\nThe following skills provide specialized instructions for specific tasks.",
		"Use the read tool to load a skill's file when the task matches its description.",
		"When a skill file references a relative path, resolve it against the skill directory (parent of SKILL.md / dirname of the path) and use that absolute path in tool commands.",
		"",
		"<available_skills>",
	)

	for _, skill := range visible {
		lines = append(lines,
			"  <skill>",
			"    <name>"+escapeXML(skill.Name)+"</name>",
			"    <description>"+escapeXML(skill.Description)+"</description>",
			"    <location>"+escapeXML(skill.FilePath)+"</location>",
			"  </skill>",
		)
	}

	lines = append(lines, "</available_skills>")
	return strings.Join(lines, "\n")
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

// parseFrontmatter extracts YAML frontmatter from a markdown file.
func parseFrontmatter(content string) SkillFrontmatter {
	fm := SkillFrontmatter{}

	if !strings.HasPrefix(content, "---") {
		return fm
	}

	// Find closing ---
	end := strings.Index(content[3:], "---")
	if end < 0 {
		return fm
	}

	fmText := strings.TrimSpace(content[3 : end+3])
	lines := strings.Split(fmText, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := parseYAMLLine(line)
		if !ok {
			continue
		}

		switch key {
		case "name":
			fm.Name = unquote(value)
		case "description":
			fm.Description = unquote(value)
		case "disable-model-invocation":
			fm.DisableModelInvocation = value == "true"
		}
	}

	return fm
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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func canonicalizePath(path string) string {
	real, err := filepath.EvalSymlinks(path)
	if err == nil {
		return real
	}
	return path
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

// Suppress unused import warning
var _ = bytes.MinRead
