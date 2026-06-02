package harness

import (
	"testing"
)

func TestEscapeXML(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"hello", "hello"},
		{"a & b", "a &amp; b"},
		{"a < b", "a &lt; b"},
		{"a > b", "a &gt; b"},
		{`a "b" c`, "a &quot;b&quot; c"},
		{"a'b", "a&apos;b"},
		{`<foo>&"bar"'`, "&lt;foo&gt;&amp;&quot;bar&quot;&apos;"},
	}
	for _, tt := range tests {
		got := escapeXML(tt.input)
		if got != tt.want {
			t.Errorf("escapeXML(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatSkillsForSystemPrompt_Empty(t *testing.T) {
	result := FormatSkillsForSystemPrompt(nil)
	if result != "" {
		t.Errorf("expected empty, got %q", result)
	}
}

func TestFormatSkillsForSystemPrompt_Disabled(t *testing.T) {
	skills := []Skill{
		{Name: "hidden", Description: "hidden skill", FilePath: "/a", DisableModelInvocation: true},
	}
	result := FormatSkillsForSystemPrompt(skills)
	if result != "" {
		t.Errorf("expected empty for disabled-only skills, got %q", result)
	}
}

func TestFormatSkillsForSystemPrompt_Visible(t *testing.T) {
	skills := []Skill{
		{Name: "test-skill", Description: "A test skill", FilePath: "/skills/test/SKILL.md"},
	}
	result := FormatSkillsForSystemPrompt(skills)
	if result == "" {
		t.Error("expected non-empty result")
	}
	if !containsSubstring(result, "<available_skills>") {
		t.Error("expected <available_skills> tag")
	}
	if !containsSubstring(result, "test-skill") {
		t.Error("expected skill name in output")
	}
	if !containsSubstring(result, "A test skill") {
		t.Error("expected skill description in output")
	}
}

func TestFormatSkillInvocation(t *testing.T) {
	skill := Skill{
		Name:        "my-skill",
		Description: "desc",
		Content:     "skill content here",
		FilePath:    "/skills/my-skill/SKILL.md",
	}
	result := FormatSkillInvocation(skill, "")
	if !containsSubstring(result, `<skill name="my-skill"`) {
		t.Error("expected skill XML tag")
	}
	if !containsSubstring(result, "skill content here") {
		t.Error("expected skill content")
	}
	if !containsSubstring(result, "References are relative to /skills/my-skill") {
		t.Error("expected references path")
	}

	resultWithInstructions := FormatSkillInvocation(skill, "do extra stuff")
	if !containsSubstring(resultWithInstructions, "do extra stuff") {
		t.Error("expected additional instructions")
	}
}

func TestPathHelpers(t *testing.T) {
	// dirnameEnvPath
	tests := []struct {
		path, want string
	}{
		{"/a/b/c.txt", "/a/b"},
		{"/root", "/"},
		{"single", "/"},
		{"/a/b/", "/a"},
	}
	for _, tt := range tests {
		got := dirnameEnvPath(tt.path)
		if got != tt.want {
			t.Errorf("dirnameEnvPath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}

	// basenameEnvPath
	basetests := []struct {
		path, want string
	}{
		{"/a/b/c.txt", "c.txt"},
		{"single", "single"},
		{"/a/b/", "b"},
	}
	for _, tt := range basetests {
		got := basenameEnvPath(tt.path)
		if got != tt.want {
			t.Errorf("basenameEnvPath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}

	// joinEnvPath
	if got := joinEnvPath("/a/b", "c.txt"); got != "/a/b/c.txt" {
		t.Errorf("joinEnvPath = %q, want /a/b/c.txt", got)
	}
	if got := joinEnvPath("/a/b/", "/c.txt"); got != "/a/b/c.txt" {
		t.Errorf("joinEnvPath = %q, want /a/b/c.txt", got)
	}

	// relativeEnvPath
	if got := relativeEnvPath("/root", "/root/a/b.txt"); got != "a/b.txt" {
		t.Errorf("relativeEnvPath = %q, want a/b.txt", got)
	}
	if got := relativeEnvPath("/root", "/root"); got != "" {
		t.Errorf("relativeEnvPath = %q, want empty", got)
	}
}
