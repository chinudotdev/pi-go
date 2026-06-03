package harness

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
)

// SkillDiagnosticCode is a stable code for skill loading diagnostics.
type SkillDiagnosticCode string

const (
	SkillDiagFileInfoFailed SkillDiagnosticCode = "file_info_failed"
	SkillDiagListFailed     SkillDiagnosticCode = "list_failed"
	SkillDiagReadFailed     SkillDiagnosticCode = "read_failed"
	SkillDiagParseFailed    SkillDiagnosticCode = "parse_failed"
	SkillDiagInvalidMeta    SkillDiagnosticCode = "invalid_metadata"
)

// SkillDiagnostic is a warning produced while loading skills.
type SkillDiagnostic struct {
	Type    SkillDiagnosticCode `json:"type"`
	Code    SkillDiagnosticCode `json:"code"`
	Message string              `json:"message"`
	Path    string              `json:"path"`
}

const (
	maxSkillNameLength        = 64
	maxSkillDescriptionLength = 1024
)

var (
	ignoreFileNames = []string{".gitignore", ".ignore", ".fdignore"}
	validNameRegex  = regexp.MustCompile(`^[a-z0-9-]+$`)
)

// LoadSkills loads skills from one or more directories.
// Traverses directories recursively, loads SKILL.md files and root .md files,
// honors .gitignore/.ignore/.fdignore files.
func LoadSkills(ctx context.Context, env ExecutionEnv, dirs []string) ([]Skill, []SkillDiagnostic) {
	var skills []Skill
	var diagnostics []SkillDiagnostic

	for _, dir := range dirs {
		infoResult := env.FileInfo(ctx, dir)
		if !infoResult.OK {
			fe, ok := infoResult.Err.(*FileError)
			if !ok || fe.Code != FileErrorNotFound {
				diagnostics = append(diagnostics, SkillDiagnostic{
					Type: SkillDiagFileInfoFailed, Code: SkillDiagFileInfoFailed,
					Message: errorString(infoResult.Err), Path: dir,
				})
			}
			continue
		}
		kind := resolveKindSync(env, ctx, infoResult.Value, &diagnostics)
		if kind != "directory" {
			continue
		}
		subSkills, subDiags := loadSkillsFromDir(ctx, env, infoResult.Value.Path, true, newIgnoreMatcher(), infoResult.Value.Path)
		skills = append(skills, subSkills...)
		diagnostics = append(diagnostics, subDiags...)
	}

	return skills, diagnostics
}

// LoadSourcedSkills loads skills from directories tagged with a source value.
func LoadSourcedSkills[TSource any](
	ctx context.Context,
	env ExecutionEnv,
	inputs []struct {
		Path   string
		Source TSource
	},
	mapSkill func(Skill, TSource) Skill,
) ([]struct {
	Skill  Skill
	Source TSource
}, []struct {
	Diagnostic SkillDiagnostic
	Source     TSource
}) {
	var skills []struct {
		Skill  Skill
		Source TSource
	}
	var diagnostics []struct {
		Diagnostic SkillDiagnostic
		Source     TSource
	}

	for _, input := range inputs {
		loadedSkills, loadedDiags := LoadSkills(ctx, env, []string{input.Path})
		for _, s := range loadedSkills {
			if mapSkill != nil {
				s = mapSkill(s, input.Source)
			}
			skills = append(skills, struct {
				Skill  Skill
				Source TSource
			}{Skill: s, Source: input.Source})
		}
		for _, d := range loadedDiags {
			diagnostics = append(diagnostics, struct {
				Diagnostic SkillDiagnostic
				Source     TSource
			}{Diagnostic: d, Source: input.Source})
		}
	}

	return skills, diagnostics
}

func loadSkillsFromDir(
	ctx context.Context,
	env ExecutionEnv,
	dir string,
	includeRootFiles bool,
	ig *ignoreMatcher,
	rootDir string,
) ([]Skill, []SkillDiagnostic) {
	var skills []Skill
	var diagnostics []SkillDiagnostic

	infoResult := env.FileInfo(ctx, dir)
	if !infoResult.OK {
		fe, ok := infoResult.Err.(*FileError)
		if !ok || fe.Code != FileErrorNotFound {
			diagnostics = append(diagnostics, SkillDiagnostic{
				Type: SkillDiagFileInfoFailed, Code: SkillDiagFileInfoFailed,
				Message: errorString(infoResult.Err), Path: dir,
			})
		}
		return skills, diagnostics
	}
	kind := resolveKindSync(env, ctx, infoResult.Value, &diagnostics)
	if kind != "directory" {
		return skills, diagnostics
	}

	addIgnoreRules(ctx, env, ig, dir, rootDir, &diagnostics)

	entriesResult := env.ListDir(ctx, dir)
	if !entriesResult.OK {
		diagnostics = append(diagnostics, SkillDiagnostic{
			Type: SkillDiagListFailed, Code: SkillDiagListFailed,
			Message: errorString(entriesResult.Err), Path: dir,
		})
		return skills, diagnostics
	}
	entries := entriesResult.Value

	// First pass: look for SKILL.md
	for _, entry := range entries {
		if entry.Name != "SKILL.md" {
			continue
		}
		kind := resolveKindSync(env, ctx, entry, &diagnostics)
		if kind != "file" {
			continue
		}
		relPath := relativeEnvPath(rootDir, entry.Path)
		if ig.matches(relPath) {
			continue
		}
		skill, diags := loadSkillFromFile(ctx, env, entry.Path)
		if skill != nil {
			skills = append(skills, *skill)
		}
		diagnostics = append(diagnostics, diags...)
		return skills, diagnostics
	}

	// Second pass: recurse into subdirectories and load root .md files
	sorted := make([]FileInfo, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	for _, entry := range sorted {
		if strings.HasPrefix(entry.Name, ".") || entry.Name == "node_modules" {
			continue
		}
		kind := resolveKindSync(env, ctx, entry, &diagnostics)
		if kind == "" {
			continue
		}

		relPath := relativeEnvPath(rootDir, entry.Path)
		ignorePath := relPath
		if kind == "directory" {
			ignorePath = relPath + "/"
		}
		if ig.matches(ignorePath) {
			continue
		}

		if kind == "directory" {
			subSkills, subDiags := loadSkillsFromDir(ctx, env, entry.Path, false, ig, rootDir)
			skills = append(skills, subSkills...)
			diagnostics = append(diagnostics, subDiags...)
			continue
		}

		if kind != "file" || !includeRootFiles || !strings.HasSuffix(entry.Name, ".md") {
			continue
		}
		skill, diags := loadSkillFromFile(ctx, env, entry.Path)
		if skill != nil {
			skills = append(skills, *skill)
		}
		diagnostics = append(diagnostics, diags...)
	}

	return skills, diagnostics
}

func loadSkillFromFile(ctx context.Context, env ExecutionEnv, filePath string) (*Skill, []SkillDiagnostic) {
	var diagnostics []SkillDiagnostic

	rawResult := env.ReadTextFile(ctx, filePath)
	if !rawResult.OK {
		diagnostics = append(diagnostics, SkillDiagnostic{
			Type: SkillDiagReadFailed, Code: SkillDiagReadFailed,
			Message: errorString(rawResult.Err), Path: filePath,
		})
		return nil, diagnostics
	}

	frontmatter, body, err := parseFrontmatter(rawResult.Value)
	if err != nil {
		diagnostics = append(diagnostics, SkillDiagnostic{
			Type: SkillDiagParseFailed, Code: SkillDiagParseFailed,
			Message: err.Error(), Path: filePath,
		})
		return nil, diagnostics
	}

	skillDir := dirnameEnvPath(filePath)
	parentDirName := basenameEnvPath(skillDir)

	description := ""
	if desc, ok := frontmatter["description"].(string); ok {
		description = desc
	}

	for _, e := range validateDescription(description) {
		diagnostics = append(diagnostics, SkillDiagnostic{
			Type: SkillDiagInvalidMeta, Code: SkillDiagInvalidMeta,
			Message: e, Path: filePath,
		})
	}

	name := parentDirName
	if n, ok := frontmatter["name"].(string); ok && n != "" {
		name = n
	}
	for _, e := range validateSkillName(name, parentDirName) {
		diagnostics = append(diagnostics, SkillDiagnostic{
			Type: SkillDiagInvalidMeta, Code: SkillDiagInvalidMeta,
			Message: e, Path: filePath,
		})
	}

	if strings.TrimSpace(description) == "" {
		return nil, diagnostics
	}

	disableModelInvocation := false
	if v, ok := frontmatter["disable-model-invocation"].(bool); ok {
		disableModelInvocation = v
	}

	return &Skill{
		Name:                   name,
		Description:            description,
		Content:                body,
		FilePath:               filePath,
		DisableModelInvocation: disableModelInvocation,
	}, diagnostics
}

func validateSkillName(name, parentDirName string) []string {
	var errs []string
	if name != parentDirName {
		errs = append(errs, fmt.Sprintf("name %q does not match parent directory %q", name, parentDirName))
	}
	if len(name) > maxSkillNameLength {
		errs = append(errs, fmt.Sprintf("name exceeds %d characters (%d)", maxSkillNameLength, len(name)))
	}
	if !validNameRegex.MatchString(name) {
		errs = append(errs, "name contains invalid characters (must be lowercase a-z, 0-9, hyphens only)")
	}
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		errs = append(errs, "name must not start or end with a hyphen")
	}
	if strings.Contains(name, "--") {
		errs = append(errs, "name must not contain consecutive hyphens")
	}
	return errs
}

func validateDescription(description string) []string {
	var errs []string
	if strings.TrimSpace(description) == "" {
		errs = append(errs, "description is required")
	} else if len(description) > maxSkillDescriptionLength {
		errs = append(errs, fmt.Sprintf("description exceeds %d characters (%d)", maxSkillDescriptionLength, len(description)))
	}
	return errs
}

// parseFrontmatter parses YAML frontmatter from markdown content.
// Returns the frontmatter map and the body (content after the closing ---).
func parseFrontmatter(content string) (map[string]any, string, error) {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")

	if !strings.HasPrefix(normalized, "---") {
		return map[string]any{}, normalized, nil
	}
	endIndex := strings.Index(normalized[3:], "\n---")
	if endIndex == -1 {
		return map[string]any{}, normalized, nil
	}
	// endIndex is relative to normalized[3:]
	// yamlString is between opening --- and closing \n---
	yamlStart := 3
	// Skip the newline after opening ---
	if normalized[yamlStart] == '\n' {
		yamlStart++
	}
	yamlEnd := 3 + endIndex // position of \n before closing ---
	if yamlStart > yamlEnd {
		yamlStart = yamlEnd
	}
	yamlString := normalized[yamlStart:yamlEnd]
	bodyStart := 3 + endIndex + 4 // skip \n---\n
	body := ""
	if bodyStart < len(normalized) {
		body = strings.TrimSpace(normalized[bodyStart:])
	}

	var frontmatter map[string]any
	if err := yaml.Unmarshal([]byte(yamlString), &frontmatter); err != nil {
		return nil, "", err
	}
	if frontmatter == nil {
		frontmatter = map[string]any{}
	}
	return frontmatter, body, nil
}

func addIgnoreRules(
	ctx context.Context,
	env ExecutionEnv,
	ig *ignoreMatcher,
	dir string,
	rootDir string,
	diagnostics *[]SkillDiagnostic,
) {
	relativeDir := relativeEnvPath(rootDir, dir)
	prefix := relativeDir
	if prefix != "" {
		prefix += "/"
	}

	for _, filename := range ignoreFileNames {
		ignorePath := joinEnvPath(dir, filename)
		info := env.FileInfo(ctx, ignorePath)
		if !info.OK {
			continue
		}
		if info.Value.Kind != FileKindFile {
			continue
		}
		content := env.ReadTextFile(ctx, ignorePath)
		if !content.OK {
			*diagnostics = append(*diagnostics, SkillDiagnostic{
				Type: SkillDiagReadFailed, Code: SkillDiagReadFailed,
				Message: errorString(content.Err), Path: ignorePath,
			})
			continue
		}
		for _, line := range strings.Split(content.Value, "\n") {
			pattern := prefixIgnorePattern(line, prefix)
			if pattern != "" {
				ig.addPattern(pattern)
			}
		}
	}
}

func prefixIgnorePattern(line, prefix string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "\\#") {
		return ""
	}

	pattern := line
	negated := false
	if strings.HasPrefix(pattern, "!") {
		negated = true
		pattern = pattern[1:]
	} else if strings.HasPrefix(pattern, "\\!") {
		pattern = pattern[1:]
	}
	if strings.HasPrefix(pattern, "/") {
		pattern = pattern[1:]
	}
	prefixed := pattern
	if prefix != "" {
		prefixed = prefix + pattern
	}
	if negated {
		return "!" + prefixed
	}
	return prefixed
}

// resolveKindSync resolves the kind of a FileInfo, following symlinks.
func resolveKindSync(env ExecutionEnv, ctx context.Context, info FileInfo, diagnostics *[]SkillDiagnostic) string {
	if info.Kind == FileKindFile || info.Kind == FileKindDirectory {
		return string(info.Kind)
	}
	canonical := env.CanonicalPath(ctx, info.Path)
	if !canonical.OK {
		fe, ok := canonical.Err.(*FileError)
		if !ok || fe.Code != FileErrorNotFound {
			*diagnostics = append(*diagnostics, SkillDiagnostic{
				Type: SkillDiagFileInfoFailed, Code: SkillDiagFileInfoFailed,
				Message: errorString(canonical.Err), Path: info.Path,
			})
		}
		return ""
	}
	target := env.FileInfo(ctx, canonical.Value)
	if !target.OK {
		fe, ok := target.Err.(*FileError)
		if !ok || fe.Code != FileErrorNotFound {
			*diagnostics = append(*diagnostics, SkillDiagnostic{
				Type: SkillDiagFileInfoFailed, Code: SkillDiagFileInfoFailed,
				Message: errorString(target.Err), Path: info.Path,
			})
		}
		return ""
	}
	if target.Value.Kind == FileKindFile || target.Value.Kind == FileKindDirectory {
		return string(target.Value.Kind)
	}
	return ""
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
