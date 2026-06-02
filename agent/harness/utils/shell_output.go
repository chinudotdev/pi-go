package truncate

import (
	"context"

	"github.com/chinudotdev/pi-go/agent/harness"
)

// ShellCaptureOptions controls shell capture behavior.
type ShellCaptureOptions struct {
	Cwd     string
	Env     map[string]string
	Timeout int // seconds
	OnChunk func(chunk string)
}

// ShellCaptureResult holds the captured output of a shell command.
type ShellCaptureResult struct {
	Output       string // Truncated output (tail)
	ExitCode     int    // 0 on success
	Cancelled    bool
	Truncated    bool
	FullOutputPath string // Path to full output log if truncated
}

// ExecuteShellWithCapture executes a shell command and captures output with truncation.
// When output exceeds 50KB, it captures the tail and optionally writes full output to a temp file.
func ExecuteShellWithCapture(
	env harness.ExecutionEnv,
	command string,
	opts *ShellCaptureOptions,
) harness.Result[ShellCaptureResult] {
	if opts == nil {
		opts = &ShellCaptureOptions{}
	}

	var outputChunks []string
	outputBytes := 0
	maxOutputBytes := DefaultMaxBytes * 2
	totalBytes := 0

	onChunk := func(chunk string) {
		totalBytes += len(chunk)
		text := SanitizeBinaryOutput(chunk)
		text = stringsReplaceCR(text)

		outputChunks = append(outputChunks, text)
		outputBytes += len(text)

		// Drop oldest chunks to stay under limit
		for outputBytes > maxOutputBytes && len(outputChunks) > 1 {
			removed := outputChunks[0]
			outputChunks = outputChunks[1:]
			outputBytes -= len(removed)
		}

		if opts.OnChunk != nil {
			opts.OnChunk(text)
		}
	}

	execOpts := &harness.ExecOptions{
		Cwd:     opts.Cwd,
		Env:     opts.Env,
		Timeout: opts.Timeout,
		OnStdout: onChunk,
		OnStderr: onChunk,
	}

	result := env.Exec(context.Background(), command, execOpts)
	if !result.OK {
		// Check for abort/cancellation
		if execErr, ok := result.Err.(*harness.ExecutionError); ok {
			if execErr.Code == "aborted" {
				tail := stringsJoin(outputChunks)
				truncResult := TruncateTail(tail, TruncationOptions{})
				return harness.OkResult(ShellCaptureResult{
					Output:    truncResult.Content,
					Cancelled: true,
					Truncated: truncResult.Truncated,
				})
			}
		}
		return harness.ErrResult[ShellCaptureResult](result.Err)
	}

	tail := stringsJoin(outputChunks)
	truncResult := TruncateTail(tail, TruncationOptions{})

	return harness.OkResult(ShellCaptureResult{
		Output:   truncResult.Content,
		ExitCode: result.Value.ExitCode,
		Truncated: truncResult.Truncated,
	})
}

func stringsReplaceCR(s string) string {
	// Fast path: no CR
	for i := 0; i < len(s); i++ {
		if s[i] == '\r' {
			return stringsReplaceAll(s, "\r", "")
		}
	}
	return s
}

func stringsReplaceAll(s, old, new string) string {
	result := ""
	for {
		i := len(s) - len(old)
		if i < 0 {
			break
		}
		// Find from start
		idx := -1
		for j := 0; j <= len(s)-len(old); j++ {
			if s[j:j+len(old)] == old {
				idx = j
				break
			}
		}
		if idx == -1 {
			break
		}
		result += s[:idx] + new
		s = s[idx+len(old):]
	}
	return result + s
}

func stringsJoin(parts []string) string {
	total := 0
	for _, p := range parts {
		total += len(p)
	}
	var b []byte
	b = make([]byte, 0, total)
	for _, p := range parts {
		b = append(b, p...)
	}
	return string(b)
}
