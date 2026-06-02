package ai

import (
	"encoding/json"
	"strings"
)

// SanitizeSurrogates removes invalid UTF-16 surrogate code points from a string.
// Some LLM APIs return strings with lone surrogates that are invalid UTF-8.
// Go's UTF-8 decoder already replaces these with U+FFFD, so we remove those.
func SanitizeSurrogates(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	modified := false
	for _, r := range s {
		if r == '\uFFFD' {
			modified = true
			continue
		}
		b.WriteRune(r)
	}
	if modified {
		return b.String()
	}
	return s
}

// ParseStreamingJSON attempts to parse potentially incomplete JSON from streaming responses.
func ParseStreamingJSON(s string) (map[string]any, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, false
	}

	var result map[string]any
	err := json.Unmarshal([]byte(s), &result)
	if err == nil {
		return result, true
	}

	repaired := repairJSON(s)
	err = json.Unmarshal([]byte(repaired), &result)
	if err == nil {
		return result, true
	}

	return nil, false
}

// ParseStreamingJSONArray attempts to parse potentially incomplete JSON arrays.
func ParseStreamingJSONArray(s string) ([]any, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, false
	}

	var result []any
	err := json.Unmarshal([]byte(s), &result)
	if err == nil {
		return result, true
	}

	repaired := repairJSON(s)
	err = json.Unmarshal([]byte(repaired), &result)
	if err == nil {
		return result, true
	}

	return nil, false
}

// repairJSON attempts to fix common streaming JSON truncation issues.
// It balances brackets/braces and closes unmatched strings, while being
// careful to avoid breaking JSON that is already structurally valid within strings.
func repairJSON(s string) string {
	// Remove trailing incomplete values (trailing commas, whitespace)
	s = strings.TrimRight(s, ", \t\n\r")

	// Track depth while respecting string boundaries
	braceDepth := 0
	bracketDepth := 0
	inString := false
	escape := false

	for _, ch := range s {
		if escape {
			escape = false
			continue
		}
		if ch == '\\' && inString {
			escape = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch ch {
		case '{':
			braceDepth++
		case '}':
			braceDepth--
		case '[':
			bracketDepth++
		case ']':
			bracketDepth--
		}
	}

	// Close unmatched string
	if inString {
		s += `"`
	}

	// Close open brackets and braces
	for i := 0; i < bracketDepth; i++ {
		s += "]"
	}
	for i := 0; i < braceDepth; i++ {
		s += "}"
	}

	return s
}
