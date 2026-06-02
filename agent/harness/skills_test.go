package harness

import (
	"testing"
)

// ============================================================================
// parseFrontmatter
// ============================================================================

func TestParseFrontmatter_NoFrontmatter(t *testing.T) {
	fm, body, err := parseFrontmatter("hello world")
	if err != nil {
		t.Fatal(err)
	}
	if len(fm) != 0 {
		t.Errorf("expected empty frontmatter, got %v", fm)
	}
	if body != "hello world" {
		t.Errorf("expected body 'hello world', got %q", body)
	}
}

func TestParseFrontmatter_WithYAML(t *testing.T) {
	input := "---\nname: test-skill\ndescription: A test\n---\nSkill body here"
	fm, body, err := parseFrontmatter(input)
	if err != nil {
		t.Fatal(err)
	}
	if fm["name"] != "test-skill" {
		t.Errorf("expected name='test-skill', got %v", fm["name"])
	}
	if fm["description"] != "A test" {
		t.Errorf("expected description='A test', got %v", fm["description"])
	}
	if body != "Skill body here" {
		t.Errorf("expected body 'Skill body here', got %q", body)
	}
}

func TestParseFrontmatter_EmptyFrontmatter(t *testing.T) {
	input := "---\n---\nBody only"
	fm, body, err := parseFrontmatter(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(fm) != 0 {
		t.Errorf("expected empty frontmatter, got %v", fm)
	}
	if body != "Body only" {
		t.Errorf("expected body 'Body only', got %q", body)
	}
}

func TestParseFrontmatter_Unterminated(t *testing.T) {
	input := "---\nname: test\nno closing"
	fm, body, err := parseFrontmatter(input)
	if err != nil {
		t.Fatal(err)
	}
	_ = fm
	// Should treat entire content as body
	if body != input {
		t.Errorf("expected original content as body when unterminated")
	}
}

// ============================================================================
// validateSkillName
// ============================================================================

func TestValidateSkillName(t *testing.T) {
	tests := []struct {
		name    string
		dirName string
		errs    int
	}{
		{"my-skill", "my-skill", 0},
		{"wrong-name", "right-name", 1},
		{"UPPER", "upper", 2},                  // case mismatch + invalid chars
		{"a", "a", 0},                           // too short? no, just 1 char
		{"-leading", "-leading", 1},             // starts with hyphen
		{"trailing-", "trailing-", 1},           // ends with hyphen
		{"double--hyphen", "double--hyphen", 1}, // consecutive hyphens
	}
	for _, tt := range tests {
		errs := validateSkillName(tt.name, tt.dirName)
		if len(errs) != tt.errs {
			t.Errorf("validateSkillName(%q, %q) = %d errors, want %d: %v",
				tt.name, tt.dirName, len(errs), tt.errs, errs)
		}
	}
}

func TestValidateSkillName_TooLong(t *testing.T) {
	longName := ""
	for i := 0; i < 100; i++ {
		longName += "a"
	}
	errs := validateSkillName(longName, longName)
	if len(errs) != 1 {
		t.Errorf("expected 1 error for long name, got %d: %v", len(errs), errs)
	}
}

// ============================================================================
// validateDescription
// ============================================================================

func TestValidateDescription(t *testing.T) {
	if len(validateDescription("")) != 1 {
		t.Error("expected 1 error for empty description")
	}
	if len(validateDescription("  ")) != 1 {
		t.Error("expected 1 error for whitespace-only description")
	}
	if len(validateDescription("A valid description")) != 0 {
		t.Error("expected 0 errors for valid description")
	}
}

// ============================================================================
// prefixIgnorePattern
// ============================================================================

func TestPrefixIgnorePattern(t *testing.T) {
	tests := []struct {
		line, prefix, want string
	}{
		{"node_modules", "", "node_modules"},
		{"node_modules", "src/", "src/node_modules"},
		{"# comment", "", ""},
		{"", "", ""},
		{"!important", "", "!important"},
		{"!keep", "src/", "!src/keep"},
		{"/absolute", "", "absolute"},
		{"/absolute", "sub/", "sub/absolute"},
		{"  ", "", ""},
	}
	for _, tt := range tests {
		got := prefixIgnorePattern(tt.line, tt.prefix)
		if got != tt.want {
			t.Errorf("prefixIgnorePattern(%q, %q) = %q, want %q",
				tt.line, tt.prefix, got, tt.want)
		}
	}
}

// ============================================================================
// globMatch
// ============================================================================

func TestGlobMatch(t *testing.T) {
	tests := []struct {
		pattern, str string
		want         bool
	}{
		{"*", "anything", true},
		{"*.log", "test.log", true},
		{"*.log", "test.txt", false},
		{"node_modules", "node_modules", true},
		{"build", "build", true},
		{"build", "build/", false},
		{"*", "", true},
	}
	for _, tt := range tests {
		got := globMatch(tt.pattern, tt.str)
		if got != tt.want {
			t.Errorf("globMatch(%q, %q) = %v, want %v", tt.pattern, tt.str, got, tt.want)
		}
	}
}
