package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "pi-skills-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func writeSkill(t *testing.T, dir, name, description string) {
	t.Helper()
	skillDir := filepath.Join(dir, name)
	os.MkdirAll(skillDir, 0755)
	content := "---\nname: " + name + "\ndescription: " + description + "\n---\nSkill content here"
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0644)
}

func TestLoadSkillsFromDir(t *testing.T) {
	dir := tempDir(t)
	writeSkill(t, dir, "find-skills", "Find and discover skills")
	writeSkill(t, dir, "hex-arch", "Apply hexagonal architecture")

	result := LoadSkillsFromDir(dir, "test")
	if len(result.Skills) != 2 {
		t.Errorf("expected 2 skills, got %d", len(result.Skills))
	}
	if len(result.Diagnostics) != 0 {
		t.Errorf("expected 0 diagnostics, got %d: %v", len(result.Diagnostics), result.Diagnostics)
	}
}

func TestLoadSkillsFromDir_Nonexistent(t *testing.T) {
	result := LoadSkillsFromDir("/nonexistent/path", "test")
	if len(result.Skills) != 0 {
		t.Errorf("expected 0 skills for nonexistent dir")
	}
}

func TestLoadSkillsFromDir_MissingDescription(t *testing.T) {
	dir := tempDir(t)
	skillDir := filepath.Join(dir, "bad-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: bad\n---\nContent"), 0644)

	result := LoadSkillsFromDir(dir, "test")
	if len(result.Skills) != 0 {
		t.Errorf("expected 0 skills for missing description")
	}
	if len(result.Diagnostics) == 0 {
		t.Error("expected diagnostics for missing description")
	}
}

func TestLoadSkillsFromDir_FallbackName(t *testing.T) {
	dir := tempDir(t)
	skillDir := filepath.Join(dir, "my-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\ndescription: A skill without explicit name\n---\nContent"), 0644)

	result := LoadSkillsFromDir(dir, "test")
	if len(result.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(result.Skills))
	}
	if result.Skills[0].Name != "my-skill" {
		t.Errorf("expected name 'my-skill' from dir, got %q", result.Skills[0].Name)
	}
}

func TestLoadSkills(t *testing.T) {
	agentDir := tempDir(t)
	cwd := tempDir(t)

	// Create global skill
	writeSkill(t, filepath.Join(agentDir, "skills"), "global-skill", "A global skill")

	// Create project skill
	writeSkill(t, filepath.Join(cwd, ".pi", "skills"), "project-skill", "A project skill")

	result := LoadSkills(LoadSkillsOptions{
		CWD:             cwd,
		AgentDir:        agentDir,
		IncludeDefaults: true,
	})

	if len(result.Skills) != 2 {
		t.Errorf("expected 2 skills, got %d", len(result.Skills))
	}

	names := make(map[string]bool)
	for _, s := range result.Skills {
		names[s.Name] = true
	}
	if !names["global-skill"] {
		t.Error("expected global-skill")
	}
	if !names["project-skill"] {
		t.Error("expected project-skill")
	}
}

func TestLoadSkills_WithExplicitPaths(t *testing.T) {
	agentDir := tempDir(t)
	cwd := tempDir(t)

	// Create explicit skill file
	explicitDir := tempDir(t)
	writeSkill(t, explicitDir, "explicit-skill", "An explicit skill")

	result := LoadSkills(LoadSkillsOptions{
		CWD:         cwd,
		AgentDir:    agentDir,
		SkillPaths:  []string{filepath.Join(explicitDir, "explicit-skill")},
	})

	if len(result.Skills) != 1 {
		t.Errorf("expected 1 skill, got %d", len(result.Skills))
	}
	if result.Skills[0].Name != "explicit-skill" {
		t.Errorf("expected explicit-skill, got %q", result.Skills[0].Name)
	}
}

func TestLoadSkills_NameCollision(t *testing.T) {
	agentDir := tempDir(t)
	cwd := tempDir(t)

	// Create same-named skill in both locations
	writeSkill(t, filepath.Join(agentDir, "skills"), "my-skill", "Global version")
	writeSkill(t, filepath.Join(cwd, ".pi", "skills"), "my-skill", "Project version")

	result := LoadSkills(LoadSkillsOptions{
		CWD:             cwd,
		AgentDir:        agentDir,
		IncludeDefaults: true,
	})

	if len(result.Skills) != 1 {
		t.Errorf("expected 1 skill (collision), got %d", len(result.Skills))
	}

	// Should have a collision diagnostic
	hasCollision := false
	for _, d := range result.Diagnostics {
		if d.Type == "collision" {
			hasCollision = true
		}
	}
	if !hasCollision {
		t.Error("expected collision diagnostic")
	}
}

func TestFormatSkillsForPrompt(t *testing.T) {
	skills := []Skill{
		{Name: "find-skills", Description: "Find and install skills", FilePath: "/path/to/find-skills/SKILL.md"},
		{Name: "hex-arch", Description: "Apply hexagonal architecture", FilePath: "/path/to/hex-arch/SKILL.md"},
	}

	prompt := FormatSkillsForPrompt(skills)
	if !strings.Contains(prompt, "<available_skills>") {
		t.Error("expected available_skills tag")
	}
	if !strings.Contains(prompt, "<name>find-skills</name>") {
		t.Error("expected find-skills in prompt")
	}
	if !strings.Contains(prompt, "<description>Find and install skills</description>") {
		t.Error("expected description in prompt")
	}
}

func TestFormatSkillsForPrompt_Disabled(t *testing.T) {
	skills := []Skill{
		{Name: "visible", Description: "Visible skill", FilePath: "/path/SKILL.md"},
		{Name: "hidden", Description: "Hidden skill", FilePath: "/path/SKILL.md", DisableModelInvocation: true},
	}

	prompt := FormatSkillsForPrompt(skills)
	if strings.Contains(prompt, "<name>hidden</name>") {
		t.Error("hidden skill should not appear in prompt")
	}
	if !strings.Contains(prompt, "<name>visible</name>") {
		t.Error("visible skill should appear in prompt")
	}
}

func TestFormatSkillsForPrompt_Empty(t *testing.T) {
	prompt := FormatSkillsForPrompt(nil)
	if prompt != "" {
		t.Errorf("expected empty prompt, got %q", prompt)
	}
}

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		valid   bool
		errCnt  int
	}{
		{"my-skill", true, 0},
		{"skill123", true, 0},
		{"My-Skill", false, 1}, // uppercase
		{"-skill", false, 1},   // starts with hyphen
		{"skill-", false, 1},   // ends with hyphen
		{"my--skill", false, 1}, // double hyphen
		{"my_skill", false, 1}, // underscore
	}
	for _, tc := range tests {
		errs := validateName(tc.name)
		if (len(errs) == 0) != tc.valid {
			t.Errorf("validateName(%q): valid=%v but got %d errors: %v", tc.name, tc.valid, len(errs), errs)
		}
	}
}

func TestEscapeXML(t *testing.T) {
	tests := []struct{ in, expected string }{
		{"hello", "hello"},
		{"a<b", "a&lt;b"},
		{"a>b", "a&gt;b"},
		{"a&b", "a&amp;b"},
		{"a\"b", "a&quot;b"},
		{"a'b", "a&apos;b"},
	}
	for _, tc := range tests {
		got := escapeXML(tc.in)
		if got != tc.expected {
			t.Errorf("escapeXML(%q) = %q, want %q", tc.in, got, tc.expected)
		}
	}
}

func TestParseFrontmatter(t *testing.T) {
	content := "---\nname: test-skill\ndescription: A test skill\n---\nSkill content"
	fm := parseFrontmatter(content)
	if fm.Name != "test-skill" {
		t.Errorf("expected name 'test-skill', got %q", fm.Name)
	}
	if fm.Description != "A test skill" {
		t.Errorf("expected description 'A test skill', got %q", fm.Description)
	}
}

func TestParseFrontmatter_Quoted(t *testing.T) {
	content := "---\nname: \"test skill\"\ndescription: 'A quoted description'\n---\nContent"
	fm := parseFrontmatter(content)
	if fm.Name != "test skill" {
		t.Errorf("expected name 'test skill', got %q", fm.Name)
	}
	if fm.Description != "A quoted description" {
		t.Errorf("expected description, got %q", fm.Description)
	}
}

func TestParseFrontmatter_NoFrontmatter(t *testing.T) {
	content := "Just regular markdown\nNo frontmatter here"
	fm := parseFrontmatter(content)
	if fm.Name != "" || fm.Description != "" {
		t.Errorf("expected empty frontmatter, got name=%q desc=%q", fm.Name, fm.Description)
	}
}

func TestParseFrontmatter_DisableModelInvocation(t *testing.T) {
	content := "---\nname: test\ndescription: test\ndisable-model-invocation: true\n---\nContent"
	fm := parseFrontmatter(content)
	if !fm.DisableModelInvocation {
		t.Error("expected DisableModelInvocation=true")
	}
}
