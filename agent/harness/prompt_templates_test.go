package harness

import (
	"testing"
)

func TestParseCommandArgs(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"hello world", []string{"hello", "world"}},
		{`hello "world foo"`, []string{"hello", "world foo"}},
		{`hello 'world foo'`, []string{"hello", "world foo"}},
		{"", nil},
		{"single", []string{"single"}},
		{`a "b c" 'd e' f`, []string{"a", "b c", "d e", "f"}},
		{"  a  b  ", []string{"a", "b"}},
	}
	for _, tt := range tests {
		got := ParseCommandArgs(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("ParseCommandArgs(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("ParseCommandArgs(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestSubstituteArgs(t *testing.T) {
	args := []string{"alpha", "beta", "gamma"}

	tests := []struct {
		template, want string
	}{
		{"$1", "alpha"},
		{"$2", "beta"},
		{"$3", "gamma"},
		{"$4", ""}, // out of range
		{"$@", "alpha beta gamma"},
		{"$ARGUMENTS", "alpha beta gamma"},
		{"${@:2}", "beta gamma"},
		{"${@:2:1}", "beta"},
		{"${@:1:2}", "alpha beta"},
		{"Hello $1, you are $2", "Hello alpha, you are beta"},
	}
	for _, tt := range tests {
		got := SubstituteArgs(tt.template, args)
		if got != tt.want {
			t.Errorf("SubstituteArgs(%q, %v) = %q, want %q", tt.template, args, got, tt.want)
		}
	}
}

func TestSubstituteArgs_Empty(t *testing.T) {
	got := SubstituteArgs("no args here $1 $2", nil)
	want := "no args here  "
	if got != want {
		t.Errorf("SubstituteArgs with nil = %q, want %q", got, want)
	}
}

func TestFormatPromptTemplateInvocation(t *testing.T) {
	tmpl := PromptTemplate{
		Name:        "test",
		Description: "A test template",
		Content:     "Hello $1, welcome to $2!",
	}
	result := FormatPromptTemplateInvocation(tmpl, []string{"Alice", "Wonderland"})
	if result != "Hello Alice, welcome to Wonderland!" {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestFormatPromptTemplateList(t *testing.T) {
	templates := []PromptTemplate{
		{Name: "test", Description: "A test"},
		{Name: "empty", Description: ""},
	}
	result := FormatPromptTemplateList(templates)
	if !containsSubstring(result, "test - A test") {
		t.Errorf("expected 'test - A test' in output, got: %s", result)
	}
	if !containsSubstring(result, "empty") {
		t.Errorf("expected 'empty' in output, got: %s", result)
	}

	emptyResult := FormatPromptTemplateList(nil)
	if !containsSubstring(emptyResult, "No prompt templates") {
		t.Errorf("expected 'No prompt templates' for nil, got: %s", emptyResult)
	}
}
