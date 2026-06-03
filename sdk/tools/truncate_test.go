package tools

import (
	"testing"
)

func TestFormatSize(t *testing.T) {
	tests := []struct{ bytes int; expected string }{
		{500, "500B"},
		{1024, "1.0KB"},
		{1536, "1.5KB"},
		{1048576, "1.0MB"},
		{1572864, "1.5MB"},
	}
	for _, tc := range tests {
		got := FormatSize(tc.bytes)
		if got != tc.expected {
			t.Errorf("FormatSize(%d) = %q, want %q", tc.bytes, got, tc.expected)
		}
	}
}

func TestTruncateHead_NoTruncation(t *testing.T) {
	content := "line1\nline2\nline3"
	result := TruncateHead(content, TruncationOptions{MaxLines: 10, MaxBytes: 1024})
	if result.Truncated {
		t.Error("expected no truncation")
	}
	if result.OutputLines != 3 {
		t.Errorf("expected 3 output lines, got %d", result.OutputLines)
	}
}

func TestTruncateHead_ByLines(t *testing.T) {
	content := ""
	for i := 0; i < 100; i++ {
		if i > 0 {
			content += "\n"
		}
		content += "line"
	}
	result := TruncateHead(content, TruncationOptions{MaxLines: 10, MaxBytes: 1024 * 1024})
	if !result.Truncated {
		t.Error("expected truncation")
	}
	if result.TruncatedBy != "lines" {
		t.Errorf("expected truncation by lines, got %q", result.TruncatedBy)
	}
	if result.OutputLines != 10 {
		t.Errorf("expected 10 output lines, got %d", result.OutputLines)
	}
	if result.TotalLines != 100 {
		t.Errorf("expected 100 total lines, got %d", result.TotalLines)
	}
}

func TestTruncateHead_FirstLineExceeds(t *testing.T) {
	// Create a line that exceeds the byte limit
	longLine := make([]byte, 200)
	for i := range longLine {
		longLine[i] = 'x'
	}
	content := string(longLine)
	result := TruncateHead(content, TruncationOptions{MaxLines: 10, MaxBytes: 100})
	if !result.FirstLineExceeds {
		t.Error("expected FirstLineExceeds")
	}
	if result.Content != "" {
		t.Errorf("expected empty content, got %q", result.Content)
	}
}

func TestTruncateTail_NoTruncation(t *testing.T) {
	content := "line1\nline2\nline3"
	result := TruncateTail(content, TruncationOptions{MaxLines: 10, MaxBytes: 1024})
	if result.Truncated {
		t.Error("expected no truncation")
	}
	if result.OutputLines != 3 {
		t.Errorf("expected 3 output lines, got %d", result.OutputLines)
	}
}

func TestTruncateTail_ByLines(t *testing.T) {
	lines := make([]string, 100)
	for i := 0; i < 100; i++ {
		lines[i] = "line"
	}
	content := ""
	for i, l := range lines {
		if i > 0 {
			content += "\n"
		}
		content += l
	}
	result := TruncateTail(content, TruncationOptions{MaxLines: 10, MaxBytes: 1024 * 1024})
	if !result.Truncated {
		t.Error("expected truncation")
	}
	if result.TruncatedBy != "lines" {
		t.Errorf("expected truncation by lines, got %q", result.TruncatedBy)
	}
	if result.OutputLines != 10 {
		t.Errorf("expected 10 output lines, got %d", result.OutputLines)
	}
	// Should contain the LAST 10 lines
	if result.Content != "line\nline\nline\nline\nline\nline\nline\nline\nline\nline" {
		t.Errorf("expected last 10 lines")
	}
}

func TestTruncateLine(t *testing.T) {
	text, truncated := TruncateLine("short")
	if truncated {
		t.Error("expected no truncation for short line")
	}
	if text != "short" {
		t.Errorf("expected 'short', got %q", text)
	}

	longLine := ""
	for i := 0; i < 1000; i++ {
		longLine += "x"
	}
	text, truncated = TruncateLine(longLine)
	if !truncated {
		t.Error("expected truncation for long line")
	}
	if !endsWith(text, "... [truncated]") {
		t.Errorf("expected truncation suffix, got length %d", len(text))
	}
}

func TestTruncateTail_ByBytes(t *testing.T) {
	// Create content where each line is ~100 bytes
	content := ""
	for i := 0; i < 20; i++ {
		if i > 0 {
			content += "\n"
		}
		line := ""
		for j := 0; j < 100; j++ {
			line += "x"
		}
		content += line
	}
	result := TruncateTail(content, TruncationOptions{MaxLines: 1000, MaxBytes: 500})
	if !result.Truncated {
		t.Error("expected truncation")
	}
	if result.TruncatedBy != "bytes" {
		t.Errorf("expected truncation by bytes, got %q", result.TruncatedBy)
	}
}

func endsWith(s, suffix string) bool {
	if len(suffix) > len(s) {
		return false
	}
	return s[len(s)-len(suffix):] == suffix
}
