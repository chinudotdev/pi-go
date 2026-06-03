// Package tools provides built-in tool implementations for the coding agent SDK.
// This file contains shared truncation utilities for tool outputs.
package tools

import (
	"fmt"
)

// Default truncation limits.
const (
	DefaultMaxLines = 2000
	DefaultMaxBytes = 50 * 1024 // 50KB
	GrepMaxLineLen  = 500       // Max chars per grep match line
)

// TruncationResult holds metadata about a truncation operation.
type TruncationResult struct {
	Content          string // The truncated content
	Truncated        bool   // Whether truncation occurred
	TruncatedBy      string // "lines", "bytes", or "" if not truncated
	TotalLines       int    // Lines in the original content
	TotalBytes       int    // Bytes in the original content
	OutputLines      int    // Complete lines in the truncated output
	OutputBytes      int    // Bytes in the truncated output
	LastLinePartial  bool   // Whether the last line was partially truncated (tail mode)
	FirstLineExceeds bool   // Whether the first line exceeded the byte limit (head mode)
	MaxLines         int    // The max lines limit applied
	MaxBytes         int    // The max bytes limit applied
}

// TruncationOptions configures truncation limits.
type TruncationOptions struct {
	MaxLines int // Maximum number of lines (default: 2000)
	MaxBytes int // Maximum number of bytes (default: 50KB)
}

// FormatSize formats bytes as a human-readable size string.
func FormatSize(bytes int) string {
	if bytes < 1024 {
		return fmt.Sprintf("%dB", bytes)
	} else if bytes < 1024*1024 {
		return fmt.Sprintf("%.1fKB", float64(bytes)/1024)
	}
	return fmt.Sprintf("%.1fMB", float64(bytes)/(1024*1024))
}

// TruncateHead truncates content from the head (keeps first N lines/bytes).
// Suitable for file reads where you want to see the beginning.
// Never returns partial lines.
func TruncateHead(content string, opts ...TruncationOptions) TruncationResult {
	opt := TruncationOptions{MaxLines: DefaultMaxLines, MaxBytes: DefaultMaxBytes}
	if len(opts) > 0 {
		if opts[0].MaxLines > 0 {
			opt.MaxLines = opts[0].MaxLines
		}
		if opts[0].MaxBytes > 0 {
			opt.MaxBytes = opts[0].MaxBytes
		}
	}

	totalBytes := len([]byte(content))
	lines := splitLines(content)
	totalLines := len(lines)

	// No truncation needed
	if totalLines <= opt.MaxLines && totalBytes <= opt.MaxBytes {
		return TruncationResult{
			Content: content, Truncated: false, TruncatedBy: "",
			TotalLines: totalLines, TotalBytes: totalBytes,
			OutputLines: totalLines, OutputBytes: totalBytes,
			MaxLines: opt.MaxLines, MaxBytes: opt.MaxBytes,
		}
	}

	// First line alone exceeds byte limit
	if len(lines) > 0 && len([]byte(lines[0])) > opt.MaxBytes {
		return TruncationResult{
			Content: "", Truncated: true, TruncatedBy: "bytes",
			TotalLines: totalLines, TotalBytes: totalBytes,
			OutputLines: 0, OutputBytes: 0,
			FirstLineExceeds: true,
			MaxLines:         opt.MaxLines, MaxBytes: opt.MaxBytes,
		}
	}

	// Collect complete lines that fit
	var outputLines []string
	outputBytes := 0
	truncatedBy := "lines"

	for i, line := range lines {
		if i >= opt.MaxLines {
			break
		}
		lineBytes := len([]byte(line))
		if i > 0 {
			lineBytes++ // +1 for newline
		}
		if outputBytes+lineBytes > opt.MaxBytes {
			truncatedBy = "bytes"
			break
		}
		outputLines = append(outputLines, line)
		outputBytes += lineBytes
	}

	if len(outputLines) >= opt.MaxLines && outputBytes <= opt.MaxBytes {
		truncatedBy = "lines"
	}

	result := joinLines(outputLines)
	return TruncationResult{
		Content: result, Truncated: true, TruncatedBy: truncatedBy,
		TotalLines: totalLines, TotalBytes: totalBytes,
		OutputLines: len(outputLines), OutputBytes: len([]byte(result)),
		MaxLines: opt.MaxLines, MaxBytes: opt.MaxBytes,
	}
}

// TruncateTail truncates content from the tail (keeps last N lines/bytes).
// Suitable for bash output where you want to see the end (errors, final results).
// May return partial first line if the last line exceeds byte limit.
func TruncateTail(content string, opts ...TruncationOptions) TruncationResult {
	opt := TruncationOptions{MaxLines: DefaultMaxLines, MaxBytes: DefaultMaxBytes}
	if len(opts) > 0 {
		if opts[0].MaxLines > 0 {
			opt.MaxLines = opts[0].MaxLines
		}
		if opts[0].MaxBytes > 0 {
			opt.MaxBytes = opts[0].MaxBytes
		}
	}

	totalBytes := len([]byte(content))
	lines := splitLines(content)
	totalLines := len(lines)

	// No truncation needed
	if totalLines <= opt.MaxLines && totalBytes <= opt.MaxBytes {
		return TruncationResult{
			Content: content, Truncated: false, TruncatedBy: "",
			TotalLines: totalLines, TotalBytes: totalBytes,
			OutputLines: totalLines, OutputBytes: totalBytes,
			MaxLines: opt.MaxLines, MaxBytes: opt.MaxBytes,
		}
	}

	// Work backwards from the end
	var outputLines []string
	outputBytes := 0
	truncatedBy := "lines"
	lastLinePartial := false

	for i := len(lines) - 1; i >= 0 && len(outputLines) < opt.MaxLines; i-- {
		line := lines[i]
		lineBytes := len([]byte(line))
		if len(outputLines) > 0 {
			lineBytes++ // +1 for newline
		}
		if outputBytes+lineBytes > opt.MaxBytes {
			truncatedBy = "bytes"
			// Edge case: no lines added yet and this line exceeds maxBytes
			if len(outputLines) == 0 {
				truncated := truncateStringFromEnd(line, opt.MaxBytes)
				outputLines = []string{truncated}
				outputBytes = len([]byte(truncated))
				lastLinePartial = true
			}
			break
		}
		outputLines = append([]string{line}, outputLines...)
		outputBytes += lineBytes
	}

	if len(outputLines) >= opt.MaxLines && outputBytes <= opt.MaxBytes {
		truncatedBy = "lines"
	}

	result := joinLines(outputLines)
	return TruncationResult{
		Content: result, Truncated: true, TruncatedBy: truncatedBy,
		TotalLines: totalLines, TotalBytes: totalBytes,
		OutputLines: len(outputLines), OutputBytes: len([]byte(result)),
		LastLinePartial: lastLinePartial,
		MaxLines:        opt.MaxLines, MaxBytes: opt.MaxBytes,
	}
}

// TruncateLine truncates a single line to maxChars, adding truncation suffix.
func TruncateLine(line string, maxChars ...int) (text string, wasTruncated bool) {
	max := GrepMaxLineLen
	if len(maxChars) > 0 && maxChars[0] > 0 {
		max = maxChars[0]
	}
	if len(line) <= max {
		return line, false
	}
	return line[:max] + "... [truncated]", true
}

// splitLines splits content into lines, handling trailing newlines.
func splitLines(content string) []string {
	if content == "" {
		return nil
	}
	lines := splitString(content, "\n")
	// Remove trailing empty line from trailing newline
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// joinLines joins lines with newlines.
func joinLines(lines []string) string {
	result := ""
	for i, line := range lines {
		if i > 0 {
			result += "\n"
		}
		result += line
	}
	return result
}

// truncateStringFromEnd truncates a string to fit within maxBytes from the end,
// respecting UTF-8 character boundaries.
func truncateStringFromEnd(s string, maxBytes int) string {
	b := []byte(s)
	if len(b) <= maxBytes {
		return s
	}
	start := len(b) - maxBytes
	// Find valid UTF-8 boundary
	for start < len(b) && (b[start]&0xC0) == 0x80 {
		start++
	}
	return string(b[start:])
}

func splitString(s, sep string) []string {
	if s == "" {
		return nil
	}
	var result []string
	for {
		idx := indexOf(s, sep)
		if idx < 0 {
			result = append(result, s)
			break
		}
		result = append(result, s[:idx])
		s = s[idx+len(sep):]
	}
	return result
}

func indexOf(s, sep string) int {
	for i := 0; i <= len(s)-len(sep); i++ {
		if s[i:i+len(sep)] == sep {
			return i
		}
	}
	return -1
}
