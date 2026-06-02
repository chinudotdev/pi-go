package truncate

import (
	"strings"
	"testing"
)

func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0B"},
		{100, "100B"},
		{1024, "1.0KB"},
		{1536, "1.5KB"},
		{1048576, "1.0MB"},
	}
	for _, tt := range tests {
		got := FormatSize(tt.bytes)
		if got != tt.want {
			t.Errorf("FormatSize(%d) = %q, want %q", tt.bytes, got, tt.want)
		}
	}
}

func TestTruncateHead_NoTruncation(t *testing.T) {
	content := "line1\nline2\nline3"
	result := TruncateHead(content, TruncationOptions{})
	if result.Truncated {
		t.Error("expected no truncation")
	}
	if result.Content != content {
		t.Error("content should be unchanged")
	}
	if result.TotalLines != 3 {
		t.Errorf("expected 3 lines, got %d", result.TotalLines)
	}
}

func TestTruncateHead_ByLines(t *testing.T) {
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "short line"
	}
	content := strings.Join(lines, "\n")

	result := TruncateHead(content, TruncationOptions{MaxLines: 10})
	if !result.Truncated {
		t.Error("expected truncation")
	}
	if result.OutputLines != 10 {
		t.Errorf("expected 10 output lines, got %d", result.OutputLines)
	}
	if result.TruncatedBy != "lines" {
		t.Errorf("expected truncatedBy='lines', got %q", result.TruncatedBy)
	}
}

func TestTruncateHead_ByBytes(t *testing.T) {
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = strings.Repeat("x", 200) // 200 bytes each
	}
	content := strings.Join(lines, "\n")

	result := TruncateHead(content, TruncationOptions{MaxBytes: 1000})
	if !result.Truncated {
		t.Error("expected truncation")
	}
	if result.OutputBytes > 1000 {
		t.Errorf("output bytes %d exceeds max 1000", result.OutputBytes)
	}
	if result.TruncatedBy != "bytes" {
		t.Errorf("expected truncatedBy='bytes', got %q", result.TruncatedBy)
	}
}

func TestTruncateHead_FirstLineExceeds(t *testing.T) {
	content := strings.Repeat("x", 1000)
	result := TruncateHead(content, TruncationOptions{MaxBytes: 100, MaxLines: 10})
	if !result.Truncated {
		t.Error("expected truncation")
	}
	if !result.FirstLineExceeds {
		t.Error("expected FirstLineExceeds")
	}
	if result.Content != "" {
		t.Errorf("expected empty content, got %q", result.Content)
	}
}

func TestTruncateTail_NoTruncation(t *testing.T) {
	content := "line1\nline2\nline3"
	result := TruncateTail(content, TruncationOptions{})
	if result.Truncated {
		t.Error("expected no truncation")
	}
	if result.Content != content {
		t.Error("content should be unchanged")
	}
}

func TestTruncateTail_ByLines(t *testing.T) {
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "short line"
	}
	content := strings.Join(lines, "\n")

	result := TruncateTail(content, TruncationOptions{MaxLines: 10})
	if !result.Truncated {
		t.Error("expected truncation")
	}
	if result.OutputLines != 10 {
		t.Errorf("expected 10 output lines, got %d", result.OutputLines)
	}
	// Should keep the LAST 10 lines
	outputLines := strings.Split(result.Content, "\n")
	if outputLines[0] != "short line" {
		t.Error("expected 'short line' in output")
	}
}

func TestTruncateTail_ByBytes(t *testing.T) {
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = strings.Repeat("y", 200)
	}
	content := strings.Join(lines, "\n")

	result := TruncateTail(content, TruncationOptions{MaxBytes: 1000})
	if !result.Truncated {
		t.Error("expected truncation")
	}
	if result.OutputBytes > 1000+200 { // some slack for partial line
		t.Errorf("output bytes %d significantly exceeds max 1000", result.OutputBytes)
	}
}

func TestTruncateTail_PartialFirstLine(t *testing.T) {
	// Single line that exceeds byte limit
	content := strings.Repeat("z", 2000)
	result := TruncateTail(content, TruncationOptions{MaxBytes: 100, MaxLines: 10})
	if !result.Truncated {
		t.Error("expected truncation")
	}
	if !result.LastLinePartial {
		t.Error("expected LastLinePartial")
	}
	// Should have some content (tail of the line)
	if len(result.Content) == 0 {
		t.Error("expected some content from partial line")
	}
}

func TestTruncateLine(t *testing.T) {
	// Short line — no truncation
	text, truncated := TruncateLine("hello", 100)
	if truncated {
		t.Error("expected no truncation")
	}
	if text != "hello" {
		t.Errorf("expected 'hello', got %q", text)
	}

	// Long line — truncation
	longLine := strings.Repeat("a", 600)
	text, truncated = TruncateLine(longLine, 500)
	if !truncated {
		t.Error("expected truncation")
	}
	if !strings.HasSuffix(text, "... [truncated]") {
		t.Errorf("expected truncation suffix, got %q", text)
	}
	if len(text) > 520 { // 500 + suffix
		t.Errorf("truncated line too long: %d", len(text))
	}
}

func TestSanitizeBinaryOutput(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"hello", "hello"},
		{"hello\tworld\n", "hello\tworld\n"},
		{"hello\x00world", "helloworld"},              // null byte removed
		{"hello\x01world", "helloworld"},              // control char removed
		{"hello\r\nworld", "hello\r\nworld"},          // CR LF kept
		{"text\xEF\xBF\xB9more", "textmore"},           // interlinear annotation removed
	}
	for _, tt := range tests {
		got := SanitizeBinaryOutput(tt.input)
		if got != tt.want {
			t.Errorf("SanitizeBinaryOutput(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestTruncateHead_Defaults(t *testing.T) {
	// Small content should not be truncated with default limits
	content := "hello"
	result := TruncateHead(content, TruncationOptions{})
	if result.Truncated {
		t.Error("expected no truncation with defaults")
	}
	if result.MaxLines != DefaultMaxLines {
		t.Errorf("expected MaxLines=%d, got %d", DefaultMaxLines, result.MaxLines)
	}
	if result.MaxBytes != DefaultMaxBytes {
		t.Errorf("expected MaxBytes=%d, got %d", DefaultMaxBytes, result.MaxBytes)
	}
}

func TestTruncateTail_TrailingNewline(t *testing.T) {
	content := "line1\nline2\n"
	result := TruncateTail(content, TruncationOptions{})
	if !result.Truncated && result.TotalLines == 3 {
		t.Error("trailing newline should not count as extra line")
	}
}
