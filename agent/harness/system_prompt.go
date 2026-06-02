package harness

import (
	"fmt"
	"strings"
)

// FormatSkillsForSystemPrompt formats available skills as an XML block for the system prompt.
// Skills with DisableModelInvocation=true are excluded.
func FormatSkillsForSystemPrompt(skills []Skill) string {
	visible := make([]Skill, 0, len(skills))
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
		"The following skills provide specialized instructions for specific tasks.",
		"Read the full skill file when the task matches its description.",
		"When a skill file references a relative path, resolve it against the skill directory (parent of SKILL.md / dirname of the path) and use that absolute path in tool commands.",
		"",
		"<available_skills>",
	)

	for _, skill := range visible {
		lines = append(lines,
			"  <skill>",
			fmt.Sprintf("    <name>%s</name>", escapeXML(skill.Name)),
			fmt.Sprintf("    <description>%s</description>", escapeXML(skill.Description)),
			fmt.Sprintf("    <location>%s</location>", escapeXML(skill.FilePath)),
			"  </skill>",
		)
	}

	lines = append(lines, "</available_skills>")
	return strings.Join(lines, "\n")
}

// FormatSkillInvocation produces a prompt that loads a skill's content into context.
func FormatSkillInvocation(skill Skill, additionalInstructions string) string {
	skillBlock := fmt.Sprintf(
		"<skill name=\"%s\" location=\"%s\">\nReferences are relative to %s.\n\n%s\n</skill>",
		escapeXML(skill.Name),
		escapeXML(skill.FilePath),
		dirnameEnvPath(skill.FilePath),
		skill.Content,
	)
	if additionalInstructions != "" {
		return skillBlock + "\n\n" + additionalInstructions
	}
	return skillBlock
}

func escapeXML(value string) string {
	s := strings.ReplaceAll(value, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

func dirnameEnvPath(path string) string {
	normalized := strings.TrimRight(path, "/")
	idx := strings.LastIndex(normalized, "/")
	if idx <= 0 {
		return "/"
	}
	return normalized[:idx]
}

func basenameEnvPath(path string) string {
	normalized := strings.TrimRight(path, "/")
	idx := strings.LastIndex(normalized, "/")
	if idx == -1 {
		return normalized
	}
	return normalized[idx+1:]
}

func joinEnvPath(base, child string) string {
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(child, "/")
}

func relativeEnvPath(root, path string) string {
	normalizedRoot := strings.TrimRight(root, "/")
	normalizedPath := strings.TrimRight(path, "/")
	if normalizedPath == normalizedRoot {
		return ""
	}
	if strings.HasPrefix(normalizedPath, normalizedRoot+"/") {
		return normalizedPath[len(normalizedRoot)+1:]
	}
	return strings.TrimLeft(normalizedPath, "/")
}
