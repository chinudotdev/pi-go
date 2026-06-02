package truncate

import (
	"fmt"
	"strings"
)

const (
	DefaultMaxLines = 2000
	DefaultMaxBytes = 50 * 1024 // 50KB
	GrepMaxLineLen  = 500
)

// TruncationResult holds the result of a truncation operation.
type TruncationResult struct {
	Content            string `json:"content"`
	Truncated          bool   `json:"truncated"`
	TruncatedBy        string `json:"truncatedBy,omitempty"` // "lines", "bytes", or ""
	TotalLines         int    `json:"totalLines"`
	TotalBytes         int    `json:"totalBytes"`
	OutputLines        int    `json:"outputLines"`
	OutputBytes        int    `json:"outputBytes"`
	LastLinePartial    bool   `json:"lastLinePartial"`
	FirstLineExceeds   bool   `json:"firstLineExceeds"`
	MaxLines           int    `json:"maxLines"`
	MaxBytes           int    `json:"maxBytes"`
}

// TruncationOptions controls truncation limits.
type TruncationOptions struct {
	MaxLines int // default: 2000
	MaxBytes int // default: 50KB
}

// FormatSize formats bytes as a human-readable size.
func FormatSize(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%dB", bytes)
	} else if bytes < 1024*1024 {
		return fmt.Sprintf("%.1fKB", float64(bytes)/1024)
	}
	return fmt.Sprintf("%.1fMB", float64(bytes)/(1024*1024))
}

// utf8ByteLength returns the UTF-8 byte length of a string.
func utf8ByteLength(s string) int {
	// Go's len() returns byte length for strings, which is UTF-8 byte length
	return len(s)
}

// TruncateHead truncates content from the head (keep first N lines/bytes).
// Suitable for file reads where you want to see the beginning.
// Never returns partial lines. If first line exceeds byte limit,
// returns empty content with FirstLineExceeds=true.
func TruncateHead(content string, opts TruncationOptions) TruncationResult {
	maxLines := opts.MaxLines
	if maxLines == 0 {
		maxLines = DefaultMaxLines
	}
	maxBytes := opts.MaxBytes
	if maxBytes == 0 {
		maxBytes = DefaultMaxBytes
	}

	totalBytes := utf8ByteLength(content)
	lines := strings.Split(content, "\n")
	totalLines := len(lines)

	if totalLines <= maxLines && totalBytes <= maxBytes {
		return TruncationResult{
			Content:     content,
			Truncated:   false,
			TotalLines:  totalLines,
			TotalBytes:  totalBytes,
			OutputLines: totalLines,
			OutputBytes: totalBytes,
			MaxLines:    maxLines,
			MaxBytes:    maxBytes,
		}
	}

	// Check if first line alone exceeds byte limit
	firstLineBytes := utf8ByteLength(lines[0])
	if firstLineBytes > maxBytes {
		return TruncationResult{
			Truncated:        true,
			TruncatedBy:      "bytes",
			TotalLines:       totalLines,
			TotalBytes:       totalBytes,
			FirstLineExceeds: true,
			MaxLines:         maxLines,
			MaxBytes:         maxBytes,
		}
	}

	// Collect complete lines that fit
	var outputLines []string
	outputBytesCount := 0
	truncatedBy := "lines"

	for i := 0; i < len(lines) && i < maxLines; i++ {
		line := lines[i]
		lineBytes := utf8ByteLength(line)
		if i > 0 {
			lineBytes++ // newline
		}

		if outputBytesCount+lineBytes > maxBytes {
			truncatedBy = "bytes"
			break
		}

		outputLines = append(outputLines, line)
		outputBytesCount += lineBytes
	}

	if len(outputLines) >= maxLines && outputBytesCount <= maxBytes {
		truncatedBy = "lines"
	}

	output := strings.Join(outputLines, "\n")
	return TruncationResult{
		Content:     output,
		Truncated:   true,
		TruncatedBy: truncatedBy,
		TotalLines:  totalLines,
		TotalBytes:  totalBytes,
		OutputLines: len(outputLines),
		OutputBytes: utf8ByteLength(output),
		MaxLines:    maxLines,
		MaxBytes:    maxBytes,
	}
}

// TruncateTail truncates content from the tail (keep last N lines/bytes).
// Suitable for bash output where you want to see the end (errors, final results).
// May return partial first line if the last line exceeds byte limit.
func TruncateTail(content string, opts TruncationOptions) TruncationResult {
	maxLines := opts.MaxLines
	if maxLines == 0 {
		maxLines = DefaultMaxLines
	}
	maxBytes := opts.MaxBytes
	if maxBytes == 0 {
		maxBytes = DefaultMaxBytes
	}

	totalBytes := utf8ByteLength(content)
	lines := strings.Split(content, "\n")
	// Remove trailing empty line from final newline
	if len(lines) > 1 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	totalLines := len(lines)

	if totalLines <= maxLines && totalBytes <= maxBytes {
		return TruncationResult{
			Content:     content,
			Truncated:   false,
			TotalLines:  totalLines,
			TotalBytes:  totalBytes,
			OutputLines: totalLines,
			OutputBytes: totalBytes,
			MaxLines:    maxLines,
			MaxBytes:    maxBytes,
		}
	}

	// Work backwards from the end
	var outputLines []string
	outputBytesCount := 0
	truncatedBy := "lines"
	lastLinePartial := false

	for i := len(lines) - 1; i >= 0 && len(outputLines) < maxLines; i-- {
		line := lines[i]
		lineBytes := utf8ByteLength(line)
		if len(outputLines) > 0 {
			lineBytes++ // newline
		}

		if outputBytesCount+lineBytes > maxBytes {
			truncatedBy = "bytes"
			// Edge case: if we haven't added ANY lines yet, take end of this line (partial)
			if len(outputLines) == 0 {
				truncatedLine := truncateStringFromEnd(line, maxBytes)
				outputLines = append([]string{truncatedLine}, outputLines...)
				outputBytesCount = utf8ByteLength(truncatedLine)
				lastLinePartial = true
			}
			break
		}

		outputLines = append([]string{line}, outputLines...)
		outputBytesCount += lineBytes
	}

	if len(outputLines) >= maxLines && outputBytesCount <= maxBytes {
		truncatedBy = "lines"
	}

	output := strings.Join(outputLines, "\n")
	return TruncationResult{
		Content:         output,
		Truncated:       true,
		TruncatedBy:     truncatedBy,
		TotalLines:      totalLines,
		TotalBytes:      totalBytes,
		OutputLines:     len(outputLines),
		OutputBytes:     utf8ByteLength(output),
		LastLinePartial: lastLinePartial,
		MaxLines:        maxLines,
		MaxBytes:        maxBytes,
	}
}

// truncateStringFromEnd truncates a string to fit within maxBytes, keeping the end.
func truncateStringFromEnd(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	// Go strings are UTF-8, len() gives byte count
	if len(s) <= maxBytes {
		return s
	}
	// Walk backwards from the end, counting bytes
	start := len(s) - maxBytes
	// If we land in the middle of a multi-byte character, advance past it
	for start < len(s) && isContinuationByte(s[start]) {
		start++
	}
	return s[start:]
}

func isContinuationByte(b byte) bool {
	return b&0xC0 == 0x80
}

// TruncateLine truncates a single line to maxChars, adding [truncated] suffix.
func TruncateLine(line string, maxChars int) (text string, wasTruncated bool) {
	if maxChars == 0 {
		maxChars = GrepMaxLineLen
	}
	if len(line) <= maxChars {
		return line, false
	}
	return line[:maxChars] + "... [truncated]", true
}

// SanitizeBinaryOutput removes control characters except tab, newline, CR.
func SanitizeBinaryOutput(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == 0x09 || r == 0x0a || r == 0x0d {
			b.WriteRune(r)
		} else if r <= 0x1f {
			continue
		} else if r >= 0xfff9 && r <= 0xfffb {
			continue
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
