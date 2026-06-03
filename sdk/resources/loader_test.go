package resources

import (
	"os"
	"path/filepath"
	"testing"
)

func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "pi-resources-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0755)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadContextFiles_WalkUp(t *testing.T) {
	root := tempDir(t)
	subDir := filepath.Join(root, "a", "b", "c")
	os.MkdirAll(subDir, 0755)

	// AGENTS.md at root level
	writeFile(t, filepath.Join(root, "AGENTS.md"), "Root instructions")
	// AGENTS.md at sub/a level
	writeFile(t, filepath.Join(root, "a", "AGENTS.md"), "Level A instructions")

	files := LoadContextFiles(subDir, "")
	if len(files) != 2 {
		t.Fatalf("expected 2 context files, got %d", len(files))
	}

	// Should be ordered: root first, then level A
	if !containsPath(files, filepath.Join(root, "AGENTS.md")) {
		t.Error("expected root AGENTS.md")
	}
	if !containsPath(files, filepath.Join(root, "a", "AGENTS.md")) {
		t.Error("expected level A AGENTS.md")
	}
}

func TestLoadContextFiles_WithGlobal(t *testing.T) {
	cwd := tempDir(t)
	agentDir := tempDir(t)

	writeFile(t, filepath.Join(agentDir, "AGENTS.md"), "Global instructions")
	writeFile(t, filepath.Join(cwd, "AGENTS.md"), "Project instructions")

	files := LoadContextFiles(cwd, agentDir)
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}

	// Global should come first
	if files[0].Path != filepath.Join(agentDir, "AGENTS.md") {
		t.Errorf("expected global first, got %s", files[0].Path)
	}
}

func TestLoadContextFiles_ClaudeMD(t *testing.T) {
	dir := tempDir(t)
	writeFile(t, filepath.Join(dir, "CLAUDE.md"), "Claude instructions")

	files := LoadContextFiles(dir, "")
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Content != "Claude instructions" {
		t.Errorf("expected 'Claude instructions', got %q", files[0].Content)
	}
}

func TestLoadContextFiles_Dedup(t *testing.T) {
	// AGENTS.md in both cwd and agentDir pointing to same file
	dir := tempDir(t)
	writeFile(t, filepath.Join(dir, "AGENTS.md"), "Instructions")

	files := LoadContextFiles(dir, dir)
	if len(files) != 1 {
		t.Fatalf("expected 1 file (deduped), got %d", len(files))
	}
}

func TestLoader_LoadSystemPrompt(t *testing.T) {
	dir := tempDir(t)
	writeFile(t, filepath.Join(dir, "SYSTEM.md"), "Custom system prompt")

	loader := NewLoader(LoaderOptions{
		CWD:      dir,
		AgentDir: dir,
	})

	result, err := loader.Load()
	if err != nil {
		t.Fatal(err)
	}
	if result.SystemPrompt == nil || *result.SystemPrompt != "Custom system prompt" {
		t.Errorf("expected system prompt, got %v", result.SystemPrompt)
	}
}

func TestLoader_LoadAppendSystemPrompt(t *testing.T) {
	dir := tempDir(t)
	writeFile(t, filepath.Join(dir, "APPEND_SYSTEM.md"), "Append this")

	loader := NewLoader(LoaderOptions{
		CWD:      dir,
		AgentDir: dir,
	})

	result, err := loader.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(result.AppendSystemPrompt) != 1 || result.AppendSystemPrompt[0] != "Append this" {
		t.Errorf("expected append prompt, got %v", result.AppendSystemPrompt)
	}
}

func TestLoader_LoadPrompts(t *testing.T) {
	dir := tempDir(t)
	promptsDir := filepath.Join(dir, ".pi", "prompts")
	os.MkdirAll(promptsDir, 0755)
	writeFile(t, filepath.Join(promptsDir, "fix.md"), "---\ndescription: Fix the code\n---\nFix all the bugs")

	loader := NewLoader(LoaderOptions{
		CWD:      dir,
		AgentDir: dir,
	})

	result, err := loader.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Prompts) != 1 {
		t.Fatalf("expected 1 prompt, got %d", len(result.Prompts))
	}
	if result.Prompts[0].Name != "fix" {
		t.Errorf("expected name 'fix', got %q", result.Prompts[0].Name)
	}
	if result.Prompts[0].Description != "Fix the code" {
		t.Errorf("expected description, got %q", result.Prompts[0].Description)
	}
}

func TestLoader_LoadSkills(t *testing.T) {
	dir := tempDir(t)
	skillsDir := filepath.Join(dir, ".pi", "skills", "test-skill")
	os.MkdirAll(skillsDir, 0755)
	writeFile(t, filepath.Join(skillsDir, "SKILL.md"), "---\nname: test-skill\ndescription: A test skill\n---\nContent")

	loader := NewLoader(LoaderOptions{
		CWD:      dir,
		AgentDir: dir,
	})

	result, err := loader.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(result.Skills))
	}
	if result.Skills[0].Name != "test-skill" {
		t.Errorf("expected skill name 'test-skill', got %q", result.Skills[0].Name)
	}
}

func TestLoader_NoSkills(t *testing.T) {
	dir := tempDir(t)
	loader := NewLoader(LoaderOptions{
		CWD:       dir,
		AgentDir:  dir,
		NoSkills:  true,
	})

	result, err := loader.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Skills) != 0 {
		t.Errorf("expected 0 skills when NoSkills=true")
	}
}

func TestLoader_SystemPromptOverride(t *testing.T) {
	dir := tempDir(t)
	override := "Custom override prompt"
	loader := NewLoader(LoaderOptions{
		CWD:                dir,
		AgentDir:           dir,
		SystemPromptSource: &override,
	})

	result, err := loader.Load()
	if err != nil {
		t.Fatal(err)
	}
	if result.SystemPrompt == nil || *result.SystemPrompt != "Custom override prompt" {
		t.Errorf("expected override, got %v", result.SystemPrompt)
	}
}

func TestParsePromptFrontmatter(t *testing.T) {
	content := "---\ndescription: My template\nargument-hint: <file>\n---\nBody content"
	fm := parsePromptFrontmatter(content)
	if fm.Description != "My template" {
		t.Errorf("expected 'My template', got %q", fm.Description)
	}
	if fm.ArgumentHint != "<file>" {
		t.Errorf("expected '<file>', got %q", fm.ArgumentHint)
	}
}

func TestExtractBody(t *testing.T) {
	content := "---\ndescription: test\n---\nBody here"
	body := extractBody(content)
	if body != "Body here" {
		t.Errorf("expected 'Body here', got %q", body)
	}
}

func TestExtractBody_NoFrontmatter(t *testing.T) {
	content := "Just content"
	body := extractBody(content)
	if body != "Just content" {
		t.Errorf("expected 'Just content', got %q", body)
	}
}

func containsPath(files []ContextFile, path string) bool {
	for _, f := range files {
		if f.Path == path {
			return true
		}
	}
	return false
}
