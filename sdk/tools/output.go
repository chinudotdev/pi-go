package tools

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"unicode/utf8"
)

// OutputSnapshot is a point-in-time view of accumulated output.
type OutputSnapshot struct {
	Content       string
	Truncation    TruncationResult
	FullOutputPath string
}

// OutputAccumulator incrementally tracks streaming output with bounded memory.
// It keeps only a decoded tail for display snapshots and optionally writes to
// a temp file when the full output needs to be preserved.
type OutputAccumulator struct {
	maxLines     int
	maxBytes     int
	maxRolling   int
	tempPrefix   string

	mu            sync.Mutex
	tailText      string
	tailBytes     int
	tailAtLineBoundary bool
	totalRawBytes int
	totalDecoded  int
	completedLines int
	totalLines    int
	currentLineBytes int
	hasOpenLine   bool
	finished      bool

	tempFilePath string
	tempFile     *os.File
	rawChunks    [][]byte
}

// NewOutputAccumulator creates a new accumulator with the given limits.
func NewOutputAccumulator(opts ...OutputAccumulatorOptions) *OutputAccumulator {
	opt := OutputAccumulatorOptions{}
	if len(opts) > 0 {
		opt = opts[0]
	}

	maxLines := DefaultMaxLines
	if opt.MaxLines > 0 {
		maxLines = opt.MaxLines
	}
	maxBytes := DefaultMaxBytes
	if opt.MaxBytes > 0 {
		maxBytes = opt.MaxBytes
	}
	prefix := "pi-output"
	if opt.TempFilePrefix != "" {
		prefix = opt.TempFilePrefix
	}

	return &OutputAccumulator{
		maxLines:   maxLines,
		maxBytes:   maxBytes,
		maxRolling: maxBytes * 2,
		tempPrefix: prefix,
	}
}

// OutputAccumulatorOptions configures the output accumulator.
type OutputAccumulatorOptions struct {
	MaxLines        int
	MaxBytes        int
	TempFilePrefix  string
}

// Append adds data to the accumulator.
func (a *OutputAccumulator) Append(data []byte) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.finished {
		return
	}

	a.totalRawBytes += len(data)
	text := string(data) // UTF-8 decode
	a.appendText(text)

	if a.tempFile != nil || a.shouldUseTempFile() {
		a.ensureTempFile()
		if a.tempFile != nil {
			a.tempFile.Write(data)
		}
	} else if len(data) > 0 {
		a.rawChunks = append(a.rawChunks, data)
	}
}

// Finish signals that no more data will be appended.
func (a *OutputAccumulator) Finish() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.finished = true
	if a.shouldUseTempFile() {
		a.ensureTempFile()
	}
}

// Snapshot returns the current output state.
func (a *OutputAccumulator) Snapshot(persistIfTruncated ...bool) OutputSnapshot {
	a.mu.Lock()
	defer a.mu.Unlock()

	snapshotText := a.getSnapshotText()
	tailTruncation := TruncateTail(snapshotText, TruncationOptions{
		MaxLines: a.maxLines,
		MaxBytes: a.maxBytes,
	})

	truncated := a.totalLines > a.maxLines || a.totalDecoded > a.maxBytes
	truncatedBy := ""
	if truncated {
		truncatedBy = tailTruncation.TruncatedBy
		if truncatedBy == "" {
			if a.totalDecoded > a.maxBytes {
				truncatedBy = "bytes"
			} else {
				truncatedBy = "lines"
			}
		}
	}

	truncation := TruncationResult{
		Content:     tailTruncation.Content,
		Truncated:   truncated,
		TruncatedBy: truncatedBy,
		TotalLines:  a.totalLines,
		TotalBytes:  a.totalDecoded,
		OutputLines: tailTruncation.OutputLines,
		OutputBytes: tailTruncation.OutputBytes,
		LastLinePartial: tailTruncation.LastLinePartial,
		MaxLines:    a.maxLines,
		MaxBytes:    a.maxBytes,
	}

	if len(persistIfTruncated) > 0 && persistIfTruncated[0] && truncation.Truncated {
		a.ensureTempFile()
	}

	return OutputSnapshot{
		Content:        truncation.Content,
		Truncation:     truncation,
		FullOutputPath: a.tempFilePath,
	}
}

// CloseTempFile closes the temp file if one was created.
func (a *OutputAccumulator) CloseTempFile() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.tempFile == nil {
		return nil
	}
	err := a.tempFile.Close()
	a.tempFile = nil
	return err
}

// GetLastLineBytes returns the byte count of the current (possibly partial) last line.
func (a *OutputAccumulator) GetLastLineBytes() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.currentLineBytes
}

func (a *OutputAccumulator) appendText(text string) {
	if text == "" {
		return
	}

	bytes := len([]byte(text))
	a.totalDecoded += bytes
	a.tailText += text
	a.tailBytes += bytes
	if a.tailBytes > a.maxRolling*2 {
		a.trimTail()
	}

	// Count newlines
	newlines := 0
	lastNewline := -1
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			newlines++
			lastNewline = i
		}
	}

	if newlines == 0 {
		a.currentLineBytes += bytes
		a.hasOpenLine = true
	} else {
		a.completedLines += newlines
		tail := text[lastNewline+1:]
		a.currentLineBytes = len([]byte(tail))
		a.hasOpenLine = len(tail) > 0
	}
	a.totalLines = a.completedLines
	if a.hasOpenLine {
		a.totalLines++
	}
}

func (a *OutputAccumulator) trimTail() {
	b := []byte(a.tailText)
	if len(b) <= a.maxRolling {
		a.tailBytes = len(b)
		return
	}

	start := len(b) - a.maxRolling
	for start < len(b) && (b[start]&0xC0) == 0x80 {
		start++
	}

	if start > 0 {
		a.tailAtLineBoundary = b[start-1] == 0x0A
	} else {
		// Keep whatever it was
	}
	a.tailText = string(b[start:])
	a.tailBytes = len([]byte(a.tailText))
}

func (a *OutputAccumulator) getSnapshotText() string {
	if a.tailAtLineBoundary {
		return a.tailText
	}
	idx := indexOf(a.tailText, "\n")
	if idx < 0 {
		return a.tailText
	}
	return a.tailText[idx+1:]
}

func (a *OutputAccumulator) shouldUseTempFile() bool {
	return a.totalRawBytes > a.maxBytes || a.totalDecoded > a.maxBytes || a.totalLines > a.maxLines
}

func (a *OutputAccumulator) ensureTempFile() {
	if a.tempFilePath != "" {
		return
	}

	id := make([]byte, 8)
	rand.Read(id)
	tmpDir := os.TempDir()
	a.tempFilePath = filepath.Join(tmpDir, fmt.Sprintf("%s-%x.log", a.tempPrefix, id))

	f, err := os.Create(a.tempFilePath)
	if err != nil {
		return
	}
	a.tempFile = f

	// Write buffered raw chunks
	for _, chunk := range a.rawChunks {
		a.tempFile.Write(chunk)
	}
	a.rawChunks = nil
}

// min for int
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Suppress unused import warning
var _ = utf8.DecodeRune
