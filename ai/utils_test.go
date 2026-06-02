package ai

import (
	"testing"
)

func TestSanitizeSurrogates_CleanString(t *testing.T) {
	input := "hello world"
	result := SanitizeSurrogates(input)
	if result != input {
		t.Errorf("SanitizeSurrogates(%q) = %q, want %q", input, result, input)
	}
}

func TestSanitizeSurrogates_RemovesReplacementChar(t *testing.T) {
	// Construct a string containing a lone surrogate.
	// Go's UTF-8 decoder will decode \xED\xA0\x80 as U+FFFD (replacement char).
	input := "hello\uFFFDworld"
	result := SanitizeSurrogates(input)
	if result != "helloworld" {
		t.Errorf("SanitizeSurrogates(%q) = %q, want %q", input, result, "helloworld")
	}
}

func TestSanitizeSurrogates_KeepsValidUnicode(t *testing.T) {
	input := "hello 🌍 world"
	result := SanitizeSurrogates(input)
	if result != input {
		t.Errorf("SanitizeSurrogates(%q) = %q, want unchanged", input, result)
	}
}

func TestParseStreamingJSON_Valid(t *testing.T) {
	result, ok := ParseStreamingJSON(`{"key": "value"}`)
	if !ok {
		t.Error("ParseStreamingJSON should parse valid JSON")
	}
	if result["key"] != "value" {
		t.Errorf("result[\"key\"] = %v, want \"value\"", result["key"])
	}
}

func TestParseStreamingJSON_Empty(t *testing.T) {
	_, ok := ParseStreamingJSON("")
	if ok {
		t.Error("ParseStreamingJSON should return false for empty string")
	}
}

func TestParseStreamingJSON_Truncated(t *testing.T) {
	// Truncated JSON: missing closing brace
	result, ok := ParseStreamingJSON(`{"key": "value"`)
	if !ok {
		t.Error("ParseStreamingJSON should repair truncated JSON")
	}
	if result["key"] != "value" {
		t.Errorf("result[\"key\"] = %v, want \"value\"", result["key"])
	}
}

func TestParseStreamingJSON_TruncatedArray(t *testing.T) {
	result, ok := ParseStreamingJSONArray(`[1, 2, 3`)
	if !ok {
		t.Error("ParseStreamingJSONArray should repair truncated array")
	}
	if len(result) != 3 {
		t.Errorf("len(result) = %d, want 3", len(result))
	}
}

func TestParseStreamingJSON_Invalid(t *testing.T) {
	_, ok := ParseStreamingJSON("not json at all")
	if ok {
		t.Error("ParseStreamingJSON should return false for non-JSON")
	}
}

func TestRepairJSON_RespectsStringBoundaries(t *testing.T) {
	// Braces inside strings should not affect depth counting
	input := `{"text": "hello {world} this is "`
	result, ok := ParseStreamingJSON(input)
	if !ok {
		t.Error("ParseStreamingJSON should repair JSON with braces in strings")
	}
	if result["text"] != "hello {world} this is " {
		t.Errorf("text = %v, want 'hello {world} this is '", result["text"])
	}
}
