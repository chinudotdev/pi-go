package jsonutil

import (
	"encoding/json"
	"testing"
)

func TestStripCommentsLineComments(t *testing.T) {
	input := `{
		// This is a comment
		"name": "test" // inline comment
	}`
	result := StripComments(input)
	var m map[string]string
	if err := json.Unmarshal([]byte(result), &m); err != nil {
		t.Errorf("invalid JSON after stripping comments: %v\n%s", err, result)
	}
	if m["name"] != "test" {
		t.Errorf("expected test, got %s", m["name"])
	}
}

func TestStripCommentsTrailingCommas(t *testing.T) {
	input := `{
		"a": 1,
		"b": 2,
	}`
	result := StripComments(input)
	var m map[string]int
	if err := json.Unmarshal([]byte(result), &m); err != nil {
		t.Errorf("invalid JSON after stripping trailing commas: %v\n%s", err, result)
	}
	if m["a"] != 1 || m["b"] != 2 {
		t.Errorf("expected a=1 b=2, got %v", m)
	}
}

func TestStripCommentsPreservesStrings(t *testing.T) {
	input := `{"url": "https://example.com//path"}`
	result := StripComments(input)
	var m map[string]string
	if err := json.Unmarshal([]byte(result), &m); err != nil {
		t.Errorf("invalid JSON: %v\n%s", err, result)
	}
	if m["url"] != "https://example.com//path" {
		t.Errorf("url was modified: %s", m["url"])
	}
}

func TestStripCommentsCombined(t *testing.T) {
	input := `{
		// Config file
		"name": "test", // name
		"items": [1, 2, 3,],
	}`
	result := StripComments(input)
	var m map[string]any
	if err := json.Unmarshal([]byte(result), &m); err != nil {
		t.Errorf("invalid JSON: %v\n%s", err, result)
	}
}
