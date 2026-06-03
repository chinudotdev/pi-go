package prompt

import (
	"strings"
	"testing"

	"github.com/chinudotdev/pi-go/sdk/resources"
	"github.com/chinudotdev/pi-go/sdk/skills"
)

func TestBuildSystemPrompt_Default(t *testing.T) {
	prompt := BuildSystemPrompt(BuildSystemPromptOptions{
		CWD:  "/home/user/project",
		Date: "2025-01-15",
	})

	if !strings.Contains(prompt, "expert coding assistant") {
		t.Error("expected default prompt header")
	}
	if !strings.Contains(prompt, "Current date: 2025-01-15") {
		t.Error("expected date")
	}
	if !strings.Contains(prompt, "Current working directory: /home/user/project") {
		t.Error("expected CWD")
	}
	if !strings.Contains(prompt, "Be concise in your responses") {
		t.Error("expected default guideline")
	}
}

func TestBuildSystemPrompt_Custom(t *testing.T) {
	prompt := BuildSystemPrompt(BuildSystemPromptOptions{
		CustomPrompt: "You are a test assistant.",
		CWD:          "/tmp",
		Date:         "2025-01-15",
	})

	if !strings.HasPrefix(prompt, "You are a test assistant.") {
		t.Error("expected custom prompt")
	}
	if !strings.Contains(prompt, "Current date: 2025-01-15") {
		t.Error("expected date after custom prompt")
	}
}

func TestBuildSystemPrompt_WithTools(t *testing.T) {
	prompt := BuildSystemPrompt(BuildSystemPromptOptions{
		SelectedTools: []string{"read", "bash"},
		ToolSnippets: map[string]string{
			"read": "Read files",
			"bash": "Execute commands",
		},
		CWD:  "/tmp",
		Date: "2025-01-15",
	})

	if !strings.Contains(prompt, "- read: Read files") {
		t.Error("expected read tool snippet")
	}
	if !strings.Contains(prompt, "- bash: Execute commands") {
		t.Error("expected bash tool snippet")
	}
}

func TestBuildSystemPrompt_WithContextFiles(t *testing.T) {
	prompt := BuildSystemPrompt(BuildSystemPromptOptions{
		ContextFiles: []resources.ContextFile{
			{Path: "/tmp/AGENTS.md", Content: "Always use tabs"},
		},
		CWD:  "/tmp",
		Date: "2025-01-15",
	})

	if !strings.Contains(prompt, "<project_context>") {
		t.Error("expected project_context section")
	}
	if !strings.Contains(prompt, "Always use tabs") {
		t.Error("expected context file content")
	}
	if !strings.Contains(prompt, `path="/tmp/AGENTS.md"`) {
		t.Error("expected context file path")
	}
}

func TestBuildSystemPrompt_WithSkills(t *testing.T) {
	sk := []skills.Skill{
		{Name: "test-skill", Description: "A test skill", FilePath: "/tmp/SKILL.md"},
	}

	prompt := BuildSystemPrompt(BuildSystemPromptOptions{
		SelectedTools: []string{"read", "bash"},
		Skills:        sk,
		CWD:           "/tmp",
		Date:          "2025-01-15",
	})

	if !strings.Contains(prompt, "<available_skills>") {
		t.Error("expected skills section")
	}
	if !strings.Contains(prompt, "<name>test-skill</name>") {
		t.Error("expected skill name")
	}
}

func TestBuildSystemPrompt_SkillsHiddenWithoutRead(t *testing.T) {
	sk := []skills.Skill{
		{Name: "test-skill", Description: "A test skill", FilePath: "/tmp/SKILL.md"},
	}

	prompt := BuildSystemPrompt(BuildSystemPromptOptions{
		SelectedTools: []string{"bash"}, // no read tool
		Skills:        sk,
		CWD:           "/tmp",
		Date:          "2025-01-15",
	})

	if strings.Contains(prompt, "<available_skills>") {
		t.Error("skills should be hidden without read tool")
	}
}

func TestBuildSystemPrompt_AppendSystemPrompt(t *testing.T) {
	prompt := BuildSystemPrompt(BuildSystemPromptOptions{
		AppendSystemPrompt: "Extra instructions",
		CWD:                "/tmp",
		Date:               "2025-01-15",
	})

	if !strings.Contains(prompt, "Extra instructions") {
		t.Error("expected append system prompt")
	}
}

func TestBuildSystemPrompt_Guidelines(t *testing.T) {
	prompt := BuildSystemPrompt(BuildSystemPromptOptions{
		SelectedTools:    []string{"bash"},
		PromptGuidelines: []string{"Always test your code", "Be thorough"},
		CWD:              "/tmp",
		Date:             "2025-01-15",
	})

	if !strings.Contains(prompt, "Always test your code") {
		t.Error("expected custom guideline")
	}
	if !strings.Contains(prompt, "Be thorough") {
		t.Error("expected custom guideline")
	}
	// Bash-only should trigger bash guideline
	if !strings.Contains(prompt, "Use bash for file operations") {
		t.Error("expected bash guideline when no grep/find/ls")
	}
}

func TestBuildSystemPrompt_GuidelineDedup(t *testing.T) {
	prompt := BuildSystemPrompt(BuildSystemPromptOptions{
		PromptGuidelines: []string{"Be concise in your responses"},
		CWD:              "/tmp",
		Date:             "2025-01-15",
	})

	// Should not appear twice
	count := strings.Count(prompt, "Be concise in your responses")
	if count != 1 {
		t.Errorf("expected guideline once, got %d times", count)
	}
}

func TestBuildToolsList(t *testing.T) {
	result := buildToolsList([]string{"read", "bash"}, map[string]string{
		"read": "Read files",
	})
	if !strings.Contains(result, "- read: Read files") {
		t.Errorf("unexpected tools list: %s", result)
	}
}

func TestBuildToolsList_NoSnippets(t *testing.T) {
	result := buildToolsList([]string{"read"}, nil)
	if result != "(none)" {
		t.Errorf("expected (none), got %q", result)
	}
}

// ============================================================================
// Prompt Template Tests
// ============================================================================

func TestParseCommandArgs(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"hello world", []string{"hello", "world"}},
		{`hello "world foo"`, []string{"hello", "world foo"}},
		{`hello 'world foo'`, []string{"hello", "world foo"}},
		{"", nil},
		{"single", []string{"single"}},
	}
	for _, tc := range tests {
		result := ParseCommandArgs(tc.input)
		if len(result) != len(tc.expected) {
			t.Errorf("ParseCommandArgs(%q): expected %v, got %v", tc.input, tc.expected, result)
			continue
		}
		for i, v := range result {
			if v != tc.expected[i] {
				t.Errorf("ParseCommandArgs(%q)[%d]: expected %q, got %q", tc.input, i, tc.expected[i], v)
			}
		}
	}
}

func TestSubstituteArgs_Positional(t *testing.T) {
	result := SubstituteArgs("Hello $1, $2", []string{"world", "foo"})
	if result != "Hello world, foo" {
		t.Errorf("expected 'Hello world, foo', got %q", result)
	}
}

func TestSubstituteArgs_AllArgs(t *testing.T) {
	result := SubstituteArgs("Args: $@", []string{"a", "b", "c"})
	if result != "Args: a b c" {
		t.Errorf("expected 'Args: a b c', got %q", result)
	}
}

func TestSubstituteArgs_Arguments(t *testing.T) {
	result := SubstituteArgs("Args: $ARGUMENTS", []string{"x", "y"})
	if result != "Args: x y" {
		t.Errorf("expected 'Args: x y', got %q", result)
	}
}

func TestSubstituteArgs_Slice(t *testing.T) {
	result := SubstituteArgs("${@:2}", []string{"a", "b", "c"})
	if result != "b c" {
		t.Errorf("expected 'b c', got %q", result)
	}
}

func TestSubstituteArgs_SliceWithLength(t *testing.T) {
	result := SubstituteArgs("${@:1:2}", []string{"a", "b", "c"})
	if result != "a b" {
		t.Errorf("expected 'a b', got %q", result)
	}
}

func TestSubstituteArgs_MissingArg(t *testing.T) {
	result := SubstituteArgs("$1 and $5", []string{"only"})
	if result != "only and " {
		t.Errorf("expected 'only and ', got %q", result)
	}
}

func TestExpandPromptTemplate(t *testing.T) {
	templates := []resources.PromptTemplate{
		{Name: "fix", Content: "Fix all bugs in $1"},
		{Name: "test", Content: "Write tests for $@",
			FilePath: "/tmp/test.md"},
	}

	// Matching template
	result := ExpandPromptTemplate("/fix main.go", templates)
	if result != "Fix all bugs in main.go" {
		t.Errorf("expected expanded, got %q", result)
	}

	// Non-matching
	result = ExpandPromptTemplate("just a regular message", templates)
	if result != "just a regular message" {
		t.Errorf("expected unchanged, got %q", result)
	}

	// No slash prefix
	result = ExpandPromptTemplate("no template", templates)
	if result != "no template" {
		t.Errorf("expected unchanged, got %q", result)
	}
}
